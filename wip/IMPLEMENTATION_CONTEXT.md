---
summary:
  constraints:
    - Registry golden files stay in current location (R2 migration is future work in #1039)
    - Must maintain backwards compatibility for registry recipe validation
    - Workflow trigger changes are out of scope (handled in #1036)
    - Directory structure must mirror recipe letter-based layout
  integration_points:
    - scripts/validate-golden.sh - needs category detection and path lookup
    - scripts/validate-all-golden.sh - needs --category flag for scoped validation
    - scripts/regenerate-golden.sh - needs category-aware output paths
    - scripts/regenerate-all-golden.sh - needs --category flag for scoped regeneration
    - testdata/golden/plans/ directory - new embedded/ subdirectory
  risks:
    - Breaking existing golden file validation during migration
    - Incorrect category auto-detection from recipe path
    - Edge cases with testdata/recipes (already handled differently)
    - CI workflows may need adjustment (but that's #1036's scope)
  approach_notes: |
    This is Stage 2 of the recipe registry separation design.

    1. Create testdata/golden/plans/embedded/{letter}/{recipe}/ structure
    2. Move golden files for 17 embedded recipes to new location
    3. Update scripts to detect category from recipe location:
       - internal/recipe/recipes/ -> embedded
       - recipes/ -> registry
    4. Add --category flag to scripts for explicit override
    5. Verify all existing tests pass with new structure
---

# Implementation Context: Issue #1034

**Source**: docs/designs/DESIGN-recipe-registry-separation.md (Stage 2)

## Design Excerpt

This issue implements Stage 2: Golden File Reorganization from the recipe registry separation design.

**Goal**: Separate golden files by recipe category so that `validate-golden-code.yml` can scope validation to just embedded recipes.

**Directory Structure After**:
```
testdata/golden/plans/
├── embedded/           # Golden files for embedded recipes (17 recipes)
│   ├── c/cmake/
│   ├── g/go/
│   ├── m/make/
│   └── ...
├── a/                  # Registry recipes remain here for now
├── b/
└── ...
```

**Key Decision**: Category is determined by recipe location:
- `internal/recipe/recipes/` = embedded
- `recipes/` = registry

**Scripts to Update**:
1. `validate-golden.sh` - look up golden files in correct subdirectory
2. `validate-all-golden.sh` - support `--category embedded|registry` flag
3. `regenerate-golden.sh` - output to correct subdirectory
4. `regenerate-all-golden.sh` - support `--category` flag

**Out of Scope**:
- Workflow trigger changes (#1036)
- R2 storage for registry golden files (#1039)
- Nightly validation workflow (#1036)
