# Issue 1038 Introspection

## Context Reviewed

- **Design doc**: `docs/designs/DESIGN-recipe-registry-separation.md` (Status: Planned)
- **Sibling issues reviewed**:
  - #1037 (feat(cache): implement registry recipe cache policy) - CLOSED 2026-01-27
- **EMBEDDED_RECIPES.md**: `docs/EMBEDDED_RECIPES.md` exists with complete list and rationale
- **CONTRIBUTING.md**: Current state reviewed for baseline

## Prior Patterns Identified

### From Cache Policy Implementation (#1037 and related PRs)

The cache policy implementation (M48 milestone, 7 issues) completed successfully with:

1. **Cache metadata infrastructure** (PR #1165): JSON sidecar files for recipe metadata
2. **TTL-based expiration** (PR #1167): 24-hour default, `TSUKU_RECIPE_CACHE_TTL` env var
3. **LRU size management** (PR #1171): 500MB default, `TSUKU_CACHE_SIZE_LIMIT` env var
4. **Stale-if-error fallback** (PR #1173): Network failure handling with warnings
5. **Cache cleanup command** (PR #1176): `tsuku cache cleanup` subcommand
6. **Cache info enhancement** (PR #1178): Registry statistics in `tsuku cache info`

### Directory Structure Established

The recipe separation is now complete:

| Directory | Purpose | Count |
|-----------|---------|-------|
| `internal/recipe/recipes/` | Embedded (action dependencies) | 17 recipes |
| `recipes/` | Registry (user-installable) | ~150+ recipes |
| `testdata/recipes/` | Integration test recipes | 23 recipes |

### Nightly Validation Workflow

`nightly-registry-validation.yml` exists and runs at 2 AM UTC per design spec.

## Gap Analysis

### Minor Gaps

1. **CONTRIBUTING.md section outdated**: The "Adding Recipes" section still refers only to `internal/recipe/recipes/` as the target directory (line 233, 250, 397, 406). The three-directory structure needs documentation.

2. **Testdata recipes include more than originally planned**: Issue spec lists 6 test recipes (netlify-cli, ruff, cargo-audit, bundler, ack, gofumpt), but `testdata/recipes/` now contains 23 recipes. The documentation should reflect the actual scope.

3. **Cache commands available but not documented in troubleshooting**: The `tsuku cache cleanup` and `tsuku cache info` commands from #1037 are available but need to be referenced in the troubleshooting section.

### Moderate Gaps

None identified. The issue spec is comprehensive and aligns with what was implemented.

### Major Gaps

None identified. All dependencies are complete:
- #1036 (Update workflows for split recipe structure) - Done
- Registry Cache Policy milestone - Done (all 7 issues closed via PR #1184)

## Recommendation

**Proceed**

The issue spec is complete and accurate. Dependencies are satisfied. Minor gaps are details that can be incorporated during implementation without changing scope.

## Proposed Amendments

No amendments needed. The following details should be incorporated during implementation:

1. Update recipe paths in CONTRIBUTING.md to match actual three-directory structure
2. Reflect that `testdata/recipes/` contains more than the 6 originally listed recipes
3. Include references to new cache management commands (`tsuku cache cleanup`, `tsuku cache info`) in troubleshooting sections
4. Verify error message templates match what was actually implemented in #1037
