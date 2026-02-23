# Maintainer Review: Issue #1936

## Review Scope

Files changed: `internal/builders/cargo.go`, `internal/builders/cargo_test.go`

Commit: `5c0a71fc7b9ed1f9eb4d0cd767fe0be8286596ca`

## Findings

### 1. Implicit "never empty" invariant on discoverExecutables return value

**File:** `internal/builders/cargo.go:152`
**Severity:** Advisory

`Build()` accesses `executables[0]` without a length check at line 152:

```go
Verify: cargoVerifySection(executables[0]),
```

This is currently safe because `discoverExecutables()` always returns at least one element -- every code path either returns the validated `executables` slice (checked non-empty at line 251) or falls back to `[]string{crateInfo.Crate.Name}`. But this invariant is implicit. A next developer adding a new return path to `discoverExecutables()` (e.g., a new fallback branch) could return an empty slice and trigger an index-out-of-bounds panic.

The invariant is documented in the function's godoc ("falling back to the crate name when bin_names is empty"), which helps. But the caller doesn't enforce it.

**Suggestion:** Either add a guard clause in `Build()`:
```go
if len(executables) == 0 {
    return nil, fmt.Errorf("no executables discovered for crate %s", req.Package)
}
```
Or add a comment at the call site: `// discoverExecutables always returns at least one element (falls back to crate name)`.

### 2. Unused context parameter in discoverExecutables

**File:** `internal/builders/cargo.go:220`
**Severity:** Advisory

The `discoverExecutables` signature retains a `context.Context` parameter (blanked as `_`) even though the function no longer makes network calls. The old implementation fetched Cargo.toml from GitHub, which needed the context. The new implementation only reads from the already-fetched `crateInfo` struct.

Other builders in the package vary: npm's `discoverExecutables` takes no context (pure data parsing), while PyPI and Gem take context (they still fetch from GitHub). The cargo builder is now in the npm category -- pure data reading -- but keeps the context parameter.

This won't mislead anyone into a bug, but it signals "this function might do I/O" when it doesn't. The calling code at line 132 passes `ctx` into a function that ignores it.

**Suggestion:** Drop the context parameter to match the function's actual behavior. This makes the signature honest: no context means no I/O. If a future change reintroduces network calls, the compiler will force the context back in.

### 3. Test structure: repeated httptest.NewServer boilerplate

**File:** `internal/builders/cargo_test.go` (lines 86-743)
**Severity:** Advisory

Eleven test functions each create their own `httptest.NewServer` with nearly identical handler logic: check URL path, return a JSON response, 404 otherwise. The only thing that varies is the JSON payload and the URL path. This is about 15 lines of boilerplate per test.

This is consistent with the existing test pattern in the file (the pre-existing CanBuild and Discover tests use the same approach), so it doesn't break codebase conventions. The tests are individually readable -- each one is self-contained with its own server and assertion.

The downside is that if the mock server pattern needs to change (e.g., adding a required header check), every test needs updating. A helper like `newCratesMockServer(t, crateName, jsonResponse)` would reduce this to one line per test. But this is a pattern consistency question, not a misread risk.

### 4. Comment referencing future issue is clear and traceable

**File:** `internal/builders/cargo.go:48-52`
**Severity:** (Positive observation)

The `cachedCrateInfo` field comment clearly explains why it exists, what populates it, and what will consume it:

```go
// cachedCrateInfo stores the last fetchCrateInfo response so that
// AuthoritativeBinaryNames() can return bin_names without re-fetching.
// Populated during Build() and read by the orchestrator via
// the BinaryNameProvider interface (#1938).
```

This is good forward-reference documentation. The next developer will understand why this field exists even before #1938 is implemented. The issue number makes it traceable.

### 5. API ordering assumption is documented

**File:** `internal/builders/cargo.go:224`
**Severity:** (Positive observation)

The comment "The crates.io API returns versions ordered by publication date (newest first)" documents a critical assumption about external API behavior. Without this comment, a next developer might wonder whether the first non-yanked version is actually the latest. The design doc (line 212) also specifies this. Good.

### 6. Test names accurately describe what they test

**File:** `internal/builders/cargo_test.go`
**Severity:** (Positive observation)

Test names like `TestCargoBuilder_Build_SqlxCli`, `TestCargoBuilder_Build_SkipsYankedVersionForBinNames`, and `TestCargoBuilder_Build_AllBinNamesInvalidFallbackToCrateName` are descriptive and match what the tests actually assert. The test for sqlx-cli is particularly valuable as a regression test for the specific monorepo failure case that motivated this change.

## Overall Assessment

The implementation is clean and well-structured. The core change -- reading `bin_names` from the crates.io API response instead of fetching Cargo.toml from GitHub -- is a minimal, focused refactor. The function responsibilities are clear: `fetchCrateInfo` gets data, `discoverExecutables` extracts binary names with validation and fallback, `Build` assembles the recipe.

No blocking findings. The implicit non-empty invariant on `discoverExecutables` is the most notable concern, but the current code does guarantee at least one element through its fallback logic, and the godoc describes this behavior. The unused context parameter is cosmetic.

The test coverage is thorough: workspace monorepos (sqlx-cli, probe-rs-tools), name mismatches (fd-find), yanked versions, invalid names (security filtering), null/empty bin_names, and the caching behavior for #1938. Each test serves as documentation of a specific edge case.
