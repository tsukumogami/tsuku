# Issue 406 Implementation Plan

## Summary

Add `tsuku plan export <tool>` command to export stored installation plans as standalone JSON files.

## Approach

Add `planExportCmd` as a new subcommand of `planCmd` in `plan.go`. Reuse existing plan retrieval logic from `runPlanShow`. Export using the same JSON format as `tsuku eval` (via `printJSON`).

### Alternatives Considered

- **Separate file `export.go`**: Rejected - command is small and fits naturally with other plan subcommands
- **Use `plan show --json > file.json`**: Rejected - issue requires default filename generation and `-` for stdout

## Files to Modify

- `cmd/tsuku/plan.go` - Add `planExportCmd` subcommand with export logic

## Files to Create

None - adding to existing plan.go

## Implementation Steps

- [x] Add `planExportCmd` with flags (`--output`/`-o`)
- [x] Add helper function `getPlanForTool` to extract common plan retrieval logic
- [x] Implement `runPlanExport` with:
  - [x] Default filename: `<tool>-<version>-<os>-<arch>.plan.json`
  - [x] `-` support for stdout
  - [x] Custom output path via `--output`
- [x] Add unit tests for filename generation
- [x] Run `go vet`, `go test`, and `go build` to verify

## Testing Strategy

- Unit tests: Test default filename generation
- Manual verification: `tsuku install gh && tsuku plan export gh`

## Success Criteria

- [x] `tsuku plan export <tool>` exports plan to default filename
- [x] Default filename is `<tool>-<version>-<os>-<arch>.plan.json`
- [x] `--output` / `-o` allows custom output path
- [x] `-` outputs to stdout (enables piping)
- [x] JSON format matches `tsuku eval` output
- [x] Clear error if tool not installed
- [x] Clear error if tool has no stored plan
- [x] Help text documents usage
- [x] All tests pass, no lint errors

## Open Questions

None.
