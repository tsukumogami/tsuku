# Issue 84 Implementation Plan

## Summary

Integrate telemetry client into install, update, and remove commands by adding telemetry event calls after successful operations, with first-run notice displayed before the first event.

## Approach

Add telemetry integration directly to the command implementations in `cmd/tsuku/`. The telemetry client and notice APIs are already implemented. The integration points are clear:

1. **install.go**: After `mgr.InstallWithOptions` succeeds, send install event
2. **update.go**: Get previous version before update, then after `runInstall` succeeds, send update event
3. **remove.go**: Get version before removal, then after `mgr.Remove` succeeds, send remove event

Key decision: Create telemetry client once per command execution (not per-tool in multi-install) to show notice only once.

### Alternatives Considered

- **Integrate in install.Manager**: Would require passing telemetry client through many layers. Not chosen because it tightly couples install logic to telemetry.
- **Hook system**: Add pre/post hooks to operations. Over-engineered for current needs.

## Files to Modify

- `cmd/tsuku/install.go` - Add telemetry client, call ShowNoticeIfNeeded, send install events
- `cmd/tsuku/update.go` - Get previous version, send update event after success
- `cmd/tsuku/remove.go` - Get version before removal, send remove event after success

## Files to Create

None required - telemetry package already has all needed APIs.

## Implementation Steps

- [ ] Step 1: Add telemetry to install.go
  - Import telemetry package
  - Create client in installCmd.Run
  - Call ShowNoticeIfNeeded before first install
  - Pass version constraint and isDependency to installWithDependencies
  - Send install event after successful InstallWithOptions

- [ ] Step 2: Add telemetry to update.go
  - Import telemetry package
  - Create client in updateCmd.Run
  - Call ShowNoticeIfNeeded
  - Capture previous version from state before update
  - Send update event after successful runInstall

- [ ] Step 3: Add telemetry to remove.go
  - Import telemetry package
  - Create client in removeCmd.Run
  - Call ShowNoticeIfNeeded
  - Capture version from state before removal
  - Send remove event after successful mgr.Remove

- [ ] Step 4: Add integration tests
  - Test that events are sent with correct data
  - Test that notice is shown on first run
  - Test that telemetry disabled env var prevents events

## Testing Strategy

- **Unit tests**: Mock telemetry endpoint, verify correct event fields
- **Integration tests**: Use TSUKU_TELEMETRY_DEBUG to capture events without network
- **Manual verification**: Run commands and check debug output

## Risks and Mitigations

- **Performance**: Telemetry is fire-and-forget (async), minimal impact
- **Privacy**: Events are anonymized, opt-out available via TSUKU_NO_TELEMETRY
- **Failure handling**: Silent failures - telemetry errors don't affect commands

## Success Criteria

- [ ] `tsuku install <tool>` sends install event with correct fields
- [ ] `tsuku update <tool>` sends update event with version_previous and version_resolved
- [ ] `tsuku remove <tool>` sends remove event with version_previous
- [ ] version_constraint captured when user specifies `@<version>`
- [ ] is_dependency correctly set (true for transitive installs)
- [ ] ShowNoticeIfNeeded called before first event
- [ ] Events only sent on successful operations
- [ ] TSUKU_NO_TELEMETRY=1 prevents all telemetry

## Open Questions

None - all APIs are implemented and clear.
