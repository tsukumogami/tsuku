# UX Implications of `target = (platform, distro)` Model

## Executive Summary

This analysis examines user experience implications of extending tsuku's platform model to include Linux distribution detection via the proposed `when = { distro = [...] }` clause. The analysis covers five key scenarios and identifies critical UX decisions.

**Key Findings:**

1. **Distro detection should be communicated proactively** - Users need to understand which distro tsuku detected and why specific actions were chosen
2. **Graceful degradation for unknown distros is essential** - NixOS, Gentoo, Alpine users should not be left stranded
3. **Sandbox distro choice has CI reproducibility implications** - Default to a "canonical" distro (Ubuntu) rather than host mirroring
4. **Cross-distro documentation requires deliberate UX** - Users should be able to query instructions for other platforms
5. **Recipe authoring needs validation tooling** - Catch incomplete distro coverage before merge

---

## Scenario Analysis

### Scenario 1: User runs `tsuku install docker` on Ubuntu

**Current behavior:**
```
$ tsuku install docker

Docker requires system dependencies that tsuku cannot install directly.

Install Docker using your system package manager:
  linux: See https://docs.docker.com/engine/install/ for platform-specific installation

After installing, run: tsuku install docker --verify
```

**Proposed behavior with distro detection:**
```
$ tsuku install docker

Docker requires system dependencies that tsuku cannot install directly.

Detected platform: linux/amd64 (Ubuntu 22.04)

For Ubuntu/Debian:

  1. Add Docker repository:
     curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
     echo "deb [signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list

  2. Install packages:
     sudo apt-get update && sudo apt-get install docker-ce docker-ce-cli containerd.io

  3. Add yourself to docker group:
     sudo usermod -aG docker $USER

  4. Enable Docker service:
     sudo systemctl enable docker

After completing these steps, run: tsuku install docker --verify
```

**UX Improvements:**
- Shows detected platform AND distro (Ubuntu 22.04)
- Provides distro-specific instructions (apt-based)
- Action items are numbered and actionable

**UX Concern: Distro Detection Visibility**

Users need to see what tsuku detected to:
1. Verify detection is correct (especially for derivatives)
2. Understand why these specific instructions were shown
3. Know where to report issues if detection is wrong

**Recommendation:** Always show `Detected platform: os/arch (distro version)` line when distro-specific actions are involved.

---

### Scenario 2: User runs `tsuku install docker` on unsupported distro (NixOS, Gentoo, Alpine)

**Challenge:** These distros have fundamentally different package paradigms:
- NixOS: declarative configuration, not imperative package install
- Gentoo: source-based compilation, `emerge` package manager
- Alpine: musl libc, `apk` package manager

**Proposed behavior:**
```
$ tsuku install docker

Docker requires system dependencies that tsuku cannot install directly.

Detected platform: linux/amd64 (nixos)

NixOS detected. Tsuku's package installation commands are not applicable.
Please use your system's native package management:

  For NixOS:
    Add to configuration.nix:
      virtualisation.docker.enable = true;
    Then run: sudo nixos-rebuild switch

  General reference: https://docs.docker.com/engine/install/

After installing, run: tsuku install docker --verify
```

**UX Design Decisions:**

1. **`ID_LIKE` fallback may be inappropriate for some distros**
   - Alpine's `ID_LIKE` is often empty or "alpine"
   - NixOS's `ID_LIKE` is typically empty
   - Gentoo's `ID_LIKE` is typically empty

   These distros genuinely diverge from parent distros; treating Alpine as "Debian-like" would produce wrong instructions.

2. **Graceful degradation strategy:**
   - If distro is unknown AND no `manual` fallback exists: show generic message with documentation links
   - If recipe has `manual` action for unknown distros: show that text
   - Never show empty instructions or silent failure

3. **Recipe-level escape hatch:**
   Recipe authors should be able to provide explicit fallback:
   ```toml
   [[steps]]
   action = "manual"
   text = """
   NixOS detected. Add to configuration.nix:
     virtualisation.docker.enable = true;
   """
   when = { distro = ["nixos"] }
   ```

**Recommendation:** Add a `distro_unsupported` CLI output template that gracefully handles unknown distros with actionable fallback.

---

### Scenario 3: User runs `tsuku install --sandbox docker`

**Design Question:** Which distro container should the sandbox use?

**Option A: Mirror host distro**
```
# On Ubuntu 24.04 host
$ tsuku install --sandbox docker
Running in sandbox (ubuntu:24.04)...
```

**Pros:**
- Test matches production environment
- Catches distro-specific issues

**Cons:**
- Requires maintaining container images for every supported distro
- Detection adds complexity
- Derivative distros (Pop!_OS, Linux Mint) may not have official images

**Option B: Use canonical distro (Ubuntu LTS)**
```
# On any host
$ tsuku install --sandbox docker
Running in sandbox (ubuntu:22.04)...
```

**Pros:**
- Predictable, reproducible
- Matches CI environment
- Official images always available
- Simpler to maintain

**Cons:**
- May not catch distro-specific issues
- User must explicitly test on target distro if needed

**Option C: Default to canonical, allow override**
```
$ tsuku install --sandbox docker
Running in sandbox (ubuntu:22.04)...

$ tsuku install --sandbox docker --sandbox-image fedora:39
Running in sandbox (fedora:39)...
```

**Recommendation: Option C**

Default to Ubuntu LTS (currently `ubuntu:22.04` as specified in `SourceBuildSandboxImage`). This provides:
- Reproducible CI behavior
- Predictable golden file generation
- Clear baseline for recipe authors

Add `--sandbox-image` flag for explicit override when testing distro-specific behavior.

**Additional UX consideration:** When sandbox uses a different distro than host, should we warn?

```
$ tsuku install --sandbox docker
Note: Sandbox uses ubuntu:22.04. Your host is Fedora 39.
      Use --sandbox-image fedora:39 to match your environment.
Running in sandbox (ubuntu:22.04)...
```

This educates users about the distinction without being intrusive.

---

### Scenario 4: User wants to see instructions for Fedora while on Ubuntu

**Use case:** User wants to:
- Document installation for different platform
- Help colleague on different distro
- Write CI scripts for multiple platforms

**Current state:** No way to query instructions for non-current platform.

**Proposed UX:**

```
# See instructions for current platform
$ tsuku info docker --system-deps
Docker requires system dependencies...
For Ubuntu/Debian:
  sudo apt-get install docker-ce...

# See instructions for specific distro
$ tsuku info docker --system-deps --distro fedora
Docker requires system dependencies...
For Fedora:
  sudo dnf install docker...

# See instructions for macOS
$ tsuku info docker --system-deps --os darwin
Docker requires system dependencies...
For macOS:
  brew install --cask docker

# List all platform instructions
$ tsuku info docker --system-deps --all-platforms
Docker requires system dependencies...

For Ubuntu/Debian (apt):
  ...

For Fedora (dnf):
  ...

For macOS (Homebrew):
  ...
```

**Implementation notes:**
- `--distro` implies `--os linux`
- `--distro` and `--os` are mutually exclusive (validation)
- `--all-platforms` shows every defined platform variant

**Recommendation:** Add `--distro`, `--os`, and `--all-platforms` flags to `tsuku info` command with `--system-deps` for viewing platform-specific instructions.

---

### Scenario 5: Recipe author wants to add support for a new distro

**Current workflow:**
1. Add distro to `when` clause
2. Add package installation action
3. Test manually

**Problems:**
1. No validation that all platforms are covered
2. No way to verify distro detection without running on that distro
3. Easy to miss edge cases

**Proposed validation tooling:**

**5a. Recipe validation (`tsuku validate`)**
```
$ tsuku validate recipes/d/docker.toml

recipes/d/docker.toml:
  WARN: No install action for distro 'arch' (pacman_install)
  WARN: No install action for distro 'fedora' (dnf_install)
  OK: apt_install covers: ubuntu, debian, mint, pop
  OK: brew_cask covers: darwin
  OK: manual fallback for unknown distros
```

**5b. Coverage matrix (`tsuku info --coverage`)**
```
$ tsuku info docker --coverage

Docker installation coverage:

| Platform     | Distro   | Action      | Status |
|--------------|----------|-------------|--------|
| darwin/amd64 | -        | brew_cask   | OK     |
| darwin/arm64 | -        | brew_cask   | OK     |
| linux/amd64  | ubuntu   | apt_install | OK     |
| linux/amd64  | debian   | apt_install | OK     |
| linux/amd64  | fedora   | -           | MANUAL |
| linux/amd64  | arch     | -           | MANUAL |
| linux/arm64  | ubuntu   | apt_install | OK     |
| linux/arm64  | debian   | apt_install | OK     |

Legend: OK=typed action, MANUAL=fallback text, -=no coverage
```

**5c. Dry-run with platform simulation**
```
$ tsuku install docker --dry-run --distro fedora

Simulating installation on linux/amd64 (fedora)...

Plan:
  1. [dnf_install] Install packages: docker
  2. [group_add] Add to group: docker
  3. [service_enable] Enable service: docker
  4. [require_command] Verify: docker
```

**Recommendation:** Add validation tooling that surfaces coverage gaps and allows dry-run with platform simulation.

---

## Cross-Cutting UX Concerns

### A. Distro Detection Error Handling

**What if `/etc/os-release` is malformed or missing?**

```
$ tsuku install docker

Warning: Could not detect Linux distribution (/etc/os-release not found)
Proceeding with generic Linux instructions.

Docker requires system dependencies...
  Visit https://docs.docker.com/engine/install/ for platform-specific installation
```

**Design principle:** Never fail silently. Always explain what was detected (or not detected) and why specific instructions were shown.

### B. Derivative Distro Communication

Linux Mint, Pop!_OS, Elementary OS are Ubuntu derivatives with `ID_LIKE=ubuntu`.

**Proposed UX:**
```
$ tsuku install docker

Detected platform: linux/amd64 (linuxmint 21.2)
Note: Linux Mint is Ubuntu-based. Using Ubuntu instructions.

For Ubuntu/Debian:
  ...
```

**Benefit:** Users understand why Ubuntu instructions are shown for their Mint system.

### C. Version Mismatch Warnings

If a recipe targets specific distro versions but user's version differs:

```
$ tsuku install docker

Detected platform: linux/amd64 (Ubuntu 18.04)
Warning: This recipe's Docker installation is tested on Ubuntu 20.04+.
         Package names or repository URLs may differ.

For Ubuntu/Debian:
  ...
```

**Note:** This requires version-aware recipe matching, which the design docs defer as future work. For now, recipes match on distro ID only.

### D. Multi-Distro Recipe Authoring Documentation

Recipe authors need clear guidance on:
1. When to use `when = { distro = [...] }` vs `when = { os = [...] }`
2. How `ID_LIKE` matching works
3. Which distros require explicit support vs. inherit from parent
4. How to test recipes for multiple distros

**Recommendation:** Add "Multi-Platform Recipes" section to CONTRIBUTING.md covering:
- Distro detection semantics
- Common patterns (apt family, dnf family, brew)
- Testing with `--sandbox-image` and `--distro` flags

---

## Summary of Recommendations

### CLI Communication

1. **Always show detected platform + distro** when distro-specific actions are involved
2. **Explain derivative distro mapping** (e.g., "Linux Mint is Ubuntu-based. Using Ubuntu instructions.")
3. **Graceful degradation for unknown distros** with actionable fallback (not empty output)
4. **Add `--distro` flag** to `tsuku info` for cross-platform documentation queries

### Sandbox Behavior

5. **Default sandbox to canonical distro** (Ubuntu LTS) for reproducibility
6. **Add `--sandbox-image` flag** for explicit distro testing
7. **Optionally warn** when sandbox distro differs from host

### Recipe Authoring

8. **Add coverage validation** to `tsuku validate` showing distro coverage matrix
9. **Add `--dry-run --distro <name>`** for simulating installation on different platforms
10. **Document multi-distro patterns** in CONTRIBUTING.md

### Error Handling

11. **Never fail silently** on distro detection issues
12. **Surface detection details** so users can verify and report issues
13. **Provide escape hatch** via `manual` action for truly unsupported platforms

---

## Open Questions for Design Review

1. **Should `ID_LIKE` matching be opt-out per-recipe?**
   Some derivatives (e.g., Alpine, NixOS) should never inherit parent instructions.

2. **How should version constraints interact with `ID_LIKE`?**
   If Ubuntu 22.04 is required, does Linux Mint 21.2 (based on Ubuntu 22.04) qualify?

3. **Should we maintain a "known distros" registry?**
   This would enable validation warnings for typos (`ubunut` vs `ubuntu`) and documentation of supported distros.

4. **What's the minimum viable distro coverage for recipes?**
   Should CI require at least apt + brew coverage, or is a single platform acceptable?
