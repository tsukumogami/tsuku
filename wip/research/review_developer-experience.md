# Developer Experience Review: Platform Compatibility Verification

**Reviewer Role:** Developer Experience Specialist
**Date:** 2026-01-24
**Design Document:** `docs/designs/DESIGN-platform-compatibility-verification.md`

---

## Executive Summary

The design adopts a "self-contained tools, system-managed dependencies" philosophy that prioritizes working tools over hermetic purity. This is the right call for developer experience. The approach handles the Alpine/musl problem correctly, but the error messaging and sudo-less user journey need more attention to match the seamless experience users expect from modern package managers.

---

## 1. User Journey Analysis

### Scenario: Alpine User Installing a Tool with Library Dependencies

**Current proposed flow:**

```
$ tsuku install nodejs
Error: nodejs requires libstdc++ which is not installed.

To install on Alpine:
  sudo apk add libstdc++
```

**Issues identified:**

1. **Blocking experience.** The user must exit tsuku, run a separate command, then retry. This is two context switches.

2. **sudo requirement in containers.** In Docker containers (Alpine's primary use case), the user is typically already root. The error message shows `sudo apk add` which is unnecessary and may confuse users unfamiliar with sudo.

3. **No progress indication.** Users don't know upfront that library deps are needed. They start an install expecting success and get blocked mid-flow.

**Suggested improvement:**

```
$ tsuku install nodejs

Checking dependencies...
  libstdc++ - not found

nodejs requires system libraries. Install them with:
  apk add libstdc++

Continue after installing? [Y/n] _
```

Better: tsuku could detect if running as root and skip the sudo prefix. The design should specify this behavior.

### Scenario: User Cannot Install System Packages

**Current proposed error:**

```
Error: nodejs requires libstdc++ which is not installed.
```

**What's missing:**

The design mentions this scenario in "Trade-offs Accepted" but doesn't specify what guidance to provide. Users in locked-down environments need actionable alternatives:

- Request IT to install the package
- Use a container with pre-installed deps
- Build from source (if applicable)

The error message should include a "need help?" pointer to documentation.

---

## 2. Error Message Evaluation

### Strengths

The design shows error messages with:
- Clear identification of what's missing
- Platform-specific install commands
- Correct package names per distribution

### Weaknesses

**Inconsistent formatting.** The design shows two different error formats:

Format 1 (Section "Component 1"):
```
missing dependency %s: run '%s'
```

Format 2 (Section "Component 2"):
```
Error: nodejs requires libstdc++ which is not installed.

To install on Debian/Ubuntu:
  sudo apt install libstdc++6

To install on Alpine:
  sudo apk add libstdc++
```

The design should commit to a single format. Format 2 is superior for UX.

**Missing context.** Error messages should indicate:
- What tool requested the dependency (if nested)
- Whether this is a build-time or runtime dependency
- What happens if the user proceeds without the library (if possible)

**Suggested standardized format:**

```
Dependency missing: <library>

<tool> requires <library> for <purpose>.

To install:
  <platform-specific command>

Documentation: https://tsuku.dev/help/dependencies
```

---

## 3. Cross-Platform Consistency

### What's Consistent

- All 5 Linux families use the same pattern: detect package manager, map library name, show install command
- macOS maintains Homebrew approach (different mechanism, same UX goal)
- Package name mapping is centralized (`internal/actions/system_deps.go`)

### What's Inconsistent

**macOS vs Linux UX divergence:**

| Aspect | Linux | macOS |
|--------|-------|-------|
| Library source | System packages | Homebrew bottles |
| User action | Run install command | Automatic |
| sudo needed | Usually yes | Usually no (Homebrew) |

On macOS, library deps "just work" because Homebrew doesn't require sudo. On Linux, users hit a wall and must take manual action. This creates a perception that tsuku is "better" on macOS.

**Mitigation options:**

1. **Accept the asymmetry** - document it clearly
2. **Check if packages are already installed** - many systems have common libs pre-installed
3. **Support non-root package managers** - Nix/Linuxbrew work without sudo

The design mentions option 2 (`isInstalled()` check) but should make it explicit that common libraries are often already present, reducing friction in practice.

### Package Manager Detection Edge Cases

The design specifies detection via `exec.LookPath()` for apt, dnf, pacman, apk, zypper. Missing edge cases:

- **Multiple package managers** (e.g., Arch with both pacman and yay)
- **Containerized package managers** (e.g., Homebrew on Linux)
- **Immutable systems** (Fedora Silverblue, NixOS, Talos)

The design should specify behavior when:
- No package manager is detected
- Multiple are available

---

## 4. Friction Points

### High Friction

1. **Install-fail-retry loop.** User must:
   - Run `tsuku install X`
   - See error about missing dep
   - Run system package manager
   - Run `tsuku install X` again

   This is the primary UX gap compared to mise, which doesn't have library dependencies for most tools.

2. **sudo requirement without explanation.** Why can't tsuku install libraries? New users may not understand the philosophical distinction between "tools" and "dependencies."

3. **No dry-run option.** Users can't check what system packages are needed before starting an install.

### Medium Friction

4. **Package name discoverability.** If the mapping is wrong or missing, users have no way to fix it themselves. Consider exposing the mapping for troubleshooting:
   ```
   tsuku deps nodejs
   # Output: libstdc++ (Alpine: libstdc++, Debian: libstdc++6, ...)
   ```

5. **Outdated system packages.** The design notes distros backport security fixes, but doesn't address what happens if a system package is too old for a tool. This can happen on LTS distributions.

### Low Friction

6. **CI/CD workflow verbosity.** Container jobs will show dependency install commands even though they're typically pre-installed. Consider a `--quiet` or `--ci` mode.

---

## 5. Competitive Comparison: mise

### mise's Approach

mise (the closest competitor mentioned in the research) handles Alpine differently:

- **Provides native musl binaries** for the mise tool itself
- **Most tools don't have library deps** - Go, Rust, and Node binaries are statically linked or include dependencies
- **No system package manager interaction** for most use cases

### tsuku vs mise on Alpine

| Aspect | mise | tsuku (proposed) |
|--------|------|------------------|
| Tool installation | Download binary, done | Download binary, may need system packages |
| Library deps | Rarely needed | Explicit `system_dependency` action |
| User friction | Low | Medium (install-retry loop) |
| Flexibility | Less (predefined builds) | More (can use any upstream binary) |

### Competitive Gap

mise's "it just works" experience on Alpine is superior for the common case. tsuku's approach is more flexible (can install tools that need specific library versions) but adds friction.

**Mitigation:** tsuku should prioritize recipes that use statically-linked or self-contained binaries where possible. Reserve `system_dependency` for tools that genuinely need dynamic libraries.

---

## 6. Identified Friction Points Summary

| ID | Friction Point | Severity | Suggested Fix |
|----|----------------|----------|---------------|
| F1 | Install-fail-retry loop | High | Add pre-flight dependency check before download |
| F2 | sudo requirement not explained | High | Add brief explanation in error message |
| F3 | No dry-run for dependencies | Medium | Add `tsuku deps <tool>` command |
| F4 | Inconsistent error formats | Medium | Standardize on multi-line format |
| F5 | macOS/Linux UX asymmetry | Medium | Document clearly, accept as trade-off |
| F6 | Missing package manager detection | Low | Specify behavior for edge cases |
| F7 | CI verbosity | Low | Add `--quiet` mode |

---

## 7. Suggested Improvements

### P0: Critical Path Improvements

1. **Pre-flight dependency check.** Before downloading the tool, check for all system dependencies. If any are missing, show the full list upfront:

   ```
   $ tsuku install cmake

   cmake requires system packages that are not installed:

     openssl-dev   (openssl library)
     zlib-dev      (compression library)

   Install with:
     apk add openssl-dev zlib-dev

   Then retry: tsuku install cmake
   ```

2. **Smart sudo detection.** If running as root, don't show `sudo` prefix. If not root, explain why sudo is needed:

   ```
   # When not root:
   Install with:
     sudo apk add openssl-dev  # requires root privileges
   ```

3. **Standardize error format.** Use the multi-line format consistently across all error paths.

### P1: High Value Improvements

4. **Add `tsuku deps <tool>` command.** Let users query dependencies before installing:

   ```
   $ tsuku deps cmake

   cmake dependencies:
     openssl - libssl-dev (Debian), openssl-dev (Alpine), ...
     zlib    - zlib1g-dev (Debian), zlib-dev (Alpine), ...

   On this system (Alpine), run:
     apk add openssl-dev zlib-dev
   ```

5. **Cache installed-package checks.** Avoid repeated package manager queries during a single tsuku run.

6. **Documentation pointer in errors.** Every error should include a URL for detailed help:

   ```
   Need help? https://tsuku.dev/docs/dependencies
   ```

### P2: Nice to Have

7. **CI mode.** Suppress informational messages, assume dependencies are pre-installed:

   ```
   tsuku install --ci cmake
   ```

8. **Offer container alternative.** When system package install fails, suggest container-based approach:

   ```
   Alternatively, use tsuku in a container with deps pre-installed:
     docker run -it ghcr.io/tsuku/dev:alpine tsuku install cmake
   ```

---

## 8. Overall Verdict

**Approve with Changes**

The design makes the correct strategic decision to prioritize working tools over hermetic purity. The "self-contained tools, system-managed dependencies" philosophy is sound and well-researched.

However, the implementation details around error messages and user flow need refinement before this delivers a competitive developer experience. The install-fail-retry loop is the biggest UX gap compared to alternatives like mise.

### Conditions for Approval

**Must address before implementation:**

1. Specify pre-flight dependency checking (don't wait until mid-install to fail)
2. Define a single, consistent error message format
3. Handle the "running as root" case for sudo-less container environments

**Should address in implementation (acceptable in follow-up):**

4. Add `tsuku deps` or equivalent command for dependency discovery
5. Document the macOS/Linux experience asymmetry
6. Specify edge case behavior for package manager detection

### What's Good

- Research is thorough and well-sourced
- Alpine's importance is correctly assessed (20% of containers)
- Trade-offs are explicitly acknowledged
- Security model is improved (distro security teams > Homebrew maintainers for libraries)
- Phased implementation approach is practical

### What Needs Work

- Error message design needs user testing
- Install flow needs interruption points for user control
- Competitive gap with mise's frictionless Alpine experience is not fully addressed

---

## Appendix: Research References Consulted

- `docs/designs/DESIGN-platform-compatibility-verification.md` - Primary design document
- `wip/research/explore_alpine-market.md` - Alpine market share data
- `wip/research/explore_hermetic-value.md` - Hermetic vs system package UX analysis
