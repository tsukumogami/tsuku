# Design Summary: distributed-recipes

## Input Context (Phase 0)
**Source PRD:** docs/prds/PRD-distributed-recipes.md
**Problem (implementation framing):** The Loader's hardcoded priority chain, source-unaware state tracking, and central-registry-coupled caching prevent clean addition of distributed recipe sources. A RecipeProvider abstraction, source-tracked state, and multi-origin caching are needed.

## Approaches Investigated (Phase 1)
- **RecipeProvider Interface**: Extract a Go interface all sources implement. Eliminates duplicated chain logic, aligns with version provider pattern. Medium complexity.
- **Extended Registry**: Grow Registry to a list of instances. Minimal conceptual change but doesn't unify local/embedded, assumes bucketed directory layout.
- **URL Resolver**: Resolve owner/repo to GitHub raw URL. Lowest ceremony but accumulates tech debt, no abstraction, fragile URL dependency.

## Selected Approach (Phase 2)
RecipeProvider Interface. It's the only approach that delivers the PRD's unified abstraction goal. The pattern already exists implicitly in the codebase and explicitly in the version provider system. It reduces Loader complexity rather than increasing it, and each provider controls its own URL construction and caching, avoiding layout assumption conflicts.

## Current Status
**Phase:** 2 - Present Approaches
**Last Updated:** 2026-03-15
