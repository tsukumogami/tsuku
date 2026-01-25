# Architecture Review: Platform Compatibility Verification

## Executive Summary

The proposed solution architecture is clear and implementable. The three components (runtime musl detection, hybrid CI matrix, verification coverage parity) are well-defined with appropriate integration points. However, some interfaces need clarification, the phase sequencing has unnecessary dependencies, and there's a simpler alternative for musl detection that warrants consideration.

**Overall Assessment:** Ready for implementation with minor refinements.

---

## Question 1: Is the Architecture Clear Enough to Implement?

### Assessment: Mostly Yes, with Clarifications Needed

**What's Clear:**

1. **Component boundaries**: The three components have distinct responsibilities:
   - `internal/platform/libc.go` - musl detection
   - CI workflow changes - native + container runners
   - Test script modifications - expanded dlopen matrix

2. **Integration points**: The design correctly identifies where musl detection should be called:
   - `internal/actions/homebrew.go` - before bottle download
   - `internal/verify/dltest.go` - before dlopen verification

3. **Detection approach**: Using `/lib/ld-musl-*.so.1` existence check is practical and well-documented in musl documentation.

**Clarifications Needed:**

1. **Interface for `DetectLibc()`**: The design says it returns "glibc", "musl", or "unknown" but doesn't specify:
   - Should it be a package-level function or attached to a struct?
   - Should it cache the result (libc doesn't change at runtime)?
   - What about systems with both? (Rare but possible in containers)

   **Recommendation:** Make it a cached package-level function:
   ```go
   var (
       libcOnce   sync.Once
       detectedLibc string
   )

   func DetectLibc() string {
       libcOnce.Do(detectLibcImpl)
       return detectedLibc
   }
   ```

2. **Error message integration**: The design mentions a clear error message but doesn't specify:
   - Where the message is constructed (in libc.go or at call sites?)
   - Whether it should be a sentinel error like `ErrMuslUnsupported`
   - How to handle the "unknown" case

   **Recommendation:** Define a sentinel error in `internal/platform/libc.go`:
   ```go
   var ErrMuslNotSupported = errors.New("embedded libraries require glibc")

   func RequireGlibc() error {
       if DetectLibc() == "musl" {
           return fmt.Errorf("%w: Alpine/musl users can use system packages or build from source", ErrMuslNotSupported)
       }
       return nil
   }
   ```

3. **homebrew.go integration point**: The design says "check before bottle download" but `homebrew.go` has two entry points:
   - `Execute()` - called at runtime
   - `Decompose()` - called during plan generation

   **Recommendation:** Check in `Decompose()` since it runs first and produces the download steps. This fails earlier with better context. The check in `Execute()` becomes a safety net (check once in Decompose, assert in Execute).

4. **dltest.go integration point**: The design says "before dlopen verification" but doesn't specify exactly where in the code flow:
   - In `EnsureDltest()` before installing the helper?
   - In `RunDlopenVerification()` before any work?
   - In `InvokeDltest()` at the lowest level?

   **Recommendation:** Check in `RunDlopenVerification()` since:
   - `EnsureDltest()` is about the helper binary, which IS musl-compatible
   - `RunDlopenVerification()` is where we decide to perform dlopen tests
   - Return early with `Skipped=true, Warning="..."` for musl detection

---

## Question 2: Are There Missing Components or Interfaces?

### Assessment: Two Gaps Identified

**Gap 1: Test Infrastructure for musl Detection**

The design doesn't address how to test the musl detection itself. The existing pattern in `internal/platform/` shows test data files in `testdata/os-release/` but libc detection uses different signals.

**Recommendation:** Add testing approach:
```
internal/platform/
├── libc.go
├── libc_test.go         # Unit tests with mocked file system
└── testdata/
    └── lib/
        ├── ld-musl-x86_64.so.1  # Marker file for musl
        └── x86_64-linux-gnu/    # Marker dir for glibc
```

The tests should inject the path prefix rather than reading system files.

**Gap 2: Container Image Specification**

The design mentions container images (fedora:41, archlinux:base, etc.) but doesn't specify:
- Whether to use the `container:` directive or explicit `docker run` commands
- How to handle Go/Rust toolchain installation in containers
- Whether to use pre-built images or build Dockerfiles

**Recommendation:** Use the `container:` directive for simplicity, matching the existing (commented) pattern in `integration-tests.yml`:
```yaml
container:
  image: fedora:41
steps:
  - name: Install build dependencies
    run: dnf install -y golang rust cargo
```

This is simpler than Dockerfiles and allows reuse of checkout/setup actions.

**Gap 3: Rust Toolchain in Containers**

The dlopen tests require building `tsuku-dltest` from source. The design doesn't address how to get Rust into containers.

**Recommendation:** Use the standard approach:
- Fedora: `dnf install rust cargo`
- Arch: `pacman -S rust`
- openSUSE: `zypper install rust cargo`

Pre-cache could help but adds complexity. Start simple and optimize if needed.

---

## Question 3: Are Implementation Phases Correctly Sequenced?

### Assessment: Dependencies Are Overstated

The design proposes:
- Phase 1: Runtime musl detection
- Phase 2: ARM64 native testing (depends on Phase 1)
- Phase 3: Container-based family tests (depends on Phase 1, 2)
- Phase 4: Documentation and cleanup

**Analysis:**

The claimed dependency of Phase 2 on Phase 1 is unnecessary:
> "Phase 2 depends on Phase 1 (musl detection prevents false failures if ARM64 Alpine ever tested)"

This is speculative. ARM64 native testing uses `ubuntu-24.04-arm`, which runs glibc. There's no ARM64 Alpine testing proposed. The phases can run in parallel:

```
Phase 1 (musl detection)     ─────────────────────►
Phase 2 (ARM64 native)       ─────────────────────►
                                                  Phase 3 (containers)
                                                          │
                                                  Phase 4 (docs)
```

**Revised Sequencing:**

- **Phase 1A: Runtime musl detection** (can start immediately)
  - Add `internal/platform/libc.go`
  - Integrate into homebrew.go and dltest.go
  - Tests with mocked file system

- **Phase 1B: ARM64 native testing** (can start immediately, parallel to 1A)
  - Add `ubuntu-24.04-arm` runner to test matrix
  - Verify dlopen tests pass on ARM64
  - No code changes needed if tests already work

- **Phase 2: Container-based family tests** (depends on 1A completing)
  - Requires musl detection so Alpine dlopen tests skip gracefully
  - Add Fedora, Arch, openSUSE container jobs
  - Ensure Rust toolchain available in each

- **Phase 3: Documentation and cleanup** (depends on 1A, 1B, 2)
  - Update platform support docs
  - Re-enable Alpine CI with skip conditions
  - Close related issues

**Benefits of Revised Sequencing:**
- ARM64 testing ships faster (may uncover issues sooner)
- Musl detection can be reviewed while ARM64 tests run
- Total timeline shorter through parallelization

---

## Question 4: Are There Simpler Alternatives We Overlooked?

### Alternative 1: Skip musl Detection at Build Time Instead

**Current design:** Runtime detection in `homebrew.go` and `dltest.go`.

**Alternative:** Use build tags to exclude embedded library support on musl at compile time.

```go
//go:build !musl

package actions

type HomebrewAction struct{ ... }
```

With a separate `homebrew_musl.go`:
```go
//go:build musl

package actions

func (a *HomebrewAction) Execute(...) error {
    return errors.New("embedded libraries not supported on musl")
}
```

**Evaluation:** This is MORE complex because:
- Requires cross-compilation with different tags
- Release matrix would need musl-specific builds
- Runtime detection is simpler and more flexible

**Verdict:** Keep runtime detection.

### Alternative 2: Use `ldd` Output Parsing Instead of File Check

**Current design:** Check `/lib/ld-musl-*.so.1` existence.

**Alternative:** Parse `ldd --version` output:
- glibc: "ldd (GNU libc) 2.35"
- musl: "musl libc (x86_64)" or similar

**Evaluation:**
- More portable across different musl installations
- Requires subprocess execution
- May not be available in minimal containers

**Verdict:** Use file check as primary, `ldd` as fallback. The design already suggests this:
> "Detection approach: Check if `/lib/ld-musl-*.so.1` exists, or parse `ldd --version` output."

### Alternative 3: Simpler CI Matrix Without Containers

**Current design:** Native runners + containers for families.

**Alternative:** Only test on native runners, rely on community bug reports for family-specific issues.

```yaml
jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, ubuntu-24.04-arm, macos-latest, macos-15-intel]
```

**Evaluation:**
- Much simpler CI configuration
- Faster CI runs
- Misses family-specific package manager differences
- The checksum/homebrew tests already use containers, so infrastructure exists

**Verdict:** Keep hybrid approach. The container infrastructure already exists and has caught real issues (musl discovery).

### Alternative 4: Defer ARM64 Testing to After Musl Detection

**Current design claims:** Phase 2 depends on Phase 1.

**Alternative:** Ship ARM64 testing immediately, independent of musl detection.

**Evaluation:** This is what I recommended in Question 3. ARM64 GitHub runners use Ubuntu (glibc), so musl detection isn't required. The dependency is artificial.

**Verdict:** Implement ARM64 testing in parallel with musl detection.

---

## Additional Observations

### Existing Code Quality

The codebase is well-structured for this change:

1. **`internal/platform/`** is the right home for `libc.go`. The existing `family.go` and `target.go` establish patterns for platform detection.

2. **`internal/verify/dltest.go`** already has good separation:
   - `EnsureDltest()` - helper installation
   - `RunDlopenVerification()` - orchestration with skip logic
   - `InvokeDltest()` - actual verification

   Adding musl check to `RunDlopenVerification()` follows the existing skip pattern:
   ```go
   // User explicitly requested skip - silent, no warning
   if skipDlopen {
       return &DlopenVerificationResult{Skipped: true}, nil
   }
   ```

3. **CI workflows** follow established patterns. The commented-out musl tests in `integration-tests.yml` and `test.yml` provide templates for the container approach.

### Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| File-based musl detection fails on exotic systems | Low | Low | Fall back to ldd parsing |
| ARM64 runner availability changes | Low | Medium | Tests skip gracefully if runner unavailable |
| Container image changes break tests | Medium | Low | Use rolling tags (fedora:latest) with periodic review |
| Rust toolchain installation slow in containers | Medium | Low | Cache cargo downloads, consider pre-built images later |

### Recommended Implementation Order

1. **PR 1: ARM64 native testing** (low risk, high value)
   - Add `ubuntu-24.04-arm` to test matrix
   - Verify existing tests pass
   - No code changes needed

2. **PR 2: Musl detection** (medium complexity)
   - Add `internal/platform/libc.go`
   - Add tests with mocked file system
   - Integrate into `homebrew.go` (Decompose)
   - Integrate into `dltest.go` (RunDlopenVerification)

3. **PR 3: Container-based family tests** (depends on PR 2)
   - Add Fedora container job
   - Add Arch container job
   - Add openSUSE container job
   - Skip dlopen on Alpine (musl detected)

4. **PR 4: Documentation** (depends on all above)
   - Update platform support matrix
   - Close related issues
   - Remove commented-out musl tests or update them

---

## Summary of Recommendations

1. **Clarify interfaces:**
   - Add `RequireGlibc() error` helper function
   - Define `ErrMuslNotSupported` sentinel error
   - Specify caching strategy for `DetectLibc()`

2. **Fill gaps:**
   - Add test strategy for musl detection (mocked file system)
   - Document container toolchain installation approach
   - Use `container:` directive for simplicity

3. **Revise phase sequencing:**
   - Run Phase 1A (musl detection) and Phase 1B (ARM64 testing) in parallel
   - Reduce total implementation time by ~1 week

4. **Keep existing decisions:**
   - Runtime detection over build-time
   - Hybrid testing (native + containers)
   - File check with ldd fallback

The architecture is sound and ready for implementation with these refinements.
