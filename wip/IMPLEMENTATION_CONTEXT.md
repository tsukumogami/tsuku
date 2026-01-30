## Issue #850: Consolidate sandbox dependencies designs

### Summary
Merge unique content from the superseded DESIGN-sandbox-implicit-dependencies.md into the parent DESIGN-sandbox-dependencies.md.

### Key Observations
- **Parent doc** focuses on eliminating code duplication between install_deps.go and install_sandbox.go
- **Superseded doc** covers the broader self-contained plans architecture with significant unique content:
  - Platform detection and LinuxFamily struct extension
  - Circular dependency detection with error messages
  - Version conflict resolution (first-encountered-wins strategy)
  - Dependency deduplication across plan generation and execution
  - Resource limits (max depth: 5, max total: 100)
  - User-facing error messages for various failure modes
  - Debugging mechanisms (verbose logging, diagnostic commands)
  - Migration path for pre-GA users
  - Plan tampering security considerations
- **No other docs** reference the superseded design (no cross-refs to update)
- Both docs share the same core decision: enable RecipeLoader in plan generation

### Approach
Incorporate the unique content from the superseded doc into the parent doc's existing sections where it fits naturally, without disrupting the parent's structure.
