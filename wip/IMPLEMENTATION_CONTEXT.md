# Implementation Context: Issue #846

## Summary
Consolidate 4 superseded plan-based installation designs into the canonical parent document.

## Key Understanding
- Parent: `docs/designs/current/DESIGN-deterministic-resolution.md`
- 4 superseded designs in `docs/designs/archive/` need content extracted
- Goal is knowledge preservation, not rewriting - unique content gets folded in
- Parent must remain coherent after consolidation
- Cross-references in other docs must be updated

## Dependencies
- #843 (design doc reorganization) - CLOSED, unblocked

## Files to Review
- `docs/designs/current/DESIGN-deterministic-resolution.md` (parent, modify)
- `docs/designs/archive/DESIGN-decomposable-actions.md` (read only)
- `docs/designs/archive/DESIGN-deterministic-execution.md` (read only)
- `docs/designs/archive/DESIGN-installation-plans-eval.md` (read only)
- `docs/designs/archive/DESIGN-plan-based-installation.md` (read only)
- Any other docs with cross-references to superseded designs
