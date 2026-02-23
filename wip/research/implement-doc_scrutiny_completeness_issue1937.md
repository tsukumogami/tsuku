# Scrutiny Review: Completeness -- Issue #1937

**Issue**: #1937 fix(builders): handle string-type `bin` field in npm builder
**Focus**: completeness
**Files changed**: `internal/builders/npm.go`, `internal/builders/npm_test.go`

## AC-by-AC Verification

### AC 1: "parseBinField accepts packageName param"
**Claimed status**: implemented
**Assessment**: Confirmed.
**Evidence**: `npm.go:261` -- `func parseBinField(bin any, packageName string) []string`. The signature now includes a `packageName string` parameter.

### AC 2: "string bin returns unscoped name"
**Claimed status**: implemented
**Assessment**: Confirmed.
**Evidence**: `npm.go:267-271` -- The `case string:` branch calls `unscopedPackageName(packageName)` and returns it as a single-element slice. Test case `TestParseBinField/"string unscoped"` at `npm_test.go:419` confirms `parseBinField("./bin/tool.js", "my-tool")` returns `["my-tool"]`.

### AC 3: "scoped packages stripped"
**Claimed status**: implemented
**Assessment**: Confirmed.
**Evidence**: `npm.go:288-293` -- `unscopedPackageName()` uses `strings.LastIndex(name, "/")` with `strings.HasPrefix(name, "@")` to strip the scope prefix. Test case `TestParseBinField/"string scoped"` at `npm_test.go:422` confirms `parseBinField("./bin/tool.js", "@scope/tool")` returns `["tool"]`. Dedicated test `TestUnscopedPackageName` at `npm_test.go:461-482` covers 6 input variants.

### AC 4: "isValidExecutableName validation"
**Claimed status**: implemented
**Assessment**: Confirmed.
**Evidence**: `npm.go:269` -- `if isValidExecutableName(name)` in the string branch. `npm.go:276` -- same check in the map branch. Both branches validate before returning.

### AC 5: "callers updated"
**Claimed status**: implemented
**Assessment**: Confirmed.
**Evidence**: `npm.go:244` -- `executables := parseBinField(versionInfo.Bin, pkgInfo.Name)`. The sole caller `discoverExecutables` now passes `pkgInfo.Name`.

### AC 6: "existing tests updated"
**Claimed status**: implemented
**Assessment**: Confirmed.
**Evidence**: The existing `TestNpmBuilder_Build` test server handler and sub-tests have been extended. Previously existing test cases (prettier, no-bin, repo-object, repo-string, multiple-bins, no-latest, invalid name, not found) still exist. The existing `parseBinField` tests would have needed updating to accept the new `packageName` parameter.

### AC 7: "new test cases added"
**Claimed status**: implemented
**Assessment**: Confirmed. Substantial test coverage added.
**Evidence**:
- `TestParseBinField` (`npm_test.go:408-458`): 7 test cases covering nil, string unscoped, string scoped, single map, multiple map, map scoped, invalid type.
- `TestUnscopedPackageName` (`npm_test.go:461-482`): 6 test cases.
- New Build sub-tests: "string bin unscoped package" (line 263), "string bin scoped package" (line 277), "map bin with multiple executables" (line 291), "map bin scoped package uses map keys" (line 309).

### AC 8: "integration tests added"
**Claimed status**: implemented
**Assessment**: **Advisory -- weak evidence.** The Build sub-tests using `httptest.NewServer` are mock-server integration-style tests (testing through the full `Build()` method), which is a reasonable interpretation. However, the existing CI workflow (`npm-builder-tests.yml`) only tests `prettier` (map-type bin) and was not extended to test a string-bin package against the real npm registry. No new CI workflow or integration test file was added. The test plan scenarios 5 and 6 mapped to this issue are unit test scenarios. If the issue body explicitly requires "real registry" integration tests, this would be a gap. Given the tests exercise the full Build pipeline with mock HTTP, this is borderline and depends on the issue body's definition of "integration test."

### AC 9: "all tests pass"
**Claimed status**: implemented
**Assessment**: Cannot fully verify from file reads alone. The state file shows `ci_status: "pending"`, meaning CI hasn't run yet. The code is syntactically and structurally sound based on reading.

### AC 10: "bin formats for BinaryNameProvider"
**Claimed status**: implemented
**Assessment**: Confirmed with a note. `parseBinField` now correctly handles both string and map formats, which is what #1938 needs to build `AuthoritativeBinaryNames()` on. However, unlike the Cargo builder which added `cachedCrateInfo` for the provider in #1936, the npm builder has no response caching. This means #1938 will need to either add caching itself or re-fetch. This is not a gap in #1937's scope (caching is #1938's responsibility), but worth noting for downstream awareness.

## Phantom AC Check

All 10 ACs in the mapping correspond to behaviors described in the design doc's scope for #1937 or are standard engineering ACs (tests pass, callers updated). No phantom ACs detected.

## Missing AC Check

The design doc describes #1937's scope as: "Fix `parseBinField()` to return the package name when `bin` is a string rather than a map. Strips scope prefixes from scoped packages (`@scope/tool` becomes `tool`) and passes the package name into the parser signature."

All described behaviors are covered by the mapping entries. No missing ACs.

## Summary

- **Blocking findings**: 0
- **Advisory findings**: 1

The "integration tests added" AC (item 8) is the only questionable claim. The mock-server Build tests exercise the full pipeline and are functionally adequate, but the npm CI workflow was not updated to test string-bin packages against the real registry. If the issue body requires real-registry integration tests, this is a gap; if it means "tests that integrate multiple components," the mock-server tests satisfy it. Advisory severity because the unit tests thoroughly cover the string-bin behavior and the mock tests exercise the full Build path.

All other ACs are solidly confirmed by the diff. The core fix (parseBinField string handling, scope stripping, validation) is clean, well-tested, and prepares the ground for #1938's BinaryNameProvider.
