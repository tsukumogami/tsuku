# Review: DESIGN-system-dependency-actions.md

**Reviewed:** 2025-12-31
**Document Version:** As committed (b16a916)
**Status:** Design is well-structured and internally consistent; minor gaps identified

---

## Summary

The design document is comprehensive and well-organized. The D1-D6 decisions form a coherent framework. The `linux_family` model is consistently applied throughout. A few gaps exist, primarily around missing actions in the vocabulary table and minor inconsistencies between sections.

---

## Issues

### I1: Action Vocabulary Table Missing `apk_install` and `zypper_install`

**Severity:** Medium

The D6 table (lines 288-293) lists `apk_install` and `zypper_install` as having hardcoded when clauses:

```
| `apk_install` | `when = { linux_family = "alpine" }` |
| `zypper_install` | `when = { linux_family = "suse" }` |
```

However, the Action Vocabulary table (lines 355-364) does not include these actions. The table only lists:
- `apt_install`, `apt_repo`, `apt_ppa`
- `dnf_install`, `dnf_repo`
- `brew_install`, `brew_cask`
- `pacman_install`

**Recommendation:** Add rows for `apk_install` and `zypper_install` to the Package Installation section of the Action Vocabulary table for consistency with D6.

### I2: Future Work Contradicts D6 for Alpine/SUSE Actions

**Severity:** Low

The "Additional Package Managers" section (lines 746-748) states:

> As needed:
> - `apk_install` for Alpine Linux - already in scope (alpine family)
> - `zypper_install` for openSUSE - already in scope (suse family)

The phrase "As needed" suggests these are future work, but D6 already includes them as having hardcoded when clauses. The text "already in scope" partially acknowledges this but the framing is confusing.

**Recommendation:** Either:
1. Remove these from Future Work since D6 already defines them, OR
2. Add a clarification that D6 defines the mapping but Phase 2 implementation is deferred

### I3: Missing `unless_command` in Action Vocabulary Table

**Severity:** Low

D3 (line 207-215) introduces an "Escape hatch" with the `unless_command` field:

```toml
[[steps]]
action = "apt_install"
packages = ["docker.io"]
unless_command = "docker"
```

This field is not documented in the Action Vocabulary table for `apt_install` or other `*_install` actions.

**Recommendation:** Add `unless_command?` to the Fields column for install actions, or clarify whether this is deferred to future work.

### I4: `apt_repo` Missing `fallback` Field

**Severity:** Low

The `apt_install`, `dnf_install`, `brew_install`, `brew_cask`, and `pacman_install` actions all list `fallback?` as an optional field. However, `apt_repo` and `dnf_repo` do not list `fallback?` even though repository setup operations can also fail.

**Recommendation:** Consider whether `apt_repo` and `dnf_repo` should also support `fallback?`, or add a note explaining why it's excluded.

### I5: Anchor Link May Not Resolve

**Severity:** Low

Line 651 references:
> See [Future Work: Host Execution](#host-execution).

The actual heading at line 677 is:
> ### Host Execution

GitHub markdown should handle this correctly, but some markdown parsers generate anchor IDs differently (e.g., `#future-work-host-execution` vs `#host-execution`).

**Recommendation:** Verify the anchor works in GitHub rendering. If not, adjust the link.

---

## Suggestions

### S1: Add Schema Definitions for All Actions

The design shows example Go structs for `AptInstallAction` and `AptRepoAction` but not for all actions. Consider adding complete field definitions for:
- `dnf_install`, `dnf_repo`
- `brew_install`, `brew_cask`
- `pacman_install`, `apk_install`, `zypper_install`
- `group_add`, `service_enable`, `service_start`
- `require_command`, `manual`

This would eliminate ambiguity during implementation.

### S2: Clarify `apt_ppa` Debian Family Scope

The `apt_ppa` action has implicit `when = { linux_family = "debian" }` per D6, but PPAs are Ubuntu-specific (not Debian). The example at line 576 notes:

> Note: PPAs are Ubuntu-specific but work on Ubuntu derivatives

Consider whether `apt_ppa` should have a more restrictive implicit constraint, or whether the current approach (debian family + documentation) is sufficient.

### S3: Add Examples for All Package Manager Actions

The Docker installation example (lines 492-532) is excellent. Consider adding brief examples for:
- `dnf_install` with `dnf_repo` (RHEL family pattern)
- `pacman_install` (Arch pattern)
- `apk_install` (Alpine pattern)

This would help recipe authors see the patterns for each ecosystem.

### S4: Consider `service_restart` Action

The system configuration actions include `service_enable` and `service_start` but not `service_restart`. Some installations require service restart after configuration changes.

**Decision needed:** Is `service_restart` in scope? If deferred, add to Future Work.

### S5: Clarify Homebrew `tap` Field Behavior

The `brew_install` action lists `tap?` as optional. Clarify whether:
- `tap` is just metadata for documentation generation, OR
- The action should also execute `brew tap owner/repo` before install

The example at line 567 suggests tap is specified but doesn't show whether tsuku runs `brew tap`.

---

## Questions

### Q1: Should `require_command` Support `unless_installed`?

D3 introduces `unless_command` for install actions to skip if the command exists. Should `require_command` have a corresponding field to control behavior when the command is missing?

Current behavior: fail the installation
Alternative: provide a fallback message or degraded mode

### Q2: What Happens When Family Detection Fails but Recipe Has No Fallback?

Line 170-171 states:
> If `/etc/os-release` is missing or family cannot be determined, steps with `linux_family` conditions are skipped.

What happens if a recipe only defines `apt_install` steps (no cross-platform coverage) and the user is on an unknown distro? The installation would silently do nothing. Is this the intended behavior?

Consider:
- Should tsuku warn when no steps match the detected platform?
- Should there be a "must-have-at-least-one-matching-step" validation?

### Q3: How Are Key Rotation Handled for `apt_repo`?

The `apt_repo` action requires `key_sha256` for content-addressing. When upstream rotates GPG keys:
1. Recipe must be updated with new hash
2. Until then, all installations fail

Is there guidance for recipe authors on:
- Monitoring for key rotations?
- Updating key hashes safely?

### Q4: What About `apt_key` Deprecation?

The `apt_repo` examples show traditional GPG key handling. Modern Debian/Ubuntu recommend `/etc/apt/keyrings/` with signed-by. Is this pattern supported, or is it a future enhancement?

### Q5: Should `group_add` Require Relogin Warning?

Adding a user to a group typically requires logout/login (or `newgrp`) to take effect. Should:
- `group_add` action's `Describe()` include this warning?
- There be a separate action for explaining the need to relogin?

---

## Cross-Reference Verification

### Links That Work

| Link | Target | Status |
|------|--------|--------|
| [#722](https://github.com/tsukumogami/tsuku/issues/722) | Issue reference | External (not verified) |
| [DESIGN-golden-plan-testing.md](DESIGN-golden-plan-testing.md) | Companion doc | EXISTS |
| [DESIGN-structured-install-guide.md](DESIGN-structured-install-guide.md) | Companion doc | EXISTS |

### Internal Anchors Used

| Anchor | Heading | Status |
|--------|---------|--------|
| #host-execution | ### Host Execution | OK |
| #d6-hardcoded-when-clauses-for-package-manager-actions | ### D6: Hardcoded When Clauses... | Should verify |
| #documentation-generation | ## Documentation Generation | OK |
| #composite-shorthand-syntax | ### Composite Shorthand Syntax... | OK |

### Research Files Referenced

| File | Status |
|------|--------|
| `wip/research/system-deps_api-design.md` | EXISTS |
| `wip/research/system-deps_platform-detection.md` | EXISTS |
| `wip/research/system-deps_security.md` | EXISTS |
| `wip/research/system-deps_authoring-ux.md` | EXISTS |
| `wip/research/system-deps_implementation.md` | EXISTS |
| `wip/research/design-fit_current-behavior.md` | EXISTS |
| `wip/research/design-fit_sandbox-executor.md` | EXISTS |
| `wip/research/design-fit_usecase-alignment.md` | EXISTS |

---

## D1-D6 Decision Consistency Check

| Decision | Topic | Consistent Throughout? |
|----------|-------|------------------------|
| D1 | Action Granularity (one action per operation) | YES |
| D2 | Linux Family Detection (`when = { linux_family = "..." }`) | YES |
| D3 | Require Semantics (idempotent install + verify) | YES |
| D4 | Post-Install Configuration (separate actions) | YES |
| D5 | Manual/Fallback (hybrid approach) | YES |
| D6 | Hardcoded When Clauses | YES (with minor gap in vocabulary table) |

---

## Future Work Items Review

| Item | Still Relevant? | Notes |
|------|----------------|-------|
| Host Execution | YES | Requires separate design work as stated |
| Composite Shorthand Syntax | YES | Ergonomic improvement for common cases |
| Additional Package Managers (apk, zypper) | CLARIFY | D6 already defines them; clarify implementation timeline |
| Version Constraints | YES | Deferred appropriately |

---

## Conclusion

The design is ready for implementation with the following actions:

1. **Required before implementation:**
   - Add `apk_install` and `zypper_install` to Action Vocabulary table (I1)
   - Clarify Future Work section wording for apk/zypper (I2)

2. **Recommended but not blocking:**
   - Add `unless_command?` to action fields documentation (I3)
   - Consider `fallback?` for repo actions (I4)
   - Verify anchor links (I5)

3. **Consider for implementation phase:**
   - Questions Q1-Q5 may need design decisions during implementation
   - Suggestions S1-S5 would improve completeness but are not blocking
