# Scrutiny Review: Intent - Issue #1937

**Issue**: #1937 (fix(builders): handle string-type `bin` field in npm builder)
**Design doc**: docs/designs/DESIGN-binary-name-discovery.md
**Files changed**: internal/builders/npm.go, internal/builders/npm_test.go

---

## Sub-check 1: Design Intent Alignment

### Design doc description for #1937

The design doc (section "2. npm builder") says:

> Fix `parseBinField()` to handle the string-type `bin` value. When `bin` is a string (not a map), it means there's a single executable whose name matches the package name. The function should return `[]string{packageName}` in this case instead of `nil`. For scoped packages (`@scope/tool`), the executable name is the unscoped part (`tool`), so the function needs the package name passed in to strip the scope prefix.

### Implementation assessment

The implementation matches the design intent precisely:

1. **`parseBinField` signature**: Changed from `parseBinField(bin any)` to `parseBinField(bin any, packageName string)` -- matches design requirement that "the function needs the package name passed in."

2. **String-type bin handling** (npm.go:267-271): The `case string:` branch calls `unscopedPackageName(packageName)`, validates through `isValidExecutableName()`, and returns `[]string{name}`. This matches the design's requirement to "return `[]string{packageName}`" with scope stripping.

3. **Scope stripping** (npm.go:288-293): The `unscopedPackageName()` helper uses `strings.LastIndex(name, "/")` with `strings.HasPrefix(name, "@")` to strip the `@scope/` prefix. This correctly handles the `@scope/tool -> tool` example from the design.

4. **isValidExecutableName validation** (npm.go:269): String-derived executable names pass through `isValidExecutableName()` before being returned. This matches the design's security requirement that "all extracted binary names validated through `isValidExecutableName()`."

5. **Caller updated** (npm.go:244): `discoverExecutables()` passes `pkgInfo.Name` as the second argument to `parseBinField()`.

**Verdict**: No design intent gap. The implementation captures what the design doc describes for this issue.

---

## Sub-check 2: Cross-Issue Enablement

### Downstream: #1938 (BinaryNameProvider and orchestrator validation)

#1938's scope: "Implements the interface on Cargo and npm builders using their cached registry data."

The npm builder needs to implement `BinaryNameProvider` with an `AuthoritativeBinaryNames() []string` method. This method will need to call `parseBinField()` with the cached package data and return the result.

**What #1937 provides for #1938:**
- `parseBinField(bin any, packageName string)` correctly handles all bin formats (string and map). This is the core function #1938's `AuthoritativeBinaryNames()` will call.
- `unscopedPackageName()` is available as a helper for scope stripping.

**What #1937 does NOT provide:**
- No cached package response on the `NpmBuilder` struct. The `Build()` method fetches `pkgInfo` and uses it locally but doesn't store it. Compare with the Cargo builder (#1936), which added `cachedCrateInfo *cratesIOCrateResponse` to the struct and sets `b.cachedCrateInfo = crateInfo` in `Build()`.

**Impact assessment:** The missing cache is a minor gap, not a structural problem. Adding it requires:
1. One field on `NpmBuilder`: `cachedPackageInfo *npmPackageResponse`
2. One line in `Build()`: `b.cachedPackageInfo = pkgInfo`

The #1938 implementer can add this in 2 lines alongside implementing `AuthoritativeBinaryNames()`. The `parseBinField` function -- which is the harder piece -- is fully working and tested. The foundation is sufficient but asymmetric with the Cargo builder's approach.

**Severity: Advisory.** The Cargo builder (#1936) proactively cached its API response for `BinaryNameProvider`. The npm builder doesn't follow this convention. This is an asymmetry worth noting, but it doesn't block #1938 -- the caching can be added there with minimal effort. The critical piece (correct `parseBinField` with all bin formats) is solid.

---

## Backward Coherence

**Previous issue**: #1936 (Cargo binary discovery)
**Previous summary**: "Added cachedCrateInfo field to CargoBuilder for future BinaryNameProvider."

#1936 established a convention: cache the API response during `Build()` so downstream `BinaryNameProvider` can use it. #1937 doesn't follow this convention for the npm builder. This is the same finding as the cross-issue enablement gap above.

No other backward coherence issues. The implementation style is consistent: both builders use mock HTTP servers in tests, both use `isValidExecutableName()` for validation, both have `discoverExecutables()` methods that delegate to field-specific parsers.

---

## Summary

| Finding | Severity | Details |
|---------|----------|---------|
| No cached package response for BinaryNameProvider | Advisory | Cargo builder (#1936) cached its API response; npm builder doesn't. #1938 will need 2 lines to add this. |

No blocking findings. The implementation matches the design doc's described behavior for #1937. The `parseBinField` fix, scope stripping, validation, and test coverage all align with the design intent. The only gap is the missing API response cache, which is an asymmetry with the Cargo builder's approach but doesn't create a structural problem for the downstream issue.
