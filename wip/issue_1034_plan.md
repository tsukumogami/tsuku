# Issue 1034 Implementation Plan

## Summary

Reorganize golden files into embedded/ (flat) and registry/ (letter-based) directories, updating 4 shell scripts to detect recipe category from path and support explicit `--category` flags.

## Approach

Use flat structure for embedded golden files (`testdata/golden/plans/embedded/{recipe}/`) and keep letter-based structure for registry (`testdata/golden/plans/{letter}/{recipe}/`). Scripts detect category automatically from recipe path but allow explicit override via `--category` flag.

### Alternatives Considered

- **Keep letter-based structure for both**: Rejected because embedded recipes are now flat (per PR #1073), so golden file structure should mirror recipe structure for consistency.
- **Create separate validation scripts for each category**: Rejected as it duplicates logic. Adding a `--category` flag to existing scripts is cleaner.
- **Detect by recipe name instead of path**: Rejected because path-based detection is more reliable and handles testdata recipes correctly.

## Files to Modify

- `scripts/validate-golden.sh` - Add `get_golden_dir()` function for category-aware path lookup and `--category` flag
- `scripts/validate-all-golden.sh` - Add `--category embedded|registry` flag to scope iteration, update iteration logic
- `scripts/regenerate-golden.sh` - Add `get_golden_dir()` function for category-aware output paths and `--category` flag
- `scripts/regenerate-all-golden.sh` - Add `--category` flag to scope regeneration
- `docs/designs/DESIGN-recipe-registry-separation.md` - Mark #1034 as completed in implementation issues table

## Files to Create

- `testdata/golden/plans/embedded/` - New directory for embedded recipe golden files

## File Moves (14 embedded recipes with existing golden files)

From `testdata/golden/plans/{letter}/{recipe}/` to `testdata/golden/plans/embedded/{recipe}/`:
1. `c/cmake/` -> `embedded/cmake/`
2. `g/gcc-libs/` -> `embedded/gcc-libs/`
3. `g/go/` -> `embedded/go/`
4. `l/libyaml/` -> `embedded/libyaml/`
5. `m/make/` -> `embedded/make/`
6. `m/meson/` -> `embedded/meson/`
7. `n/ninja/` -> `embedded/ninja/`
8. `o/openssl/` -> `embedded/openssl/`
9. `p/patchelf/` -> `embedded/patchelf/`
10. `p/perl/` -> `embedded/perl/`
11. `p/pkg-config/` -> `embedded/pkg-config/`
12. `p/python-standalone/` -> `embedded/python-standalone/`
13. `r/ruby/` -> `embedded/ruby/`
14. `z/zig/` -> `embedded/zig/`

Note: 3 embedded recipes (nodejs, rust, zlib) don't have golden files yet (rust and zlib are excluded per issue #931).

## Implementation Steps

- [x] Create `testdata/golden/plans/embedded/` directory
- [x] Move 14 embedded recipe golden directories to flat structure under `embedded/`
- [x] Update `validate-golden.sh`:
  - Add `--category embedded|registry` flag
  - Add `detect_category()` function (checks `internal/recipe/recipes/` for embedded, `recipes/` for registry)
  - Add `get_golden_dir()` function that returns correct path based on category
  - Update `GOLDEN_DIR` assignment to use `get_golden_dir()`
- [x] Update `validate-all-golden.sh`:
  - Add `--category embedded|registry` flag
  - When `--category embedded`: iterate over `testdata/golden/plans/embedded/*/`
  - When `--category registry`: iterate over `testdata/golden/plans/{letter}/*/` (skip `embedded/`)
  - When no category: iterate over both (current behavior, updated for new structure)
- [x] Update `regenerate-golden.sh`:
  - Add `--category embedded|registry` flag
  - Add same `detect_category()` and `get_golden_dir()` functions
  - Update `GOLDEN_DIR` assignment to use `get_golden_dir()`
- [x] Update `regenerate-all-golden.sh`:
  - Add `--category` flag
  - When `--category embedded`: iterate over embedded recipes from `internal/recipe/recipes/`
  - When `--category registry`: iterate over existing golden dirs in letter structure
  - When no category: iterate over both
- [x] Verify all golden file tests pass with new structure
- [x] Update `docs/designs/DESIGN-recipe-registry-separation.md`:
  - Mark #1034 as completed in the implementation issues table
  - Also mark any other completed issues (#1033, #1071 if listed)

## Testing Strategy

- **Unit tests**: No Go code changes, so no new unit tests needed
- **Integration tests**:
  - Run `./scripts/validate-all-golden.sh` (full validation)
  - Run `./scripts/validate-all-golden.sh --category embedded` (embedded only)
  - Run `./scripts/validate-all-golden.sh --category registry` (registry only)
- **Manual verification**:
  - Verify `validate-golden.sh go` works (embedded recipe)
  - Verify `validate-golden.sh fzf` works (registry recipe)
  - Verify `validate-golden.sh build-tools-system --recipe testdata/recipes/build-tools-system.toml` works (testdata recipe)

## Risks and Mitigations

- **Breaking existing validation during migration**: Mitigate by keeping both iteration patterns in validate-all-golden.sh until all moves complete. Run full validation after each step.
- **Incorrect category detection for custom recipes**: The `--recipe` flag should preserve current behavior (look up in original letter-based structure). Add explicit test case.
- **Edge case: testdata/recipes entries**: Already handled specially in current scripts. Keep that logic unchanged.
- **Stale exclusions files**: Both exclusions files reference recipes by name only, not path. No changes needed.

## Success Criteria

- [ ] `testdata/golden/plans/embedded/` contains 14 recipe directories with flat structure
- [ ] `testdata/golden/plans/{letter}/` contains only registry recipes (no embedded recipes)
- [ ] `./scripts/validate-all-golden.sh` passes (validates all recipes)
- [ ] `./scripts/validate-all-golden.sh --category embedded` validates only embedded recipes
- [ ] `./scripts/validate-all-golden.sh --category registry` validates only registry recipes
- [ ] `./scripts/validate-golden.sh go` finds golden files in embedded path
- [ ] `./scripts/validate-golden.sh fzf` finds golden files in registry path
- [ ] `./scripts/regenerate-golden.sh go` outputs to embedded path
- [ ] `./scripts/regenerate-golden.sh fzf` outputs to registry path

## Open Questions

None - requirements are clear from the issue and introspection session.
