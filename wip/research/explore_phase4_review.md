# Design Review: DESIGN-library-verify-dlopen.md

**Reviewer**: Claude (Phase 4 Critical Review)
**Date**: 2026-01-18
**Document**: `docs/designs/DESIGN-library-verify-dlopen.md`

## Executive Summary

The dlopen load testing design is well-structured and addresses its stated scope appropriately. The problem statement is clear, the options analysis is fair, and the chosen approach aligns with the umbrella design's direction. However, several areas need clarification or strengthening before implementation.

**Overall Assessment**: Ready for implementation with minor revisions.

---

## 1. Problem Statement Analysis

### Strengths

The problem statement clearly articulates:
- Why Levels 1-2 are insufficient (structural validation vs. actual loadability)
- The specific failure modes caught only by dlopen (linker incompatibilities, corrupted code sections, symbol versioning)
- Why a helper binary is needed (CGO_ENABLED=0 constraint)
- Security implications of running initialization code

### Weaknesses

**Missing quantification of the problem**: The design asserts that "runtime-only link failures" exist but doesn't provide concrete examples or data. Questions:
- How often do libraries pass header validation but fail dlopen?
- Are there real-world examples from tsuku users or similar tools?

**Recommendation**: Add a brief section with examples or reference research that motivated Level 3. Even anecdotal evidence ("we observed X during testing") would strengthen the case.

**Ambiguity in "corrupted code sections"**: The design mentions dlopen catches "corrupted code sections" but later acknowledges "dlopen verification doesn't detect code corruption until function is called." This is contradictory.

**Recommendation**: Clarify that dlopen catches corruptions that prevent loading (e.g., invalid relocations, malformed segments) but not silent corruptions in function bodies.

---

## 2. Missing Alternatives Analysis

### Alternatives Considered

The design evaluates three dlopen approaches (helper binary, Python ctypes, CGO_ENABLED=1), three protocols (JSON, exit codes, line-based), and two batching strategies (batched, per-file). This coverage is adequate.

### Missing Alternatives Worth Mentioning

**1. stdin-based input instead of command-line arguments**

For very large batches, command-line argument limits could be hit (ARG_MAX is typically 128KB-2MB). The design should either:
- Document the batch size limit this implies (~1000 paths of 100 chars each)
- Or consider stdin as an alternative input method

**2. Long-running daemon mode**

For repeated verifications (e.g., `tsuku verify --all`), spawning the helper once and sending multiple batches via stdio could eliminate per-invocation overhead entirely.

**Trade-off**: More complex implementation, but worth mentioning as a future optimization path.

**3. RTLD_NOLOAD for "already loaded" detection**

On Linux, `dlopen(path, RTLD_NOLOAD)` checks if a library is already loaded without actually loading it. Not directly applicable to verification, but the design could mention this flag exists (for completeness and to preempt "why not use RTLD_NOLOAD?" questions).

### Verdict

The missing alternatives don't invalidate the chosen approach. They should be noted in an "Alternatives Not Considered" or "Future Considerations" section.

---

## 3. Pros/Cons Fairness Assessment

### Option 1A (Helper Binary) - Fair

Pros and cons are balanced. The "4 binaries to build" con is real but manageable with existing CI (goreleaser handles multi-platform builds).

### Option 1B (Python ctypes) - Fair

The design fairly notes Python's availability issues and startup overhead. The "100ms vs 5ms" comparison is realistic for Python interpreter startup.

**Minor quibble**: "Python version differences (2 vs 3)" is less relevant in 2026 since Python 2 is EOL. Could simplify to "Python may not be installed."

### Option 1C (CGO in main tsuku) - Fair

The table comparing CGO_ENABLED=0 vs CGO_ENABLED=1 is well-researched. The build complexity concerns are legitimate.

**Potential strawman concern**: This option is dismissed partly due to "build infrastructure" complexity, but tsuku already uses goreleaser which supports CGO cross-compilation with zig. The dismissal is still valid (user portability is the real issue), but the "complex build infrastructure" argument is weaker than presented.

### Option 2B (Exit codes only) - Fair

Correctly identified as non-debuggable. Not a strawman; exit-code-only tools are a legitimate pattern for simple cases.

### Option 2C (Line-based) - Borderline Fair

The "fragile parsing" con about newlines in error messages is valid, but solvable (base64 encoding, length prefixes). The con makes line-based seem worse than it is.

**Recommendation**: Acknowledge that line-based parsing is solvable but JSON is more extensible with less custom code.

### Option 3A vs 3B (Batched vs Per-file) - Fair

The performance numbers (50 files x 5ms = 250ms vs ~10ms batched) are realistic. The trade-off between isolation and performance is correctly identified.

**Missing nuance**: The "crash affects only one result" benefit of per-file invocation could be achieved in batched mode by forking per-library within the helper. This hybrid wasn't considered.

---

## 4. Unstated Assumptions

### Assumptions That Should Be Explicit

**1. Libraries don't depend on environment variables for initialization**

Some libraries read environment variables during initialization (e.g., locale settings, debug flags). The helper runs in a minimal environment, which might cause false failures.

**Recommendation**: Document whether the helper inherits tsuku's environment or runs in a clean environment.

**2. Library initialization completes quickly**

The design mentions a 5-second timeout but doesn't discuss what happens if initialization legitimately takes longer (e.g., a library that does network initialization).

**Recommendation**: Clarify that slow-initializing libraries will fail verification and users should use `--skip-dlopen` for such cases.

**3. dlclose actually unloads the library**

The design assumes `dlclose()` releases memory, but some libraries use `RTLD_NODELETE` or have reference counting issues that prevent unloading. This could cause memory growth across batches.

**Recommendation**: The "Uncertainties" section mentions this but should be stronger: "Batch size must be conservative because dlclose doesn't guarantee unloading."

**4. The helper binary can be built for all target platforms**

The design assumes goreleaser can produce CGO-enabled binaries for all 4 platforms. This requires:
- A macOS machine (or cross-compiler) for darwin builds
- musl-gcc for truly portable Linux builds

**Recommendation**: Document the build requirements in a "Build Infrastructure" section.

**5. Users can download the helper binary**

The design assumes network access to GitHub releases. Air-gapped environments would need a different distribution mechanism (bundled with tsuku, local mirror).

**Recommendation**: Add a note about air-gapped environments in the "Fallback Behavior" section.

---

## 5. Strawman Analysis

**Verdict**: No options appear designed to fail.

Each option has legitimate use cases:
- Python ctypes: Reasonable for tools that already depend on Python
- CGO in main binary: Used by many Go projects (Docker, kubernetes tools)
- Exit codes: Used by simple checker tools
- Per-file invocation: Maximum isolation, used by security-critical tools

The evaluation matrix fairly shows trade-offs rather than biasing toward the recommended option.

---

## 6. Consistency with Umbrella Design

The dlopen design aligns well with DESIGN-library-verification.md:

| Umbrella Design Requirement | dlopen Design Coverage |
|----------------------------|------------------------|
| Helper binary approach | Yes (tsuku-dltest) |
| JSON protocol | Yes (Option 2A selected) |
| Embedded checksums | Yes (dltestChecksums map) |
| Batched verification | Yes (Option 3A selected) |
| Timeout handling | Yes (5-second timeout mentioned) |
| --skip-dlopen flag | Yes |
| Graceful degradation | Yes (Level 1-2 still run) |

**No conflicts identified.**

---

## 7. Technical Concerns

### 7.1 RTLD_LAZY vs RTLD_NOW

The design states "RTLD_NOW is preferred to catch symbol resolution failures" but the example code uses `RTLD_LAZY`:

```go
handle := C.dlopen(cpath, C.RTLD_LAZY)
```

**Recommendation**: The implementation should use `RTLD_NOW` to match the stated preference, or the text should explain why RTLD_LAZY was chosen (faster, sufficient for "does it load?" question).

### 7.2 Error Message Extraction on macOS

On macOS, `dlerror()` returns thread-local state that can be overwritten by subsequent calls. The design's simple "call dlerror after failed dlopen" pattern is correct, but should note that error strings should be copied immediately.

### 7.3 Helper Binary Version Coordination

The design mentions checksums are embedded in tsuku source, but doesn't specify how version mismatches are handled. Scenarios:
- User has old helper, new tsuku
- User has new helper, old tsuku (shouldn't happen if checksums change)

**Recommendation**: Add explicit version checking: helper outputs version, tsuku verifies it matches expected version before using results.

---

## 8. Recommendations Summary

### Must Fix (Before Implementation)

1. **Resolve RTLD_LAZY/RTLD_NOW inconsistency**: Update example code or text to be consistent.
2. **Document environment inheritance**: Specify whether helper inherits tsuku's environment.

### Should Fix (During Implementation)

3. **Add batch size limit**: Document explicit limit (e.g., 100 libraries) based on ARG_MAX and memory concerns.
4. **Clarify "corrupted code sections"**: Distinguish load-blocking corruption from silent corruption.
5. **Add version checking protocol**: Helper should output its version for tsuku to verify.

### Nice to Have (Future Considerations)

6. **Document stdin input as future option**: For very large batches.
7. **Mention daemon mode possibility**: For repeated verifications.
8. **Note air-gapped environment workaround**: Bundle helper or local mirror.

---

## 9. Conclusion

The dlopen design is solid and ready for implementation after addressing the RTLD_LAZY/RTLD_NOW inconsistency. The problem statement is sufficiently specific, the options analysis is fair, and no strawman options were detected. The identified assumptions should be documented but don't block implementation.

The design appropriately balances:
- Security (process isolation, opt-out flag)
- Performance (batching)
- Usability (graceful degradation)
- Maintainability (follows existing helper binary pattern)

**Recommendation**: Approve with minor revisions.
