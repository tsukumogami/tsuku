# Issue 507 Implementation Plan

## Summary

Add `--plan` flag to install command that loads and executes external plans, bypassing normal recipe-based evaluation. Uses existing `loadPlanFromSource()` and `validateExternalPlan()` utilities from issue #506.

## Approach

The implementation adds a `--plan` flag to `installCmd` and creates a new `runPlanBasedInstall()` function. When `--plan` is provided, the command bypasses the normal recipe lookup and plan generation, directly loading and executing the provided plan.

### Alternatives Considered

- **Separate command (`tsuku plan install`)**: Rejected because it fragments the user experience. Users expect `install` to handle all installation methods.
- **Automatic plan detection (no flag)**: Rejected because it's ambiguous - a file path argument could be confused with a tool name.

## Files to Modify

- `cmd/tsuku/install.go` - Add `--plan` flag and modify command logic
- `cmd/tsuku/plan_install.go` - New file with `runPlanBasedInstall()` implementation

## Files to Create

- `cmd/tsuku/plan_install_test.go` - Integration tests for plan-based installation

## Implementation Steps

- [ ] Add `--plan` flag to install command in `install.go`
- [ ] Modify install command Run function to detect and route plan-based installs
- [ ] Create `plan_install.go` with `runPlanBasedInstall()` function
- [ ] Store plan in state.json after successful execution
- [ ] Add unit tests for CLI flag handling and argument validation
- [ ] Add integration tests for plan-based installation workflow

## Testing Strategy

- Unit tests: CLI flag parsing, argument validation errors (multiple tools with --plan)
- Integration tests:
  - `tsuku install --plan <file>` workflow
  - `tsuku eval tool | tsuku install --plan -` piping (use loadPlanFromSourceWithReader for stdin mocking)
  - Platform mismatch error
  - Tool name mismatch error

## Risks and Mitigations

- **Stdin pipe testing**: Using `loadPlanFromSourceWithReader()` with mock stdin readers
- **State storage compatibility**: Using existing `executor.ToStoragePlan()` conversion for consistency

## Success Criteria

- [ ] `--plan <path>` flag accepts file path or "-" for stdin
- [ ] Tool name is optional when `--plan` is provided (defaults from plan)
- [ ] Tool name mismatch with plan produces clear error
- [ ] Multiple tools with `--plan` flag produces clear error
- [ ] Plan stored in state.json after successful installation
- [ ] All existing install tests pass
- [ ] New tests pass

## Open Questions

None - design decisions made in DESIGN-plan-based-installation.md (decisions 1B, 2B, 3C).
