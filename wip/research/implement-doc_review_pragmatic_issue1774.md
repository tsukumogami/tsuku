# Pragmatic Review: #1774 feat(recipe): add gpu field to WhenClause

**Reviewer focus**: pragmatic (simplicity, YAGNI, KISS)

## Files Changed

- `internal/recipe/types.go`
- `internal/recipe/platform.go`
- `internal/recipe/types_test.go`
- `internal/recipe/when_test.go`

## Summary

No blocking findings. The implementation follows the existing libc pattern exactly, adding GPU as a parallel dimension. No unnecessary abstractions, no speculative generality, no dead code.

## Advisory Findings

### 1. ADVISORY: `MergeWhenClause` multi-GPU leaves Constraint.GPU empty without documentation

`internal/recipe/types.go:684` -- When `when.GPU` has multiple values (e.g., `["amd", "intel"]`), `MergeWhenClause` silently leaves `result.GPU = ""` (empty). This parallels the multi-OS behavior at line 659 and is the correct approach. However, downstream consumers in #1775 will need to know that `Constraint.GPU == ""` can mean either "unconstrained" or "multi-GPU filter that didn't collapse to one value." The inline comment "no conflict detection needed" is good but could note the multi-value case explicitly.

Not blocking because: the behavior is correct and consistent with the OS pattern; #1775 will handle plan-time filtering through `WhenClause.Matches()` rather than through `Constraint.GPU`.

### 2. ADVISORY: No test for MergeWhenClause with pre-existing implicit GPU constraint

`internal/recipe/types_test.go:2392-2423` -- Tests cover single-GPU propagation and multi-GPU-leaves-empty, but there's no test where the implicit `Constraint` already has a `GPU` value and the `WhenClause` also specifies GPU. Today no action has an implicit GPU constraint, so this path is unreachable. But `Constraint.GPU` exists and `Clone()` copies it, so a future action could theoretically set one. The current code would silently overwrite the implicit GPU without conflict detection.

Not blocking because: no action defines an implicit GPU constraint today, and the risk is low and bounded.

## Correct Design Choices (not flagged)

- GPU is not gated to Linux-only in `Matches()` (unlike libc), which is correct because macOS has "apple" and future platforms could have GPU values.
- `ValidGPUTypes` from the platform package is reused for validation in `ValidateStepsAgainstPlatforms()` rather than defining a second list. No duplication.
- TOML unmarshaling handles both `gpu = "nvidia"` (single string) and `gpu = ["nvidia"]` (array), matching the libc and OS patterns.
- No new abstractions introduced. The GPU field is added to existing structs and methods without any new helper functions or interfaces.
- Test count (27 new tests) is proportionate to the change surface.
