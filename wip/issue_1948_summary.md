# Issue 1948 Summary

## What Was Implemented

Made libc when clauses visible to the family policy analyzer. Recipes using `libc = ["musl"]` or `libc = ["glibc"]` now produce family-specific plans instead of being silently treated as family-agnostic.

## Changes Made
- `internal/recipe/types.go`: Added `Libc` field to `Constraint` struct, updated `Clone()` and `Validate()`, added libc propagation with conflict detection in `MergeWhenClause()`
- `internal/recipe/policy.go`: Added `FamiliesForLibc()` helper that derives families from libc using `platform.LibcForFamily`, updated `AnalyzeRecipe()` to treat libc-constrained steps as family-scoped
- `internal/recipe/types_test.go`: 8 new tests for MergeWhenClause libc handling, Clone, and Validate
- `internal/recipe/policy_test.go`: 8 new tests for AnalyzeRecipe/SupportedPlatforms libc scenarios, plus FamiliesForLibc

## Key Decisions
- Libc field as string (not []string): Follows the same pattern as GPU -- single value from WhenClause array, multi-value leaves empty
- LinuxFamily takes precedence over Libc: When both are set on a constraint, the existing LinuxFamily branch handles it first, avoiding double-counting
- FamiliesForLibc derives from AllLinuxFamilies + platform.LibcForFamily: Stays consistent if new families are added

## Test Coverage
- New tests added: 16 (8 in types_test.go, 8 in policy_test.go)
- All 39 existing test packages pass without modification

## Known Limitations
- Multi-value libc (e.g., `["glibc", "musl"]`) leaves Constraint.Libc empty, treating the step as unconstrained. This is consistent with the multi-OS and multi-GPU patterns.

## Requirements Mapping

| AC | Status | Evidence |
|----|--------|----------|
| Constraint struct includes Libc field | Implemented | types.go:601 |
| MergeWhenClause propagates libc | Implemented | types.go:693-700 |
| AnalyzeRecipe maps libc to families | Implemented | policy.go:123-127 |
| musl steps produce FamilySpecific with alpine | Implemented | TestAnalyzeRecipe_LibcMuslOnly |
| glibc steps produce FamilySpecific with debian/rhel/arch/suse | Implemented | TestAnalyzeRecipe_LibcGlibcOnly |
| Split glibc+musl produces all 5 families | Implemented | TestAnalyzeRecipe_LibcBothSplit |
| SupportedPlatforms generates family-qualified platforms | Implemented | TestSupportedPlatforms_LibcMuslOnly |
