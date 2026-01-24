# Issue 1016 Implementation Plan

## Summary

Add batch processing and timeout handling to the `InvokeDltest` function in `internal/verify/dltest.go`.

## Approach

Wrap the existing single-invocation logic with batch splitting and retry behavior:

1. **Batch splitting**: Split input paths into chunks of 50
2. **Per-batch timeout**: Create child context with 5-second timeout for each batch
3. **Crash detection**: Detect crashes via exit code (not 0, 1, or 2) or context signals
4. **Retry with halving**: On crash, retry batch with half the size until success or batch size 1
5. **Result aggregation**: Combine results from all batches into single slice

## Files to Modify

| File | Change Type | Description |
|------|-------------|-------------|
| `internal/verify/dltest.go` | Modify | Add batching, timeout, and retry logic |
| `internal/verify/dltest_test.go` | Modify | Add tests for batch, timeout, and retry behavior |

## Implementation Steps

### Step 1: Add constants and types

```go
const (
    // DefaultBatchSize is the maximum number of libraries per helper invocation.
    DefaultBatchSize = 50

    // BatchTimeout is the timeout for each batch invocation.
    BatchTimeout = 5 * time.Second
)

// BatchError represents a failure during batch processing.
type BatchError struct {
    Batch  []string // The library paths in the failed batch
    Cause  error    // The underlying error
    IsTimeout bool   // True if the error was a timeout
}

func (e *BatchError) Error() string {
    if e.IsTimeout {
        return fmt.Sprintf("batch timed out after %v for %d libraries", BatchTimeout, len(e.Batch))
    }
    return fmt.Sprintf("batch failed for %d libraries: %v", len(e.Batch), e.Cause)
}
```

### Step 2: Add batch splitting helper

```go
// splitIntoBatches divides paths into chunks of at most batchSize.
func splitIntoBatches(paths []string, batchSize int) [][]string {
    if batchSize <= 0 {
        batchSize = DefaultBatchSize
    }
    var batches [][]string
    for i := 0; i < len(paths); i += batchSize {
        end := i + batchSize
        if end > len(paths) {
            end = len(paths)
        }
        batches = append(batches, paths[i:end])
    }
    return batches
}
```

### Step 3: Add single-batch invocation with timeout

```go
// invokeBatch executes the helper on a single batch with timeout.
// Returns results, or error if timeout/crash occurred.
func invokeBatch(ctx context.Context, helperPath string, batch []string) ([]DlopenResult, error) {
    // Create timeout context for this batch
    batchCtx, cancel := context.WithTimeout(ctx, BatchTimeout)
    defer cancel()

    cmd := exec.CommandContext(batchCtx, helperPath, batch...)

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    err := cmd.Run()

    // Check for timeout
    if batchCtx.Err() == context.DeadlineExceeded {
        return nil, &BatchError{Batch: batch, IsTimeout: true}
    }

    // Check for parent context cancellation
    if ctx.Err() != nil {
        return nil, ctx.Err()
    }

    // Check for crash (signal or unexpected exit)
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            code := exitErr.ExitCode()
            // Exit codes: 0 = all ok, 1 = some failed, 2 = usage error
            // Anything else (or -1 for signal) indicates crash
            if code != 0 && code != 1 && code != 2 {
                return nil, &BatchError{Batch: batch, Cause: err}
            }
        } else {
            // Non-exit error (couldn't start process, etc.)
            return nil, &BatchError{Batch: batch, Cause: err}
        }
    }

    // Parse JSON
    var results []DlopenResult
    if parseErr := json.Unmarshal(stdout.Bytes(), &results); parseErr != nil {
        if err != nil {
            return nil, &BatchError{Batch: batch, Cause: fmt.Errorf("parse failed: %w (stderr: %s)", parseErr, stderr.String())}
        }
        return nil, &BatchError{Batch: batch, Cause: fmt.Errorf("parse failed: %w", parseErr)}
    }

    return results, nil
}
```

### Step 4: Add retry logic

```go
// invokeBatchWithRetry executes a batch, retrying with halved size on crash.
func invokeBatchWithRetry(ctx context.Context, helperPath string, batch []string) ([]DlopenResult, error) {
    results, err := invokeBatch(ctx, helperPath, batch)
    if err == nil {
        return results, nil
    }

    // Check if it's a retriable error (crash, not timeout)
    batchErr, ok := err.(*BatchError)
    if !ok || batchErr.IsTimeout {
        return nil, err
    }

    // Don't retry if batch size is 1 - the crash is for this specific library
    if len(batch) == 1 {
        return nil, err
    }

    // Retry with halved batch size
    mid := len(batch) / 2
    firstHalf := batch[:mid]
    secondHalf := batch[mid:]

    var allResults []DlopenResult

    firstResults, err := invokeBatchWithRetry(ctx, helperPath, firstHalf)
    if err != nil {
        return nil, err
    }
    allResults = append(allResults, firstResults...)

    secondResults, err := invokeBatchWithRetry(ctx, helperPath, secondHalf)
    if err != nil {
        return nil, err
    }
    allResults = append(allResults, secondResults...)

    return allResults, nil
}
```

### Step 5: Modify InvokeDltest to use batching

Replace current `InvokeDltest` implementation:

```go
// InvokeDltest calls the tsuku-dltest helper to test dlopen on the given library paths.
// It returns a DlopenResult for each path, preserving order.
//
// Paths are processed in batches of up to 50 to avoid ARG_MAX limits and limit
// the impact of crashes. Each batch has a 5-second timeout. If a batch crashes,
// it is retried with halved batch size until the problematic library is isolated.
func InvokeDltest(ctx context.Context, helperPath string, paths []string) ([]DlopenResult, error) {
    if len(paths) == 0 {
        return nil, nil
    }

    batches := splitIntoBatches(paths, DefaultBatchSize)
    var allResults []DlopenResult

    for _, batch := range batches {
        results, err := invokeBatchWithRetry(ctx, helperPath, batch)
        if err != nil {
            return nil, err
        }
        allResults = append(allResults, results...)
    }

    return allResults, nil
}
```

### Step 6: Add tests

Add to `dltest_test.go`:

1. `TestSplitIntoBatches` - verify batch splitting logic
2. `TestInvokeDltest_BatchSplitting` - verify paths are batched correctly
3. `TestInvokeDltest_TimeoutHandling` - verify timeout detection
4. `TestInvokeDltest_RetryOnCrash` - verify retry with halving

Note: Tests should use mock scenarios where possible to avoid subprocess recursion.

## Testing Strategy

- Unit tests for `splitIntoBatches` (pure function, easy to test)
- For timeout/crash tests, we can test the error types directly without invoking real helpers
- Validate that the exported API (`InvokeDltest`) still works with existing integration paths

## Risks

1. **Test complexity**: Testing crash/retry behavior without actually crashing requires careful mock design
2. **Exit code handling**: Need to correctly distinguish exit 1 (dlopen failures) from crashes

## Validation

```bash
go test -v -run TestSplitIntoBatches ./internal/verify/...
go test -v -run TestInvokeDltest ./internal/verify/...
go build ./cmd/tsuku
```
