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

## Current Status
**Phase:** 0 - Setup (Freeform)
**Last Updated:** 2026-03-17
