# Issue 829 Summary

## What Was Implemented

Updated `regenerate-golden.sh` and `validate-golden.sh` scripts to support family-aware recipes. The scripts now query recipe metadata to determine which platform+family combinations need golden files, generate/validate files with the appropriate naming convention, and pass `--linux-family` when simulating family-specific plans.

## Changes Made

- `scripts/regenerate-golden.sh`: Refactored to parse `supported_platforms` as JSON objects, extract `linux_family` when present, pass `--linux-family` to eval command, and use family-aware file naming
- `scripts/validate-golden.sh`: Updated to build expected file list from metadata including family variants, pass `--linux-family` during plan generation, and report missing files as errors
- `.github/workflows/validate-golden-execution.yml`: Added matrix strategy for family-aware validation, updated to handle both naming conventions
- `docs/DESIGN-golden-family-support.md`: Minor clarifications to design doc
- `testdata/golden/plans/f/fzf/`: Regenerated golden files to include darwin platform files

## Key Decisions

- **Preserve backward compatibility**: Family-agnostic recipes continue to use `{version}-{os}-{arch}.json` naming while family-aware recipes use `{version}-{os}-{family}-{arch}.json`
- **Metadata-driven file list**: Scripts derive expected files from `tsuku info` metadata rather than filesystem patterns, ensuring completeness
- **CI matrix expansion**: Workflow uses matrix strategy to test family variants explicitly

## Trade-offs Accepted

- **Increased CI complexity**: Matrix strategy adds more jobs but ensures each platform/family combination is validated independently

## Test Coverage

- No new unit tests (shell scripts without test framework)
- Integration tested via regenerated fzf golden files
- CI workflow validates scripts work correctly

## Known Limitations

- linux-arm64 platforms continue to be excluded (no CI runner)
- Family-aware recipe testing requires a family-aware recipe to exist in the registry
