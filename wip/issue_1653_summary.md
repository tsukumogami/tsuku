# Issue 1653 Summary

## What Was Implemented

Added CLI handling for `AmbiguousMatchError` in both `create` and `install` commands. When ecosystem discovery finds multiple matches without a clear winner, users now see actionable `--from` suggestions.

## Changes Made

- `cmd/tsuku/exitcodes.go`: Added `ExitAmbiguous = 10` exit code for disambiguation required
- `cmd/tsuku/create.go`: Added `handleAmbiguousError` function that displays the multi-line error message and exits with code 10
- `cmd/tsuku/install.go`: Added `handleAmbiguousInstallError` function with JSON support. When `--json` flag is set, outputs structured JSON with matches array; otherwise displays text error.

## Key Decisions

- **Separate exit code**: Used `ExitAmbiguous = 10` rather than `ExitUsage` because this isn't a syntax error - it's a valid request that requires disambiguation
- **JSON only for install**: The `create` command doesn't have a `--json` flag, so only `install` gets structured JSON output
- **Preserve multi-line format**: The error message includes `--from` suggestions on separate lines, so we print directly rather than using `errmsg.Fprint`

## JSON Output Format

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

## Test Coverage

- Existing tests in `disambiguate_test.go` cover the `Error()` formatting extensively
- CLI tests pass

## Known Limitations

- None
