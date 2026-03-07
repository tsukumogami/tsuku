# Scrutiny Review: Completeness -- Issue #2

**Issue:** feat(registry): add deprecation notice parsing and warning display
**Commit:** fdb5352b
**Files changed:** cmd/tsuku/helpers.go, cmd/tsuku/helpers_test.go, cmd/tsuku/update_registry.go, internal/registry/manifest.go, internal/registry/manifest_test.go

## AC-by-AC Verification

### AC 1: `DeprecationNotice` struct with `SunsetDate`, `MinCLIVersion`, `Message`, `UpgradeURL` fields

**Mapping claim:** implemented
**Verdict:** CONFIRMED

Evidence: `internal/registry/manifest.go` diff adds:
```go
type DeprecationNotice struct {
    SunsetDate    string `json:"sunset_date"`
    MinCLIVersion string `json:"min_cli_version"`
    Message       string `json:"message"`
    UpgradeURL    string `json:"upgrade_url,omitempty"`
}
```
All four fields present with correct JSON tags.

### AC 2: `Deprecation *DeprecationNotice` pointer field on `Manifest` (nil when absent)

**Mapping claim:** implemented
**Verdict:** CONFIRMED

Evidence: `internal/registry/manifest.go` diff adds `Deprecation *DeprecationNotice` with `json:"deprecation,omitempty"` to the `Manifest` struct. Pointer semantics ensure nil when absent from JSON.

### AC 3: Manifest with deprecation object parses correctly; manifest without it has nil `Deprecation`

**Mapping claim:** implemented
**Verdict:** CONFIRMED

Evidence: `internal/registry/manifest_test.go` adds `TestParseManifest_DeprecationPresent` (verifies all four fields after parse) and `TestParseManifest_DeprecationAbsent` (verifies nil). Additionally `TestParseManifest_DeprecationWithoutUpgradeURL` covers the optional field case.

### AC 4: `printWarning()` helper in `cmd/tsuku/helpers.go` writes to stderr, respects `--quiet`

**Mapping claim:** implemented
**Verdict:** CONFIRMED

Evidence: `cmd/tsuku/helpers.go` diff adds `printWarning(msg string)` that writes to stderr via `fmt.Fprintln(os.Stderr, msg)` and checks `quietFlag` before writing. Tests `TestPrintWarning_WritesToStderr` and `TestPrintWarning_QuietSuppresses` verify both branches.

### AC 5: Warning fires at most once per CLI invocation via `sync.Once`

**Mapping claim:** implemented
**Verdict:** CONFIRMED

Evidence: `cmd/tsuku/helpers.go` adds `var deprecationWarningOnce sync.Once` and `checkDeprecationWarning` wraps the warning in `deprecationWarningOnce.Do(...)`. Test `TestCheckDeprecationWarning_FiresOnce` calls it three times and verifies output appears exactly once.

### AC 6: Warning identifies registry by actual fetch URL (from `manifestURL()`), not hardcoded

**Mapping claim:** implemented
**Verdict:** CONFIRMED

Evidence: `manifestURL()` was exported to `ManifestURL()` in the diff. In `cmd/tsuku/update_registry.go`, the call is `checkDeprecationWarning(manifest, reg.ManifestURL())`, passing the actual URL. The `formatDeprecationWarning` function uses the `registryURL` parameter in the format string. No hardcoded URL anywhere in the warning path.

### AC 7: Warning format: `Warning: Registry at <url> reports: <message>`

**Mapping claim:** implemented
**Verdict:** CONFIRMED

Evidence: `formatDeprecationWarning` produces `fmt.Sprintf("Warning: Registry at %s reports: %s", registryURL, dep.Message)`. Tests verify this exact format (e.g., `TestCheckDeprecationWarning_DisplaysWarning` checks for `"Warning: Registry at https://tsuku.dev/recipes.json reports: Schema v2 coming soon."`).

### AC 8: When CLI version >= `min_cli_version`: shows "your CLI already supports the new format"

**Mapping claim:** implemented
**Verdict:** CONFIRMED

Evidence: In `formatDeprecationWarning`, when `version.CompareVersions(cliVersion, dep.MinCLIVersion) >= 0`, the message includes `"Your CLI (%s) already supports the new format. Run 'tsuku update-registry' after the migration."`. Tests `TestFormatDeprecationWarning_CLIMeetsMinVersion` (equal) and `TestFormatDeprecationWarning_CLIAboveMinVersion` (above) both verify.

### AC 9: When CLI version < `min_cli_version`: shows "upgrade to vX.Y"

**Mapping claim:** implemented
**Verdict:** CONFIRMED

Evidence: When `cmp < 0`, the code produces `"Update tsuku to %s or later"` with optional URL. Test `TestFormatDeprecationWarning_CLIBelowMinVersion` verifies with CLI v0.3.0 < min v0.5.0, checking for `"Update tsuku to v0.5.0 or later"` and the URL.

### AC 10: Dev builds (`dev-*`, `dev`, `unknown`) skip version comparison, treated as current

**Mapping claim:** implemented
**Verdict:** CONFIRMED

Evidence: `isDevBuild()` checks for `"dev"`, `"unknown"`, and `strings.HasPrefix(ver, "dev-")`. Called at the top of the version comparison branch in `formatDeprecationWarning`. Test `TestFormatDeprecationWarning_DevBuildSkipsComparison` verifies four dev version strings produce neither upgrade nor "already supports" text. `TestIsDevBuild` tests eight version strings with expected boolean results.

### AC 11: CLI never suggests downgrading (downgrade prevention rule)

**Mapping claim:** implemented
**Verdict:** CONFIRMED

Evidence: The comparison logic in `formatDeprecationWarning` uses `CompareVersions(cliVersion, dep.MinCLIVersion)`. When `cmp >= 0` (CLI is at or above min), it says "already supports" -- never "Update tsuku to" a lower version. Test `TestFormatDeprecationWarning_NeverSuggestsDowngrade` explicitly verifies CLI v1.0.0 with min v0.5.0 does NOT contain "Update tsuku to v0.5.0".

### AC 12: `upgrade_url` displayed as text only, never auto-opened

**Mapping claim:** implemented
**Verdict:** CONFIRMED

Evidence: The URL is appended as a string via `fmt.Fprintf(&b, ": %s", dep.UpgradeURL)`. No `exec.Command`, `os.Open`, `browser.Open`, or similar call exists anywhere in the diff. Test `TestFormatDeprecationWarning_NoUpgradeURL` verifies the URL is absent when the field is empty; `TestFormatDeprecationWarning_CLIBelowMinVersion` verifies it appears as text when present.

### AC 13: Tests for: deprecation parsing, nil when absent, warning display, quiet suppression, dev build handling, version comparison branches

**Mapping claim:** implemented
**Verdict:** CONFIRMED

Evidence in `cmd/tsuku/helpers_test.go` (new file, 394 lines):
- Deprecation parsing: covered in manifest_test.go (TestParseManifest_DeprecationPresent)
- Nil when absent: TestParseManifest_DeprecationAbsent, TestCheckDeprecationWarning_NoDeprecation
- Warning display: TestCheckDeprecationWarning_DisplaysWarning
- Quiet suppression: TestPrintWarning_QuietSuppresses, TestCheckDeprecationWarning_QuietSuppresses
- Dev build handling: TestFormatDeprecationWarning_DevBuildSkipsComparison, TestIsDevBuild
- Version comparison branches: TestFormatDeprecationWarning_CLIBelowMinVersion, _CLIMeetsMinVersion, _CLIAboveMinVersion

All six test categories explicitly named in the AC are covered.

## Missing ACs

None found. All 13 acceptance criteria from the PLAN doc Issue 2 have corresponding mapping entries with verifiable evidence in the diff.

## Phantom ACs

None found. All mapping entries correspond to ACs from the PLAN doc.

## Summary

All claims verified. No blocking or advisory findings.
