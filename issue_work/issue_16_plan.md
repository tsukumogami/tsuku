# Issue 16 Implementation Plan

## Summary

Add a global `--quiet` (`-q`) flag to suppress informational output across all commands, keeping only error messages on stderr.

## Approach

Use a global persistent flag on the root command with a helper function (`printInfo`) that checks the quiet flag before printing. This centralizes the quiet logic and avoids modifying every single print statement.

### Alternatives Considered
- **Per-command flags**: Each command gets its own --quiet flag. Rejected because it requires more code duplication and inconsistent UX.
- **io.Writer abstraction**: Replace stdout with a no-op writer when quiet. Rejected because it's more complex and harder to debug.
- **Modify every fmt.Printf call inline**: Check `quietFlag` at each call site. Rejected because it's error-prone and verbose.

## Files to Modify
- `cmd/tsuku/main.go` - Add persistent `--quiet` flag to rootCmd
- `cmd/tsuku/helpers.go` - Add `printInfo` helper function
- `cmd/tsuku/install.go` - Replace informational prints with `printInfo`
- `cmd/tsuku/update.go` - Replace informational prints with `printInfo`
- `cmd/tsuku/remove.go` - Replace informational prints with `printInfo`
- `cmd/tsuku/list.go` - Replace informational prints with `printInfo`
- `cmd/tsuku/recipes.go` - Replace informational prints with `printInfo`
- `cmd/tsuku/outdated.go` - Replace informational prints with `printInfo`
- `cmd/tsuku/create.go` - Replace informational prints with `printInfo`
- `cmd/tsuku/search.go` - Replace informational prints with `printInfo`
- `cmd/tsuku/update_registry.go` - Replace informational prints with `printInfo`
- `cmd/tsuku/verify.go` - Replace informational prints with `printInfo`
- `cmd/tsuku/config.go` - NOT modified (output is the command's primary function)
- `cmd/tsuku/versions.go` - NOT modified (output is the command's primary function)
- `cmd/tsuku/info.go` - NOT modified (output is the command's primary function)

## Implementation Steps
- [ ] Add global `quietFlag` variable and persistent flag to rootCmd in main.go
- [ ] Add `printInfo` and `printInfof` helpers in helpers.go
- [ ] Update install.go to use printInfo for informational messages
- [ ] Update update.go to use printInfo for informational messages
- [ ] Update remove.go to use printInfo for informational messages
- [ ] Update list.go to use printInfo for informational messages
- [ ] Update recipes.go to use printInfo for informational messages
- [ ] Update outdated.go to use printInfo for informational messages
- [ ] Update create.go to use printInfo for informational messages
- [ ] Update search.go to use printInfo for informational messages
- [ ] Update update_registry.go to use printInfo for informational messages
- [ ] Update verify.go to use printInfo for informational messages
- [ ] Verify all tests pass

## Testing Strategy
- Unit tests: None needed for simple flag checking
- Manual verification:
  - `tsuku install nodejs --quiet` produces no output on success
  - `tsuku install nonexistent --quiet` still shows errors
  - `tsuku list --quiet` suppresses header text
  - Exit codes unchanged

## Risks and Mitigations
- **Missing some print calls**: Comprehensive grep search identifies all calls
- **Breaking scripting that parses output**: Users should use --quiet for scripts, not parse verbose output

## Success Criteria
- [ ] `--quiet` / `-q` flag available on all commands
- [ ] Informational output suppressed when flag is set
- [ ] Errors still printed to stderr
- [ ] Exit codes unchanged
- [ ] All tests pass

## Output Classification

Messages to suppress (use printInfo):
- Progress messages: "Installing...", "Downloading...", "Checking dependencies..."
- Success confirmations: "Installation successful!", "Removed X"
- Informational headers: "Installed tools (N total):"
- Warnings that don't affect success

Messages to keep (continue using fmt.Fprintf to stderr):
- Error messages
- Fatal errors that cause exit(1)

Messages that are the command's purpose (keep as-is):
- `list` output (tool names/versions) - these ARE the result
- `info` output - this IS the result
- `config get` output - this IS the result
- `versions` output - this IS the result
- `search` output - this IS the result when found

## Open Questions
None.
