# Issue 1568 Summary

## What Was Implemented

Restored the ability to track which packages are blocked by missing dependencies in the per-recipe failure format used by the batch validation workflow.

## Changes Made

- `.github/workflows/batch-generate.yml`:
  - Added `--json` flag to all 4 validation jobs (linux-x86_64, linux-arm64, darwin-arm64, darwin-x86_64)
  - Added `BLOCKED_BY` variable to capture `missing_recipes` from JSON output when exit code is 8
  - Added `blocked_by` field to validation results JSON
  - Updated failure JSONL writing to include `blocked_by` for `missing_dep` category
  - Added exit code 8 mapping to `missing_dep` category (was falling through to `deterministic`)
  - Installed `jq` in validation containers for JSON parsing

- `internal/dashboard/dashboard.go`:
  - Added `BlockedBy []string` field to `FailureRecord` struct for per-recipe format
  - Updated `loadFailures()` to populate `details` and `blockers` maps from per-recipe format records with `blocked_by`

- `internal/dashboard/dashboard_test.go`:
  - Added `TestLoadFailures_perRecipeWithBlockedBy` test
  - Updated comment in existing test to clarify records without `blocked_by`

## Key Decisions

- **Capture JSON in validation step**: Chose to capture full JSON output during validation rather than post-processing. This is more reliable because `missing_recipes` comes from the structured error response.

- **Assume homebrew ecosystem for package ID**: Per-recipe format doesn't include ecosystem, so we generate package IDs as "homebrew:" + recipe. This matches the current workflow usage.

## Trade-offs Accepted

- **Added jq dependency to validation containers**: Installing jq adds ~1MB to each container run but provides reliable JSON parsing.

- **Ecosystem assumption**: Per-recipe format hardcodes "homebrew" for package ID generation. If other ecosystems use this format, dashboard code would need adjustment.

## Test Coverage

- New tests added: 1 (`TestLoadFailures_perRecipeWithBlockedBy`)
- Coverage maintained: dashboard tests all pass

## Known Limitations

- Per-recipe failure format doesn't include ecosystem, so package IDs are assumed to be "homebrew:"
- Dashboard blockers section aggregates from both legacy and per-recipe formats

## Future Improvements

- Add ecosystem field to per-recipe failure format for multi-ecosystem support
- Consider consolidating failure formats into a single schema
