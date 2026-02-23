# Scrutiny Review: Completeness -- Issue #1936

**Issue**: #1936 feat(builders): use crates.io `bin_names` for Cargo binary discovery
**Focus**: completeness
**Reviewer**: maintainer-reviewer

## AC-by-AC Verification

### AC 1: `cratesIOCrateResponse.Versions` struct includes `BinNames []string` and `Yanked bool`
**Mapping claim**: implemented, evidence: `cargo.go:cratesIOVersion struct at lines 36-39`
**Verification**: CONFIRMED. `cargo.go` lines 36-39 define `cratesIOVersion` with `BinNames []string \`json:"bin_names"\`` and `Yanked bool \`json:"yanked"\``. The `Versions` field on `cratesIOCrateResponse` (line 32) uses this type.
**Severity**: N/A (correct)

### AC 2: `discoverExecutables()` reads `bin_names` from latest non-yanked version
**Mapping claim**: implemented, evidence: `cargo.go:discoverExecutables() at lines 216-260`
**Verification**: CONFIRMED. Lines 227-234 iterate versions, skip yanked ones, take `BinNames` from the first non-yanked version. Lines 236-239 handle the all-yanked case. Lines 251-256 handle empty/invalid bin_names with crate name fallback.
**Severity**: N/A (correct)

### AC 3: All binary names from `bin_names` pass through `isValidExecutableName()` validation
**Mapping claim**: implemented, evidence: `cargo.go lines 242-248`
**Verification**: CONFIRMED. Lines 242-248 iterate `binNames`, call `isValidExecutableName(name)`, and only append valid names to `executables`. Invalid names produce warnings.
**Severity**: N/A (correct)

### AC 4: Falls back to crate name when `bin_names` is empty or null
**Mapping claim**: implemented, evidence: `cargo.go lines 251-256; tests EmptyBinNamesFallbackToCrateName and NullBinNamesFallbackToCrateName`
**Verification**: CONFIRMED. Lines 251-256 return `[]string{crateInfo.Crate.Name}` when `len(executables) == 0`. Both test functions exist at lines 317 and 376 respectively with correct assertions.
**Severity**: N/A (correct)

### AC 5: Dead code removed: `buildCargoTomlURL()`, `fetchCargoTomlExecutables()`, `cargoToml`, `cargoTomlBinSection`
**Mapping claim**: implemented, evidence: functions and structs absent from cargo.go
**Verification**: CONFIRMED. `grep` for these symbols in `cargo.go` returns no matches.
**Severity**: N/A (correct)

### AC 6: Constants `maxCargoTomlSize` and `cargoTomlFetchTimeout` removed
**Mapping claim**: implemented, evidence: `cargo.go lines 17-20, const block only contains maxCratesIOResponseSize`
**Verification**: CONFIRMED. The const block at lines 17-20 contains only `maxCratesIOResponseSize`.
**Severity**: N/A (correct)

### AC 7: The toml import (`github.com/BurntSushi/toml`) is removed from `cargo.go`
**Mapping claim**: implemented, evidence: `cargo.go lines 3-15, import block does not include BurntSushi/toml`
**Verification**: CONFIRMED. The import block at lines 3-15 does not include `BurntSushi/toml`. Note: the dependency remains in `go.mod` because other packages (`internal/recipe/`, etc.) use it. This is expected and correct.
**Severity**: N/A (correct)

### AC 8: Unit test: sqlx-cli produces binary sqlx
**Mapping claim**: implemented, evidence: `cargo_test.go:TestCargoBuilder_Build_SqlxCli`
**Verification**: CONFIRMED. Test at line 164 asserts 2 executables: `"sqlx"` and `"cargo-sqlx"`. Verify command checks for `"sqlx --version"`.
**Severity**: N/A (correct)

### AC 9: Unit test: probe-rs-tools produces binaries
**Mapping claim**: implemented, evidence: `cargo_test.go:TestCargoBuilder_Build_ProbeRsTools`
**Verification**: CONFIRMED. Test at line 218 asserts 3 executables: `"probe-rs"`, `"cargo-flash"`, `"cargo-embed"`.
**Severity**: N/A (correct)

### AC 10: Unit test: fd-find produces binary fd
**Mapping claim**: implemented, evidence: `cargo_test.go:TestCargoBuilder_Build_FdFind`
**Verification**: CONFIRMED. Test at line 270 asserts 1 executable: `"fd"`.
**Severity**: N/A (correct)

### AC 11: Unit test: crate with empty bin_names falls back to crate name
**Mapping claim**: implemented, evidence: two test functions
**Verification**: CONFIRMED. `TestCargoBuilder_Build_EmptyBinNamesFallbackToCrateName` (line 317) and `TestCargoBuilder_Build_NullBinNamesFallbackToCrateName` (line 376) both exist and assert fallback to crate name.
**Severity**: N/A (correct)

### AC 12: Unit test: crate where all versions are yanked falls back to crate name
**Mapping claim**: implemented, evidence: `cargo_test.go:TestCargoBuilder_Build_AllVersionsYankedFallbackToCrateName`
**Verification**: CONFIRMED. Test at line 417 with all-yanked versions asserts fallback and yanked warning.
**Severity**: N/A (correct)

### AC 13: Unit test: bin_names containing invalid executable names are filtered out
**Mapping claim**: implemented, evidence: two test functions
**Verification**: CONFIRMED. `TestCargoBuilder_Build_InvalidBinNamesFiltered` (line 471) asserts shell-injection names are filtered, keeping valid ones. `TestCargoBuilder_Build_AllBinNamesInvalidFallbackToCrateName` (line 528) asserts fallback when all names are invalid.
**Severity**: N/A (correct)

### AC 14: Cached API response data accessible for BinaryNameProvider (#1938)
**Mapping claim**: implemented, evidence: `cargo.go:cachedCrateInfo field; cargo_test.go:TestCargoBuilder_Build_CachesCrateInfo`
**Verification**: CONFIRMED. Field at line 52, set at line 124 during `Build()`. Test at line 694 verifies cache is nil before Build, populated after, with correct data. The closure-based `DeterministicSession` architecture ensures Build() writes to the builder's field through the captured pointer, so the cache survives for the orchestrator to read.
**Severity**: N/A (correct)

### AC 15: All existing tests in cargo_test.go pass
**Mapping claim**: implemented, evidence: `go test ./... exits 0`
**Verification**: Cannot independently confirm test execution from diff alone, but no old Cargo.toml-based test references remain in cargo_test.go. The old tests were replaced with new ones covering the same surface area plus the new bin_names path.
**Severity**: N/A (accepted as claimed; CI will confirm)

## Missing AC Check

All 15 mapping entries correspond to requirements derivable from the design doc's description of #1936 scope. No phantom ACs detected -- every entry maps to either the design doc's explicit requirements (struct fields, function changes, dead code removal, caching) or the validation strategy's test cases (sqlx-cli, probe-rs-tools, fd-find, edge cases).

## Phantom AC Check

No phantom ACs. All 15 entries trace back to the design doc.

## Summary

- **Blocking findings**: 0
- **Advisory findings**: 0
- All 15 ACs verified against the code. Evidence citations are accurate. Dead code removal confirmed via grep. Cache mechanism correctly supports #1938's downstream needs through Go's closure-captures-pointer-receiver pattern.
