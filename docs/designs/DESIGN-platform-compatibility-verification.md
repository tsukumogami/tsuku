---
status: Proposed
problem: tsuku claims support for multiple platforms and Linux families but testing doesn't verify actual compatibility, as discovered when musl-based systems couldn't load embedded libraries.
decision: Adopt "self-contained tools, system-managed dependencies" philosophy - tools remain self-contained binaries, but library dependencies use system package managers rather than embedded Homebrew bottles.
rationale: System packages are available on all Linux families (including Alpine), are security-reviewed by distro teams, and eliminate the glibc/musl incompatibility. This trades hermetic version control for working tools across all platforms.
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
- Windows support
- Non-Linux/macOS platforms

**Scope clarification:** This design DOES change the approach for library dependencies. The original assumption that "bottles remain the primary source" is revisited based on research findings.

## Decision Drivers

- **Accuracy over speed**: Real environment tests are slower but catch real issues; simulated tests are fast but miss environment-specific bugs
- **Release parity**: Every binary released should have corresponding integration tests
- **Fail-fast discovery**: Platform incompatibilities should be caught in CI, not by users
- **Maintainability**: Test infrastructure should be sustainable as platforms evolve
- **CI resource constraints**: GitHub Actions has limited ARM64 runners and container support varies by runner type
- **Self-contained philosophy**: tsuku should remain self-contained for tools, but "self-contained" should not mean unnecessary duplication of system-provided libraries

## Research Findings

Analysis of competing package managers (Homebrew, Nix, asdf/mise, Cargo, Docker) and tsuku's own architecture revealed key insights that reshape the approach to this problem. Extended research into Alpine's market position and the viability of hermetic APK extraction further reinforced these conclusions.

### Alpine's Strategic Importance

Research into Alpine Linux's market position found:

- **~20% of all Docker containers use Alpine** as their base image
- **Over 100 million container image downloads per month** with significant Alpine usage
- Alpine is ~30x smaller than Debian (5 MB vs 75+ MB)
- **95%+ of musl Linux users are on Alpine** - other musl distros (Void, Chimera, Adelie) are statistically negligible
- mise (a direct competitor to tsuku) explicitly provides native Alpine/musl support

This makes Alpine a first-class citizen that tsuku must support, not a niche target.

### The Value of Embedded Libraries Was Misunderstood

The original assumption was that Homebrew bottles provide value through:
- Fresher versions than distro packages
- Hermetic, reproducible builds
- Self-contained installation without system dependencies

**Research found this is not the case:**

| Library | Homebrew | Ubuntu 24.04 | Fedora 41 | Alpine |
|---------|----------|--------------|-----------|--------|
| zlib | ~1.3 | 1.3 | 2.2.3 | 1.3.1 |
| openssl | ~3.x | 3.0.13 | 3.2.2 | 3.1.8 |

Distro packages are often as current or newer than Homebrew. Arch and Fedora frequently lead. The real value of Homebrew bottles is **"no build tools required"** - users don't need gcc/make/etc. But for library dependencies, this is unnecessary complexity.

### Hermetic APK Extraction Was Considered and Rejected

We investigated whether to build a custom APK extraction system (~500 LOC) that would download Alpine packages directly from Alpine's CDN - essentially "Homebrew bottles for musl." This would have provided hermetic version control on Alpine.

**Research found this approach is flawed:**

1. **Alpine doesn't retain old package versions.** Version-pinned Docker builds break within days/weeks when packages are updated. There's no snapshot service like Debian's snapshot.debian.org.

2. **APK extraction only works on Alpine.** Other musl distros use different package formats:
   - Void Linux uses xbps
   - Chimera Linux uses APKv3 (incompatible with Alpine's APKv2)
   - Each distro maintains separate repositories

3. **True reproducibility requires infrastructure tsuku doesn't have.** Users who need hermetic builds on Alpine already use Nix, which works on Alpine.

4. **The value proposition doesn't hold.** Homebrew bottles work across glibc distros because glibc provides ABI stability. No equivalent exists for musl - APK packages are Alpine-specific.

This research reinforced the system packages approach: `apk_install` already exists, works immediately, and solves the problem with zero new code.

### System Packages Are Security-Reviewed

Distro security teams:
- Actively monitor for CVEs and backport fixes
- Sign packages with institutional key management
- Have rapid-response patching infrastructure (6-24 hours for critical CVEs)

Homebrew bottles are rebuilt from source but lack the institutional security review that distros provide. For library dependencies that users don't directly interact with, distro security teams provide better ongoing protection.

### The Dependency Graph Is Shallow

tsuku only has 4 embedded library recipes:
- zlib (no deps)
- libyaml (no deps)
- openssl (depends on zlib)
- gcc-libs (Linux only)

Only 4 tool recipes depend on these: ruby, nodejs, cmake, and tools using openssl. Maximum dependency depth is 2 (cmake → openssl → zlib). This makes migration straightforward.

### tsuku Already Has System Package Support

The `apk_install`, `apt_install`, `dnf_install`, `pacman_install` actions exist in `internal/actions/`. They're currently stub implementations that describe commands to users. The infrastructure exists; recipes just don't use it.

### Philosophy Shift: Self-Contained Tools, System-Managed Dependencies

Based on this research, we adopt a new philosophy:

| Component | Source | Rationale |
|-----------|--------|-----------|
| **Tools** (what users install) | Pre-built binaries | Self-contained, version-controlled by tsuku |
| **Library dependencies** | System packages | Security-reviewed, native to each platform |

This means:
- `tsuku install nodejs` downloads the pre-built Node.js binary (self-contained)
- If nodejs needs libstdc++, tsuku guides the user to `apt install libstdc++6` (system-managed)
- Tools work on all platforms because dependencies come from native package managers

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

Detect musl at runtime and provide actionable guidance. When a user on Alpine tries to install or use embedded libraries, tsuku would warn them about the incompatibility and suggest alternatives.

This option is complementary to 1D - it provides the error handling when system packages can't be installed (e.g., user lacks sudo, package not available).

**Pros:**
- Fail-fast with clear, actionable error messages
- Works as fallback when system packages unavailable
- No silent failures or cryptic dlopen errors
- Guides users toward solutions (system packages, build from source)

**Cons:**
- Doesn't solve the problem by itself (needs 1D for actual fix)
- Requires runtime detection logic
- Only useful as error handling, not as primary solution

#### Option 1D: System package fallback (Recommended)

Replace embedded library recipes with system package dependencies across ALL Linux families, not just Alpine. This aligns with the "self-contained tools, system-managed dependencies" philosophy.

**Package name mapping:**

| Library | Debian | RHEL/Fedora | Arch | Alpine | SUSE |
|---------|--------|-------------|------|--------|------|
| zlib | zlib1g-dev | zlib-devel | zlib | zlib-dev | zlib-devel |
| libyaml | libyaml-dev | libyaml-devel | libyaml | yaml-dev | libyaml-devel |
| openssl | libssl-dev | openssl-devel | openssl | openssl-dev | openssl-devel |
| gcc-libs | libstdc++6 | libstdc++ | gcc-libs | libstdc++ | libstdc++6 |

**Implementation approach:**

Introduce a new `system_dependency` action that abstracts package manager differences:

```toml
[[steps]]
action = "system_dependency"
name = "openssl"
packages = {
    debian = "libssl-dev",
    rhel = "openssl-devel",
    arch = "openssl",
    alpine = "openssl-dev",
    suse = "openssl-devel"
}
```

The action:
1. Detects the system package manager (apt, dnf, pacman, apk, zypper)
2. Maps the library name to the correct package name
3. Checks if already installed; if not, shows the install command
4. User runs the command (tsuku doesn't require sudo)

**Pros:**
- Tools work on ALL Linux families, including Alpine/musl
- Leverages distro security teams for library updates
- No binary maintenance burden for libraries
- Consistent user experience across distributions
- Native packages are optimized for each platform
- Reduces tsuku's attack surface (fewer binaries to distribute)

**Cons:**
- Trades hermetic version control for working tools
- Library versions vary by distro (but distros backport security fixes)
- Requires user to have package manager access or pre-installed packages
- Adds package name mapping maintenance
- macOS still needs Homebrew for library deps (no system package manager)

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

| Option | Accuracy | Release Parity | Fail-Fast | Maintainability | CI Resources | Platform Coverage |
|--------|----------|----------------|-----------|-----------------|--------------|-------------------|
| 1A (Document) | Fair | Good | Poor | Good | Good | Poor (Alpine broken) |
| 1B (Musl binaries) | Good | Good | Good | Poor | Fair | Good |
| 1C (Runtime detect) | Good | Good | Good | Good | Good | Poor (error only) |
| 1D (System fallback) | Good | Good | Good | Good | Good | **Good (all families)** |
| **1D+1C (Combined)** | **Good** | **Good** | **Good** | **Good** | **Good** | **Good** |
| 2A (Containers) | Good | Good | Good | Fair | Good | - |
| 2B (Native only) | Good | Good | Good | Fair | Good | - |
| 2C (Hybrid) | Good | Good | Good | Fair | Good | - |
| 3A (Match release) | Good | Good | Good | Good | Good | - |
| 3B (Representative) | Fair | Fair | Fair | Good | Good | - |
| 3C (Full matrix) | Good | Good | Good | Poor | Poor | - |

**Key insight**: 1D (system packages) provides platform coverage that no other option achieves. Combined with 1C (runtime detection for errors), it provides both working tools AND clear guidance when things go wrong.

Note: Option 2B rates "Good" for release parity because GitHub provides native runners for all 4 release targets (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64). Family-specific testing is a separate concern addressed by containers.

### Uncertainties

- **ARM64 container behavior**: Running ARM64 containers on amd64 runners via QEMU may have different behavior than native ARM64
- **CI time impact**: Adding container-based tests for all families could significantly increase CI time; actual impact depends on parallelization
- **openssl conflict scope**: The system library conflict (#1090) may be addressable with better LD_LIBRARY_PATH isolation during verification
- **Recipe conditional logic complexity**: Adding family-aware conditionals to recipes may complicate the recipe format and validation

### Resolved Through Research

- **Homebrew musl bottles**: Research confirmed Homebrew does NOT provide musl bottles - they're glibc-only
- **Alpine user impact**: Research found Alpine represents ~20% of container market share - significant enough to require first-class support
- **System package acceptability**: Research found hermetic APK extraction wouldn't actually provide reproducibility (Alpine removes old packages), making system packages the pragmatic choice
- **Hermetic APK extraction viability**: Research found this wouldn't work - Alpine doesn't retain old package versions, and APK is Alpine-only (not portable to other musl distros)

## Decision Outcome

**Chosen: 1D+1C (System packages + Runtime detection) + 2C (Hybrid testing) + 3A (Match release matrix)**

### Summary

We adopt the philosophy of **"self-contained tools, system-managed dependencies"**. Tool binaries remain self-contained (downloaded from upstream), but library dependencies use system package managers rather than embedded Homebrew bottles. Runtime detection provides clear error messages when system packages aren't available. This approach makes tools work on ALL Linux families, including Alpine/musl, while leveraging distro security teams for library maintenance.

### Rationale

**Decision 1 - System packages with runtime detection (1D+1C):**

The research findings fundamentally changed our approach:

1. **Homebrew bottles don't provide version freshness** - distro packages are often as current
2. **System packages are security-reviewed** - distros have dedicated security teams
3. **The dependency graph is shallow** - only 4 libraries, easy to migrate
4. **tsuku already has package manager support** - apt_install, apk_install, etc. exist

System packages (1D) solve the musl problem completely - Alpine's packages are built for musl. Runtime detection (1C) provides the error handling layer when system packages can't be installed.

This trades hermetic version control for working tools across all platforms. Given that:
- Distros backport security fixes (often faster than Homebrew)
- Library versions rarely matter for compatibility
- Users expect tools to work on their platform

...this trade-off is acceptable.

**Decision 2 - Hybrid testing (2C):**
This maximizes coverage with available resources. Native runners provide ground-truth verification for release platforms, while containers enable family-specific testing. With system packages, container tests now verify that the `system_dependency` action correctly maps library names to distro-specific package names.

**Decision 3 - Match release matrix (3A):**
This establishes a clear contract: if we release it, we test it. Matching the release matrix (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64) focuses effort on what matters most.

**Alternatives rejected:**
- **1A (Document only)**: Doesn't solve the problem; users still can't use tools on Alpine
- **1B (Musl binaries)**: Unnecessary complexity; system packages solve the same problem with less maintenance
- **1C alone**: Error messages without a solution; frustrating user experience
- **Embedded libraries (current approach)**: Creates glibc/musl split; unnecessary duplication of system-provided packages

### Trade-offs Accepted

By choosing this approach, we accept:

1. **Loss of hermetic library versions**: Library versions are controlled by distros, not tsuku. This is acceptable because distros provide security backports and version differences rarely cause compatibility issues.

2. **Package manager dependency**: Users need access to their system package manager (or pre-installed packages). This is acceptable because most users have this access, and the `system_dependency` action provides clear guidance for those who don't.

3. **macOS still uses Homebrew**: macOS lacks a system package manager, so library deps on macOS continue to use Homebrew. This is acceptable because macOS uses a single libc (no glibc/musl split).

4. **Package name mapping maintenance**: We maintain a mapping of library names to distro-specific package names. This is acceptable because the list is small (4 libraries) and changes infrequently.

## Solution Architecture

### Overview

The solution has four components that work together:

1. **System dependency action** - New action that abstracts package manager differences for library dependencies
2. **Runtime detection and guidance** - Detects platform/libc and provides actionable guidance when system packages unavailable
3. **Hybrid CI test matrix** - Native runners for release platforms + container jobs for family verification
4. **Verification coverage parity** - Ensuring dlopen tests run on all tested configurations

### Component 1: System Dependency Action

Introduce a new `system_dependency` action that replaces embedded library recipes with system package guidance.

```
internal/actions/
├── system_dependency.go  # New: abstracts package manager differences
├── linux_pm_actions.go   # Existing: apt_install, dnf_install, etc.
└── brew_actions.go       # Existing: brew_install for macOS
```

**Action interface:**

```go
type SystemDependencyAction struct{ BaseAction }

func (a *SystemDependencyAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    // 1. Detect system package manager
    pm := platform.DetectPackageManager()

    // 2. Look up package name for this library
    libraryName := params["name"].(string)
    pkgName := ResolvePackageName(libraryName, pm)

    // 3. Check if already installed
    if isInstalled(pkgName, pm) {
        return nil // Already satisfied
    }

    // 4. Show install command (tsuku doesn't run sudo)
    cmd := getInstallCommand(pkgName, pm)
    return fmt.Errorf("missing dependency %s: run '%s'", libraryName, cmd)
}
```

**Package name mapping** (internal/actions/system_deps.go):

```go
var systemPackageNames = map[string]map[string]string{
    "zlib": {
        "debian": "zlib1g-dev",
        "rhel":   "zlib-devel",
        "arch":   "zlib",
        "alpine": "zlib-dev",
        "suse":   "zlib-devel",
    },
    "openssl": {
        "debian": "libssl-dev",
        "rhel":   "openssl-devel",
        "arch":   "openssl",
        "alpine": "openssl-dev",
        "suse":   "openssl-devel",
    },
    // ... libyaml, gcc-libs
}
```

**Recipe changes:**

```toml
# Before (embedded library)
[[steps]]
action = "homebrew"
formula = "openssl@3"

# After (system dependency)
[[steps]]
action = "system_dependency"
name = "openssl"
when = { os = ["linux"] }

[[steps]]
action = "brew_install"
packages = ["openssl@3"]
when = { os = ["darwin"] }
```

### Component 2: Runtime Detection and Guidance

Add platform detection for error handling when system packages can't be installed.

```
internal/platform/
├── libc.go          # New: libc detection (glibc vs musl)
├── pm.go            # New: package manager detection
└── family.go        # Existing: Linux family detection
```

**Libc detection:** Check if `/lib/ld-musl-*.so.1` exists (primary), with `ldd --version` parsing as fallback.

**Package manager detection:**

```go
func DetectPackageManager() PackageManager {
    if _, err := exec.LookPath("apt"); err == nil {
        return Apt
    }
    if _, err := exec.LookPath("dnf"); err == nil {
        return Dnf
    }
    // ... pacman, apk, zypper, brew
    return Unknown
}
```

**Error messages:**

When a system dependency is missing:
```
Error: nodejs requires libstdc++ which is not installed.

To install on Debian/Ubuntu:
  sudo apt install libstdc++6

To install on Alpine:
  sudo apk add libstdc++
```

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
User runs: tsuku install nodejs (which depends on gcc-libs)

1. Platform detection
   └─> Detect OS, arch, family
   └─> Detect package manager (apt, dnf, apk, etc.)

2. Recipe resolution
   └─> Parse nodejs recipe
   └─> Find system_dependency step for gcc-libs

3. System dependency check
   └─> Map "gcc-libs" to package name for detected family
       - debian: libstdc++6
       - alpine: libstdc++
       - etc.
   └─> Check if package installed
       └─> If yes: Continue
       └─> If no: Show install command, halt

4. Tool installation
   └─> Download nodejs binary from upstream
   └─> Verify checksum
   └─> Extract to $TSUKU_HOME/tools/

5. Verification
   └─> Level 1: File existence
   └─> Level 2: Dependency resolution (ldd check)
   └─> Level 3: dlopen test (works on all libc now!)
```

**macOS flow** (unchanged):

```
User runs: tsuku install cmake (which depends on openssl)

1. Platform detection
   └─> OS = darwin, no package manager detection needed

2. Recipe resolution
   └─> Find brew_install step for openssl (darwin-only)

3. Homebrew installation
   └─> Download openssl bottle from GHCR
   └─> Verify checksum
   └─> Extract to $TSUKU_HOME/libs/

4. Tool installation
   └─> Download cmake, link to openssl

5. Verification
   └─> Standard verification chain
```

## Implementation Approach

### Phase 1: Platform Detection Infrastructure

**Goal**: Add infrastructure to detect package managers and provide guidance.

**Changes**:
1. Add `internal/platform/pm.go` with `DetectPackageManager() PackageManager`
2. Add `internal/platform/libc.go` with `DetectLibc() string` (for error messages)
3. Add unit tests using mocked command lookups
4. Map package managers to Linux families (apt→debian, dnf→rhel, pacman→arch, apk→alpine, zypper→suse)

**Dependencies**: None

### Phase 2: System Dependency Action

**Goal**: Create the `system_dependency` action that abstracts package manager differences.

**Changes**:
1. Add `internal/actions/system_dependency.go` implementing the new action
2. Add `internal/actions/system_deps.go` with package name mapping table
3. Implement `isInstalled()` check for each package manager
4. Implement `getInstallCommand()` for clear user guidance
5. Register action in action registry
6. Add comprehensive tests for each Linux family

**Dependencies**: Phase 1

### Phase 3: Recipe Migration

**Goal**: Migrate embedded library recipes to use system_dependency.

**Changes**:
1. Update `zlib.toml`: Replace homebrew action with system_dependency (Linux) + brew_install (macOS)
2. Update `libyaml.toml`: Same pattern
3. Update `openssl.toml`: Same pattern
4. Update `gcc-libs.toml`: Same pattern (Linux-only, no macOS equivalent needed)
5. Update dependent recipes (ruby, nodejs, cmake) to use new library recipes
6. Add CI tests verifying system_dependency works on each family

**Dependencies**: Phase 2

### Phase 4: ARM64 Native Testing (parallel with Phase 3)

**Goal**: Verify ARM64 Linux binaries with real native tests.

**Changes**:
1. Add `ubuntu-24.04-arm` runner to integration tests
2. Add ARM64 to library-dlopen test matrix
3. Update release workflow to run integration tests on ARM64

**Dependencies**: None

### Phase 5: Container-based Family Tests

**Goal**: Verify system_dependency action works on all Linux families.

**Changes**:
1. Create container job variants for dlopen tests (Fedora, Arch, openSUSE, Alpine)
2. Install system packages in each container before running tests
3. Build tsuku-dltest from source in each container
4. Verify dlopen works on ALL families including Alpine (no more skips!)

**Dependencies**: Phase 3 (recipes migrated to system_dependency)

### Phase 6: Documentation and Cleanup

**Goal**: Document the new philosophy and clean up.

**Changes**:
1. Update README with platform support matrix (now includes Alpine fully!)
2. Close #1092 (musl support) with system dependency solution
3. Archive embedded library recipes or mark as deprecated
4. Document the "self-contained tools, system-managed dependencies" philosophy
5. Add troubleshooting guide for users who can't install system packages

## Security Considerations

### Download Verification

**Tool binaries**: Tool binaries continue to be downloaded from upstream sources with SHA256 checksum verification. This is unchanged.

**Library dependencies (Linux)**: Library dependencies now come from system package managers (apt, dnf, apk, etc.) rather than Homebrew bottles. This changes the security model:

- **Before**: tsuku downloaded and verified library binaries from Homebrew GHCR
- **After**: Users install libraries via their distro's package manager, which has its own verification (GPG signatures, repo checksums)

This is a security improvement because distro package managers have institutional security review and rapid CVE response infrastructure.

**Library dependencies (macOS)**: macOS continues to use Homebrew for library deps. The existing checksum verification remains unchanged.

### Execution Isolation

**System dependency action**: The new `system_dependency` action does NOT execute package manager commands with elevated privileges. It:
1. Checks if a package is installed (read-only operation)
2. If missing, displays the command the user should run (e.g., `sudo apt install libssl-dev`)
3. The user runs the command themselves (tsuku never runs sudo)

This preserves tsuku's principle of not requiring or using elevated privileges.

**Package manager detection**: Detection reads command availability (`which apt`, `which dnf`, etc.) with normal user permissions. No privilege escalation.

**CI test isolation**: Container-based tests run with default container isolation. The tsuku-dltest helper runs with sanitized environment variables, which is existing behavior unchanged by this design.

### Supply Chain Risks

**Shift to distro packages**: By using system packages instead of Homebrew bottles, we shift supply chain trust from Homebrew to distribution maintainers.

| Aspect | Homebrew | Distro Packages |
|--------|----------|-----------------|
| Binary source | Homebrew GHCR | Distro mirrors |
| Signing | Homebrew bottle checksums | GPG-signed packages |
| Security review | Homebrew maintainers | Distro security teams |
| CVE response | Homebrew rebuild | Distro backport (often faster) |
| Auditability | Recipe git history | Distro package changelogs |

For library dependencies (which users don't directly interact with), distro packages provide better institutional security guarantees.

**CI container images**: Using official distribution images introduces a supply chain dependency on Docker Hub. This is acceptable for CI testing purposes. A compromise would affect CI results, not user-installed binaries.

### User Data Exposure

**No new data collection**: This design does not introduce new data collection or transmission. Package manager detection runs entirely locally.

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Distro package compromised | Distro security teams + GPG signing | Same risk as any system software |
| Package manager detection tricked | Multiple detection methods | Exotic environments might misdetect |
| User can't install system packages | Clear error message with exact command | User may need IT help in locked-down environments |
| Compromised CI container image | Use official distribution images | CI could pass falsely; periodic audit recommended |

## Consequences

### Positive

- **Full Alpine/musl support**: Tools now work on Alpine Linux. The glibc/musl incompatibility is solved by using native system packages
- **Accurate platform coverage**: Every released binary (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64) will have corresponding integration tests
- **Reduced maintenance burden**: No need to maintain embedded library binaries. Distros handle library updates and security patches
- **Better security posture**: Library dependencies are managed by distro security teams with institutional review processes
- **Family-specific verification**: dlopen tests run on ALL distribution families including Alpine
- **Clear philosophy**: "Self-contained tools, system-managed dependencies" provides clear guidance for future recipe development

### Negative

- **Loss of hermetic library versions**: Library versions are controlled by distros, not tsuku. Version may differ across systems
- **Package manager dependency**: Users need access to system package managers (or pre-installed packages)
- **macOS asymmetry**: macOS still uses Homebrew for library deps since there's no system package manager
- **Package name mapping maintenance**: Must maintain a table mapping library names to distro-specific package names
- **Increased CI time**: Container-based family tests add overhead, though parallelization mitigates this

### Mitigations

- **Version differences**: Distros backport security fixes; version differences rarely cause compatibility issues for common libraries
- **Package manager access**: Most users have this. For locked-down environments, the `system_dependency` action provides clear install commands that users can request from IT
- **macOS asymmetry**: macOS has a single libc (no glibc/musl split), so Homebrew works fine there
- **Package name mapping**: The list is small (4 libraries currently) and changes infrequently. CI tests verify the mapping works
- **CI time**: Tests run in parallel; container setup is cached

## Research References

The following research documents informed this design:

### Core Research
- `wip/research/explore_full_synthesis.md` - Initial synthesis of Homebrew vs system packages
- `wip/research/explore_apk-synthesis.md` - APK extraction viability analysis

### APK Deep Dive
- `wip/research/explore_apk-format.md` - APK file format (3 gzip streams, no placeholders)
- `wip/research/explore_apk-infrastructure.md` - Alpine CDN, APKINDEX parsing
- `wip/research/explore_apk-portability.md` - Cross-musl-distro compatibility (Alpine-only)
- `wip/research/explore_apk-download.md` - Implementation gap analysis (~500 LOC)
- `wip/research/explore_apk-relocation.md` - RPATH and dlopen verification

### Market and Value Analysis
- `wip/research/explore_alpine-market.md` - Alpine market share (~20% of containers)
- `wip/research/explore_musl-landscape.md` - musl distro comparison (Alpine 95%+)
- `wip/research/explore_hermetic-value.md` - Why hermetic APK extraction doesn't provide value

### Phase 8 Reviews
- `wip/research/explore_phase8_architecture-review.md` - Architecture clarity review
- `wip/research/explore_phase8_security-review.md` - Security analysis
