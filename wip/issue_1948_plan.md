# Issue 1948 Implementation Plan

## Summary

Add a `Libc` field to `Constraint`, propagate it through `MergeWhenClause`, and teach `AnalyzeRecipe` that libc-scoped steps imply family-specific plan generation. This makes recipes using `libc = ["musl"]` / `libc = ["glibc"]` when clauses produce per-family plans (e.g., `linux-alpine-amd64` for musl) instead of generic `linux-amd64`.

## Approach

The fix follows the existing pattern established by `LinuxFamily` propagation. Libc values map deterministically to Linux families (musl -> alpine, glibc -> debian/rhel/arch/suse), so a libc constraint on a step can be translated into the set of families it applies to. The `Constraint` struct gains a `Libc` field, `MergeWhenClause` propagates it from `WhenClause.Libc`, and `AnalyzeRecipe` treats libc-constrained steps the same way it treats family-constrained steps: they contribute to `familiesUsed` and prevent `FamilyAgnostic` classification.

### Alternatives Considered

- **Translate libc to LinuxFamily in MergeWhenClause**: Instead of adding a `Libc` field, convert `libc = ["musl"]` to `LinuxFamily = "alpine"` in `MergeWhenClause` itself. Rejected because it loses information (a multi-value libc like `["glibc"]` maps to 4 families, not one) and would make `Constraint` lie about its source. Also, libc and LinuxFamily are orthogonal dimensions -- a future family could use either libc.
- **Handle libc entirely in AnalyzeRecipe by inspecting WhenClause directly**: Skip the `Constraint` struct and have `AnalyzeRecipe` look at `step.When.Libc` instead. Rejected because `AnalyzeRecipe` is designed to operate on pre-computed `StepAnalysis` / `Constraint` data, not raw when clauses. Breaking that layering would be inconsistent with the existing architecture.

## Files to Modify

- `internal/recipe/types.go` -- Add `Libc` field to `Constraint` struct; update `Clone()` and `Validate()` to handle Libc; add libc propagation logic in `MergeWhenClause()`
- `internal/recipe/policy.go` -- Update `AnalyzeRecipe()` to map libc-constrained steps to their corresponding families (musl -> alpine, glibc -> debian/rhel/arch/suse), contributing to `familiesUsed`
- `internal/recipe/analysis_test.go` -- Add tests for `ComputeAnalysis` with libc when clauses (single libc, multi-libc, conflict detection)
- `internal/recipe/policy_test.go` -- Add tests for `AnalyzeRecipe` with libc-constrained steps: verifying FamilySpecific/FamilyMixed policy and correct `familiesUsed` output

## Files to Create

None.

## Implementation Steps

- [x] Add `Libc` field to `Constraint` struct in `types.go` (string field, e.g., "glibc", "musl", or empty for unconstrained)
- [x] Update `Constraint.Clone()` to copy the `Libc` field
- [x] Update `Constraint.Validate()` to check that `Libc` is only set when `OS` is empty or `"linux"` (same pattern as `LinuxFamily` validation)
- [x] Add libc propagation in `MergeWhenClause()`: when `when.Libc` has exactly 1 value, set `result.Libc` to that value (with conflict detection against implicit constraint); when multi-value (e.g., `["glibc", "musl"]`), leave `result.Libc` empty since it spans both
- [x] Add helper function `FamiliesForLibc(libc string) []string` in `policy.go` that maps "musl" -> ["alpine"] and "glibc" -> ["debian", "rhel", "arch", "suse"]; uses `AllLinuxFamilies` and `platform.LibcForFamily` for consistency
- [x] Update `AnalyzeRecipe()` in `policy.go` to check `analysis.Constraint.Libc`:
  - If `Libc` is set and `LinuxFamily` is not set, derive the family set via `FamiliesForLibc` and add each to `familiesUsed`
  - If both `Libc` and `LinuxFamily` are set, the existing `LinuxFamily` logic already handles it (no change needed)
  - A libc-constrained step without a family constraint should NOT count as `hasUnconstrainedLinuxSteps` (it is family-scoped)
- [x] Add unit tests in `types_test.go`:
  - `TestMergeWhenClause_SingleLibc`: when clause `libc = ["musl"]` produces `Constraint{Libc: "musl"}`
  - `TestMergeWhenClause_MultiLibcLeavesEmpty`: when clause `libc = ["glibc", "musl"]` leaves `Constraint.Libc` empty
  - `TestMergeWhenClause_LibcConflict`: implicit constraint with `Libc: "glibc"` + when `libc = ["musl"]` produces error
  - `TestMergeWhenClause_LibcRedundant`: redundant libc (same value) merges correctly
  - `TestConstraint_Clone_IncludesLibc`, `TestConstraint_Validate_LibcOnNonLinux`, etc.
- [x] Add unit tests in `policy_test.go`:
  - `TestAnalyzeRecipe_LibcMuslOnly`: recipe with only `libc = ["musl"]` steps -> `FamilySpecific` with `alpine`
  - `TestAnalyzeRecipe_LibcGlibcOnly`: recipe with only `libc = ["glibc"]` steps -> `FamilySpecific` with `debian`, `rhel`, `arch`, `suse`
  - `TestAnalyzeRecipe_LibcBothSplit`: recipe with separate glibc and musl steps -> `FamilySpecific` with all 5 families
  - `TestAnalyzeRecipe_LibcMixedWithUnconstrained`: recipe with libc-constrained + unconstrained steps -> `FamilyMixed`
  - `TestSupportedPlatforms_LibcMuslOnly`: verifies only alpine platforms are generated
  - `TestAnalyzeRecipe_LibcWithLinuxFamilyPreference`: LinuxFamily takes precedence over Libc
  - `TestFamiliesForLibc`: verifies musl->alpine and glibc->debian/rhel/arch/suse mapping
- [x] Run `go test ./internal/recipe/...` to verify all existing and new tests pass
- [x] Run `go vet ./...` and `go build ./cmd/tsuku` to verify no regressions

## Testing Strategy

- **Unit tests**: Focus on three layers:
  1. `MergeWhenClause` / `ComputeAnalysis`: verify libc propagation into `Constraint`, conflict detection, and single-vs-multi-value handling
  2. `AnalyzeRecipe`: verify the libc-to-family mapping produces correct `RecipeFamilyPolicy` and `FamiliesUsed`
  3. `SupportedPlatforms`: verify the end-to-end chain produces the right platform list for libc-constrained recipes
- **Regression tests**: All 39 existing test packages must continue to pass. The fontconfig recipe pattern (glibc + musl steps) is a good real-world reference for validating the fix.
- **Manual verification**: Load a recipe like `fontconfig.toml` and call `SupportedPlatforms` to confirm family-specific platforms are generated instead of generic linux platforms.

## Risks and Mitigations

- **Multi-value libc ambiguity**: A when clause like `libc = ["glibc", "musl"]` maps to all families, which is equivalent to no constraint. Mitigation: leave `Constraint.Libc` empty for multi-value and let the existing unconstrained path handle it. This is the same pattern used for multi-OS when clauses.
- **Future libc types**: If a new libc type is added (e.g., "bionic" for Android), `FamiliesForLibc` needs updating. Mitigation: derive the mapping from `AllLinuxFamilies` + `platform.LibcForFamily` so it stays consistent with the existing family list.
- **Interaction with LinuxFamily constraint**: A step could theoretically have both `libc = ["musl"]` and `linux_family = "alpine"` -- this is redundant but valid. Mitigation: when both are set, `LinuxFamily` takes precedence in `AnalyzeRecipe` (existing code path), and `Libc` adds no additional families.

## Success Criteria

- [ ] A recipe with `when = { os = ["linux"], libc = ["glibc"] }` and `when = { os = ["linux"], libc = ["musl"] }` steps produces `FamilySpecific` policy with all 5 families
- [ ] A recipe with only `libc = ["musl"]` steps produces `FamilySpecific` policy with only `alpine`
- [ ] `SupportedPlatforms` generates family-qualified Linux platforms (e.g., `linux/amd64/alpine`) instead of generic `linux/amd64` for libc-constrained recipes
- [ ] All 39 existing test packages pass without modification
- [ ] `Constraint.Libc` is correctly propagated through `MergeWhenClause` with conflict detection

## Open Questions

None. The mapping between libc and families is well-established in the codebase (`platform.LibcForFamily`), and the policy analysis pattern for family-scoped steps is already proven with `LinuxFamily`.
