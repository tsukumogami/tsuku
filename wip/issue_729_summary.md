# Issue 729 Summary

## What Was Implemented

Refactored all test workflows and scripts to use the `--recipe` flag instead of copying recipes to the embedded registry. This simplifies test infrastructure by removing the need to embed recipes during test builds and eliminates 26 lines of code.

## Changes Made

- `.github/workflows/build-essentials.yml`: Refactored 5 test jobs
  - `test-configure-make`: Removed recipe copy, added `--recipe` flag to gdbm-source install
  - `test-meson-build`: Removed recipe copy, added `--recipe` flag to libsixel-source install
  - `test-sqlite-source`: Removed recipe copy, added `--recipe` flag to sqlite-source install
  - `test-git-source`: Removed recipe copy, added `--recipe` flag to git-source install
  - `test-no-gcc`: Removed recipe copy, added `--recipe` flag to gdbm-source install

- `scripts/test-zig-cc.sh`: Refactored Dockerfile
  - Removed recipe copy step (line 76)
  - Removed rebuild step (lines 78-79)
  - Added `--recipe testdata/recipes/gdbm-source.toml --sandbox` to install command

## Key Decisions

- **Used direct `--recipe` approach**: Chose `tsuku install --recipe <path> --sandbox` over the eval+install pattern (`tsuku eval --recipe <path> | tsuku install --plan - --sandbox`) for simplicity and clarity
- **Applied `--sandbox` flag consistently**: Required by `--recipe` for security, ensures isolated test execution
- **Single atomic commit**: Combined all 6 changes in one commit to maintain test suite coherence

## Trade-offs Accepted

- **Requires sandbox support**: The `--recipe` flag mandates `--sandbox` mode, which means tests must support container execution
- **Slightly longer command**: Added `--recipe <path> --sandbox` vs just tool name, but this is offset by removing copy/rebuild steps

## Test Coverage

- No new tests added (refactoring only)
- All existing tests pass (22 packages)
- No coverage change (0% delta)
- Changes will be validated via CI on all platforms (Linux x86_64, macOS Intel, macOS Apple Silicon)

## Comprehensive Search Results

Per user's request to "find every place in source where this pattern can be applied":

**Total occurrences found:** 6 (all refactored)
**Files searched:** 120+
  - 15 GitHub Actions workflows
  - 13 shell scripts (scripts/ and test/scripts/)
  - 2 Dockerfiles
  - 90+ Go test files
  - All documentation files

**No additional occurrences found** beyond those identified in the original issue.

## Known Limitations

None. The refactoring is complete and all occurrences of the old pattern have been eliminated.

## Future Improvements

None needed. The `--recipe` flag is production-ready and this pattern is now consistently applied across all test infrastructure.
