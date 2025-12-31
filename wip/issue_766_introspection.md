# Issue 766 Introspection

## Context Reviewed

- Design doc: `docs/DESIGN-system-dependency-actions.md`
- Sibling issues reviewed: #754, #755, #756, #759, #760, #761, #762, #763, #764, #765
- Prior patterns identified:
  - `internal/platform/family.go`: `DetectFamily()`, `DetectTarget()` implemented
  - `internal/actions/system_action.go`: `SystemAction` interface with `Describe(params map[string]interface{}) string`
  - `internal/executor/filter.go`: `FilterStepsByTarget(steps, target)` implemented
  - `cmd/tsuku/main.go`: Global `--quiet` flag via `quietFlag` bool, sets log level
  - Constraint matching via `MatchesTarget(target platform.Target)` in action's implicit constraints

## Gap Analysis

### Minor Gaps

1. **File locations established**: All infrastructure is in place:
   - Platform detection: `internal/platform/family.go`
   - Plan filtering: `internal/executor/filter.go`
   - System actions with `Describe()`: `internal/actions/*.go`
   - CLI command patterns: `cmd/tsuku/*.go`

2. **Describe() method signature**: All system actions implement `Describe(params map[string]interface{}) string` - takes step params, returns shell command string.

3. **Constraint matching pattern**: Use `action.ImplicitConstraint()` to get constraint, then `constraint.MatchesTarget(target)` to check applicability.

4. **Quiet mode pattern**: Use global `quietFlag` variable for conditional output suppression.

5. **Platform detection integration**: Use `platform.DetectTarget()` which returns `Target{Platform, LinuxFamily}`.

6. **Family header naming**: From design doc examples:
   - `For Ubuntu/Debian:` (family = debian)
   - Derive human-readable names from linux_family values

### Moderate Gaps

1. **Where should instruction display logic live?**
   - Issue says "CLI to display system dependency instructions" but doesn't specify which command or file
   - Options: (a) integrate into `install.go` during install flow, (b) create new subcommand, (c) add to `info.go`
   - **Recommendation**: Integrate into `install.go` - display when recipe has system actions and they aren't satisfied. This matches design doc's example output ("Docker requires system dependencies...").

2. **When should instructions be displayed?**
   - Issue says "when recipe has system dependency actions" but doesn't clarify trigger
   - **Recommendation**: Display before installation proceeds, when filtered system actions exist for current target.

3. **Integration with #764 (`--verify` flag)**
   - Issue #766 mentions `require_command` verification at the end
   - Issue #764 is still open and adds `--verify` flag separately
   - **Recommendation**: For #766, just show the `require_command` descriptions at end of instructions. Actual verification execution is #764's scope.

4. **Grouping by family header format**
   - Design doc shows: "For Ubuntu/Debian:" with numbered instructions
   - Need to define family-to-display-name mapping
   - **Recommendation**: Create simple mapping in platform package or inline in CLI.

### Major Gaps

None identified. All infrastructure dependencies (#759, #761, #763, #755, #756) are implemented.

## Recommendation

**Proceed** - All blocking dependencies are complete. The acceptance criteria are clear and infrastructure is in place.

## Proposed Amendments

Document for the issue (moderate gaps):

1. **Location**: Add instruction display logic to `cmd/tsuku/install.go`, called after recipe loading but before actual installation.

2. **Trigger**: Display instructions when recipe contains system actions matching current target (filtered via `FilterStepsByTarget`).

3. **Output format** (per design doc):
   ```
   <Tool> requires system dependencies that tsuku cannot install directly.

   For <Family Name>:

     1. <Action description from Describe()>
     2. <Next action description>
     ...

   After completing these steps, run: tsuku install <tool> --verify
   ```

4. **Family display names** mapping:
   - `debian` -> "Ubuntu/Debian"
   - `rhel` -> "Fedora/RHEL"
   - `arch` -> "Arch Linux"
   - `alpine` -> "Alpine Linux"
   - `suse` -> "openSUSE/SLES"
   - `darwin` (non-linux) -> "macOS"

5. **--target-family flag**: Add to install command flags, accepts family string to override `DetectFamily()` result.

## Implementation Notes

Key code references for implementation:

- `platform.DetectTarget()` - Get current target tuple
- `executor.FilterStepsByTarget(recipe.Steps, target)` - Filter to matching steps
- `actions.Get(step.Action)` - Get action by name
- Type assert to `actions.SystemAction` to call `Describe(step.Params)`
- Check `quietFlag` before printing instructions
- Parse family from `target.LinuxFamily` or infer from `target.OS()` for darwin
