# Phase 4 Review: Platform Compatibility Verification Design

## Review Summary

This review analyzes the DESIGN-platform-compatibility-verification.md document against the stated problem, evaluating option completeness, fairness, and hidden assumptions.

---

## 1. Problem Statement Evaluation

### Strengths

The problem statement is well-grounded in a concrete incident (glibc/musl incompatibility with Homebrew bottles). It identifies specific gaps:
- ARM64 Linux untested despite releasing binaries
- dlopen verification only on Debian
- openssl conflicts noted

### Weaknesses

**Missing quantification of impact:**
- How many users are affected by musl incompatibility? Alpine usage statistics would inform priority.
- No data on ARM64 Linux user base versus effort to test.

**Scope boundary ambiguity:**
The statement says "changing the Homebrew bottle approach for library distribution" is out of scope, but Option 1B (musl-specific library binaries) and 1C (static libraries for musl) effectively change the distribution approach for musl targets. This creates tension between the stated scope and considered options.

**Recommendation:** Clarify scope as "changing the *glibc* Homebrew bottle approach" since musl is explicitly being addressed.

---

## 2. Missing Alternatives Analysis

### Decision 1 (musl compatibility): Missing Options

**1D: Runtime detection with user warning**
Instead of failing silently or documenting glibc requirement, tsuku could detect musl at runtime and provide actionable guidance:
```
Warning: Embedded library 'zlib' uses glibc, but this system uses musl.
  Option 1: Install system zlib via 'apk add zlib'
  Option 2: Use --system-libs flag to prefer system libraries
```

This preserves tsuku's value for non-library recipes on Alpine while being honest about library limitations.

**1E: nix-portable fallback for musl**
tsuku already uses nix-portable for complex builds. Libraries could use nix-portable on musl systems, providing hermetic builds without maintaining separate binaries.

**Pros:**
- Leverages existing nix-portable infrastructure
- True hermetic builds on musl
- No double maintenance burden

**Cons:**
- nix-portable adds ~100MB overhead
- Slower first-time installs
- Adds complexity to library installation path

### Decision 2 (real environments): Missing Options

**2D: QEMU user-mode emulation for ARM64**
GitHub Actions can run ARM64 binaries via QEMU user-mode emulation on amd64 runners:
```yaml
- uses: docker/setup-qemu-action@v3
  with:
    platforms: linux/arm64
- run: docker run --platform linux/arm64 ...
```

This is slower than native runners but doesn't require special runner access.

**Pros:**
- Available on all GitHub runners
- No additional cost
- Works for most binary testing

**Cons:**
- 5-10x slower than native
- Some syscall edge cases behave differently
- Not true "real environment" testing

This option could be a fallback when native ARM64 runners are unavailable.

### Decision 3 (matrix scope): No missing options identified

The three options (match release, representative subset, full matrix) cover the reasonable spectrum.

---

## 3. Pros/Cons Fairness Analysis

### Option 1A (Document glibc requirement)

**Current pros/cons are fair.** However, missing:

**Additional pro:**
- Immediate actionability (can ship with next release)
- Doesn't preclude future musl support

**Missing context:**
The con "Reduces tsuku's value proposition for Alpine users" needs quantification. If Alpine users are 2% of user base and 80% of them don't use embedded libraries, the impact is small.

### Option 1B (Provide musl-specific library binaries)

**Understated complexity:**
The con "Doubles library maintenance burden" understates the challenge:
- Homebrew doesn't provide musl bottles, so binaries must be built from source
- Source builds require build dependencies (compilers, headers)
- Version matching between glibc and musl builds adds testing burden
- CI time could increase significantly (4 new build targets per library)

**Missing pro:**
- Once implemented, provides consistent user experience across all Linux targets

### Option 1C (Static library distribution for musl)

**Understated con:**
"Not all consumers can use static libraries" - this is a significant limitation. Many Homebrew formulae and tools expect shared libraries. dlopen inherently requires shared libraries.

**This option may be a strawman** - it's presented as simpler than 1B but doesn't actually solve the dlopen verification problem since static libraries can't be dlopen'd.

### Option 2A (Container-based testing)

**Missing important caveat:**
The pro "Tests real package managers, paths, and system libraries" is only partially true. Container tests run on the *host kernel*, not the target kernel. For example:
- Alpine container on Ubuntu runner uses Ubuntu's kernel
- Kernel-specific behaviors (cgroups, namespaces, etc.) aren't tested
- File system behaviors may differ

This matters less for tsuku (mostly userspace) but is worth noting.

### Option 2B (Native runners only)

**Overstated limitation:**
The con "Limited to what GitHub provides" is less limiting than implied:
- ubuntu-24.04-arm exists for ARM64 Linux (free for public repos)
- macOS runners cover darwin-amd64 and darwin-arm64
- Only non-Ubuntu Linux families lack native runners

**Missing pro:**
- Simpler CI configuration (no Docker layer)
- Faster execution (no container overhead)

### Option 2C (Hybrid approach)

**Fairly presented.** The "inconsistent testing methodology" con is honest.

### Option 3A (Match release matrix)

**Missing important implication:**
This option implies accepting that family-specific issues won't be caught for non-Debian glibc families. The current CI already runs homebrew-recipe tests on all families - would those continue under 3A?

### Option 3B (Representative subset)

**Fairly presented** but "requires judgment about what's representative" is actually a feature - it forces explicit reasoning about coverage.

### Option 3C (Full matrix)

**The math is misleading:**
"4 platforms x 5 families = up to 24 configurations" - but:
- macOS doesn't have families (2 darwin configs only)
- 5 families apply to Linux only (2 archs x 5 = 10 Linux configs)
- Total: 2 (macOS) + 10 (Linux) = 12 configurations, not 24

The "diminishing returns" argument is valid but should acknowledge that Debian/RHEL/SUSE all use glibc - testing all three catches different *package manager* issues, not libc issues.

---

## 4. Unstated Assumptions

### Assumption 1: Container tests on Ubuntu runners provide meaningful family coverage

The current CI runs "checksum-pinning" and "homebrew-recipe" tests for all 5 families on Ubuntu runners. These tests use Docker containers with family-specific base images.

**Reality check:** The design document says "current tests use simulation (running Alpine-family tests on Ubuntu runners)" as a criticism, but the existing container tests DO run on real Alpine, Fedora, etc. images. The "simulation" criticism applies mainly to:
1. ARM64 testing (no containers run on ARM64)
2. Kernel-specific behavior
3. dlopen tests (which are disabled for Alpine)

The design should distinguish between "simulated family" (which is real container) and "simulated architecture" (which isn't happening).

### Assumption 2: dlopen verification failure on musl is the primary Alpine pain point

The document focuses on dlopen verification, but the underlying issue is that embedded libraries don't work on musl at all - not just for verification. Even without verification, a user running `tsuku install nodejs` would get a nodejs that can't find its dependencies on Alpine.

This suggests Option 1A (document glibc requirement) affects more than just verification - it affects the entire embedded library feature for musl users.

### Assumption 3: GitHub Actions ARM64 runners are freely available

The document states "GitHub provides free native ARM64 Linux runners for public repos (`ubuntu-24.04-arm`)". This is correct as of 2024, but:
- Availability may change
- Private repos pay for ARM64 minutes
- Runner availability can cause job queuing delays

### Assumption 4: openssl conflicts are inherent and unfixable

The document notes "openssl can't be verified due to system library conflicts" but doesn't explain why. The issue (#1090) should be referenced for context.

Actually, the CI workflow comment says: "openssl excluded: system libcrypto.so.3 conflicts with tsuku-installed version". This is a specific technical issue that could potentially be addressed by:
- Setting LD_LIBRARY_PATH during dlopen verification
- Using a clean library path for verification
- Isolating the verification environment

This deserves an explicit uncertainty entry.

### Assumption 5: Test coverage for RHEL/Arch/SUSE is equivalent to Debian

The design treats Debian as the "primary" glibc target (dlopen tests run on Debian only). But:
- RHEL uses different glibc compilation flags
- Arch may have newer glibc versions
- SUSE has its own libc patches

If the goal is "accuracy over speed", why isn't dlopen testing expanded to other glibc families before addressing musl?

---

## 5. Strawman Analysis

### Option 1C (Static libraries for musl) shows strawman characteristics

1. **Contradicts decision driver:** "Cross-platform" is a decision driver from the umbrella design, and static libraries fundamentally don't work with dlopen
2. **Presented with cons that make it unworkable:** "Not all consumers can use static libraries" invalidates the option for the dlopen verification use case
3. **Missing critical analysis:** The design never explicitly states that static libraries can't be dlopen'd, which is the core function being verified

**Verdict:** 1C may be included for completeness but is not a viable option for the dlopen verification problem. It should either be explicitly rejected in the analysis or removed.

### Other options appear genuine

Options 1A, 1B, 2A, 2B, 2C, 3A, 3B, 3C all have legitimate trade-offs and could reasonably be chosen depending on priorities.

---

## 6. Evaluation Matrix Concerns

### Evaluation Against Decision Drivers table issues

1. **1A has N/A for accuracy, release parity, fail-fast** - this seems wrong. Documenting glibc requirement IS accurate (it's honest), maintains release parity (doesn't claim unsupported targets), and is fail-fast (recipe validation could reject musl-incompatible libs on Alpine).

2. **2B rated "Poor" for release parity** - but GitHub provides native runners for all 4 release targets (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64). The "poor" rating seems to conflate "platform" with "family".

3. **Missing "effort" column** - decision drivers mention "maintainability" but the evaluation doesn't clearly show implementation effort. 1A is trivially implementable; 1B requires significant infrastructure work.

---

## 7. Recommendations

### Immediate clarifications needed

1. **Clarify scope boundary** for Homebrew bottle approach - musl options do change distribution approach
2. **Add Option 1D** (runtime detection with warning) as a middle ground
3. **Fix Option 3C math** - 12 configurations, not 24
4. **Address or remove Option 1C** - static libraries can't solve dlopen verification
5. **Add "implementation effort" to evaluation matrix**

### Additional analysis recommended

1. **Quantify Alpine/musl user base** - informs priority of musl support
2. **Clarify relationship between existing container tests and proposed real environment tests** - current homebrew-recipe tests DO use real containers
3. **Investigate openssl conflict** (#1090) - may be solvable without excluding openssl
4. **Consider phased approach:** 1A now (document limitation) + 1D later (runtime detection) + 1B eventually (full musl support)

### Questions for decision makers

1. Is musl support a blocking requirement, or acceptable as future work?
2. Are family-specific tests (RHEL, Arch, SUSE) providing value beyond Debian for glibc targets?
3. Should ARM64 testing priority be higher than musl support given that ARM64 binaries are already released?

---

## 8. Summary of Key Findings

| Finding | Severity | Recommendation |
|---------|----------|----------------|
| Option 1C is a strawman (static libs can't dlopen) | High | Remove or explicitly reject |
| Scope boundary unclear for musl options | Medium | Clarify in problem statement |
| Missing Option 1D (runtime detection) | Medium | Add as alternative |
| Evaluation matrix has rating inconsistencies | Medium | Fix 1A and 2B ratings |
| Option 3C math error (24 vs 12) | Low | Correct calculation |
| Existing container tests undervalued | Low | Acknowledge in "Implementation Context" |

---

## Appendix: Current Test Coverage Analysis

Based on `.github/workflows/integration-tests.yml`:

| Test Type | Families Covered | Platforms | Real Environment |
|-----------|------------------|-----------|------------------|
| checksum-pinning | All 5 | amd64 only | Yes (Docker containers) |
| homebrew-recipe | All 5 | amd64 only | Yes (Docker containers) |
| library-integrity | Debian only | amd64 only | Native runner |
| library-dlopen-glibc | Debian only | amd64 only | Native runner |
| library-dlopen-musl | Disabled | N/A | N/A |
| library-dlopen-macos | darwin | arm64 only | Native runner |

**Gap summary:**
- ARM64 Linux: No integration tests
- Non-Debian glibc families: No dlopen tests
- musl/Alpine: dlopen tests disabled
- darwin-amd64: No dlopen tests (only macos-latest = arm64)
