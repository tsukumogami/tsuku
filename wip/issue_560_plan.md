# Issue 560 Implementation Plan

## Summary
Implement the `require_system` primitive action that detects system-installed dependencies, validates versions, and provides platform-specific installation guidance when dependencies are missing or outdated.

## Approach
This implementation creates a new primitive action that uses direct command execution (no shell) to detect system dependencies, parse versions via regex, and compare against minimum requirements. The action follows the existing action pattern (BaseAction embedding, parameter extraction via GetString/GetMapStringString helpers, structured error handling). Platform-specific guidance is provided via install_guide maps keyed by platform (darwin, linux, etc.).

### Alternatives Considered
- **Shell-based detection**: Rejected for security reasons (design requirement: no shell execution)
- **Runtime validation by default**: Deferred to future enhancement (Issue 560 focuses on command presence and version checks only)
- **Inline error messages**: Rejected in favor of structured install_guide maps for maintainability

## Files to Modify
- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/actions/action.go` - Register RequireSystemAction in init() function
- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/version/version_utils.go` - Export compareVersions function (currently unexported)

## Files to Create
- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/actions/require_system.go` - Core action implementation
- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/actions/require_system_test.go` - Unit tests

## Implementation Steps
- [ ] Create require_system.go with RequireSystemAction struct, Name(), and IsDeterministic() methods
- [ ] Implement Execute() method: parameter extraction (command, version_flag, version_regex, min_version, install_guide)
- [ ] Implement detectCommand() helper: use exec.LookPath() to find command, exec.Command() to run version check
- [ ] Implement parseVersion() helper: apply regex to version output, extract version string
- [ ] Export compareVersions() in version/version_utils.go by capitalizing function name
- [ ] Implement version comparison logic using exported CompareVersions()
- [ ] Implement getPlatformGuide() helper: detect platform (runtime.GOOS), return matching guide from install_guide map
- [ ] Define custom error types: SystemDepMissingError, SystemDepVersionError
- [ ] Implement error messages with platform-specific guidance
- [ ] Register RequireSystemAction in action.go init() function
- [ ] Create comprehensive unit tests: command found, command missing, version too old, version sufficient, regex parsing, platform guide selection
- [ ] Add edge case tests: invalid regex, missing required params, empty install_guide, version comparison edge cases
- [ ] Run `go test ./internal/actions/` to verify all tests pass
- [ ] Run `go build -o tsuku ./cmd/tsuku` to verify build succeeds

## Testing Strategy
- Unit tests:
  - Parameter validation (required params, optional params, missing params)
  - Command detection (found vs not found via mock commands in PATH)
  - Version parsing (regex extraction from various output formats)
  - Version comparison (min_version satisfied vs insufficient)
  - Platform guide selection (darwin, linux, fallback)
  - Error message formatting (missing dependency, version mismatch)
- Integration tests: Manual testing via example recipe (docker.toml or similar) after implementation
- Manual verification: Create test recipe with require_system action, verify error messages show correct platform guidance

## Risks and Mitigations
- **Risk**: exec.LookPath() behavior differs across platforms
  - **Mitigation**: Test on Linux and macOS, document platform-specific behaviors in code comments
- **Risk**: Version regex patterns may not cover all tool output formats
  - **Mitigation**: Make version_regex configurable per recipe, provide clear error when regex fails to match
- **Risk**: Version comparison logic may not handle pre-release versions correctly
  - **Mitigation**: Use existing compareVersions() from version/version_utils.go, add test cases for common formats
- **Risk**: Platform detection may not distinguish Linux distros
  - **Mitigation**: Start with simple runtime.GOOS detection, add hierarchical matching (linux.ubuntu, linux.fedora) in future enhancement

## Success Criteria
- [ ] RequireSystemAction registered in action registry
- [ ] Action detects command presence without shell execution
- [ ] Action parses version via configurable regex
- [ ] Action validates minimum version if specified
- [ ] Action returns clear error with platform-specific guidance when dependency missing
- [ ] All unit tests pass (`go test ./internal/actions/`)
- [ ] Build succeeds (`go build -o tsuku ./cmd/tsuku`)
- [ ] No new golangci-lint warnings
- [ ] Code follows existing action patterns (parameter extraction, error handling, logging)
- [ ] Error messages use HTTPS-only URLs in install guides (enforced via testing)

## Open Questions
None - design document (DESIGN-dependency-provisioning.md lines 512-690) provides sufficient specification for initial implementation. Future enhancements (runtime validation, hierarchical platform matching, assisted install) are deferred to separate issues.
