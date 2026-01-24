# Issue 1018 Implementation Plan

## Overview

Add --skip-dlopen flag behavior and fallback handling for Level 3 (dlopen) verification.

## Changes

### 1. Add helper availability error types (internal/verify/dltest.go)

Create sentinel errors to distinguish different failure modes:

```go
var (
    // ErrHelperUnavailable indicates the helper could not be obtained
    // but for a recoverable reason (network, not installed, etc.)
    ErrHelperUnavailable = errors.New("tsuku-dltest helper unavailable")

    // ErrChecksumMismatch indicates the helper checksum verification failed
    // This is a security-critical error that should NOT be handled as fallback
    ErrChecksumMismatch = errors.New("helper binary checksum verification failed")
)
```

### 2. Modify installDltest error handling (internal/verify/dltest.go)

Detect checksum mismatch in the stderr output and return a typed error:

```go
func installDltest(version string) error {
    // ... existing code ...
    if err := cmd.Run(); err != nil {
        errMsg := strings.TrimSpace(stderr.String())
        // Check for checksum failure
        if strings.Contains(errMsg, "checksum") ||
           strings.Contains(errMsg, "Checksum") ||
           strings.Contains(errMsg, "verification failed") {
            return fmt.Errorf("%w: %s", ErrChecksumMismatch, errMsg)
        }
        return fmt.Errorf("%w: %s", ErrHelperUnavailable, errMsg)
    }
    return nil
}
```

### 3. Update EnsureDltest error wrapping (internal/verify/dltest.go)

Ensure errors from installDltest propagate with the correct sentinel:

```go
// In EnsureDltest, when calling installDltest:
if err := installDltest(version); err != nil {
    return "", err  // Already wrapped with sentinel
}
```

### 4. Add RunDlopenVerification function (internal/verify/dltest.go)

Create a high-level function that handles all fallback logic:

```go
// DlopenVerificationResult holds the outcome of dlopen verification
type DlopenVerificationResult struct {
    Results []DlopenResult
    Skipped bool
    Warning string
}

// RunDlopenVerification performs Level 3 dlopen verification with fallback.
// If skipDlopen is true, returns immediately with Skipped=true and no warning.
// If the helper is unavailable (non-checksum reason), returns Skipped=true with warning.
// If the helper checksum fails, returns an error (security-critical).
func RunDlopenVerification(ctx context.Context, cfg *config.Config, paths []string, skipDlopen bool) (*DlopenVerificationResult, error) {
    if skipDlopen {
        return &DlopenVerificationResult{Skipped: true}, nil
    }

    helperPath, err := EnsureDltest(cfg)
    if err != nil {
        if errors.Is(err, ErrChecksumMismatch) {
            return nil, fmt.Errorf("helper binary checksum verification failed, refusing to execute")
        }
        // Non-checksum failure: skip with warning
        return &DlopenVerificationResult{
            Skipped: true,
            Warning: "Warning: tsuku-dltest helper not available, skipping load test\n  Run 'tsuku install tsuku-dltest' to enable full verification",
        }, nil
    }

    results, err := InvokeDltest(ctx, helperPath, paths, cfg.HomeDir)
    if err != nil {
        return nil, err
    }

    return &DlopenVerificationResult{Results: results}, nil
}
```

### 5. Update verifyLibrary in cmd/tsuku/verify.go

Replace the Tier 3 stub with actual implementation:

```go
// Tier 3: dlopen load testing
if opts.SkipDlopen {
    // Silent skip - no output when flag is passed
} else if len(libFiles) > 0 {
    printInfo("  Tier 3: dlopen load testing...\n")
    result, err := verify.RunDlopenVerification(
        context.Background(),
        cfg,
        libFiles,
        false, // skipDlopen already handled above
    )
    if err != nil {
        return fmt.Errorf("dlopen verification failed: %w", err)
    }
    if result.Warning != "" {
        fmt.Fprintf(os.Stderr, "  %s\n", result.Warning)
    }
    if !result.Skipped {
        // Display results
        passed, failed := 0, 0
        for _, r := range result.Results {
            if r.OK {
                passed++
            } else {
                failed++
                printInfof("    %s: FAIL - %s\n", filepath.Base(r.Path), r.Error)
            }
        }
        if failed > 0 {
            return fmt.Errorf("dlopen failed for %d of %d libraries", failed, passed+failed)
        }
        printInfof("  Tier 3: %d libraries loaded successfully\n", passed)
    }
}
```

### 6. Add tests

1. Test `--skip-dlopen` flag produces no Tier 3 output
2. Test helper unavailable produces warning
3. Test checksum mismatch produces error
4. Test successful dlopen displays results

## Test Plan

1. Run existing tests (should pass)
2. Add unit tests for error handling
3. Manual test: `./tsuku verify gcc-libs --skip-dlopen` (should skip silently)
4. Manual test: with helper not installed (should warn)
