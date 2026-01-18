# Exploration Summary: Embedded Recipe List Generation

## Problem (Phase 1)

The recipe registry separation design requires a validated list of embedded recipes before migration can proceed. The current 15-20 estimate is based on manual analysis; we need automated dependency extraction with transitive closure computation to ensure no action dependency is missed.

## Decision Drivers (Phase 1)

- **Accuracy**: Must capture ALL action dependencies including transitive ones
- **Maintainability**: CI validation to prevent embedded list drift
- **Visibility**: Human-readable documentation of why each recipe is embedded
- **Simplicity**: Script should be easy to audit and understand
- **Issue #644 resolved**: Composite actions now auto-aggregate primitive deps

## Key Research Findings

### Dependencies() Infrastructure (Issue #644 - CLOSED)
- `ActionDeps` struct defines InstallTime, Runtime, EvalTime, plus platform-specific variants
- `aggregatePrimitiveDeps()` in resolver.go automatically inherits deps from primitive actions
- 22 action files implement Dependencies()
- Transitive resolution with cycle detection exists in ResolveTransitive()

### Embedded Recipe Dependencies
- 171 total recipes in internal/recipe/recipes/
- 24 recipes declare explicit dependencies
- Max depth: 4 levels (spatialite chain)
- Key chains: libcurl (8 deps), cmake (2 deps), ruby (1 dep)

## Research Findings (Phase 2)

### Upstream Design Context
The parent design (DESIGN-recipe-registry-separation.md) specifies Stage 0 requirements:
- Build-time script extracts Dependencies() returns from action code
- Computes transitive closure by following recipe dependencies
- Accounts for known gaps (issue #644 - now resolved)
- Generates EMBEDDED_RECIPES.md at repo root
- CI validation via scripts/verify-embedded-recipes.sh

### Existing Infrastructure to Leverage
1. **resolver.go**: ResolveDependencies(), ResolveTransitive() with cycle detection
2. **action.go**: ActionDeps struct, Action registry with Dependencies() methods
3. **dependencies.go**: DetectShadowedDeps() for validation
4. **embedded.go**: EmbeddedRegistry for listing recipes
5. **generate-registry.py**: Existing recipe parser (Python) for reference

### Key Insight
Issue #644 is CLOSED - composite action aggregation is implemented. We don't need to manually track patchelf for homebrew; the infrastructure handles it. The main challenge is:
1. Extracting which actions require which tools (already in code)
2. Computing transitive recipe closure (recipes depend on recipes)
3. Generating human-readable documentation

## Options (Phase 3)

1. **Implementation Language**: Go program (1A) vs Go test (1B) vs shell+go run (1C) vs resolver wrapper (1D)
2. **Output Format**: Markdown only (2A) vs JSON+MD (2B) vs MD with regex (2C)
3. **CI Validation**: Regenerate-and-compare (3A) vs validate-only (3B)

## Decision (Phase 5)

**Problem:** The recipe registry separation design requires a validated embedded recipe list, but the current 15-20 estimate relies on manual analysis that could miss transitive dependencies.

**Decision:** Use a Go program that wraps the existing resolver infrastructure to extract action dependencies, compute transitive recipe closure, and generate EMBEDDED_RECIPES.md with CI validation via regenerate-and-compare.

**Rationale:** Leveraging resolver.go ensures consistency with runtime behavior and avoids duplicating extraction logic. Markdown-only output matches upstream design requirements and enables easy PR review. Regenerate-and-compare validation is simple and reliable.

## Current Status
**Phase:** 5 - Decision (complete)
**Last Updated:** 2026-01-18
