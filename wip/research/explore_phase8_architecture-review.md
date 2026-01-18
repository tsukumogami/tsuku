# Architecture Review: dlopen Load Testing Design (Level 3)

**Reviewer:** Architecture Review Agent
**Date:** 2026-01-18
**Design Document:** `docs/designs/DESIGN-library-verify-dlopen.md`

## Executive Summary

The dlopen load testing design is well-structured and implementation-ready. It follows established patterns in the codebase (nix-portable) and makes sound architectural decisions. A few gaps exist in error handling specifics and macOS-specific behavior, but these are minor and can be addressed during implementation.

**Overall Assessment:** Ready to implement with minor clarifications needed.

---

## 1. Architecture Clarity Assessment

### 1.1 Strong Points

**Clear separation of concerns:**
- Helper binary (`tsuku-dltest`) handles cgo/dlopen complexity
- Main tsuku binary orchestrates invocation, batching, and result aggregation
- Trust chain verification is isolated in `internal/verify/dltest.go`

**Well-defined protocol:**
- JSON schema for helper output is explicit and extensible
- Exit codes are documented (0=success, 1=failure, 2=usage error)
- Version protocol (`--version` on stderr) enables upgrade detection

**Consistent with existing patterns:**
- Follows `nix_portable.go` pattern for helper binary management
- Uses same trust chain approach (embedded checksums, version file, atomic rename)
- File locking pattern from nix-portable should be applied here too

### 1.2 Gaps Identified

**Gap 1: File locking not explicitly mentioned**

The nix-portable implementation uses `syscall.Flock()` to prevent race conditions during concurrent downloads. The dlopen design should explicitly state the same pattern will be used for `tsuku-dltest` installation.

*Recommendation:* Add file locking to the trust chain verification flow in the design.

**Gap 2: macOS dlopen flag differences**

The design shows `RTLD_NOW | RTLD_LOCAL` for Linux, but macOS has subtly different semantics:
- `RTLD_FIRST` (macOS-only): limits symbol search to this library only
- The design should note whether macOS uses identical flags or adapts

*Recommendation:* Clarify that macOS uses the same flags (`RTLD_NOW | RTLD_LOCAL`) since the goal is load verification, not symbol lookup.

**Gap 3: Environment variable handling for library search paths**

The design mentions inheriting `LD_LIBRARY_PATH` and `DYLD_LIBRARY_PATH`, but doesn't specify:
- Whether these should be augmented with `$TSUKU_HOME/libs/*` paths
- What happens if user has restrictive environment settings

*Recommendation:* Document that tsuku should prepend `$TSUKU_HOME/libs/{library}-{version}` to the appropriate environment variable before invoking the helper.

**Gap 4: Stderr handling**

The design specifies stdout for JSON output and stderr for version output, but doesn't clarify:
- What happens to stderr during normal verification (libraries may print to stderr)
- Whether stderr should be captured and included in error messages

*Recommendation:* Capture stderr and include in error messages when dlopen fails, but don't parse it as structured data.

---

## 2. Missing Components or Interfaces

### 2.1 Interface Definitions

The design provides good pseudocode but lacks explicit Go interface definitions.

**Suggested interface additions:**

```go
// internal/verify/dltest.go

// DlopenResult represents a single library's load test outcome
type DlopenResult struct {
    Path  string `json:"path"`
    OK    bool   `json:"ok"`
    Error string `json:"error,omitempty"`
}

// DltestRunner abstracts helper binary invocation for testing
type DltestRunner interface {
    // Invoke runs dlopen on the given paths and returns results
    Invoke(ctx context.Context, paths []string) ([]DlopenResult, error)

    // Available returns true if the helper is ready to use
    Available() bool

    // Ensure downloads and verifies the helper if needed
    Ensure(ctx context.Context) error
}
```

*Benefit:* Enables mocking in unit tests without requiring actual cgo builds.

### 2.2 Error Type Hierarchy

The design mentions several error conditions but doesn't define structured error types:

```go
// Suggested error types
var (
    ErrHelperUnavailable   = errors.New("dltest helper not available")
    ErrChecksumMismatch    = errors.New("helper checksum verification failed")
    ErrHelperTimeout       = errors.New("helper timed out")
    ErrHelperCrash         = errors.New("helper crashed during verification")
    ErrBatchPartialFailure = errors.New("some libraries in batch failed to load")
)
```

*Benefit:* Enables callers to programmatically handle specific failure modes.

### 2.3 Configuration Options

The design mentions a 50-library batch size and 5-second timeout but doesn't show where these are configured:

```go
// Suggested configuration structure
type DltestConfig struct {
    BatchSize     int           // Default: 50
    Timeout       time.Duration // Default: 5s
    RetryOnCrash  bool          // Default: true
    RetryBatchSize int          // Default: 10 (smaller batches on retry)
}
```

*Recommendation:* These could be internal constants initially, with configuration exposure as a future enhancement.

---

## 3. Implementation Phase Sequencing

### 3.1 Current Sequence (from design)

1. Step 1: Helper Binary (`cmd/tsuku-dltest/main.go`)
2. Step 2: Trust Chain Module (`internal/verify/dltest.go`)
3. Step 3: Invocation Module (extension of Step 2)
4. Step 4: Integration (`internal/verify/library.go`)
5. Step 5: Build Infrastructure (`.goreleaser.yaml`)
6. Step 6: Tests

### 3.2 Sequence Assessment

**Issue: Build infrastructure too late in sequence**

The helper binary (Step 1) can't be tested without build infrastructure (Step 5). This creates a dependency gap.

**Revised sequence:**

1. **Phase 1: Foundation**
   - Step 5 first: Set up goreleaser config for `tsuku-dltest`
   - Step 1: Implement helper binary
   - CI verification: Ensure helper builds on all 4 platforms

2. **Phase 2: Trust Chain**
   - Step 2: Trust chain module (checksums initially placeholder)
   - Step 3: Invocation module with timeout handling
   - Unit tests with mock helper

3. **Phase 3: Integration**
   - Step 4: Wire into `verify.go`
   - Integration tests with real helper
   - Update checksums in source code

4. **Phase 4: Polish**
   - Step 6: Full test suite
   - Documentation updates

### 3.3 Dependency on M38 Completion

The design correctly notes this depends on M38 (Tier 2 Dependency Validation). The implementation should:

1. Verify M38 is complete before starting
2. Integrate with the existing `VerifyLibrary` function from M38
3. Add Level 3 as an additional verification step (not a replacement)

---

## 4. Simpler Alternatives Considered

### 4.1 Alternative: Shell Script Helper

Instead of a Go+cgo binary, use a shell script that wraps Python's ctypes:

```bash
#!/bin/bash
# tsuku-dltest.sh
python3 -c "
import ctypes
import json
import sys
results = []
for path in sys.argv[1:]:
    try:
        ctypes.CDLL(path)
        results.append({'path': path, 'ok': True})
    except Exception as e:
        results.append({'path': path, 'ok': False, 'error': str(e)})
print(json.dumps(results))
" "$@"
```

**Rejected because:**
- Python may not be available in minimal environments
- Python version differences (2 vs 3, ctypes behavior)
- Slower startup (~100ms vs ~5ms)
- Less control over error messages

**Verdict:** The design's rejection of this alternative is correct.

### 4.2 Alternative: LD_DEBUG Environment Variable

On Linux, setting `LD_DEBUG=all` causes the dynamic linker to print detailed loading information without actually running code. Could parse this output.

**Rejected because:**
- Linux-only (no macOS equivalent)
- Still executes some library code
- Parsing text output is fragile
- Doesn't work with nix-portable's modified linker

**Verdict:** Not a viable alternative.

### 4.3 Alternative: Static Analysis Only

Skip dlopen entirely and rely on header + dependency validation (Levels 1-2) to infer loadability.

**Rejected because:**
- Can't catch: symbol version mismatches, TLS initialization issues, corrupted code sections
- False confidence in verification results
- The umbrella design explicitly requires Level 3 for "definitive load answer"

**Verdict:** Not acceptable given design requirements.

### 4.4 Alternative: Lazy Helper Installation

Instead of auto-installing on first verification, require explicit `tsuku install tsuku-dltest`:

**Pros:**
- No surprise downloads during verification
- Explicit user consent for helper binary

**Cons:**
- Worse UX (user must remember extra step)
- Inconsistent with nix-portable pattern (auto-installs)

**Verdict:** Current design (auto-install) is correct. The fallback behavior handles cases where download fails.

---

## 5. Risk Assessment

### 5.1 Low Risk

| Risk | Likelihood | Impact | Mitigation in Design |
|------|------------|--------|---------------------|
| Helper download fails | Low | Low | Graceful degradation to Level 1-2 |
| JSON parsing error | Very Low | Low | Well-defined schema, Go's encoding/json |
| Batch size exceeds ARG_MAX | Very Low | Low | Conservative 50-library default |

### 5.2 Medium Risk

| Risk | Likelihood | Impact | Mitigation in Design |
|------|------------|--------|---------------------|
| Library initialization hangs | Medium | Medium | 5-second timeout |
| macOS Gatekeeper blocks helper | Medium | Medium | Code signing noted as future work |
| Helper crashes on specific library | Medium | Low | Retry with smaller batches |

### 5.3 High Risk (Needs Attention)

| Risk | Likelihood | Impact | Current Mitigation | Recommendation |
|------|------------|--------|-------------------|----------------|
| Memory growth across large batches | Medium | Medium | dlclose after each | Add explicit memory limit or batch count limit |

**Recommendation:** Add a maximum total libraries per helper invocation (e.g., 200) to limit memory pressure, with automatic process restart for larger verifications.

---

## 6. Security Review Notes

The design's security section is thorough. Additional notes:

### 6.1 Code Execution Window

The design accepts a 5-second window for library initialization code. Consider documenting:

- Libraries are already trusted (user chose to install them)
- The helper runs with same privileges as user
- No privilege escalation possible through this path

### 6.2 Helper Binary as Attack Surface

If an attacker could modify `$TSUKU_HOME/.dltest/tsuku-dltest`, they could:
- Return false "ok: true" for malicious libraries
- Execute arbitrary code when tsuku runs verification

**Mitigation (already in design):**
- Checksum verification before each execution
- Version file prevents rollback attacks
- Atomic rename prevents partial writes

**Additional recommendation:** Consider re-verifying checksum before each invocation (not just on install). The performance cost is negligible (~100us) and provides defense against post-install tampering.

---

## 7. Actionable Recommendations

### Critical (Must Address Before Implementation)

1. **Add file locking** to trust chain verification (match nix-portable pattern)
2. **Clarify stderr handling** in helper invocation
3. **Reorder implementation phases** to build infrastructure first

### Important (Should Address)

4. **Define Go interfaces** for `DltestRunner` to enable unit testing
5. **Add structured error types** for programmatic error handling
6. **Document macOS flag behavior** (confirm `RTLD_NOW | RTLD_LOCAL` works)

### Nice to Have (Consider for Implementation)

7. **Add per-invocation checksum verification** for defense in depth
8. **Add memory limit** for very large library verifications
9. **Add configuration for timeout** (some legitimate libraries may be slow)

---

## 8. Conclusion

The dlopen load testing design is solid and ready for implementation. The architecture follows established patterns in the codebase, makes defensible trade-offs, and addresses security concerns appropriately.

The main gaps are minor clarifications rather than fundamental issues:
- File locking (copy from nix-portable)
- Phase ordering (build infrastructure first)
- Interface definitions (can be added during implementation)

**Recommendation:** Proceed with implementation after addressing the three critical items above.
