# Architect Review: Issue #1936 - Cargo bin_names binary discovery

## Review Scope

Files changed: `internal/builders/cargo.go`, `internal/builders/cargo_test.go`

## Design Alignment

The implementation matches the design doc's intent for Phase 1:

1. **`BinNames` field on `cratesIOVersion`**: Added as specified. Uses the version-level `bin_names` array from the crates.io API response.

2. **`discoverExecutables()` rewritten**: No longer fetches Cargo.toml from GitHub. Reads `bin_names` from the API response passed by `Build()`. Falls back to crate name when `bin_names` is empty or all versions are yanked, as the design specifies.

3. **Dead code removed**: `buildCargoTomlURL()`, `fetchCargoTomlExecutables()`, `cargoToml`, and `cargoTomlBinSection` are all absent from the codebase. The `github.com/BurntSushi/toml` dependency should also be gone from cargo.go (no import for it found).

4. **`cachedCrateInfo` field**: Added to `CargoBuilder` struct to support the future `BinaryNameProvider` interface (#1938).

5. **`isValidExecutableName()` applied to bin_names**: Every name from `bin_names` is validated, with warnings for filtered entries. Consistent with how other builders use this function.

## Pattern Consistency Assessment

### Builder pattern: Consistent

The `discoverExecutables()` signature follows the established pattern:
- **Cargo**: `discoverExecutables(_ context.Context, crateInfo *cratesIOCrateResponse) ([]string, []string)`
- **npm**: `discoverExecutables(pkgInfo *npmPackageResponse) ([]string, []string)`
- **PyPI**: `discoverExecutables(ctx context.Context, pkgInfo *pypiPackageResponse) ([]string, []string)`
- **Gem**: `discoverExecutables(ctx context.Context, gemInfo *rubyGemsGemResponse) ([]string, []string)`

All return `(executables, warnings)` and all fall back to the package name when discovery fails. Cargo's new implementation is consistent with this pattern.

### Builder interface: No violations

`CargoBuilder` still implements `SessionBuilder` through `Name()`, `RequiresLLM()`, `CanBuild()`, and `NewSession()`. It uses `DeterministicSession` wrapping `b.Build` -- same as other ecosystem builders.

### Caching pattern: New but justified

The `cachedCrateInfo` field is a new pattern -- no other builder caches response data for post-build access. However, it's explicitly scoped to support the `BinaryNameProvider` interface in #1938, which the design doc calls out. The design doc says the orchestrator will type-assert `SessionBuilder` to `BinaryNameProvider` in `Create()`, then call `AuthoritativeBinaryNames()` after `session.Generate()` returns. Since `Build()` runs inside `DeterministicSession.Generate()`, the cache is populated by the time the orchestrator reads it. The data flow is sound.

This cache introduces mutable state on the builder struct, which could be surprising if a builder is reused across multiple `Build()` calls (each call overwrites the cache with the latest crate's data). However, the orchestrator creates sessions per-build, so in practice only the last build's data matters. Not a structural problem given the current usage pattern.

### Dependency direction: Clean

`cargo.go` imports only standard library packages plus `internal/recipe`. No upward dependencies, no circular imports.

## Findings

### Advisory: `isValidExecutableName` regex is recompiled on every call

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/cargo.go:278`

The `isValidExecutableName()` function calls `regexp.MatchString()` on every invocation, which compiles the regex each time. In contrast, `isValidCrateName()` uses a pre-compiled `crateNameRegex`. Since `isValidExecutableName()` is called from multiple builders (cargo, npm, go, gem, pypi), pre-compiling would be consistent with the existing pattern. Not blocking because performance impact is negligible in practice (called a few times per build).

## Overall Assessment

The implementation fits the existing architecture cleanly. It replaces the GitHub-based discovery with registry API data without introducing new patterns, new packages, or new abstractions beyond what the design doc prescribes. The `cachedCrateInfo` field is the only new structural element, and it's scoped correctly for the planned #1938 integration. The dead code from the old approach has been removed. No blocking findings.
