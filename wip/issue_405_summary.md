# Issue 405 Summary

## Changes Made

### Files Created
- `cmd/tsuku/plan.go` - Plan command with show subcommand
- `cmd/tsuku/plan_test.go` - Unit tests for formatting functions

### Files Modified
- `cmd/tsuku/main.go` - Registered `planCmd` with root command

## Implementation Details

Added `tsuku plan show <tool>` command that:
- Displays stored installation plans in human-readable format
- Shows tool name, version, platform, generation timestamp, and recipe source
- Lists all steps with action types, URLs, checksums, and parameters
- Marks non-evaluable steps (e.g., run_command) with "(non-evaluable)" indicator
- Supports `--json` flag for machine-readable JSON output
- Provides clear error messages for:
  - Tool not installed
  - Tool installed but no plan stored (pre-dates plan feature)

## Testing

- Unit tests verify formatting functions (formatBytes, truncateHash, formatValue, formatParams)
- All tests pass: `go test ./cmd/tsuku/... ok`
- Build succeeds: `go build ./cmd/tsuku`
- No linter errors: `go vet ./cmd/tsuku/...`

## Verification Commands

```bash
# Build and test
go build -o tsuku ./cmd/tsuku
go test ./cmd/tsuku/...
go vet ./cmd/tsuku/...

# Manual verification (after installing a tool)
./tsuku plan show gh
./tsuku plan show gh --json
./tsuku plan show nonexistent  # should show error
```
