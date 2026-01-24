# Issue 1034 Introspection

## Context Reviewed

- Design doc: `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/docs/designs/DESIGN-recipe-registry-separation.md`
- Sibling issues reviewed: #1033 (closed), #1071 (closed - recently merged PR #1073)
- Prior patterns identified:
  - Embedded recipes now use flat structure (`internal/recipe/recipes/*.toml`)
  - Registry recipes still use letter-based structure (`recipes/{letter}/*.toml`)
  - Validation scripts updated for new embedded flat structure

## Gap Analysis

### Major Gaps

**1. Issue spec assumes letter-based directory structure for embedded recipes**

The issue spec describes:
- "Create `testdata/golden/plans/embedded/{letter}/{recipe}/` mirroring the current structure"
- References `internal/recipe/recipes/{letter}/` paths which no longer exist

After #1071 (PR #1073 merged 2026-01-24), embedded recipes are now flat:
- Actual location: `internal/recipe/recipes/*.toml` (no letter subdirectories)
- The validation scripts have been updated to handle this

**Impact**: The directory structure for embedded golden files should be flat, not letter-based. The spec's requirement #1 is obsolete.

**2. Script references need reconsideration**

The issue spec mentions updating scripts with `--category` flag and "category detection from recipe location". The current scripts already implement recipe location detection, but they use:
- Embedded: `$REPO_ROOT/internal/recipe/recipes/$recipe.toml` (flat)
- Registry: `$REPO_ROOT/recipes/$first_letter/$recipe.toml` (letter-based)

The issue's proposed category detection logic is correct in principle but the path examples in the spec are outdated.

### Minor Gaps

**1. Golden file path pattern should be flat for embedded**

Current golden files use `testdata/golden/plans/{letter}/{recipe}/` for all recipes. The issue correctly identifies splitting into `embedded/` and keeping `{letter}/{recipe}/` for registry, but:
- Embedded golden path should be `testdata/golden/plans/embedded/{recipe}/` (flat, no letter)
- Registry golden path stays `testdata/golden/plans/{letter}/{recipe}/` (letter-based)

This aligns with the new embedded recipe structure.

**2. validate-golden.sh already has some embedded-aware logic**

Looking at `validate-all-golden.sh` lines 62-68, the script already checks for flat embedded recipes:
```bash
EMBEDDED_RECIPE="$REPO_ROOT/internal/recipe/recipes/$recipe.toml"
```

The scripts can be extended without conflict, but the spec should reference this existing pattern.

### Moderate Gaps

None identified. The changes needed are structural (directory layout) not scope changes.

## Recommendation

**Amend**

The issue should be amended to reflect the flattened embedded recipe structure. The core intent (separate golden files by category) remains valid, but the specific directory structure needs updating.

## Proposed Amendments

1. **Update directory structure requirement**:
   - OLD: "Create `testdata/golden/plans/embedded/{letter}/{recipe}/`"
   - NEW: "Create `testdata/golden/plans/embedded/{recipe}/` (flat, no letter subdirectory)"

2. **Update file migration task**:
   - Move golden files for embedded recipes to `testdata/golden/plans/embedded/{recipe}/`
   - Embedded recipes (17 total): cmake, gcc-libs, go, libyaml, make, meson, ninja, nodejs, openssl, patchelf, perl, pkg-config, python-standalone, ruby, rust, zig, zlib

3. **Update script paths**:
   - `GOLDEN_BASE` for embedded recipes: `testdata/golden/plans/embedded/$recipe/`
   - `GOLDEN_BASE` for registry recipes: `testdata/golden/plans/$first_letter/$recipe/`
   - Category detection: if recipe exists in `internal/recipe/recipes/*.toml` (flat) -> embedded; if in `recipes/{letter}/*.toml` -> registry

4. **Update validation tasks**:
   - Task: "Update `validate-golden.sh` with category detection logic" should note the flat vs letter-based distinction
   - Task: "Update `validate-all-golden.sh` to support category-scoped validation" should handle the different directory structures per category

5. **Update notes section**:
   - Remove reference to `EMBEDDED_RECIPES.md` providing the "letter" structure
   - Note that embedded recipes are now flat (per #1071)

## Summary

The issue spec was written before #1071 flattened the embedded recipes directory structure. The core goal (split golden files into embedded/registry directories) is still valid, but the directory layout for embedded golden files should now be flat (`embedded/{recipe}/`) rather than letter-based (`embedded/{letter}/{recipe}/`). This is a structural change to the spec, not a scope change - all tasks remain relevant with updated paths.
