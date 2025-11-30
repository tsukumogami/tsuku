# Issue 22 Implementation Plan

## Summary

Add signal handling to support graceful cancellation when users press Ctrl+C, ensuring proper cleanup of partial installations and temp files.

## Approach

Use Go's signal package to intercept SIGINT/SIGTERM in main.go, create a cancellable context, and propagate it through the execution chain. Actions will use exec.CommandContext() for child processes and http.NewRequestWithContext() for HTTP requests to enable cancellation.

### Alternatives Considered

1. **Per-goroutine signal handling**: Each long-running goroutine handles signals independently
   - Why not: Complex, race conditions, harder to coordinate cleanup

2. **Using os.Interrupt channel in individual commands**: Handle interrupts at the Cobra command level
   - Why not: Requires modifying each command; central handler is cleaner

## Files to Modify

- `cmd/tsuku/main.go` - Add signal handler, create cancellable context
- `cmd/tsuku/install.go` - Pass context through install functions
- `internal/actions/run_command.go` - Use exec.CommandContext for child processes
- `internal/actions/download.go` - Use http.NewRequestWithContext for HTTP requests
- `internal/actions/npm_install.go` - Use exec.CommandContext for npm commands
- `internal/actions/cargo_install.go` - Use exec.CommandContext for cargo commands
- `internal/actions/gem_install.go` - Use exec.CommandContext for gem commands
- `internal/actions/pipx_install.go` - Use exec.CommandContext for pip commands
- `internal/actions/nix_install.go` - Use exec.CommandContext for nix commands
- `internal/actions/nix_portable.go` - Use http.NewRequestWithContext for downloads

## Files to Create

None - all changes are to existing files.

## Implementation Steps

- [ ] Step 1: Add signal handler in main.go with cancellable context
- [ ] Step 2: Update install.go to pass context from signal handler
- [ ] Step 3: Update run_command.go to use exec.CommandContext
- [ ] Step 4: Update download.go to use http.NewRequestWithContext
- [ ] Step 5: Update package manager actions (npm, cargo, gem, pipx, nix) to use exec.CommandContext
- [ ] Step 6: Update nix_portable.go download to use context
- [ ] Step 7: Add cleanup message on cancellation
- [ ] Step 8: Add unit tests for signal handling

Mark each step [x] after it is implemented and committed. This enables clear resume detection.

## Testing Strategy

- Unit tests: Test context cancellation propagates correctly through functions
- Manual verification:
  1. Run `tsuku install <large-tool>` and press Ctrl+C during download
  2. Verify operation stops promptly
  3. Verify no partial files left in `$TSUKU_HOME/tools/`
  4. Verify temp directory is cleaned up

## Risks and Mitigations

- **Risk**: Child processes may not terminate immediately when context is cancelled
  - **Mitigation**: exec.CommandContext sends SIGKILL after context cancellation; processes should terminate

- **Risk**: Partial extraction may leave corrupted files
  - **Mitigation**: Use defer cleanup() to remove temp dirs on cancellation; install happens atomically at the end

- **Risk**: HTTP downloads may hang despite context cancellation
  - **Mitigation**: NewRequestWithContext cancels request properly; test with slow/hanging servers

## Success Criteria

- [ ] Ctrl+C cancels ongoing operations within 2-3 seconds
- [ ] No partial installations left in `$TSUKU_HOME/tools/`
- [ ] Temp directories are cleaned up on cancellation
- [ ] Exit code is appropriate (non-zero) on cancellation
- [ ] All existing tests pass
- [ ] No new linter warnings

## Open Questions

None - implementation approach is clear.
