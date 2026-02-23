# Scrutiny Review: Intent -- Issue #1936

**Issue**: #1936 (feat(builders): use crates.io `bin_names` for Cargo binary discovery)
**Focus**: intent (design alignment + cross-issue enablement)

---

## Sub-check 1: Design Intent Alignment

### AC: cratesIOCrateResponse.Versions struct includes BinNames and Yanked fields

**Claimed**: implemented, evidence `cargo.go:cratesIOVersion struct`
**Assessment**: PASS

The design doc says: "The `cratesIOCrateResponse` struct needs a `BinNames` field added to capture the version-level `bin_names` array."

Implementation at `cargo.go:36-39`:
```go
type cratesIOVersion struct {
    BinNames []string `json:"bin_names"`
    Yanked   bool     `json:"yanked"`
}
```

This is a separate type `cratesIOVersion` rather than inline fields on the existing response struct. The response struct's `Versions` field changed to `[]cratesIOVersion`. This matches the design doc's intent -- the version-level fields are captured and available.

### AC: discoverExecutables() reads bin_names from latest non-yanked version

**Claimed**: implemented, evidence `cargo.go:discoverExecutables()`
**Assessment**: PASS

The design doc says: "read `bin_names` directly from the crates.io API response that `fetchCrateInfo()` already returns... use `bin_names` from the latest non-yanked version."

Implementation at `cargo.go:220-259`:
- Takes `crateInfo *cratesIOCrateResponse` as parameter (reads from API response)
- Iterates versions, skipping yanked ones, taking first non-yanked version's `BinNames`
- Falls back to crate name when bin_names is empty or all versions yanked
- No longer fetches Cargo.toml from GitHub

This matches the design doc's described behavior exactly.

### AC: All binary names pass through isValidExecutableName() validation

**Claimed**: implemented, evidence `cargo.go lines 242-248`
**Assessment**: PASS

The design doc says: "The existing `isValidExecutableName()` regex... must be applied to every binary name from `bin_names` before it enters a recipe."

Implementation at `cargo.go:242-248`:
```go
for _, name := range binNames {
    if isValidExecutableName(name) {
        executables = append(executables, name)
    } else {
        warnings = append(warnings, ...)
    }
}
```

Every bin_name is validated. Invalid names are filtered with warnings.

### AC: Falls back to crate name when bin_names is empty

**Claimed**: implemented, evidence `cargo.go lines 251-256`
**Assessment**: PASS

The design doc says: "falling back to the crate name only if `bin_names` is empty or null."

Implementation handles three fallback cases:
1. All versions yanked (line 236-238)
2. Empty bin_names array (lines 251-256)
3. All bin_names invalid (also lines 251-256, since `executables` would be empty)

### AC: Dead code removed

**Claimed**: implemented
**Assessment**: PASS

The design doc says: "Remove `buildCargoTomlURL()`, `fetchCargoTomlExecutables()`, and the `cargoToml`/`cargoTomlBinSection` structs."

Grep confirms none of these symbols exist in the codebase.

### AC: Constants removed

**Claimed**: implemented
**Assessment**: PASS

`maxCargoTomlSize` and `cargoTomlFetchTimeout` are not present. Only `maxCratesIOResponseSize` remains.

### AC: toml import removed

**Claimed**: implemented
**Assessment**: PASS

No toml imports in `cargo.go`.

### AC: Unit tests for known-mismatch packages

**Claimed**: implemented
**Assessment**: PASS

Tests present for:
- `sqlx-cli` (TestCargoBuilder_Build_SqlxCli) -- produces `sqlx`, `cargo-sqlx`
- `probe-rs-tools` (TestCargoBuilder_Build_ProbeRsTools) -- produces `probe-rs`, `cargo-flash`, `cargo-embed`
- `fd-find` (TestCargoBuilder_Build_FdFind) -- produces `fd`
- Empty bin_names (TestCargoBuilder_Build_EmptyBinNamesFallbackToCrateName)
- All yanked (TestCargoBuilder_Build_AllVersionsYankedFallbackToCrateName)
- Invalid names (TestCargoBuilder_Build_InvalidBinNamesFiltered, TestCargoBuilder_Build_AllBinNamesInvalidFallbackToCrateName)
- Yanked version skipping (TestCargoBuilder_Build_SkipsYankedVersionForBinNames)
- Null bin_names (TestCargoBuilder_Build_NullBinNamesFallbackToCrateName)
- No versions (TestCargoBuilder_Build_NoVersionsFallbackToCrateName)

Matches design doc validation strategy section.

### AC: Cached API response for BinaryNameProvider

**Claimed**: implemented, evidence `cargo.go:cachedCrateInfo field; cargo_test.go:CachesCrateInfo`
**Assessment**: PASS

The design doc says: "The builder must cache the API response from `fetchCrateInfo()` so that `AuthoritativeBinaryNames()` can return the data later when the orchestrator calls it."

Implementation:
- `cachedCrateInfo *cratesIOCrateResponse` field on `CargoBuilder` (line 52)
- Set in `Build()` at line 124: `b.cachedCrateInfo = crateInfo`
- Test `TestCargoBuilder_Build_CachesCrateInfo` verifies the field is nil before Build and populated after

### AC: All existing tests pass

**Claimed**: implemented
**Assessment**: PASS (accepted at face value per scope -- correctness verification is the tester's domain)

---

## Sub-check 2: Cross-Issue Enablement

### Downstream: #1938 -- BinaryNameProvider interface and orchestrator validation

**Key ACs from #1938**:
1. "Define BinaryNameProvider interface with AuthoritativeBinaryNames() []string method"
2. "Implement BinaryNameProvider on CargoBuilder using the cached crates.io API response (bin_names field added in #1936)"
3. "The response cache must survive between Build() and AuthoritativeBinaryNames() call by orchestrator"

**Analysis**:

The cache mechanism works correctly for the orchestrator flow. The sequence is:
1. `Orchestrator.Create()` receives `builder SessionBuilder` (the `CargoBuilder` instance)
2. `builder.NewSession()` creates a `DeterministicSession` wrapping `b.Build` as a method value
3. `session.Generate()` calls `b.Build()`, which populates `b.cachedCrateInfo`
4. After `Generate()` returns, `builder` variable still holds the same `CargoBuilder` instance
5. #1938 can type-assert `builder.(BinaryNameProvider)` and call `AuthoritativeBinaryNames()`

The `cachedCrateInfo` is of type `*cratesIOCrateResponse`, which is unexported. Since both the field and the future `AuthoritativeBinaryNames()` method live in the same `builders` package, this is not a problem.

The cached data contains the full version list with `BinNames` and `Yanked` fields, so `AuthoritativeBinaryNames()` has everything it needs to replicate or call `discoverExecutables()` logic.

**Verdict**: The foundation is sufficient for #1938. No missing fields, no structural barriers.

### Backward Coherence

No previous issues -- this is the first in the sequence. Skipped.

---

## Summary

| Finding | Severity | Details |
|---------|----------|--------|
| None | -- | All ACs align with design intent |

**Blocking findings**: 0
**Advisory findings**: 0

The implementation faithfully follows the design doc's architecture for the Cargo builder changes. The `cratesIOVersion` struct captures the needed fields, `discoverExecutables()` reads from the API response instead of GitHub, dead code is removed, validation is applied to all names, and the caching mechanism is properly structured for #1938's `BinaryNameProvider` implementation.
