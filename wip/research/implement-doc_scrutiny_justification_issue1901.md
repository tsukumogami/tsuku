# Scrutiny Review: Justification -- Issue #1901

**Issue**: #1901 (refactor(sandbox): create containerimages package and centralized config)
**Focus**: justification
**Reviewer**: pragmatic-reviewer

## Summary

No ACs were reported as deviated, so the core justification lens (evaluating deviation explanations) has minimal material. However, the implementation contains unrequested scope that was not surfaced in the requirements mapping.

## Findings

### Finding 1: Unrequested `Families()` export (Advisory)

**Location**: `internal/containerimages/containerimages.go:52-59`

The `Families()` function is an exported API addition not requested by any AC, not listed in the design doc's Key Interfaces section (which explicitly enumerates only `ImageForFamily` and `DefaultImage`), and has zero callers outside its own test file.

The state file's `key_decisions` field claims it was "added for downstream issues," but the three downstream issues are:
- #1902: CI workflow migration (uses `jq` on JSON, not Go code)
- #1903: Renovate config and drift-check CI job (YAML/JSON, not Go)
- #1904: CODEOWNERS update (plain text file)

None of these would call a Go `Families()` function. This is speculative generality -- a function added for a hypothetical future caller that doesn't exist in the planned work.

**Severity**: Advisory. The function is small (7 lines), inert, and doesn't create a maintenance burden. It could be removed, but its presence doesn't block anything.

### Finding 2: Redundant debian check in `DefaultImage()` (Advisory)

**Location**: `internal/containerimages/containerimages.go:44-49`

`DefaultImage()` checks `images["debian"]` and panics if missing. But `init()` on lines 28-30 already validates the debian key exists and panics if it doesn't. The function's own comment acknowledges this: "but the init function validates this so a panic here means the binary was built with a corrupt embed."

This is impossible-case handling. The `init()` guarantee means the `ok` check in `DefaultImage()` can never be false in a running program. A simpler implementation would be `return images["debian"]`.

**Severity**: Advisory. Defensive coding that doesn't cause harm but adds dead code.

### Finding 3: Requirements mapping omits `Families()` (Advisory)

The requirements mapping lists 19 AC items, all reported as "implemented." None mention the `Families()` function or its test. This means unrequested scope was added without being surfaced to the review process. The mapping should either include it as an explicit addition with justification, or the function should not have been added.

**Severity**: Advisory. The omission from the mapping is a process gap, not a code defect.

## Overall Assessment

All reported ACs check out against the implementation. The implementation is clean and well-aligned with the design doc's intent. The only concerns are minor: an unrequested `Families()` export with no current callers, and a redundant nil check in `DefaultImage()`. Neither is blocking.
