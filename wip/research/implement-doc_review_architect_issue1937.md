# Architect Review: Issue #1937 -- Handle string-type `bin` field in npm builder

**Commit**: 828aaadca79175078598fba1c4bf8d8d6e538ae9
**Files**: `internal/builders/npm.go`, `internal/builders/npm_test.go`

---

## Change Summary

The change fixes `parseBinField()` in the npm builder to handle the case where npm's `bin` field is a string (single executable path) rather than a map. When `bin` is a string, the executable name equals the (unscoped) package name. The change also adds `unscopedPackageName()` to strip `@scope/` prefixes from scoped packages.

Key additions:
- `parseBinField()` now accepts a `packageName` parameter and handles `case string:` by returning the unscoped package name
- New `unscopedPackageName()` helper
- `npmVersionInfo.Bin` changed from `map[string]string` to `any` to accommodate both string and map types
- Comprehensive tests for string bin (scoped and unscoped), map bin, and edge cases

---

## Architectural Assessment

### 1. Fits the existing builder pattern -- No issues

The change follows the established ecosystem builder structure exactly:

- `discoverExecutables()` method on the builder struct returns `([]string, []string)` -- same signature as `CargoBuilder.discoverExecutables()`, `GemBuilder.discoverExecutables()`, and `PyPIBuilder.discoverExecutables()`
- Uses `isValidExecutableName()` from `cargo.go` for executable validation -- reuses the shared function rather than introducing a parallel validator
- `parseBinField()` remains a pure function (package-level, not on the struct) -- consistent with the existing pattern where field-specific parsing is separated from the discovery orchestration
- Test structure mirrors other builders: httptest server, table-driven subtests, order-independent comparison for multi-executable cases

### 2. Interface contracts maintained -- No issues

- `SessionBuilder` interface: `Name()`, `RequiresLLM()`, `CanBuild()`, `NewSession()` all unchanged
- `BuildResult` fields: no new fields added, existing fields used correctly
- Uses `DeterministicSession` wrapper (same as Cargo, Go, PyPI, Gem builders)
- No changes to `BuildRequest` or `SessionOptions`

### 3. Dependency direction correct -- No issues

- `npm.go` imports only `internal/recipe` (same dependency as all ecosystem builders)
- No upward imports (no `cmd/`, no `internal/controller`, no `internal/template`)
- No new external dependencies

### 4. Cross-issue enablement for #1938 (BinaryNameProvider)

The design doc (DESIGN-binary-name-discovery.md, line 232) defines:

```go
type BinaryNameProvider interface {
    AuthoritativeBinaryNames() []string
}
```

The Cargo builder (#1936) proactively added caching to support this:

```go
// cargo.go:48-52
cachedCrateInfo *cratesIOCrateResponse  // populated during Build()
```

```go
// cargo.go:123-124
b.cachedCrateInfo = crateInfo  // Cache the response for BinaryNameProvider (#1938)
```

The npm builder does **not** cache `pkgInfo` on the struct. The `Build()` method fetches it into a local variable and discards it after use. When #1938 implements `AuthoritativeBinaryNames()` on `NpmBuilder`, it will need:

1. A `cachedPackageInfo *npmPackageResponse` field on the struct
2. One line in `Build()`: `b.cachedPackageInfo = pkgInfo`
3. The `AuthoritativeBinaryNames()` method itself, which calls `parseBinField(b.cachedPackageInfo.Versions[latest].Bin, b.cachedPackageInfo.Name)`

The critical piece -- `parseBinField()` handling both string and map bin formats -- is done and tested. The missing cache is 2 lines of code that #1938 can add alongside the interface implementation.

**Severity: Advisory.** This is an asymmetry with the Cargo builder's approach, but it doesn't compound. No other code will copy the npm builder's pattern expecting the cache to exist, because the `BinaryNameProvider` interface hasn't been defined in Go code yet. The #1938 implementer will add both the cache and the interface method together.

---

## Findings

| # | Finding | Severity | Location |
|---|---------|----------|----------|
| 1 | Missing cached package response for #1938 BinaryNameProvider | Advisory | `npm.go:124-128` -- `Build()` uses `pkgInfo` locally but doesn't cache it on the struct, unlike Cargo builder's `cachedCrateInfo` |

### Finding 1 Detail

**What**: `NpmBuilder.Build()` fetches `pkgInfo` but doesn't store it on the struct. The Cargo builder (#1936) added `cachedCrateInfo` proactively for downstream `BinaryNameProvider` use.

**Impact**: When #1938 adds `AuthoritativeBinaryNames()` to `NpmBuilder`, the implementer will need to also add the cache field and the assignment line. This is trivial (2 lines) and contained to the npm builder file.

**Why advisory, not blocking**: The asymmetry doesn't compound. No consumer reads a `cachedPackageInfo` field today, and the `BinaryNameProvider` interface doesn't exist in Go code yet. Adding the cache in #1938 alongside the interface implementation is cleaner than pre-adding a field with no consumer (which would itself be a state contract concern -- an unused field).

---

## Verdict

The change fits the architecture well. It follows the ecosystem builder pattern (per-builder `discoverExecutables()`, shared `isValidExecutableName()`, `DeterministicSession` wrapper), maintains all interface contracts, and keeps dependency direction correct. The `parseBinField()` function is cleanly separated and testable, handling all npm bin field variants documented in the npm spec.

One advisory finding: the npm builder doesn't pre-cache its API response for downstream `BinaryNameProvider` use, unlike the Cargo builder. This is a minor asymmetry that #1938 can resolve in 2 lines.

**0 blocking, 1 advisory.**
