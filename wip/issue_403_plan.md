# Issue 403 Implementation Plan

## Summary

Implement `tsuku eval <tool>[@version]` command that outputs JSON installation plans to stdout, with `--os` and `--arch` flags for cross-platform plan generation.

## Approach

Follow existing command patterns (info.go, install.go) to create eval.go. Use the existing `Executor.GeneratePlan()` method from #402 to generate plans. Validate platform flags against a whitelist for security. Output JSON to stdout with warnings to stderr.

### Alternatives Considered

- **Custom plan generator in cmd package**: Rejected - plan generation logic already exists in executor package
- **Reuse install --dry-run**: Rejected - dry-run only prints steps, doesn't generate structured JSON with checksums

## Files to Create

- `cmd/tsuku/eval.go` - Main eval command implementation

## Files to Modify

- `cmd/tsuku/main.go` - Register evalCmd with rootCmd

## Implementation Steps

- [x] Create `cmd/tsuku/eval.go` with:
  - [x] Platform flag validation (whitelist for os/arch values)
  - [x] Recipe loading and executor creation
  - [x] Plan generation via `Executor.GeneratePlan()`
  - [x] JSON output to stdout (pretty-printed)
  - [x] Warnings to stderr for non-evaluable actions
  - [x] Help text and usage examples
- [x] Register `evalCmd` in `cmd/tsuku/main.go`
- [x] Add unit tests for platform validation
- [x] Run `go vet`, `go test`, and `go build` to verify

## Testing Strategy

- Unit tests: Platform flag validation (whitelist enforcement, rejection of invalid values)
- Integration tests: Not needed - uses existing infrastructure
- Manual verification: `go build && ./tsuku eval gh` should output JSON plan

## Risks and Mitigations

- **Security: Path traversal via platform flags**: Mitigation: Strict whitelist validation before use
- **Network errors during checksum computation**: Mitigation: Clear error messages, follows PreDownloader error handling

## Success Criteria

- [ ] `tsuku eval <tool>[@version]` outputs JSON plan to stdout
- [ ] `--os` and `--arch` flags work with whitelist validation
- [ ] Invalid platform values are rejected with clear error message
- [ ] Non-evaluable action warnings go to stderr (not mixed with JSON)
- [ ] Exit code 0 on success, non-zero on failure
- [ ] `tsuku eval --help` shows usage information
- [ ] All tests pass, no lint errors

## Open Questions

None - design document provides clear guidance.
