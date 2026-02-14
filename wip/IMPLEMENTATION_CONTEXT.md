---
summary:
  constraints:
    - Error() must format matches as actionable `--from` suggestions (copy-paste ready)
    - Matches must be sorted by popularity before formatting
    - Must handle variable match counts (2+)
    - Must not break existing E2E flow (skeleton remains functional)
  integration_points:
    - internal/discover/resolver.go - AmbiguousMatchError type already exists, needs Error() enhancement
    - CLI error handling (#1653) - downstream will display this error to users
    - Batch orchestrator (#1654) - already implemented, can detect AmbiguousMatchError for tracking
  risks:
    - Breaking existing callers if type signature changes (keep backward compatible)
    - Incorrect popularity sorting affecting --from suggestion order
    - Edge cases with single matches or zero downloads
  approach_notes: |
    The AmbiguousMatchError type already exists in resolver.go (lines 145-178) with basic fields.
    The Error() method currently returns a simple message. Enhance it to:
    1. Sort matches by popularity (downloads DESC, then version count DESC)
    2. Format each match as `tsuku install <tool> --from <builder>:<source>`
    3. Return multi-line error suitable for terminal output

    The DiscoveryMatch type already has Downloads, VersionCount, and HasRepository fields
    needed for sorting. No schema changes required - just enhance the Error() method.
---

# Implementation Context: Issue #1652

**Source**: docs/designs/DESIGN-disambiguation.md (Phase 4: Non-Interactive Error)

## Design Excerpt

From DESIGN-disambiguation.md, Section "Component 3: Interactive Prompt" and "Phase 4: Non-Interactive Error":

In non-interactive mode (piped stdin or `--yes` with ambiguous matches), print the same list as an error and suggest `--from`:

```
Error: Multiple sources found for "bat". Use --from to specify:
  tsuku install bat --from crates.io:sharkdp/bat
  tsuku install bat --from npm:bat-cli
  tsuku install bat --from rubygems:bat
```

This enables CI/pipeline usage where interactive prompts aren't possible. The CLI can catch AmbiguousMatchError and display actionable guidance.

## Key Points

1. **Format specification**: Error message includes ranked matches with source identifiers for copy-paste convenience
2. **Sorting**: Matches are sorted by popularity before formatting (downloads DESC)
3. **Integration**: #1653 (CLI error handling) will catch and display this error
4. **Batch tracking**: #1654 (already done) uses AmbiguousMatchError detection for disambiguation record tracking
