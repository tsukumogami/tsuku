# Scrutiny Review: Justification Focus -- Issue #1936

**Issue**: #1936 (feat(builders): use crates.io `bin_names` for Cargo binary discovery)
**Scrutiny Focus**: justification
**Reviewer**: pragmatic-reviewer

## Summary

No deviations reported in the requirements mapping. All 15 ACs are claimed as "implemented". The justification focus evaluates deviation quality, so the primary question is whether any "implemented" claim conceals a deviation.

## Findings

### No Deviations to Evaluate

The mapping contains zero deviation entries. Every AC is marked "implemented" with evidence pointing to specific locations in `cargo.go` and `cargo_test.go`.

### Verification of "Implemented" Claims for Hidden Deviations

I checked each claim against the current file contents to confirm none are disguised shortcuts:

1. **`cratesIOVersion` struct fields**: Confirmed at `cargo.go:36-39`. `BinNames []string` with `json:"bin_names"` and `Yanked bool` with `json:"yanked"` are present.

2. **`discoverExecutables()` rewrite**: Confirmed at `cargo.go:216-260`. Reads `bin_names` from the first non-yanked version. No GitHub fetching. No Cargo.toml parsing.

3. **Validation through `isValidExecutableName()`**: Confirmed at `cargo.go:242-248`. Loop filters each bin_name.

4. **Fallback to crate name**: Confirmed at `cargo.go:251-256`. Covers both empty `bin_names` and all-invalid cases.

5. **Dead code removal**: `buildCargoTomlURL`, `fetchCargoTomlExecutables`, `cargoToml`, `cargoTomlBinSection` -- none found in `cargo.go`. Confirmed absent.

6. **Constants removed**: `maxCargoTomlSize` and `cargoTomlFetchTimeout` absent from `cargo.go`. The remaining constant is `maxCratesIOResponseSize` (different purpose).

7. **toml import removed from cargo.go**: Confirmed. Imports at lines 3-15 don't include `BurntSushi/toml`. The dependency remains in `go.mod` because other packages use it.

8. **Unit tests**: All named tests exist in `cargo_test.go`:
   - `TestCargoBuilder_Build_SqlxCli` (line 164)
   - `TestCargoBuilder_Build_ProbeRsTools` (line 218)
   - `TestCargoBuilder_Build_FdFind` (line 270)
   - `TestCargoBuilder_Build_EmptyBinNamesFallbackToCrateName` (line 317)
   - `TestCargoBuilder_Build_NullBinNamesFallbackToCrateName` (line 376)
   - `TestCargoBuilder_Build_AllVersionsYankedFallbackToCrateName` (line 417)
   - `TestCargoBuilder_Build_InvalidBinNamesFiltered` (line 471)
   - `TestCargoBuilder_Build_AllBinNamesInvalidFallbackToCrateName` (line 528)

9. **Cached API response**: `cachedCrateInfo` field at `cargo.go:52`, populated at `cargo.go:124`. Test at `cargo_test.go:694`.

10. **All existing tests pass**: Cannot independently verify (no shell access), but no structural reason they would break -- the dead code was removed, the API response format changed, and tests were updated to match.

### Cross-Issue Enablement Check

Downstream issue #1938 needs:
- A cached API response on `CargoBuilder` to implement `AuthoritativeBinaryNames()` -- provided via `cachedCrateInfo` field
- The `cratesIOVersion.BinNames` field to extract authoritative names -- present
- Logic to find latest non-yanked version's bin_names -- present in `discoverExecutables()`, reusable or reimplementable in `AuthoritativeBinaryNames()`

The foundation is adequate for #1938.

## Severity Counts

- **Blocking**: 0
- **Advisory**: 0
