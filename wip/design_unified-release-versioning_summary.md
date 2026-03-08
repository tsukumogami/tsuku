# Design Summary: unified-release-versioning

## Input Context (Phase 0)
**Source:** /explore handoff
**Problem:** Tsuku's three binaries (CLI, dltest, llm) lack unified versioning. They release independently with inconsistent naming, different version resolution, and incomplete version enforcement. Only dltest has compile-time pinning; llm accepts any version.
**Constraints:**
- GPU builds (CUDA/Vulkan) add ~10 min but parallelize fully
- No consumers of separate llm release tags exist (zero migration risk)
- Backend suffix asymmetry (macOS implicit Metal, Linux explicit cuda/vulkan) is intentional
- Proto evolution for gRPC handshake is safe in proto3 but requires coordinated release

## Approaches Investigated (Phase 1)
- **Incremental Migration**: Phased rollout (recipes, pinning, pipeline, naming). Low risk per change, but phases have ordering dependencies and longer calendar time.
- **Full Consolidation**: All dimensions addressed at once. Atomic transition, single coherent design, but high review burden and rollback cost.
- **Minimal: Pinning + Recipe Fix**: Smallest change surface, solves core safety problem. But leaves two workflows with release race condition, and defers naming/pipeline cleanup.

## Selected Approach (Phase 2)
Incremental Migration chosen over Full Consolidation and Minimal. Each dimension ships as an independent PR with progressive validation. Phases have ordering dependencies but each is independently deployable and reversible.

## Current Status
**Phase:** 2 - Present Approaches
**Last Updated:** 2026-03-08
