---
summary:
  constraints:
    - AmbiguousMatchError.Error() already produces the formatted multi-line output
    - errmsg.Fprint adds "Error: " prefix automatically
    - Must use errors.As() pattern consistent with other CLI error handling
    - --json flag requires structured output with matches array
    - Exit code should distinguish this error type (suggest ExitUsage for user-actionable errors)
  integration_points:
    - cmd/tsuku/create.go - runCreate calls runDiscovery at line 480-484
    - cmd/tsuku/install.go - tryDiscoveryFallback calls runDiscoveryWithOptions at line 388-398
    - internal/errmsg/errmsg.go - Fprint adds "Error: " prefix to error messages
    - internal/discover/resolver.go - AmbiguousMatchError.Error() produces multi-line output
  risks:
    - JSON output structure must be defined (matches array with builder/source fields)
    - Need to ensure error message displays correctly (multi-line)
  approach_notes: |
    The AmbiguousMatchError.Error() method already produces:
    ```
    Multiple sources found for "bat". Use --from to specify:
      tsuku install bat --from crates.io:sharkdp/bat
      tsuku install bat --from npm:bat-cli
    ```

    Current flow (create.go lines 480-484):
    1. runDiscovery returns error
    2. printError(err) calls errmsg.Fprint which adds "Error: " prefix
    3. exitWithCode(ExitRecipeNotFound)

    Changes needed:
    1. In create.go: Add errors.As check for AmbiguousMatchError before generic printError
    2. In install.go: Same pattern in tryDiscoveryFallback
    3. Use ExitUsage exit code (user needs to take action with --from)
    4. For --json, output structured JSON with matches array
---

# Implementation Context: Issue #1653

**Source**: docs/designs/DESIGN-disambiguation.md (Phase 4: Non-Interactive Error)

## Design Reference

From DESIGN-disambiguation.md:
```
Error: Multiple sources found for "bat". Use --from to specify:
  tsuku install bat --from crates.io:sharkdp/bat
  tsuku install bat --from npm:bat-cli
  tsuku install bat --from rubygems:bat
```

The error format is already implemented in AmbiguousMatchError.Error() from #1652.
This issue handles displaying that error in the CLI with proper exit code and JSON output.

## Key Implementation Points

1. **create.go line 480-484**: Currently calls `printError(err)` then `exitWithCode(ExitRecipeNotFound)`. Change to check for AmbiguousMatchError first.

2. **install.go line 388-398**: Same pattern in `tryDiscoveryFallback`.

3. **JSON output**: When `jsonFlag` is true, output structured JSON matching the error response pattern used elsewhere.

4. **Exit code**: Use ExitUsage (user must provide --from flag).
