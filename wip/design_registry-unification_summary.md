# Design Summary: registry-unification

## Input Context (Phase 0)
**Source:** Freeform topic (post-implementation refactoring of distributed recipes)
**Problem:** Five registry types each have their own provider, cache, and resolution logic instead of sharing a single code path driven by a manifest.
**Constraints:** No git dependency, no users yet (breaking changes OK), HTTP-only transport, manifest lives inside registry directory

## Pre-Design Research
Extensive discussion captured in `wip/research/design_registry-unification_discussion.md`.
Six research agents investigated: code paths, design gaps, duplicated logic, local registry patterns, manifest-based unification, and cache unification feasibility.

## Key Decisions Already Made
1. Keep HTTP, no git dependency
2. Manifest inside registry directory (not external URL)
3. Manifest schema: `layout` (flat/grouped) + optional `index_url`
4. Central registry = baked-in manifest, same code path
5. Embedded recipes = baked-in registry with in-memory backing
6. recipes.json stays as website asset; CLI uses manifest's index_url
7. No users yet, breaking changes acceptable

## Open Questions
- Should $TSUKU_HOME/recipes/ (local recipe drop folder) survive?
- Security model for index_url (hostname allowlisting, HTTPS-only?)
- Cache unification specifics (unified RecipeCache interface parameters)

## Approaches Investigated (Phase 1)
- **Manifest-Driven Single Provider**: Replace all four providers with one RegistryProvider parameterized by manifest config and a BackingStore interface. Eliminates ~500 lines of duplication, all type assertions. Large scope (~15-20 files, 2-3 weeks).
- **Layered Storage Abstraction**: Separate storage (byte fetching) from registry (recipe resolution) with cache as composable middleware. Similar deduplication wins, cleaner separation of concerns. Large scope (~15-20 files, 3-4 weeks). Design tension around conditional HTTP requests in the cache layer.
- **Progressive Extraction**: Keep four provider types, extract shared helpers (satisfies, bucketing, cache interface). Low risk, incremental delivery. Small-medium scope (~8-10 files, 2-4 days). Doesn't eliminate GetFromSource switch or dual cache systems.

## Current Status
**Phase:** 1 - Approach Discovery
**Last Updated:** 2026-03-17
