# Cross-Consistency Review: Design Documents and Research Findings

## Overview

This review cross-checks the two design documents against research findings to identify inconsistencies, missing coverage, and areas needing clarification.

**Documents Reviewed:**
- `docs/DESIGN-system-dependency-actions.md` (Action Vocabulary)
- `docs/DESIGN-structured-install-guide.md` (Sandbox Container Building)

**Research Findings Reviewed:**
- 25 findings files in `wip/research/findings_*.md`

---

## Section 1: Inconsistencies

### 1.1 Terminology: `linux_family` vs `package_manager` vs `distro`

**Status: RESOLVED in design, but terminology differs from research**

| Source | Terminology | Meaning |
|--------|-------------|---------|
| **Design docs** | `linux_family` | Debian, RHEL, Arch, Alpine, SUSE |
| **findings_targeting-model-recommendation.md** | `package_manager` | apt, dnf, apk, pacman, zypper |
| **findings_pm-recommendations.md** | `package_manager` or `distro_family` | Both suggested, PM preferred |
| **findings_distro-pm-mapping.md** | "Family" | Debian, RHEL, Arch, Alpine, SUSE |

**Analysis:**
- The research recommends `package_manager` as the targeting dimension
- The design docs chose `linux_family` which maps 1:1 to package manager
- This is a conscious design decision documented in D2, so no inconsistency

**Verdict:** CONSISTENT. Research suggested `package_manager`, design chose `linux_family` with clear rationale. The 1:1 mapping is documented.

### 1.2 Detection Method: Binary Check vs /etc/os-release

**Status: POTENTIAL INCONSISTENCY**

| Source | Recommendation |
|--------|----------------|
| **DESIGN-system-dependency-actions.md** | Parse `/etc/os-release`, use `ID` and `ID_LIKE` fields |
| **findings_pm-detection.md** | Binary detection (`type cmd`) as PRIMARY, os-release as fallback |
| **findings_targeting-model-recommendation.md** | Binary detection (`commandExists`) |

**Analysis:**
- Research strongly recommends binary detection as primary method
- Design document focuses on `/etc/os-release` parsing
- The design provides Go code using `ParseOSRelease()` but research shows binary detection is more reliable (e.g., `microdnf` on minimal RHEL images)

**Recommendation:** Update design to clarify detection order:
1. Use `/etc/os-release` for family determination (as currently documented)
2. Use binary detection to verify PM is actually available
3. Handle `microdnf` as equivalent to `dnf`

### 1.3 Package Manager Scope: Initial vs Extended

**Status: CONSISTENT with minor clarification needed**

| Source | Initial Scope | Extended Scope |
|--------|---------------|----------------|
| **Design docs** | apt, brew, dnf, pacman | apk, zypper |
| **findings_pm-recommendations.md** | apt, dnf (P0), pacman (P1) | apk, zypper (P2), brew (P2) |
| **findings_pm-action-coverage.md** | apt, dnf, pacman | apk, zypper, brew |

**Verdict:** CONSISTENT. Design includes brew in initial scope (for macOS); research focused on Linux only.

### 1.4 microdnf Handling

**Status: MISSING FROM DESIGN**

**Issue:** Research identifies that minimal RHEL images (AlmaLinux, Rocky Linux) use `microdnf` instead of `dnf`. The design documents do not mention `microdnf`.

| Source | microdnf Handling |
|--------|-------------------|
| **findings_pm-detection.md** | "microdnf should be treated as equivalent to dnf" |
| **findings_no-pm-strategy.md** | Lists `microdnf` in supported PM detection |
| **DESIGN docs** | Not mentioned |

**Recommendation:** Add to design:
- `microdnf` detection alongside `dnf`
- Treat as `linux_family = "rhel"` equivalent
- Document in D2 rationale section

---

## Section 2: Missing Coverage

### 2.1 Bootstrap Requirements for Debian/Ubuntu

**Status: PARTIALLY ADDRESSED**

**Research Finding (findings_bootstrap-requirements.md):**
> "Debian and Ubuntu base images ship WITHOUT CA certificates. HTTPS requests will fail."

**Design Coverage:**
- DESIGN-structured-install-guide.md mentions bootstrap in "Context and Problem Statement" indirectly
- No explicit handling of CA certificate bootstrap scenario

**Gap:** Neither design explicitly addresses what happens when a user runs tsuku on a fresh Debian/Ubuntu system that lacks `ca-certificates`. The bootstrap problem affects tsuku's ability to download recipes.

**Recommendation:** Add to DESIGN-structured-install-guide.md:
- "Bootstrap Requirements" section
- Detection mechanism for missing CA certificates
- Clear error message with package-manager-specific fix command

### 2.2 Alpine/musl Binary Compatibility

**Status: NOT ADDRESSED IN SYSTEM DEPS CONTEXT**

**Research Finding (findings_binary-compatibility.md):**
> "Static linking is the key, not glibc vs musl"
> "glibc dynamic binaries DO NOT work on Alpine"

**Design Coverage:**
- Design mentions Alpine support for `apk_install`
- No discussion of musl compatibility for tsuku itself or downloaded tools

**Gap:** Sandbox containers for Alpine may need special handling:
1. tsuku binary must be musl-compatible or static
2. Recipes may need to specify musl vs glibc binary variants
3. Error message `no such file or directory` for glibc binaries on Alpine

**Recommendation:** Add to DESIGN-structured-install-guide.md:
- Clarify tsuku binary compatibility (Go static = works everywhere)
- Document Alpine-specific considerations for sandbox containers
- Consider adding `libc` to platform targeting for recipes

### 2.3 yum vs dnf Legacy Handling

**Status: MISSING**

**Research Finding (findings_pm-detection.md):**
> "`yum` on Fedora/Amazon Linux is typically a symlink to `dnf`"
> Detection should check dnf first, then microdnf, then yum

**Design Coverage:**
- Design mentions `dnf_install` and `dnf_repo`
- No mention of `yum` or how to handle older RHEL/CentOS systems

**Gap:** RHEL 7 and CentOS 7 (EOL 2024 but still in use) use `yum` natively, not as a symlink.

**Recommendation:** Either:
1. Add `yum_install` action for legacy systems, OR
2. Document RHEL 7/CentOS 7 as unsupported, OR
3. Add note that `yum` systems should work with `dnf_install` (symlink scenario)

### 2.4 Package Name Mapping

**Status: PARTIALLY ADDRESSED**

**Research Finding (findings_package-name-mapping.md):**
- `xz` vs `xz-utils` (apt)
- `wget` vs `wget2` (dnf/Fedora)
- `python3` vs `python` (pacman)
- Development headers vary significantly

**Design Coverage:**
- DESIGN-system-dependency-actions.md mentions packages field
- No discussion of package name translation or mapping

**Gap:** Recipes specifying `packages = ["xz"]` will fail on apt-based systems which use `xz-utils`.

**Recommendation:** Add to design one of:
1. Explicit mapping table for common packages (adds complexity)
2. Document that package names must be per-PM (current implicit approach)
3. Future work section for "canonical package names with translation"

### 2.5 NixOS and Gentoo Handling

**Status: ADDRESSED but could be clearer**

**Research Finding (findings_no-pm-strategy.md):**
> "NixOS: Tsuku cannot install system packages on NixOS"
> "Gentoo: Installing via emerge may take significant time"

**Design Coverage:**
- DESIGN-system-dependency-actions.md mentions Nix/Gentoo in "Future Work: Additional Package Managers" as "Explicitly out of scope"

**Gap:** The design says these are "out of scope" but doesn't describe what happens when a user on NixOS/Gentoo tries to install a recipe with system dependencies.

**Recommendation:** Add to DESIGN-system-dependency-actions.md:
- What happens on unsupported systems
- Suggest `manual` action as fallback
- Link to research findings for user guidance

### 2.6 Composite Shorthand Syntax Details

**Status: DOCUMENTED as future work, but research had more detail**

**Research Finding (findings_pm-recommendations.md):**
- Question Q2 discusses package name handling approaches
- Option A (separate blocks per PM) vs Option B (mapping)

**Design Coverage:**
- DESIGN-system-dependency-actions.md has "Future Work: Composite Shorthand Syntax" section
- Proposes `system_dependency` with `overrides` map

**Observation:** Research recommends explicit separate blocks (Option A) while future work proposes a mapping approach (similar to Option B). Both are valid; this is a deferred decision.

**Verdict:** CONSISTENT for current scope. Future design needed for composite syntax.

---

## Section 3: Cross-Reference Validation

### 3.1 Section Links Between Documents

**Status: VALIDATED**

Checked internal links:

| Link | Status |
|------|--------|
| `DESIGN-system-dependency-actions.md#d6-hardcoded-when-clauses...` | Valid anchor |
| `DESIGN-system-dependency-actions.md#example-docker-installation` | Valid anchor |
| `DESIGN-system-dependency-actions.md#documentation-generation` | Valid anchor |
| `DESIGN-system-dependency-actions.md#host-execution` | Valid anchor |
| `DESIGN-structured-install-guide.md#sandbox-executor-changes` | Valid anchor |
| Cross-reference to `DESIGN-golden-plan-testing.md` | External (not reviewed) |

**Verdict:** All internal cross-references are consistent.

### 3.2 Scope Boundaries

**Status: CLEAR**

| Concern | Assigned To |
|---------|-------------|
| Action vocabulary | DESIGN-system-dependency-actions.md |
| Platform filtering (`when`, `linux_family`) | DESIGN-system-dependency-actions.md |
| Documentation generation (`Describe()`) | DESIGN-system-dependency-actions.md |
| Container building | DESIGN-structured-install-guide.md |
| Container caching | DESIGN-structured-install-guide.md |
| Sandbox execution | DESIGN-structured-install-guide.md |
| Host execution | Future (deferred) |

**Verdict:** CLEAR. Both documents have explicit scope tables and consistent boundaries.

### 3.3 Implementation Phases Alignment

**Status: MOSTLY ALIGNED**

**DESIGN-system-dependency-actions.md phases:**
1. Infrastructure (linux_family, detection, WhenClause)
2. Action Vocabulary (types, validation, Describe)
3. Documentation Generation (CLI output, --verify flag)
4. Sandbox Integration (ExtractPackages, container building)

**DESIGN-structured-install-guide.md phases:**
1. Adopt Action Vocabulary
2. Documentation Generation
3. Sandbox Container Building
4. Extension

**Observation:** Phases are sequentially consistent:
- System-dependency-actions Phase 1-3 feeds into structured-install-guide Phase 1-2
- System-dependency-actions Phase 4 == structured-install-guide Phase 3

**Verdict:** ALIGNED. Dependency chain is clear.

---

## Section 4: Recommendations

### 4.1 High Priority (Should address before implementation)

1. **Add microdnf handling to design**
   - Treat as equivalent to `dnf` for `linux_family = "rhel"`
   - Update detection code example

2. **Clarify detection order**
   - Primary: `/etc/os-release` for family
   - Secondary: Binary presence check for PM availability
   - Handle case where os-release says Debian but apt is missing (distroless)

3. **Add bootstrap requirements section**
   - Explicit handling for Debian/Ubuntu CA certificate absence
   - Document as prerequisite for tsuku operation

### 4.2 Medium Priority (Should document)

4. **Add Alpine/musl compatibility note**
   - tsuku binary compatibility (Go static = OK)
   - Sandbox container base image requirements

5. **Document unsupported system behavior**
   - What happens on NixOS/Gentoo when system deps needed
   - Graceful degradation to `manual` action

6. **Clarify yum legacy status**
   - Either support explicitly or document as unsupported

### 4.3 Low Priority (Can defer)

7. **Package name mapping approach**
   - Document current approach (recipe author responsibility)
   - Defer canonical names to future work

8. **Add detection code for additional edge cases**
   - WSL detection
   - Read-only filesystem detection
   - Distroless container detection

---

## Summary

The two design documents are **well-aligned** and **internally consistent**. The main gaps are:

| Gap | Severity | Resolution |
|-----|----------|------------|
| microdnf not mentioned | Medium | Add to design |
| Bootstrap requirements not explicit | Medium | Add section |
| Detection order (binary vs os-release) | Low | Clarify in design |
| Alpine/musl compatibility | Low | Add note |
| yum legacy handling | Low | Document decision |
| Unsupported system UX | Low | Add fallback behavior |

The designs successfully incorporate the key research recommendations:
- `linux_family` aligns with research "family-based hierarchy" pattern
- `/etc/os-release` as detection source aligns with research
- Hardcoded when clauses for PM actions prevents the "apt on RHEL" mistake
- Typed actions with no shell commands aligns with security research
- Minimal container strategy aligns with "expose hidden dependencies" goal

**Overall Assessment:** Designs are consistent and comprehensive. Minor additions recommended before implementation.
