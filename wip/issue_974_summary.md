# Issue 974 Summary

## What Was Implemented

Extended the exclusion validation system to also validate `code-validation-exclusions.json` for stale (closed) issue references, preventing exclusions that reference resolved issues from slipping through CI.

## Changes Made

- `scripts/validate-golden-exclusions.sh`: Added `--file <path>` argument to make the exclusion file configurable (defaults to `exclusions.json` for backward compatibility). Also improved handling of exclusions without `platform` field.
- `.github/workflows/validate-golden-code.yml`: Added validation step for `code-validation-exclusions.json` and included the file in workflow path triggers.
- `testdata/golden/code-validation-exclusions.json`: Removed 12 stale exclusions referencing closed issue #953, leaving 5 valid exclusions for open issue #961.

## Key Decisions

- **Parameterize existing script**: Chose to add `--file` argument rather than create a new script, to keep validation logic centralized and consistent.
- **Default to exclusions.json**: Maintained backward compatibility so existing workflow invocations continue to work.
- **Handle missing platform gracefully**: Code-validation exclusions don't have platform fields; script now displays "all platforms" for these.

## Trade-offs Accepted

- **Two validation invocations**: The validate-golden-code workflow now runs the validation script twice (once for each file). This doubles API calls but is acceptable given low exclusion counts.

## Test Coverage

- No new automated tests added (script changes are bash, not Go)
- Manual testing verified all argument combinations work correctly

## Known Limitations

- The validation is per-workflow: `validate-golden-recipes.yml` validates `exclusions.json`, `validate-golden-code.yml` validates `code-validation-exclusions.json`. A future improvement could consolidate validation.

## Future Improvements

- Could add a CI job that validates all exclusion files at once to catch gaps early
