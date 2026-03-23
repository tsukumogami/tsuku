# Crystallize: shell-integration-auto-install

## Artifact Type: Design Document Update + New Issue

### What

Update `docs/designs/DESIGN-shell-integration-building-blocks.md` to:
1. Add Block 6 (Project-Aware Exec Wrapper) to the architecture
2. Add the dependency arrow: Block 6 depends on Block 3 (Auto-Install) + Block 4 (Project Config)
3. Clarify that Track B is independent of Track A's binary index
4. Add the convergence flow diagram showing how Block 6 bridges the tracks
5. Add a new milestone issue: "docs: design project-aware exec wrapper"

### Why Not a New Design Doc

The parent design is the right home for this. Block 6 is an architectural addition
to an existing "Planned" design -- not a standalone feature. Adding it here keeps
all shell integration building blocks in one document and one milestone.

### Why Not a PRD

Requirements are clear. The "what to build" question is answered: `tsuku exec`
command + optional shim generation. The design question is "how" -- appropriate
for a design doc update, not a PRD.

### Decisions Made

- Block 6 is the right fix (not enhancing Block 3 alone, which requires command changes)
- Shims are optional but `tsuku exec` is required for the convergence to work
- koto/shirabe don't need to change; tsuku delivers the behavior
- Binary index (issue #1677) is NOT a prerequisite for project config (issues #1680/#1681)

### Handoff

Produce:
1. Updated DESIGN-shell-integration-building-blocks.md with Block 6 added
2. New GitHub issue for "docs: design project-aware exec wrapper (Block 6)"
3. Updated implementation issues table + dependency graph in the design doc
