# Issue 973 Summary

## What Was Implemented

Consolidated two binary verification scripts (`verify-relocation.sh` and `verify-no-system-deps.sh`) into a single `verify-binary.sh` script that performs both relocation and dependency checks in one binary iteration pass.

## Changes Made

- `test/scripts/verify-binary.sh`: Created combined script (244 lines) that:
  - Iterates binaries once instead of twice
  - Performs both RPATH/relocation checks and dependency resolution checks
  - Uses unified error/warning labeling: `(relocation)` and `(dependency)`
  - Preserves all existing checks from both original scripts

- `test/scripts/verify-relocation.sh`: Removed (151 lines)

- `test/scripts/verify-no-system-deps.sh`: Removed (206 lines)

- `.github/workflows/build-essentials.yml`: Updated 6 call sites:
  - Lines 80-81: Direct matrix call → single `verify-binary.sh`
  - Lines 238-239: Zig verification → single `verify-binary.sh`
  - macOS Apple Silicon: Changed from `verify_relocation`/`verify_no_system_deps` params to single `verify_binary` param
  - macOS Intel: Same parameter consolidation

## Key Decisions

- **Include `.a` files in scan**: The relocation script included static libraries while the deps script excluded them. Chose to include them (superset) since static libraries may have embedded paths.

- **Single function parameter**: Changed macOS workflow functions from two boolean params (`verify_relocation`, `verify_no_system_deps`) to single boolean (`verify_binary`) for cleaner API.

- **Unified output format**: Added `(relocation)` and `(dependency)` labels to messages for clarity about which check produced the warning/error.

## Trade-offs Accepted

- Script is slightly longer (~244 lines vs ~150+200) due to combining checks, but this is offset by eliminated duplication in the binary iteration loop.

## Test Coverage

- No Go tests affected (this is a shell script refactor)
- Workflow changes will be validated by CI

## Known Limitations

- Cannot be tested locally without installing a tool first (requires `$TSUKU_HOME/tools/<tool>-*` directory)
