# Issue 155 Implementation Plan

## Summary

Add a `Run` function to the existing `configCmd` that displays all configuration values when `tsuku config` is invoked without a subcommand. Support `--json` flag for machine-readable output.

## Approach

Modify the existing `config.go` to add a `Run` handler to `configCmd` that displays:
1. Environment-based settings (TSUKU_HOME, TSUKU_API_TIMEOUT, TSUKU_VERSION_CACHE_TTL)
2. User config settings (telemetry)

For sensitive values like tokens, show "(set)" or "(not set)" instead of the actual value.

### Alternatives Considered

- **Create a separate `show` subcommand**: Would require users to type `tsuku config show` instead of just `tsuku config`. Less ergonomic, not chosen.
- **Print full paths for all directories**: Would clutter output. Not chosen - show only the key settings users might want to check.

## Files to Modify

- `cmd/tsuku/config.go` - Add Run handler to configCmd with --json support

## Files to Create

None

## Implementation Steps

- [ ] Add `--json` flag to configCmd
- [ ] Implement Run handler that displays all configuration values
- [ ] Support "(set)" indicator for sensitive values (GITHUB_TOKEN)
- [ ] Add JSON output struct and handling
- [ ] Add tests for the config command output

## Testing Strategy

- Unit tests: Test JSON output structure
- Manual verification: Run `tsuku config` and `tsuku config --json`

## Risks and Mitigations

- **Breaking existing behavior**: The current `tsuku config` shows help. Adding Run will change this to show config. Acceptable since this is the intended behavior per the issue.

## Success Criteria

- [ ] `tsuku config` displays all configuration values
- [ ] `tsuku config --json` outputs JSON format
- [ ] Sensitive values show "(set)" instead of actual value
- [ ] All tests pass

## Open Questions

None
