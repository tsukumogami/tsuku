# Issue 1033 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-recipe-registry-separation.md`
- Sibling issues reviewed: #1032 (closed - create EMBEDDED_RECIPES.md)
- Prior patterns identified:
  - M44 milestone (Embedded Recipe List Validation) completed with 6 issues
  - `EMBEDDED_RECIPES.md` created at `docs/EMBEDDED_RECIPES.md`
  - `--require-embedded` flag implemented in loader and CLI
  - `embedded-validation-exclusions.json` created at repo root
  - CI workflow `validate-embedded-deps.yml` added

## Gap Analysis

### Minor Gaps

1. **EMBEDDED_RECIPES.md location**: Issue body references `EMBEDDED_RECIPES.md` at repo root, but it was created at `docs/EMBEDDED_RECIPES.md`. The issue's validation script checks for `EMBEDDED_RECIPES.md` at root - this path needs updating in implementation.

2. **Embedded recipe list source**: Issue body says "Read `EMBEDDED_RECIPES.md` to get the list of embedded recipes." The actual file at `docs/EMBEDDED_RECIPES.md` uses a table format with columns for Recipe, Required By, and Rationale. Implementation should parse the Recipe column from the tables.

3. **Registry URL update detail**: Issue says to change `DefaultRegistryURL` from:
   ```go
   DefaultRegistryURL = "https://raw.githubusercontent.com/tsukumogami/tsuku/main/internal/recipe"
   ```
   to:
   ```go
   DefaultRegistryURL = "https://raw.githubusercontent.com/tsukumogami/tsuku/main"
   ```
   Current code in `internal/registry/registry.go:21` matches the "from" value. The `recipeURL` function at line 69 appends `/recipes/{letter}/{name}.toml`, so this update is correct.

4. **recipes/ directory structure**: Issue mentions creating `recipes/` with letter subdirectories. Currently `recipes/` exists at repo root but only contains `CLAUDE.local.md` - no recipe subdirectories yet. This is expected since migration hasn't started.

### Moderate Gaps

None identified. The issue spec is complete and aligns with what M44 implemented.

### Major Gaps

None identified. The prerequisite (M44 - Embedded Recipe List Validation) is fully complete:
- `docs/EMBEDDED_RECIPES.md` exists with 16 embedded recipes documented
- `--require-embedded` flag works in loader and CLI
- CI validation workflow is in place
- Exclusions file exists for tracking gaps

## Recommendation

**Proceed**

The issue spec is complete and ready for implementation. M44 prerequisites are finished. The minor gaps are implementation details that can be resolved during development:

1. Read `docs/EMBEDDED_RECIPES.md` (not root-level path)
2. Parse recipe names from the markdown table format
3. Update validation script in PR to use correct path

## Proposed Amendments

None required. The minor gaps are easily addressable during implementation without changing the issue scope or intent.
