# Scrutiny Review: Justification Focus -- Issue 2

**Issue:** feat(registry): add deprecation notice parsing and warning display
**Focus:** justification (evaluate quality of deviation explanations)
**Reviewer perspective:** Are there hidden deviations masked as "implemented"? Are any AC items technically satisfied but substantively thin?

## Requirements Mapping (Untrusted Input)

--- BEGIN UNTRUSTED REQUIREMENTS MAPPING ---
All 13 AC items reported as "implemented" with no deviations.
--- END UNTRUSTED REQUIREMENTS MAPPING ---

## Analysis

### Deviation Assessment

The requirements mapping reports zero deviations. All 13 AC items are marked "implemented." The justification focus specifically evaluates the quality of deviation explanations -- when there are no deviations claimed, the relevant question becomes: are any "implemented" claims actually disguised deviations?

### Verification of "Implemented" Claims Against Diff

I examined the diff for all 5 changed files against each AC to determine whether any "implemented" claim conceals a shortcut or partial implementation.

#### AC: DeprecationNotice struct
**Claim:** Implemented
**Verdict:** Confirmed. `DeprecationNotice` struct in `internal/registry/manifest.go` with all four fields (`SunsetDate`, `MinCLIVersion`, `Message`, `UpgradeURL`). JSON tags match the design doc's schema exactly, including `omitempty` on `upgrade_url`.

#### AC: Deprecation pointer field
**Claim:** Implemented
**Verdict:** Confirmed. `Deprecation *DeprecationNotice` pointer field on `Manifest` struct with `json:"deprecation,omitempty"`.

#### AC: Parsing present/absent
**Claim:** Implemented
**Verdict:** Confirmed. `TestParseManifest_DeprecationPresent` verifies all four fields populate correctly. `TestParseManifest_DeprecationAbsent` verifies nil when the deprecation object is missing. `TestParseManifest_DeprecationWithoutUpgradeURL` covers the optional field case. Parsing goes through `parseManifest()` which is already the single validation chokepoint per the design.

#### AC: printWarning()
**Claim:** Implemented
**Verdict:** Confirmed. `printWarning()` in `cmd/tsuku/helpers.go` writes to stderr via `fmt.Fprintln(os.Stderr, msg)` and respects `quietFlag`. Tests confirm both behaviors.

#### AC: sync.Once
**Claim:** Implemented
**Verdict:** Confirmed. `deprecationWarningOnce sync.Once` variable gates `checkDeprecationWarning()`. `TestCheckDeprecationWarning_FiresOnce` calls the function three times and verifies the warning appears exactly once.

#### AC: Registry URL
**Claim:** Implemented
**Verdict:** Confirmed. `checkDeprecationWarning` accepts `registryURL string` parameter. The call site in `update_registry.go` passes `reg.ManifestURL()` (the actual fetch URL). `manifestURL()` was promoted to exported `ManifestURL()` to enable this. Warning format uses the passed URL, not a hardcoded value.

#### AC: Warning format
**Claim:** Implemented
**Verdict:** Confirmed. Format is `Warning: Registry at <url> reports: <message>`, matching the design doc's specified format exactly.

#### AC: CLI >= min_cli_version
**Claim:** Implemented
**Verdict:** Confirmed. `formatDeprecationWarning` uses `version.CompareVersions(cliVersion, dep.MinCLIVersion)` and when `cmp >= 0`, outputs "Your CLI (<version>) already supports the new format. Run 'tsuku update-registry' after the migration." `TestFormatDeprecationWarning_CLIMeetsMinVersion` and `TestFormatDeprecationWarning_CLIAboveMinVersion` both verify this branch.

#### AC: CLI < min_cli_version
**Claim:** Implemented
**Verdict:** Confirmed. When `cmp < 0`, outputs "Update tsuku to <min_version> or later" with optional URL appended. `TestFormatDeprecationWarning_CLIBelowMinVersion` verifies this path including the upgrade URL.

#### AC: Dev builds
**Claim:** Implemented
**Verdict:** Confirmed. `isDevBuild()` checks for "dev", "dev-*" prefix, and "unknown". When true, the version comparison is skipped entirely (no upgrade or "already supports" message). `TestFormatDeprecationWarning_DevBuildSkipsComparison` covers four dev version strings. `TestIsDevBuild` covers 8 cases including negative cases.

#### AC: Downgrade prevention
**Claim:** Implemented
**Verdict:** Confirmed. The version comparison logic structurally prevents downgrade suggestions: when CLI >= min_version, it says "already supports" instead of suggesting an upgrade. `TestFormatDeprecationWarning_NeverSuggestsDowngrade` explicitly tests this with CLI v1.0.0 and min v0.5.0.

#### AC: upgrade_url text only
**Claim:** Implemented
**Verdict:** Confirmed. `UpgradeURL` is only referenced in `fmt.Fprintf(&b, ": %s", dep.UpgradeURL)` -- pure string formatting, never passed to `os.Exec`, `http.Get`, or any browser-opening function.

#### AC: Tests
**Claim:** Implemented
**Verdict:** Confirmed. `helpers_test.go` (394 lines, new file) covers: deprecation parsing, nil when absent, warning display, quiet suppression, dev build handling, version comparison branches (above, at, below min_version), downgrade prevention, sync.Once dedup, isDevBuild edge cases. `manifest_test.go` adds 3 tests for parsing (present, absent, without upgrade URL).

## Findings

### Blocking Findings

None.

### Advisory Findings

None. All 13 AC items are genuinely implemented with code that matches the claimed behavior. The diff is consistent across all files and there are no signs of selective effort, disguised shortcuts, or token implementations. No deviations were claimed and none were found.

## Overall Assessment

The requirements mapping is accurate. All items claimed as "implemented" are confirmed by the diff. There are no deviations to evaluate for justification quality, and no hidden deviations were detected. The implementation is proportionate -- the changes span 5 files with substantial test coverage (394 lines of new test code in `helpers_test.go` plus 77 lines added to `manifest_test.go`), which is appropriate for 13 AC items.
