# Design Summary: command-not-found

## Input Context (Phase 0)
**Source:** Issue #1678 (docs: design command-not-found handler)
**Parent design:** docs/designs/DESIGN-shell-integration-building-blocks.md
**Branch:** docs/shell-integration-auto-install

**Problem:** When users type unknown commands, shells fire command_not_found hooks. Tsuku
needs to register these hooks to suggest installable recipes via the binary index (Block 1).
The key open questions are: how hooks reach users (auto via install script vs manual),
what happens with existing handlers, and what `tsuku suggest` outputs.

**Constraints:**
- 50ms budget (binary index lookup is SQLite — fast; hook overhead must stay minimal)
- Must not clobber existing command_not_found handlers
- Clean uninstall (rc file must be left as-before after uninstall)
- Public repo — no internal references
- Shells: bash, zsh, fish

**User guidance:** Not limited by parent design's Block 2 sketch. Free to revisit UX
including whether `tsuku hook install` should run automatically during setup. Verify
against precedent and security implications.

## Decisions to Resolve
1. Hook delivery and lifecycle (auto-install during setup? source file vs snippet?)
2. Chaining with existing command_not_found handlers
3. `tsuku suggest` output format and interface

## Security Review (Phase 5)
**Outcome:** Option 2 - Document considerations
**Summary:** No design changes needed. Three implementation constraints documented: atomic rc file writes, network-free enforcement for tsuku suggest, and eval scope (bash-only). Hook file permissions (0644) and suggest timeout added after Phase 6 architecture review.

## Current Status
**Phase:** 6 - Final review complete, ready for commit
**Last Updated:** 2026-03-24
