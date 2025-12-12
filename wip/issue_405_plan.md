# Issue 405 Implementation Plan

## Summary

Add `tsuku plan show <tool>` command to display stored installation plans in human-readable format.

## Approach

Create a new `plan.go` file with parent `planCmd` and child `planShowCmd`. Follow the `cache.go` subcommand pattern. Read plan from state.json via StateManager and format for display.

### Alternatives Considered

- **Single command `tsuku plan <tool>`**: Rejected - leaves room for future plan subcommands (export, replay)
- **JSON-only output**: Rejected - issue requires human-readable format (with optional --json flag)

## Files to Create

- `cmd/tsuku/plan.go` - Main plan command with show subcommand
- `cmd/tsuku/plan_test.go` - Unit tests

## Files to Modify

- `cmd/tsuku/main.go` - Register planCmd with rootCmd

## Implementation Steps

- [x] Create `cmd/tsuku/plan.go` with:
  - [x] Parent `planCmd` (container for subcommands)
  - [x] Child `planShowCmd` with tool argument
  - [x] Load state via StateManager
  - [x] Format plan output (tool, version, platform, steps)
  - [x] Highlight non-evaluable steps
  - [x] Error handling for: tool not installed, no plan stored
  - [x] --json flag for JSON output
- [x] Register `planCmd` in `cmd/tsuku/main.go`
- [x] Add unit tests
- [x] Run `go vet`, `go test`, and `go build` to verify

## Output Format Design

```
Plan for gh@2.40.0

Platform: linux/amd64
Generated: 2024-12-12 21:30:00 UTC
Recipe:   registry (hash: abc123...)

Steps:
  1. [download_archive] https://github.com/...
     Checksum: sha256:deadbeef...
     Size: 12.5 MB
  2. [extract] format=tar.gz
  3. [install_binaries] binaries=gh
  4. [run_command] (non-evaluable)
     command: ./configure && make
```

## Testing Strategy

- Unit tests: Verify output formatting
- Unit tests: Verify error messages for missing tool/plan
- Manual verification: `tsuku install gh && tsuku plan show gh`

## Success Criteria

- [x] `tsuku plan show <tool>` displays formatted plan
- [x] Non-evaluable steps clearly marked
- [x] Clear error if tool not installed
- [x] Clear error if tool has no plan
- [x] `--json` flag outputs raw JSON
- [x] Help text documents usage
- [x] All tests pass, no lint errors

## Open Questions

None.
