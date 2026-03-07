# Validation Report: Issue #2 (Deprecation Signaling)

## Environment

- Platform: linux/amd64
- Go version: system default
- Branch: current working branch
- Test runner: `go test`

## Scenario 6: Deprecation notice parses when present

**Status**: PASSED

### Tests executed

- `TestParseManifest_DeprecationPresent`: Manifest with full deprecation object (sunset_date, min_cli_version, message, upgrade_url) parses into non-nil `Manifest.Deprecation` pointer. All four fields verified correct.
- `TestParseManifest_DeprecationAbsent`: Manifest without "deprecation" key results in `Manifest.Deprecation == nil`.
- `TestParseManifest_DeprecationWithoutUpgradeURL`: Manifest with deprecation but no upgrade_url field parses correctly with `UpgradeURL == ""`.

### Coverage of expected outcomes

| Expected behavior | Test covering it | Result |
|---|---|---|
| Deprecation with all fields parses to non-nil pointer | TestParseManifest_DeprecationPresent | PASS |
| All 4 fields (sunset_date, min_cli_version, message, upgrade_url) populated | TestParseManifest_DeprecationPresent | PASS |
| Missing deprecation key results in nil Deprecation | TestParseManifest_DeprecationAbsent | PASS |
| Optional upgrade_url handled when absent | TestParseManifest_DeprecationWithoutUpgradeURL | PASS |

---

## Scenario 7: Deprecation warning respects --quiet flag

**Status**: PASSED

### Tests executed

- `TestPrintWarning_WritesToStderr`: Confirms printWarning writes to stderr when quietFlag is false.
- `TestPrintWarning_QuietSuppresses`: Confirms printWarning produces no output when quietFlag is true.
- `TestCheckDeprecationWarning_NilManifest`: No output for nil manifest.
- `TestCheckDeprecationWarning_NoDeprecation`: No output when Deprecation is nil.
- `TestCheckDeprecationWarning_DisplaysWarning`: Warning displayed for manifest with deprecation.
- `TestCheckDeprecationWarning_FiresOnce`: Called 3 times, warning appears exactly 1 time (sync.Once).
- `TestCheckDeprecationWarning_QuietSuppresses`: No output in quiet mode even with deprecation present.
- `TestCheckDeprecationWarning_UpgradeNeeded`: Basic warning format validated for upgrade-needed case.

### Coverage of expected outcomes

| Expected behavior | Test covering it | Result |
|---|---|---|
| printWarning writes to stderr when not quiet | TestPrintWarning_WritesToStderr | PASS |
| printWarning suppressed when --quiet is set | TestPrintWarning_QuietSuppresses | PASS |
| sync.Once fires at most once per invocation | TestCheckDeprecationWarning_FiresOnce | PASS |
| Quiet mode suppresses deprecation warning | TestCheckDeprecationWarning_QuietSuppresses | PASS |

---

## Scenario 8: Deprecation warning shows correct version guidance

**Status**: PASSED

### Tests executed

- `TestFormatDeprecationWarning_CLIBelowMinVersion`: CLI v0.3.0 < min v0.5.0 produces "Update tsuku to v0.5.0 or later" with upgrade URL.
- `TestFormatDeprecationWarning_CLIMeetsMinVersion`: CLI v0.5.0 == min v0.5.0 produces "Your CLI (v0.5.0) already supports the new format."
- `TestFormatDeprecationWarning_CLIAboveMinVersion`: CLI v1.0.0 > min v0.5.0 produces "already supports", does NOT suggest upgrading.
- `TestFormatDeprecationWarning_DevBuildSkipsComparison`: All dev version strings ("dev", "dev-abc123", "dev-abc123-dirty", "unknown") skip comparison entirely -- no upgrade suggestion, no "already supports".
- `TestFormatDeprecationWarning_NoUpgradeURL`: When upgrade_url is empty, no URL appended to upgrade instruction.
- `TestFormatDeprecationWarning_NeverSuggestsDowngrade`: CLI v1.0.0 with min v0.5.0 does NOT say "Update tsuku to v0.5.0".
- `TestIsDevBuild`: Confirms correct classification of dev/unknown/normal version strings.

### Coverage of expected outcomes

| Expected behavior | Test covering it | Result |
|---|---|---|
| CLI >= min_cli_version: "already supports the new format" | CLIMeetsMinVersion, CLIAboveMinVersion | PASS |
| CLI < min_cli_version: "upgrade to vX.Y" | CLIBelowMinVersion | PASS |
| Dev builds (dev, dev-*, unknown) skip comparison | DevBuildSkipsComparison (4 variants) | PASS |
| Never suggests downgrading | NeverSuggestsDowngrade | PASS |
| Upgrade URL included when present | CLIBelowMinVersion | PASS |
| Upgrade URL omitted when empty | NoUpgradeURL | PASS |

---

## Summary

| Scenario | Status |
|---|---|
| scenario-6 | PASSED |
| scenario-7 | PASSED |
| scenario-8 | PASSED |

All 3 scenarios passed. The implementation fully covers the test plan expectations for Issue #2.
