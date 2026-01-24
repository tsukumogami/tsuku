---
summary:
  constraints:
    - Tests should skip in short mode (integration tests are slow)
    - Tests cannot rely on tsuku-dltest being installed (may not exist in test env)
    - Path validation tests must use real filesystem operations (symlinks, canonicalization)
    - Timeout tests need actual slow-running helpers (not mocks)
    - Environment sanitization tests need a helper that reports its environment
  integration_points:
    - internal/verify/dltest.go - InvokeDltest, RunDlopenVerification, EnsureDltest
    - internal/verify/dltest_test.go - existing unit tests to extend
    - testdata/dlopen/ - new test fixtures directory for mock libraries
  risks:
    - Creating real .so files for tests may be platform-specific
    - Timeout tests are inherently slow and may be flaky in CI
    - Security tests (path traversal, symlink escape) need careful filesystem setup
    - Helper version mismatch test requires way to override expected version
  approach_notes: |
    Focus on testing the Go code paths, not the Rust helper itself.
    Use mock helper scripts where possible (shell scripts that output JSON).
    Existing tests in dltest_test.go provide good patterns to follow.
    Many acceptance criteria overlap with existing unit tests - identify gaps.
---

# Implementation Context: Issue #1019

**Source**: docs/designs/DESIGN-library-verify-dlopen.md

## Key Design Points for Testing

### Test Categories (from design Step 7)
1. Unit tests for JSON parsing (Go side) - existing tests cover this
2. Integration tests invoking helper on real libraries
3. Timeout behavior tests
4. Fallback behavior tests
5. Environment sanitization tests
6. Path validation tests
7. Batch processing tests

### Acceptance Criteria Mapping to Existing Tests

Reviewing `dltest_test.go`, these are already covered:
- JSON parsing (TestDlopenResult_JSONParsing_*)
- Batch splitting (TestSplitIntoBatches_*)
- Timeout handling (TestInvokeDltest_Timeout)
- Path validation (TestValidateLibraryPaths_*, TestInvokeDltest_RejectsInvalidPaths)
- Environment sanitization (TestSanitizeEnvForHelper_*)
- Skip flag behavior (TestRunDlopenVerification_SkipDlopenFlag)
- Empty paths (TestRunDlopenVerification_EmptyPaths)
- Helper not found (TestInvokeDltest_HelperNotFound)
- Exit code 1 not treated as crash (TestInvokeDltest_ExitCode1_NotCrash)
- Retry on crash (TestInvokeDltest_RetryOnCrash)
- Mock helper success (TestInvokeDltest_MockHelper_*)

### Gaps to Fill (new integration tests)
1. Real dlopen success/failure (requires actual .so files or installed library)
2. Missing dependency scenario (library that fails dlopen)
3. Corrupt library scenario
4. Results aggregated correctly across batches (more than 50 libraries)
5. Partial batch failure (some ok, some fail)
6. Helper version mismatch handling

### Test Fixture Strategy
- Use mock shell scripts as helper for most tests (already done in existing tests)
- For real dlopen tests, could use libc.so or other system library
- For failure tests, create files that look like .so but aren't valid
