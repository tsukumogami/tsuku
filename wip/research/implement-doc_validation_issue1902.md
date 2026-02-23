# Validation Report: Issue #1902

**Date**: 2026-02-22
**Branch**: docs/sandbox-image-unification
**Scenarios tested**: scenario-10, scenario-11

---

## Scenario 10: no hardcoded image strings remain in CI workflow files

**Status**: PASSED

### Test executed

Ran grep with the pattern from the test plan against all seven target files:

```
grep -rEn '"(debian:bookworm-slim|fedora:(39|41)|archlinux:base|opensuse/(tumbleweed|leap:15)|alpine:3\.(19|21))"' \
  .github/workflows/recipe-validation-core.yml \
  .github/workflows/test-recipe.yml \
  .github/workflows/batch-generate.yml \
  .github/workflows/validate-golden-execution.yml \
  .github/workflows/platform-integration.yml \
  .github/workflows/release.yml \
  test/scripts/test-checksum-pinning.sh
```

**Result**: exit code 1 (no matches found).

### Additional verification

Also ran a broader pattern without quotes (`debian:bookworm|fedora:|archlinux:|opensuse/|alpine:3\.`) against all seven files. Zero matches.

### Positive check

Confirmed that all six workflow files now reference `container-images.json` via `jq` calls. Specific examples:

- `recipe-validation-core.yml`: `jq -r '.${{ matrix.family }}' container-images.json`
- `test-recipe.yml`: `jq -r --arg f "$f" '.[$f]' container-images.json`
- `batch-generate.yml`: `jq -r --arg f "$f" '.[$f]' container-images.json`
- `validate-golden-execution.yml`: `jq -r --arg f "$FAMILY" '.[$f] // empty' container-images.json`
- `platform-integration.yml`: `jq -r '.alpine' container-images.json`, `jq -r '.debian' container-images.json`, etc.
- `release.yml`: `jq -r '.alpine' container-images.json`

All image references are now derived from the centralized config file.

---

## Scenario 11: test-checksum-pinning.sh reads images from config file

**Status**: PASSED

### Tests executed

1. **jq reference check**: `grep -q 'jq' test/scripts/test-checksum-pinning.sh` -- exit code 0 (found)
2. **container-images.json reference check**: `grep -q 'container-images.json' test/scripts/test-checksum-pinning.sh` -- exit code 0 (found)
3. **No stale references check**: `grep -qE 'fedora:39|alpine:3\.19' test/scripts/test-checksum-pinning.sh` -- exit code 1 (not found, as expected)

### Additional verification

Reviewed the full script content (288 lines). Key findings:

- **jq availability validation** at line 24: The script checks `command -v jq` early (before any Docker or build operations) and exits with a clear error message including installation suggestions.
- **Config file reading** at lines 50-61: Uses `jq -r --arg f "$FAMILY" '.[$f] // empty' "$CONFIG_FILE"` to read the base image from `container-images.json`.
- **Error handling**: Provides clear error if config file is missing (line 52-54) or if family is not found in config (line 57-61), including listing valid families via `jq -r 'keys | join(", ")'`.
- **No hardcoded image strings**: The script does not contain any hardcoded `fedora:39`, `alpine:3.19`, or other distribution-specific image tags. The `BASE_IMAGE` is entirely derived from `container-images.json`.

---

## Summary

| Scenario | ID | Status |
|----------|-----|--------|
| No hardcoded image strings in CI files | scenario-10 | PASSED |
| test-checksum-pinning.sh reads from config | scenario-11 | PASSED |

Both scenarios confirm that issue #1902's CI migration work is complete: hardcoded image strings have been removed from all target files and replaced with `jq`-based reads from `container-images.json`.
