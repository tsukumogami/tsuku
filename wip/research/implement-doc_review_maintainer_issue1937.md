# Maintainer Review: Issue #1937

## Review Scope

Files changed: `internal/builders/npm.go`, `internal/builders/npm_test.go`

Commit: `828aaadca79175078598fba1c4bf8d8d6e538ae9`

## Findings

### 1. Unused type-switch variable in string bin case

**File:** `internal/builders/npm.go:267`
**Severity:** Advisory

```go
case string:
    name := unscopedPackageName(packageName)
```

The `case string:` branch binds the type-switch variable `v` (the actual string value like `"./bin/tool.js"`), but never uses it. This is intentional and correct: per npm's `package.json` spec, when `bin` is a string, the executable name is the package name (not the string value, which is a file path). But the next developer reading `case string:` will wonder why the matched string is ignored.

Compare this to the `case string:` in `extractRepositoryURL` at line 303, which *does* use `v`. The same pattern with different behavior in two functions in the same file is a minor readability trap.

A one-line comment would prevent the double-take:

```go
case string:
    // npm convention: when bin is a string, the executable name is the package name,
    // not the string value (which is the script path).
    name := unscopedPackageName(packageName)
```

Alternatively, use `case string:` without the type switch binding (`switch bin.(type)` instead of `switch v := bin.(type)`) -- but that would require restructuring the function since `v` is used in the `map[string]any` branch.

This won't cause a bug, but it will cause a "wait, is this a mistake?" pause.

### 2. Same implicit "never empty" invariant as Cargo builder

**File:** `internal/builders/npm.go:163`
**Severity:** Advisory

```go
Verify: &recipe.VerifySection{
    Command: fmt.Sprintf("%s --version", executables[0]),
},
```

`Build()` accesses `executables[0]` without a length check, relying on `discoverExecutables()` always returning at least one element. This is the same pattern flagged as advisory in the #1936 cargo builder review. The invariant holds: `discoverExecutables` has three fallback paths that all return `[]string{pkgInfo.Name}`, plus the happy path returns at least one element (validated by `len(executables) == 0` at line 245). But it's implicit.

This is a cross-builder pattern. If #1938 (BinaryNameProvider + orchestrator validation) introduces a new code path that changes the invariant, this will become a latent panic. The previous review suggested either a guard clause or a comment. Neither was added here -- the same implicit contract is replicated.

**Suggestion:** Same as the #1936 review. Either add `if len(executables) == 0 { return nil, fmt.Errorf(...) }` before accessing `executables[0]`, or document the invariant at the call site.

### 3. parseBinField godoc accurately describes behavior

**File:** `internal/builders/npm.go:253-260`
**Severity:** (Positive observation)

```go
// parseBinField extracts executable names from the bin field.
// The bin field can be:
// - string: the executable name equals the (unscoped) package name
// - map[string]string: keys are executable names, values are paths
//
// packageName is required so that string-type bin values can derive the
// executable name. For scoped packages (@scope/tool), the scope prefix
// is stripped so the executable name is "tool".
```

This godoc tells the next developer exactly why `packageName` is needed and what "string-type bin" means. The comment says `map[string]string` for the type annotation but the code uses `map[string]any` (which is what `encoding/json` actually deserializes to). This is a minor inaccuracy in the comment -- not the code -- and won't mislead anyone into a bug since the comment describes the *npm convention* not the Go type.

### 4. unscopedPackageName uses correct edge-case handling

**File:** `internal/builders/npm.go:288-293`
**Severity:** (Positive observation)

```go
func unscopedPackageName(name string) string {
    if idx := strings.LastIndex(name, "/"); idx >= 0 && strings.HasPrefix(name, "@") {
        return name[idx+1:]
    }
    return name
}
```

Using `strings.LastIndex` (not `strings.Index`) is the right call. While current npm scopes have exactly one slash (`@scope/name`), using `LastIndex` avoids a hypothetical edge case where a malformed input contains multiple slashes. The `strings.HasPrefix(name, "@")` guard prevents non-scoped names that happen to contain `/` from being split. Both conditions together correctly define "scoped npm package name."

The function is tested with 6 cases including edge cases (`"@a/b"`, `"plain"`). Good.

### 5. Test names and assertions match

**File:** `internal/builders/npm_test.go:263-318`
**Severity:** (Positive observation)

The new test cases are well-named and assert the right things:

- `"string bin unscoped package"` -- asserts executable equals package name, no warnings
- `"string bin scoped package"` -- asserts executable is scope-stripped, no warnings
- `"map bin with multiple executables"` -- asserts order-independent set comparison
- `"map bin scoped package uses map keys"` -- asserts map keys override package name

Each test name matches its assertion. The scope-stripping test correctly uses `@scope/string-bin-scoped` and expects `"string-bin-scoped"`. The `TestParseBinField` unit tests at line 408 provide the lower-level coverage, while the `TestNpmBuilder_Build` integration tests verify the full pipeline. Good layering.

### 6. TestParseBinField uses order-independent comparison for map results

**File:** `internal/builders/npm_test.go:447-457`
**Severity:** (Positive observation)

```go
want := make(map[string]bool, len(tc.wantNames))
for _, n := range tc.wantNames {
    want[n] = true
}
for _, n := range got {
    if !want[n] {
        t.Errorf(...)
    }
}
```

Since `parseBinField` iterates over a map (non-deterministic order), using a set comparison rather than slice equality is correct. This avoids flaky tests from map iteration order. Consistent with how the `"multiple binaries"` and `"map bin with multiple executables"` tests in `TestNpmBuilder_Build` also use set-based comparisons.

## Overall Assessment

The implementation is clean and focused. The core change -- adding string-type `bin` support to `parseBinField` and introducing `unscopedPackageName` for scope stripping -- is well-structured, well-documented, and well-tested.

No blocking findings. The two advisory items are:
1. The unused type-switch variable in the `case string:` branch is a minor readability trap (one comment fixes it).
2. The `executables[0]` access without a length guard is the same implicit invariant flagged in the #1936 review, replicated here without the suggested mitigation. This is a pattern to address at the builder level, likely in #1938 when the orchestrator validation work adds structure around executable discovery.

The test coverage is thorough: dedicated unit tests for `parseBinField` (7 cases), `unscopedPackageName` (6 cases), and integration tests through `Build()` covering both string and map bin fields for scoped and unscoped packages. The test hierarchy (unit tests on the parser function, integration tests through the builder) gives the next developer two levels of feedback if they break something.
