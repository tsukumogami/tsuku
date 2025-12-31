# Review: Targeting Model Concern Status

This document tracks the status of all concerns raised during the platform targeting model research, identifying which have been resolved, which are no longer valid, and which remain open.

## Summary

| Status | Count |
|--------|-------|
| Resolved | 9 |
| No Longer Valid | 3 |
| Still Open | 4 |

---

## Resolved Concerns

### 1. Oversimplification of Linux Ecosystem (Hierarchy vs Flat Model)

**Where Raised:** DISCUSSION-targeting-model.md, Concern 1

**Original Concern:** The initial proposal treated Linux as a flat list of distros, ignoring the hierarchical nature (family -> distro -> version).

**Resolution:** The `linux_family` model was adopted instead of distro-level targeting. The family dimension maps 1:1 to package manager capability:

| linux_family | Package Manager |
|--------------|-----------------|
| debian | apt |
| rhel | dnf |
| arch | pacman |
| alpine | apk |
| suse | zypper |

**Documented In:**
- `docs/DESIGN-system-dependency-actions.md` (D2: Linux Family Detection)
- `wip/research/findings_targeting-model-recommendation.md`

---

### 2. Plan Validity Across Distros (Ubuntu vs Debian)

**Where Raised:** DISCUSSION-targeting-model.md, Concern 2

**Original Concern:** If we test a plan on Ubuntu, will it work on Debian? Plans might assume packages that exist on one but not the other.

**Resolution:** Research confirmed that Ubuntu and Debian are functionally equivalent for tsuku's purposes:
- Same package manager (apt)
- Same package names
- Same baseline capabilities (both lack curl, wget, CA certificates in base images)
- ID_LIKE in Ubuntu points to "debian"

By targeting `linux_family = "debian"` instead of individual distros, plans work across all Debian-family distros.

**Documented In:**
- `wip/research/findings_pm-baselines.md` (Debian vs Ubuntu Comparison)
- `wip/research/findings_targeting-model-recommendation.md` (Model Analysis)

---

### 3. Recipe Noise (Repeating Same Action for Every Distro)

**Where Raised:** DISCUSSION-targeting-model.md, Concern 3

**Original Concern:** Writing the same apt_install action for ubuntu, debian, mint, etc. creates noise without value.

**Resolution:** Hardcoded when clauses eliminate this problem. Package manager actions have immutable, built-in constraints:

```toml
# Recipe author writes:
[[steps]]
action = "apt_install"
packages = ["curl"]

# apt_install has implicit when = { linux_family = "debian" }
# No need to list individual distros
```

**Documented In:**
- `docs/DESIGN-system-dependency-actions.md` (D6: Hardcoded When Clauses)

---

### 4. Package Manager as Capability Boundary

**Where Raised:** DISCUSSION-targeting-model.md, Key Insight section

**Original Concern:** The discussion identified that "we cannot install a package manager on a system that lacks one" - so what should be the actual targeting dimension?

**Resolution:** This insight led to the `linux_family` model. Family maps 1:1 to package manager, so targeting by family is equivalent to targeting by package manager capability, but with a more intuitive name for recipe authors.

**Documented In:**
- `wip/research/DISCUSSION-targeting-model.md` (Key Insight section)
- `docs/DESIGN-system-dependency-actions.md` (D2 rationale)

---

### 5. Package Name Mapping Exceptions (~5% Different Names)

**Where Raised:** DISCUSSION-targeting-model.md, Open Questions section (Question 3)

**Original Concern:** Research showed ~95% of packages have the same name across PMs. How to handle the 5% exceptions?

**Resolution:** Multi-layered approach documented:

1. **Common case:** `system_install` composite action (future) uses same name for all PMs
2. **Known exceptions:** Internal mapping table for common differences (xz-utils vs xz, build-essential vs base-devel)
3. **Edge cases:** Recipe authors can use explicit `*_install` actions with PM-specific package names

**Documented In:**
- `docs/DESIGN-system-dependency-actions.md` (Future Work: Composite Shorthand Syntax)
- `wip/research/findings_package-name-mapping.md` (complete mapping table)

---

### 6. Detection Mechanism for Linux Family

**Where Raised:** Research phase - how to reliably detect which family a system belongs to

**Original Concern:** How does tsuku know it's running on a "debian" family system vs "rhel" family?

**Resolution:** Parse `/etc/os-release` using ID and ID_LIKE fields:

```go
func DetectFamily() (string, error) {
    // Parse /etc/os-release
    // Try ID first (e.g., "ubuntu" -> "debian")
    // Fall back to ID_LIKE chain (e.g., "pop" has ID_LIKE="ubuntu debian")
}
```

**Documented In:**
- `docs/DESIGN-system-dependency-actions.md` (D2: Linux Family Detection, detection mechanism)
- `wip/research/findings_pm-detection.md`

---

### 7. Universal Baseline Assumptions

**Where Raised:** Research phase - what can tsuku assume exists on any Linux system?

**Original Concern:** What tools/capabilities are truly universal across all Linux distributions?

**Resolution:** Empirical testing across 11 distributions established:

**Always Present:**
- /bin/sh (POSIX shell)
- Coreutils (cat, cp, mv, rm, mkdir, chmod, etc.)
- tar and gzip
- $HOME and $PATH
- Writable /tmp

**NOT Universal:**
- bash (missing on Alpine)
- curl or wget (neither on Debian/Ubuntu base)
- CA certificates (missing on Debian/Ubuntu base)
- unzip, xz, bzip2, zstd

**Documented In:**
- `wip/research/findings_universal-baseline.md`
- `wip/research/findings_pm-baselines.md`

---

### 8. Bootstrap Problem (CA Certificates on Debian/Ubuntu)

**Where Raised:** findings_universal-baseline.md, findings_bootstrap-requirements.md

**Original Concern:** Debian/Ubuntu base images lack CA certificates, creating a bootstrap paradox where tsuku needs HTTPS to download, but HTTPS needs certificates.

**Resolution:** Strategy A adopted - require prerequisites with graceful error:
1. Document CA certificates as prerequisite
2. Detect at startup with helpful error message
3. Show package-manager-specific bootstrap command

**Documented In:**
- `wip/research/findings_bootstrap-requirements.md` (Recommendation section)

---

### 9. Platform Filtering Mechanism Consistency

**Where Raised:** DESIGN-structured-install-guide.md, Context section

**Original Concern:** The old `install_guide` field used platform keys inside parameters, inconsistent with how other actions use `when` clauses.

**Resolution:** All platform filtering now uses step-level `when` clause. Package manager actions have hardcoded implicit when clauses based on `linux_family`. No more embedded platform keys in parameters.

**Documented In:**
- `docs/DESIGN-structured-install-guide.md` (Decision 1B)
- `docs/DESIGN-system-dependency-actions.md` (D6)

---

## No Longer Valid Concerns

### 1. Model B vs Model C Debate (package_manager vs linux_family)

**Where Raised:** DISCUSSION-targeting-model.md, Evolution of the Model section

**Original Concern:** Should the targeting model use `package_manager` directly or `linux_family`?

**Why No Longer Valid:** This was a naming debate, not a substantive difference. Both achieve the same goal. The final decision was `linux_family` because it's more intuitive for recipe authors while still mapping 1:1 to package manager. The `findings_targeting-model-recommendation.md` recommends Model B (package_manager), but the design docs adopted the equivalent `linux_family` terminology.

**Status:** Resolved by terminology choice - functionally equivalent.

---

### 2. Binary Compatibility (glibc vs musl)

**Where Raised:** RESEARCH-HANDOFF.md, investigation_binary-compatibility.md

**Original Concern:** Do we need separate binaries for glibc and musl systems?

**Why No Longer Valid:** Research showed this is primarily an Alpine-specific concern, and the system dependency model doesn't directly address binary compatibility. This is handled at the recipe level (download different binaries per platform), not at the targeting model level. The `linux_family = "alpine"` distinction handles this implicitly.

**Status:** Out of scope for targeting model - handled by existing platform/arch targeting in recipes.

---

### 3. Need for Distro-Specific Testing

**Where Raised:** Early design discussions

**Original Concern:** Do we need to test on Ubuntu AND Debian AND Mint separately?

**Why No Longer Valid:** Research confirmed that distros within the same family are functionally equivalent for tsuku's purposes. Testing on one Debian-family distro validates behavior for all. Container base image strategy uses one canonical distro per family.

**Status:** Resolved by family-level testing strategy.

---

## Still Open Concerns

### 1. Darwin/macOS Handling

**Where Raised:** DISCUSSION-targeting-model.md, Open Questions section (Question 1)

**Current Status:** macOS doesn't fit the `linux_family` model. The design docs show examples with `brew_install` and `brew_cask` having implicit `when = { os = "darwin" }`, but there's no comprehensive Darwin handling strategy.

**What's Needed:**
- Clarify: Is Homebrew required for all system deps on macOS?
- Define fallback behavior when Homebrew is not installed
- Document relationship between darwin/arm64 and darwin/amd64
- Consider Apple Silicon vs Intel differences for system deps

**Documented but Incomplete:**
- `docs/DESIGN-system-dependency-actions.md` mentions darwin examples
- DISCUSSION-targeting-model.md marks as "TBD"

---

### 2. Unsupported Systems (NixOS, Gentoo)

**Where Raised:** DISCUSSION-targeting-model.md, Open Questions section (Question 2)

**Current Status:** Explicitly marked as "Skip for now, flag as future solution." Research documented that:
- NixOS uses declarative package management (fundamentally different paradigm)
- Gentoo uses source-based packages (too slow for tsuku's use case)
- Both have core functionality (CA certs, tar, gzip) and can run tsuku

**What's Needed:**
- Formal user-facing error messages when running on unsupported systems
- Documentation explaining why these systems are unsupported
- Potential future path: nix-portable as universal fallback

**Documented In:**
- `wip/research/findings_no-pm-strategy.md` (detailed strategy per system)
- `docs/DESIGN-system-dependency-actions.md` (Future Work: Additional Package Managers)

**Action Required:** Implement detection and graceful error messages per `findings_no-pm-strategy.md` recommendations.

---

### 3. Version Constraints

**Where Raised:** DISCUSSION-targeting-model.md, Open Questions (implicit in discussion of hierarchy)

**Current Status:** The design explicitly defers version constraints:

> "Version constraint syntax adds significant complexity due to non-uniform versioning schemes across distros. Defer to feature detection via `require_command` instead."

**What's Needed:**
- Define when version constraints become necessary (specific use cases)
- Design version comparison semantics if/when needed
- Syntax proposal: `when = { linux_family = "debian", version = ">=22.04" }`

**Documented but Deferred:**
- `docs/DESIGN-system-dependency-actions.md` (D2, explicitly states not in initial implementation)
- `docs/DESIGN-structured-install-guide.md` (Future Work: Platform Version Constraints)

**Action Required:** None for now - explicitly deferred until demonstrated need.

---

### 4. Composite Shorthand Syntax Implementation

**Where Raised:** docs/DESIGN-system-dependency-actions.md, Future Work section

**Current Status:** The verbose step-per-PM syntax is recognized as a trade-off. A composite `system_dependency` shorthand is proposed but not designed:

```toml
[system_dependency]
command = "curl"
packages = ["curl", "ca-certificates"]
overrides = { debian = ["build-essential"], rhel = ["@development-tools"] }
```

**What's Needed:**
- Detailed specification of shorthand syntax
- How overrides map to linux_family
- Expansion algorithm (shorthand -> multiple PM-specific steps)
- Validation rules

**Documented In:**
- `docs/DESIGN-system-dependency-actions.md` (Future Work: Composite Shorthand Syntax)

**Action Required:** Design and implement when recipe verbosity becomes a scaling problem.

---

## Research Artifacts Reference

| Document | Primary Content | Status |
|----------|----------------|--------|
| `DISCUSSION-targeting-model.md` | Design evolution, model comparison | Complete |
| `findings_targeting-model-recommendation.md` | Final model recommendation | Complete |
| `findings_package-name-mapping.md` | PM-specific package names | Complete |
| `findings_no-pm-strategy.md` | NixOS/Gentoo handling | Complete |
| `findings_universal-baseline.md` | Universal Linux capabilities | Complete |
| `findings_pm-baselines.md` | Per-PM baseline packages | Complete |
| `findings_bootstrap-requirements.md` | CA certificate bootstrap | Complete |
| `DESIGN-system-dependency-actions.md` | Action vocabulary design | Complete |
| `DESIGN-structured-install-guide.md` | Sandbox container building | Complete |

---

## Summary by Category

### Fully Resolved (No Action Needed)
1. linux_family model adopted (not distro-level)
2. Plan validity across same-family distros
3. Recipe noise eliminated via hardcoded when clauses
4. Package manager as capability boundary
5. Package name mapping strategy defined
6. Detection mechanism via /etc/os-release
7. Universal baseline established empirically
8. Bootstrap strategy for Debian/Ubuntu
9. Consistent platform filtering via when clause

### No Longer Valid (Can Be Closed)
1. package_manager vs linux_family naming (equivalent)
2. glibc vs musl binary concerns (out of scope for targeting model)
3. Need for distro-specific testing (family-level suffices)

### Open (Action Required)
1. Darwin/macOS handling - needs comprehensive strategy
2. Unsupported systems - needs user-facing error messages
3. Version constraints - explicitly deferred, monitor need
4. Composite shorthand - implement when scaling requires

---

*Generated from review of DISCUSSION-targeting-model.md and related research artifacts.*
