# Issue 1653 Implementation Plan

## Goal

Handle `AmbiguousMatchError` in CLI with proper error display, exit code, and JSON output.

## Files to Modify

- `cmd/tsuku/create.go` - Add AmbiguousMatchError handling in runCreate discovery error path
- `cmd/tsuku/install.go` - Add AmbiguousMatchError handling in tryDiscoveryFallback
- `cmd/tsuku/exitcodes.go` - Add ExitAmbiguous = 10 (new exit code for disambiguation required)

## Design Decisions

### Exit Code

Use a new exit code `ExitAmbiguous = 10` rather than `ExitUsage = 2` because:
- The error is not a usage error (valid syntax, valid tool name)
- Batch scripts may want to detect "needs disambiguation" specifically
- Consistent with pattern of specific exit codes for specific failures

### JSON Output

For `--json` mode, output structured JSON with matches array:
```json
{
  "status": "error",
  "category": "ambiguous",
  "message": "Multiple sources found for \"bat\". Use --from to specify.",
  "tool": "bat",
  "matches": [
    {"builder": "crates.io", "source": "sharkdp/bat"},
    {"builder": "npm", "source": "bat-cli"}
  ],
  "exit_code": 10
}
```

This enables batch scripts to parse matches and decide which source to use.

### Text Output

For non-JSON mode, use `fmt.Fprintln(os.Stderr, err.Error())` directly to preserve multi-line formatting. The `errmsg.Fprint` function adds "Error: " prefix, but the AmbiguousMatchError.Error() already starts with "Multiple sources found..." which reads naturally.

## Implementation Steps

- [ ] Add `ExitAmbiguous = 10` to exitcodes.go
- [ ] Add `ambiguousError` struct for JSON output in create.go
- [ ] Add `handleAmbiguousError` helper function
- [ ] Update create.go runCreate error handling (lines 480-484)
- [ ] Update install.go tryDiscoveryFallback error handling (lines 388-398)
- [ ] Add unit tests for error handling
- [ ] Run tests and verify

## Test Cases

1. AmbiguousMatchError text output includes all matches with --from suggestions
2. AmbiguousMatchError JSON output includes structured matches array
3. Exit code is ExitAmbiguous (10)
4. Other discovery errors still use existing handling
