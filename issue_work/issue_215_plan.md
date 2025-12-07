# Issue 215 Implementation Plan

## Summary

Add `LibsDir` field to Config struct for shared library storage at `$TSUKU_HOME/libs/`.

## Approach

Follow existing patterns in config.go: add field to struct, initialize in DefaultConfig(), include in EnsureDirectories(), and add helper method for versioned library paths.

### Alternatives Considered

- **Separate libs config file**: Rejected - unnecessary complexity; libs are part of core tsuku state
- **Nested under tools/**: Rejected - design doc specifies separate top-level directory for clarity

## Files to Modify

- `internal/config/config.go` - Add LibsDir field and LibDir helper method
- `internal/config/config_test.go` - Add tests for new field and method

## Files to Create

None

## Implementation Steps

- [x] Add `LibsDir` field to Config struct
- [x] Initialize LibsDir in DefaultConfig()
- [x] Add LibsDir to EnsureDirectories()
- [x] Add LibDir(name, version) helper method
- [x] Add tests for LibsDir in DefaultConfig
- [x] Add tests for LibsDir in EnsureDirectories
- [x] Add test for LibDir helper method

## Testing Strategy

- **Unit tests**: Verify LibsDir is correctly set in DefaultConfig, respects TSUKU_HOME, and is created by EnsureDirectories

## Risks and Mitigations

- **None significant**: This is a simple additive change following existing patterns

## Success Criteria

- [x] `LibsDir` added to config struct
- [x] Directory created during tsuku initialization
- [x] Unit tests for config

## Open Questions

None
