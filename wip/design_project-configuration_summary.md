# Design Summary: project-configuration

## Input Context (Phase 0)
**Source:** Freeform topic (issue #1680, Block 4 of shell integration building blocks)
**Problem:** Tsuku has no per-directory tool requirements. Projects can't declare which tools and versions they need, forcing manual discovery and installation by each developer.
**Constraints:** Must use TOML (existing convention), parse within 50ms, produce stable ProjectConfig interface for #1681 and #2168, support monorepos via directory traversal.

## Decisions (Phase 2)
1. **File naming**: `tsuku.toml` (non-dotfile, single name) with parent traversal (first-match, no merge), $HOME ceiling
2. **Schema**: Mixed map with string shorthand in [tools] section, version constraints: exact, prefix, latest
3. **Compatibility**: No .tool-versions support (native TOML only), defer migration tooling
4. **CLI**: Minimal non-interactive init + lenient batch install with interactive confirmation

## Security Review (Phase 5)
**Outcome:** Option 1 (approve with amendments)
**Summary:** Added interactive confirmation for batch install, tool count cap (256), unconditional $HOME ceiling, symlink resolution before traversal.

## Current Status
**Phase:** 6 - Final Review
**Last Updated:** 2026-03-27
