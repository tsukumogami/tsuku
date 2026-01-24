---
summary:
  constraints:
    - Batch size limit of 50 libraries per invocation (stays under ARG_MAX)
    - 5-second timeout per batch invocation (prevents hangs from library init code)
    - Retry with halved batch size on crash (minimum batch size of 1)
    - Must preserve existing InvokeDltest API from #1014 skeleton
  integration_points:
    - internal/verify/dltest.go - extend InvokeDltest with batching/timeout
    - DlopenResult struct already defined for JSON parsing
    - exec.CommandContext already used for helper invocation
  risks:
    - Timeout detection must distinguish context.DeadlineExceeded from other errors
    - Retry logic must avoid infinite loops (stop at batch size 1)
    - Tests must avoid invoking actual tsuku subprocess (causes recursion)
  approach_notes: |
    1. Add splitIntoBatches helper function (chunks of 50)
    2. Wrap each batch invocation with context.WithTimeout(ctx, 5*time.Second)
    3. On crash/signal, retry with halved batch size
    4. Aggregate results from all batches into single slice
    5. Report timeout errors with batch context (which libraries were affected)
---

# Implementation Context: Issue #1016

**Source**: docs/designs/DESIGN-library-verify-dlopen.md

## Key Design Sections

### Batch Size Limits (from design)

Default batch size: 50 libraries per invocation

**Rationale**:
- ARG_MAX is typically 128KB-2MB
- 50 paths × 256 bytes average = 12.8KB (well under limit)
- 50 libraries × ~200KB average memory = 10MB (acceptable)
- If batch crashes, max loss is 50 results (retryable)

### Invocation Protocol (from design)

```go
// Create context with timeout
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

cmd := exec.CommandContext(ctx, helperPath, paths...)

// ...

if ctx.Err() == context.DeadlineExceeded {
    return nil, fmt.Errorf("helper timed out after 5 seconds")
}
```

### Fallback Behavior (from design)

| Scenario | Behavior |
|----------|----------|
| Helper times out | Error: "load test timed out" for affected batch |
| Helper crashes | Retry batch in smaller chunks; if still fails, report crash |

## Existing Code Reference

The skeleton implementation in `internal/verify/dltest.go` already has:
- `DlopenResult` struct for JSON parsing
- `InvokeDltest(ctx, helperPath, paths)` basic implementation
- `EnsureDltest(cfg)` for helper installation

This issue adds:
- Batch splitting logic
- Timeout handling with 5-second limit
- Retry on crash with halved batch size
- Result aggregation from multiple batches
