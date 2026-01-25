---
status: Proposed
problem: tsuku claims support for multiple platforms and Linux families but testing doesn't verify actual compatibility, as discovered when musl-based systems couldn't load embedded libraries.
decision: Implement runtime musl detection with user guidance, hybrid testing (native runners + containers), and test coverage matching the release matrix exactly.
rationale: This balances accuracy with maintainability by using native runners for platform verification and containers for family-specific testing, while runtime detection provides fail-fast behavior for Alpine users without requiring immediate musl binary support.
---

# Platform Compatibility Verification

## Status

**Proposed**

## Context and Problem Statement

tsuku builds and releases binaries for 4 platform combinations (Linux/macOS x amd64/arm64) and claims support for 5 Linux distribution families (Debian, RHEL, Arch, Alpine, SUSE). However, the testing infrastructure doesn't adequately verify that tsuku actually works on all these targets.

This gap was exposed when adding dlopen integration tests: embedded library recipes (zlib, libyaml, gcc-libs, openssl) use Homebrew bottles built for glibc, which fail on musl-based systems like Alpine Linux. The libraries install without error, but fail at runtime with "Dynamic loading not supported" because they link against `libc.so.6` which doesn't exist on musl systems.

This reveals a broader pattern: tsuku's CI uses simulation (running Alpine-family tests on Ubuntu runners) rather than real environment verification. Other untested gaps likely exist:
- ARM64 Linux binaries are released but never integration-tested
- Only Debian family receives library dlopen verification
- openssl can't be verified due to system library conflicts

The current approach creates false confidence: tests pass on simulated environments while users on real systems encounter failures.

### Scope

**In scope:**
- Ensuring tests run on real target environments (actual containers/runners, not simulation)
- Verifying embedded library compatibility across libc implementations (glibc vs musl)
- Expanding dlopen verification to all supported Linux families
- Testing ARM64 Linux binaries
- Establishing a verification matrix that matches release targets

**Out of scope:**
- Adding new platform targets beyond current release matrix
- Changing the glibc Homebrew bottle approach (bottles remain the primary source for glibc systems)
- Windows support
- Non-Linux/macOS platforms

## Decision Drivers

- **Accuracy over speed**: Real environment tests are slower but catch real issues; simulated tests are fast but miss environment-specific bugs
- **Release parity**: Every binary released should have corresponding integration tests
- **Fail-fast discovery**: Platform incompatibilities should be caught in CI, not by users
- **Maintainability**: Test infrastructure should be sustainable as platforms evolve
- **CI resource constraints**: GitHub Actions has limited ARM64 runners and container support varies by runner type

## Implementation Context

### Current Testing Infrastructure

The codebase has established patterns for platform-aware testing:

**Container-based family testing**: The `test/scripts/` directory contains scripts that accept a `family` parameter (debian, rhel, arch, alpine, suse). These use dynamically-generated Dockerfiles or tsuku's sandbox system to run tests in family-specific environments.

**Platform detection**: `internal/platform/family.go` maps distribution IDs to families using `/etc/os-release` parsing, with fallback to `ID_LIKE` for derivative distributions.

**Homebrew bottle distribution**: Embedded libraries are downloaded from GHCR as Homebrew bottles with platform tags (`x86_64_linux`, `arm64_linux`). These bottles are built for glibc and include RPATH fixup via patchelf.

**dlopen verification**: The `tsuku-dltest` Rust helper performs Level 3 verification by calling dlopen on installed libraries. The Go code in `internal/verify/dltest.go` handles batching, timeouts, and retry logic.

### Industry Patterns

Research into how other projects handle cross-platform testing reveals:

**Native ARM64 runners**: GitHub Actions provides free native ARM64 Linux runners (`ubuntu-24.04-arm`) for public repos. These provide ~40% better performance than emulation.

**musl compatibility approaches**:
1. Static linking with musl for portable binaries (performance trade-offs)
2. Separate binary distribution for glibc and musl targets
3. Using Zig as a cross-compilation toolchain with bundled libc versions

**Testing strategy**: Projects like ripgrep distribute separate binaries per libc and run native tests on each target rather than cross-compile and emulate.

### Existing Gaps

| Test Type | Debian | RHEL | Arch | Alpine | SUSE | ARM64 |
|-----------|--------|------|------|--------|------|-------|
| checksum-pinning | glibc | glibc | glibc | musl | glibc | No |
| homebrew-recipe | glibc | glibc | glibc | musl | glibc | No |
| library-dlopen | glibc | No | No | Disabled | No | No |

The checksum and homebrew tests run in containers and do exercise real environments. However, library dlopen tests only run on Debian (glibc), and no tests run on ARM64 Linux.

## Considered Options

This design addresses three independent questions:

### Decision 1: How to verify musl compatibility?

The core issue is that embedded libraries (Homebrew bottles) are built for glibc and don't work on musl.

#### Option 1A: Document glibc requirement

Explicitly document that embedded library recipes require glibc. Users on musl systems would need to build tools from source or use alternative recipes.

**Pros:**
- No additional CI complexity
- Honest about current limitations
- No risk of breaking working configurations

**Cons:**
- Reduces tsuku's value proposition for Alpine users
- Embedded libraries are a key feature for hermetic builds
- Doesn't solve the underlying compatibility gap

#### Option 1B: Provide musl-specific library binaries

Maintain separate library binaries for musl targets, either by building from source in CI or sourcing from Alpine packages.

**Pros:**
- Full platform support
- Consistent experience across Linux distributions
- Enables hermetic builds on Alpine

**Cons:**
- Doubles library maintenance burden
- Build-from-source adds CI time and complexity
- Alpine packages may have different versioning than Homebrew

#### Option 1C: Runtime detection with user guidance

Detect musl at runtime and provide actionable guidance. When a user on Alpine tries to install or use embedded libraries, tsuku would warn them about the incompatibility and suggest alternatives (build from source, use system packages).

**Pros:**
- Preserves value for non-library recipes on Alpine
- Honest about limitations without hiding them in documentation
- Fail-fast with clear user guidance
- No additional binary maintenance

**Cons:**
- Doesn't solve the underlying compatibility gap
- Requires runtime detection logic
- May frustrate users who expected full Alpine support

### Decision 2: How to ensure tests run on real environments?

Current tests use the "family" parameter but run on Ubuntu runners, which doesn't catch environment-specific issues.

#### Option 2A: Container-based testing with real images

Run tests inside Docker containers using official distribution images (Alpine, Fedora, Arch, etc.). This is already done for checksum-pinning and homebrew-recipe tests.

**Pros:**
- Tests real package managers, paths, and system libraries
- Works on standard GitHub runners
- Consistent with existing test patterns

**Cons:**
- Container overhead adds CI time
- Some tests (like dlopen) may behave differently in containers vs native
- Requires maintaining container setup for each family

#### Option 2B: Native runners for each platform

Use GitHub's native runners for each platform: `ubuntu-latest` for Debian, `ubuntu-24.04-arm` for ARM64, and potentially self-hosted runners for other families.

**Pros:**
- Most accurate representation of user environments
- No container overhead or behavioral differences
- ARM64 runners are now available for free on public repos

**Cons:**
- Limited to what GitHub provides (no native RHEL, Arch, Alpine, SUSE runners)
- Self-hosted runners add maintenance burden
- Inconsistent approach across families

#### Option 2C: Hybrid approach

Use native runners where available (Ubuntu amd64/arm64, macOS) and containers for other families. Container tests verify family-specific behavior while native tests verify platform-specific behavior.

**Pros:**
- Best coverage with available resources
- Uses native ARM64 runner for ARM64 verification
- Containers fill gaps where native runners don't exist

**Cons:**
- Inconsistent testing methodology across targets
- More complex CI configuration
- Some gaps may still exist (e.g., real RHEL kernel behavior)

### Decision 3: How comprehensive should the verification matrix be?

The current matrix tests some combinations but not all release targets.

#### Option 3A: Match release matrix exactly

Test every binary that's released: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64. Skip family-specific tests for platforms that can't be tested in CI.

**Pros:**
- Clear contract: if we release it, we test it
- Focused effort on achievable goals
- No false confidence from partial testing

**Cons:**
- Some family-specific issues may not be caught
- Doesn't verify the full support matrix claimed

#### Option 3B: Test representative subset

Test a representative subset that exercises key code paths: one glibc distro (Debian), one musl distro (Alpine if supported), one macOS, and ARM64 where possible.

**Pros:**
- Efficient use of CI resources
- Catches most issues with minimal overhead
- Can expand coverage incrementally

**Cons:**
- May miss distro-specific edge cases
- Requires judgment about what's "representative"

#### Option 3C: Full matrix testing

Test all combinations: 2 macOS platforms + 10 Linux configurations (5 families x 2 architectures) = 12 configurations, using containers and emulation where native runners aren't available.

**Pros:**
- Maximum confidence in compatibility claims
- Catches edge cases across all targets
- Clear support story

**Cons:**
- Significant CI resource consumption
- Some combinations may be impractical (ARM64 Alpine container on amd64 runner)
- Diminishing returns for rare configurations

### Evaluation Against Decision Drivers

| Option | Accuracy | Release Parity | Fail-Fast | Maintainability | CI Resources |
|--------|----------|----------------|-----------|-----------------|--------------|
| 1A (Document) | Good | Good | Fair | Good | Good |
| 1B (Musl binaries) | Good | Good | Good | Fair | Fair |
| 1C (Runtime detect) | Good | Good | Good | Good | Good |
| 2A (Containers) | Good | Good | Good | Fair | Good |
| 2B (Native only) | Good | Good | Good | Fair | Good |
| 2C (Hybrid) | Good | Good | Good | Fair | Good |
| 3A (Match release) | Good | Good | Good | Good | Good |
| 3B (Representative) | Fair | Fair | Fair | Good | Good |
| 3C (Full matrix) | Good | Good | Good | Poor | Poor |

Note: Option 2B rates "Good" for release parity because GitHub provides native runners for all 4 release targets (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64). Family-specific testing is a separate concern addressed by containers.

### Uncertainties

- **Homebrew musl bottles**: It's unclear if Homebrew provides musl-compatible bottles or if they would need to be built from source
- **ARM64 container behavior**: Running ARM64 containers on amd64 runners via QEMU may have different behavior than native ARM64
- **CI time impact**: Adding container-based tests for all families could significantly increase CI time; actual impact depends on parallelization
- **Alpine user impact**: We don't have data on how many users run tsuku on Alpine or how critical embedded libraries are to those users
- **openssl conflict scope**: The system library conflict (#1090) may be addressable with better LD_LIBRARY_PATH isolation during verification

## Decision Outcome

**Chosen: 1C (Runtime detection) + 2C (Hybrid testing) + 3A (Match release matrix)**

### Summary

We will implement runtime detection to warn musl users about embedded library incompatibility, use a hybrid testing approach combining native runners (for release platforms) with containers (for family-specific verification), and ensure every released binary has corresponding integration tests. This combination balances accuracy with maintainability while being honest about current limitations.

### Rationale

**Decision 1 - Runtime detection (1C):**
This provides the best balance of user experience and maintenance burden. Documenting glibc requirements (1A) hides the limitation where users won't see it. Providing musl binaries (1B) doubles maintenance burden without clear user demand data. Runtime detection catches the problem early with actionable guidance, preserves value for non-library recipes on Alpine, and can be upgraded to full musl support (1B) later if demand justifies it.

**Decision 2 - Hybrid testing (2C):**
This maximizes coverage with available resources. Native runners provide ground-truth verification for release platforms (critical for accuracy), while containers enable family-specific testing that would otherwise be impossible. The inconsistent methodology is acceptable because we're verifying different aspects: native tests verify platform behavior, container tests verify family-specific package manager integration.

**Decision 3 - Match release matrix (3A):**
This establishes a clear contract: if we release it, we test it. The representative subset approach (3B) requires subjective judgments about what's "representative." Full matrix testing (3C) has diminishing returns and significant CI cost. Matching the release matrix (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64) focuses effort on what matters most.

**Alternatives rejected:**
- **1A (Document only)**: Doesn't provide fail-fast behavior; users discover the problem too late
- **1B (Musl binaries)**: Premature without Alpine user demand data; can add later
- **2A (Containers only)**: Misses platform-level verification; ARM64 emulation has known issues
- **2B (Native only)**: Can't verify family-specific behavior (RHEL, Arch, Alpine, SUSE)
- **3B (Representative)**: Too subjective; doesn't give clear coverage guarantees
- **3C (Full matrix)**: 12+ configurations with diminishing returns; unmaintainable

### Trade-offs Accepted

By choosing this approach, we accept:

1. **Alpine embedded library support deferred**: Users on Alpine won't have working embedded libraries until musl binaries are added (if ever). This is acceptable because we don't have evidence of significant Alpine user demand, and runtime detection provides clear guidance.

2. **Container tests may not catch all issues**: Containers on Ubuntu runners may behave differently than bare-metal RHEL, Arch, or SUSE systems. This is acceptable because container tests catch family-specific package manager behavior, which is the primary variation.

3. **No ARM64 family-specific testing**: We test ARM64 on native runners (Ubuntu only), not containers for other families. This is acceptable because family differences are primarily in package managers and paths, which are architecture-independent.

## Solution Architecture

### Overview

The solution has three components that work together:

1. **Runtime musl detection** - Go code that detects musl libc and warns users attempting to use embedded libraries
2. **Hybrid CI test matrix** - Native runners for release platforms + container jobs for family verification
3. **Verification coverage parity** - Ensuring dlopen tests run on all tested configurations

### Component 1: Runtime musl Detection

Add musl detection to the library installation and verification paths. When tsuku detects musl:
- Embedded library recipes will fail with a clear error message
- Non-library recipes continue to work normally
- The error message suggests alternatives (system packages, build from source)

```
internal/platform/
├── libc.go          # New: libc detection (glibc vs musl)
└── family.go        # Existing: Linux family detection
```

Detection approach: Check if `/lib/ld-musl-*.so.1` exists (primary), with `ldd --version` parsing as fallback. This is more reliable than checking for the absence of glibc.

Interface: `DetectLibc() string` returns "glibc", "musl", or "unknown". A helper `RequireGlibc() error` returns a sentinel `ErrMuslNotSupported` error for callers that need glibc.

Integration points:
- `internal/actions/homebrew.go` - Check in `Decompose()` (runs during plan generation, provides better error context)
- `internal/verify/dltest.go` - Check in `RunDlopenVerification()` (existing skip patterns return `Skipped=true` with warning)

### Component 2: Hybrid CI Test Matrix

Restructure CI to use the appropriate test environment for each verification type:

**Native runners (platform verification):**
| Platform | Runner | Tests |
|----------|--------|-------|
| linux-amd64 | `ubuntu-latest` | Full integration + dlopen |
| linux-arm64 | `ubuntu-24.04-arm` | Full integration + dlopen |
| darwin-amd64 | `macos-15-intel` | Full integration + dlopen |
| darwin-arm64 | `macos-latest` | Full integration + dlopen |

**Container jobs (family verification):**
| Family | Base Image | Tests |
|--------|------------|-------|
| debian | `debian:bookworm-slim` | Checksum, homebrew, dlopen |
| rhel | `fedora:41` | Checksum, homebrew, dlopen |
| arch | `archlinux:base` | Checksum, homebrew, dlopen |
| alpine | `alpine:3.19` | Checksum, homebrew (no dlopen - musl) |
| suse | `opensuse/leap:15` | Checksum, homebrew, dlopen |

### Component 3: Verification Coverage Parity

Expand dlopen tests to all glibc families and ARM64:

```yaml
# Current state
library-dlopen-glibc:
  matrix:
    library: [zlib, libyaml, gcc-libs]
    # Only runs on ubuntu-latest (debian family, amd64)

# Target state
library-dlopen:
  strategy:
    matrix:
      include:
        # Native platform tests
        - runner: ubuntu-latest
          family: debian
        - runner: ubuntu-24.04-arm
          family: debian
        - runner: macos-15-intel
          family: darwin
        - runner: macos-latest
          family: darwin
        # Container family tests (amd64 only, on ubuntu-latest)
        - runner: ubuntu-latest
          container: fedora:41
          family: rhel
        - runner: ubuntu-latest
          container: archlinux:base
          family: arch
        - runner: ubuntu-latest
          container: opensuse/leap:15
          family: suse
        # Alpine skipped - musl incompatible
```

### Data Flow

```
User runs: tsuku install zlib

1. Platform detection
   └─> Detect OS, arch, family
   └─> Detect libc (glibc vs musl)

2. Recipe resolution
   └─> If embedded library recipe:
       └─> If musl: ERROR with guidance
       └─> If glibc: Continue to Homebrew bottle

3. Installation
   └─> Download bottle from GHCR
   └─> Verify checksum
   └─> Extract and relocate (patchelf on Linux)

4. Verification
   └─> Level 1: File existence
   └─> Level 2: Dependency resolution
   └─> Level 3: dlopen test (glibc only)
```

## Implementation Approach

### Phase 1A: Runtime musl Detection

**Goal**: Prevent confusing failures on Alpine by detecting musl early.

**Changes**:
1. Add `internal/platform/libc.go` with `DetectLibc() string` returning "glibc", "musl", or "unknown"
2. Add `RequireGlibc() error` helper returning `ErrMuslNotSupported`
3. Add check in `internal/actions/homebrew.go` Decompose() before bottle planning
4. Add check in `internal/verify/dltest.go` RunDlopenVerification() returning skip with warning
5. Clear error message: "Embedded libraries require glibc. Alpine/musl users can: (1) use system packages, (2) build from source"
6. Add unit tests using mocked file paths (similar to existing `testdata/os-release/` pattern)

**Dependencies**: None

### Phase 1B: ARM64 Native Testing (parallel with 1A)

**Goal**: Verify ARM64 Linux binaries with real native tests.

**Changes**:
1. Add `ubuntu-24.04-arm` runner to integration tests
2. Add ARM64 to library-dlopen test matrix
3. Update release workflow to run integration tests on ARM64

**Dependencies**: None (ARM64 runners use Ubuntu/glibc, not Alpine/musl)

### Phase 2: Container-based Family Tests

**Goal**: Run dlopen verification in real family environments.

**Changes**:
1. Create container job variants for dlopen tests (Fedora, Arch, openSUSE)
2. Skip Alpine container dlopen tests (musl incompatible, graceful skip via Phase 1A detection)
3. Install Go via distro package managers (dnf, pacman, zypper) in containers
4. Build tsuku-dltest from source in each container

**Dependencies**: Phase 1A (musl detection for graceful Alpine handling)

### Phase 4: Documentation and Cleanup

**Goal**: Document the verification matrix and clean up related issues.

**Changes**:
1. Update README with platform support matrix
2. Close #1092 (musl support) with runtime detection solution
3. Re-enable musl CI jobs with appropriate skip conditions
4. Document how to add musl binary support in future if demand justifies

## Security Considerations

### Download Verification

**Impact**: This design does not introduce new download paths. The existing Homebrew bottle download mechanism with SHA256 checksum verification remains unchanged. The musl detection feature blocks downloads that would fail anyway, improving user experience without changing the security model.

**CI containers**: Container images used in testing (Fedora, Arch, Alpine, openSUSE) are pulled from official Docker Hub repositories. These are used only in CI, not in user-facing code. Image pinning by digest could be added for reproducibility but is not strictly necessary for test infrastructure.

### Execution Isolation

**Runtime musl detection**: The libc detection code reads system files (`/lib/ld-musl-*.so.1` or `ldd --version` output) with normal user permissions. No privilege escalation is required or introduced.

**CI test isolation**: Container-based tests run with default container isolation. The tsuku-dltest helper runs with sanitized environment variables (LD_PRELOAD, DYLD_INSERT_LIBRARIES stripped), which is existing behavior unchanged by this design.

**ARM64 native runners**: GitHub-hosted ARM64 runners have the same security posture as existing amd64 runners. No additional permissions or isolation changes are needed.

### Supply Chain Risks

**No new binary sources**: This design does not introduce new sources of binaries. Embedded libraries continue to come from Homebrew GHCR (already checksummed). The tsuku-dltest helper continues to be built from source in CI.

**CI container images**: Using official distribution images (fedora, archlinux, alpine, opensuse/leap) introduces a supply chain dependency on Docker Hub and the distributions' container image pipelines. This is acceptable for CI testing purposes. A compromise of these images could cause CI failures or false test passes, but would not affect user-installed binaries.

**Mitigation**: For higher assurance, images could be pinned by digest and periodically audited, but this adds maintenance burden for limited benefit in a test-only context.

### User Data Exposure

**No new data collection**: This design does not introduce new data collection or transmission. The musl detection runs entirely locally. CI tests don't transmit user data - they run on GitHub's infrastructure with test fixtures.

**libc detection**: The detection reads system files to determine libc type. This is purely local and doesn't expose any user data.

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Compromised CI container image | Use official distribution images; CI failures don't affect user binaries | CI could pass falsely; periodic image audit recommended |
| libc detection could be tricked | Detection uses multiple signals (file presence + ldd output) | Exotic environments might misdetect; acceptable given rarity |
| ARM64 runner security | Use GitHub-hosted runners with standard isolation | Same trust model as existing amd64 runners |

## Consequences

### Positive

- **Accurate platform coverage**: Every released binary (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64) will have corresponding integration tests, eliminating untested release artifacts
- **Fail-fast for musl users**: Alpine users discover the embedded library limitation immediately with actionable guidance, rather than encountering cryptic dlopen failures
- **Family-specific verification**: dlopen tests run in real distribution environments (Fedora, Arch, openSUSE), catching family-specific issues like path differences or package manager quirks
- **Clear support contract**: The verification matrix exactly matches the release matrix, making support claims verifiable

### Negative

- **Alpine embedded libraries deferred**: Users on Alpine/musl systems cannot use embedded library recipes. They must use system packages or build from source
- **Increased CI time**: Container-based family tests add overhead, though parallelization mitigates this
- **Container test limitations**: Container tests on Ubuntu runners may not catch kernel-level or hardware-level issues specific to other distributions
- **Maintenance burden**: More test configurations to maintain as families evolve (new Fedora versions, Arch changes, etc.)

### Mitigations

- **Alpine limitation**: Runtime detection provides clear guidance; the limitation can be lifted later if musl binaries are added
- **CI time**: Tests run in parallel across matrix; container setup is cached where possible
- **Container limitations**: Focus container tests on package manager and path behavior, which containers do represent accurately
- **Maintenance**: Use latest/rolling tags where appropriate (e.g., `archlinux:base`) to reduce version pinning burden
