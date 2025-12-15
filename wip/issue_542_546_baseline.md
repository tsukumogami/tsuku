# Issues #542 and #546 Baseline

## Environment
- Date: 2025-12-15
- Branch: feature/542-546-zig-m4
- Base commit: 4e93619

## Issues
- #542: feat(recipes): add zig recipe and validate cc wrapper
- #546: feat(recipes): add m4 recipe to validate compilation without system gcc

## Test Results
- All tests pass
- Build succeeds

## Pre-existing Issues
None - baseline is clean.

## Implementation Notes

The codebase already has:
- `ResolveZig()` function that looks for zig in `~/.tsuku/tools/zig-*`
- `SetupCCompilerEnv()` that creates CC/CXX wrapper scripts
- `buildAutotoolsEnv()` that uses zig if no system compiler available
- `configure_make` action for autotools builds

Tasks:
1. Create zig recipe (download from GitHub releases)
2. Create m4 recipe using configure_make
3. Add to CI test matrix
