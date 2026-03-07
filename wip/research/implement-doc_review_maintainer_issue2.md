# Maintainer Review: Issue #2 (Deprecation Notice Parsing and Warning Display)

## Review Focus: Maintainability (clarity, readability, duplication)

## Files Reviewed

- `internal/registry/manifest.go` (DeprecationNotice struct, Manifest field, parsing)
- `internal/registry/manifest_test.go` (deprecation parsing tests)
- `cmd/tsuku/helpers.go` (printWarning, isDevBuild, checkDeprecationWarning, formatDeprecationWarning)
- `cmd/tsuku/helpers_test.go` (all deprecation-related tests)
- `cmd/tsuku/update_registry.go` (refreshManifest call site)
- `internal/registry/errors.go` (ErrTypeSchemaVersion, Suggestion)

## Findings

### 1. Advisory: Test stderr capture pattern repeated 9 times

**File:** `cmd/tsuku/helpers_test.go`, lines 20-34, 43-57, 64-78, 82-105, 107-141, 143-180, 182-214, 216-254

Every test that checks stderr output repeats the same 8-line pipe/capture/restore boilerplate:

```go
old := os.Stderr
r, w, _ := os.Pipe()
os.Stderr = w
// ... do something ...
w.Close()
os.Stderr = old
var buf bytes.Buffer
_, _ = buf.ReadFrom(r)
```

This appears 9 times with no helper. A `captureStderr(func()) string` helper would reduce each test by 6 lines and remove the risk of a test forgetting to restore `os.Stderr` on a panic path (none of these use `defer` for the restore). The next developer adding a 10th deprecation test will copy-paste the pattern again.

Not blocking because the tests are readable despite the duplication, and the missing `defer` is low-risk in test code.

**Severity:** Advisory

### 2. Advisory: `SunsetDate` field parsed but never used

**File:** `internal/registry/manifest.go:36`, `cmd/tsuku/helpers.go:105-127`

`DeprecationNotice.SunsetDate` is defined, parsed, and tested for round-trip correctness, but `formatDeprecationWarning` never includes it in the warning output. The next developer reading the struct will expect the sunset date to appear in the warning message and may wonder if its omission is a bug or intentional.

The design doc's acceptance criteria says the field should exist on the struct, so parsing it is correct. But a brief comment on the struct field explaining its purpose (e.g., "informational; not currently displayed in warnings") would prevent the misread.

**Severity:** Advisory

### 3. Advisory: `resetDeprecationWarning` exported for testing via naming convention only

**File:** `cmd/tsuku/helpers.go:72-74`

```go
func resetDeprecationWarning() {
    deprecationWarningOnce = sync.Once{}
}
```

This function exists solely for testing (resetting `sync.Once` between test cases). It's unexported, so it's scoped to `package main`, which is where the tests live -- this is fine structurally. But the function has no comment explaining it's test-only, and the name doesn't signal that. A `// resetDeprecationWarning resets the sync.Once for testing purposes.` comment exists on line 71, which is good. No action needed.

**Severity:** Not a finding (comment already present on review)

### 4. Advisory: `formatDeprecationWarning` separated from `checkDeprecationWarning` for testability -- good pattern

**File:** `cmd/tsuku/helpers.go:92-127`

The split between `checkDeprecationWarning` (which uses `buildinfo.Version()` and `sync.Once`) and `formatDeprecationWarning` (which accepts the CLI version as a parameter) is well done. It makes the version comparison branches directly testable without mocking `buildinfo`. The tests in `helpers_test.go` exploit this correctly: integration-style tests go through `checkDeprecationWarning`, and branch-specific tests call `formatDeprecationWarning` directly.

### 5. Advisory: `TestCheckDeprecationWarning_UpgradeNeeded` name mismatch

**File:** `cmd/tsuku/helpers_test.go:216-254`

The test is named `TestCheckDeprecationWarning_UpgradeNeeded` and sets `MinCLIVersion: "v99.0.0"`, which would normally trigger the upgrade branch. But the comment at lines 247-250 explains that since the test binary runs as a dev build, the version comparison is skipped entirely. The test only verifies the basic warning format, not the upgrade-needed branch.

The next developer will read the test name, expect it to verify upgrade messaging, and be confused when the assertions only check the warning header. The actual upgrade-needed branch is tested by `TestFormatDeprecationWarning_CLIBelowMinVersion` instead.

Consider renaming to `TestCheckDeprecationWarning_BasicFormat` or `TestCheckDeprecationWarning_DisplaysWithDevBuild` to match what it actually verifies.

**Severity:** Advisory

## Overall Assessment

The implementation is clean and well-structured. The separation between parsing (`internal/registry`) and display (`cmd/tsuku`) follows the existing codebase patterns. Function names accurately describe behavior. The `formatDeprecationWarning` extraction for testability is a good design choice that makes the version comparison branches easy to verify.

No blocking findings. The advisory items are minor: test boilerplate duplication, one slightly misleading test name, and a parsed-but-unused field that could use a clarifying comment.
