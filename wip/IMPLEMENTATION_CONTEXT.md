---
summary:
  constraints:
    - Warning message format is specified: "Warning: tsuku-dltest helper not available, skipping load test"
    - Install hint format specified: "Run 'tsuku install tsuku-dltest' to enable full verification"
    - Checksum mismatch must error (not warn): "helper binary checksum verification failed, refusing to execute"
    - Network failure during download produces skip with warning (not error)
    - --skip-dlopen skips Level 3 entirely with NO warning (silent skip)
  integration_points:
    - internal/verify/dltest.go - EnsureDltest() already exists, need to handle unavailability gracefully
    - cmd/tsuku/verify.go or similar - add --skip-dlopen flag to CLI
    - Fallback behavior integrates at point where InvokeDltest is called
  risks:
    - Need to identify where the verify command is implemented (cmd structure)
    - Must not break existing E2E flow (skeleton must still work)
    - Network failure detection needs to differentiate from checksum mismatch
  approach_notes: |
    1. Add --skip-dlopen flag to verify command
    2. Modify EnsureDltest or create wrapper to return graceful fallback on:
       - Network unavailable: skip with warning
       - Not installed + download fails: skip with warning
    3. Checksum mismatch should remain an error (security-critical)
    4. When --skip-dlopen is passed, skip Level 3 entirely without warning
    5. When helper unavailable (non-checksum reason), skip with warning + hint
---

# Implementation Context: Issue #1018

**Source**: docs/designs/DESIGN-library-verify-dlopen.md

## Key Requirements from Design

### Fallback Behavior Table

| Scenario | Behavior |
|----------|----------|
| Helper not installed | Attempt download; if fails, skip Level 3 with warning |
| Checksum mismatch | Error: "helper binary checksum verification failed, refusing to execute" |
| Helper times out | Error: "load test timed out" for affected batch |
| Helper crashes | Retry batch in smaller chunks; if still fails, report crash |
| `--skip-dlopen` flag | Skip Level 3 entirely, no warning |
| Network unavailable | Skip Level 3 with warning: "helper unavailable, skipping load test" |

### Warning Message Format

```
Warning: tsuku-dltest helper not available, skipping load test
  Run 'tsuku install tsuku-dltest' to enable full verification
```

### Existing Code Reference

The `EnsureDltest` function in `internal/verify/dltest.go` handles installation. The `InvokeDltest` function handles invocation with batching, timeout, and environment sanitization (from #1016, #1017).

Need to find:
1. Where the verify command is implemented
2. Where Level 3 verification is (or would be) called
3. How to add the --skip-dlopen flag
