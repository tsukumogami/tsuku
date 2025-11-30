# Issue 15 Implementation Plan

## Overview
Add --dry-run flag to install and update commands to preview actions without execution.

## Analysis

### Key Components
1. `cmd/tsuku/install.go` - Install command, calls `runInstallWithTelemetry`
2. `cmd/tsuku/update.go` - Update command, calls `runInstallWithTelemetry`
3. `internal/executor/executor.go` - Executes recipe steps

### Data Flow
1. User runs `tsuku install tool --dry-run`
2. Command parses --dry-run flag
3. Recipe is loaded and version resolved (same as real install)
4. Instead of executing, display planned actions

## Implementation

### Step 1: Add DryRun method to Executor
Add `DryRun(ctx context.Context) error` to executor.go that:
- Resolves version (same as Execute)
- Lists each step with action name and key parameters
- Does NOT execute any actions
- Shows version and actions list

### Step 2: Add --dry-run flag to install command
Modify install.go:
- Add `var dryRun bool` flag
- Pass dryRun through to a modified runInstallWithTelemetry
- When dryRun is true, call executor.DryRun instead of Execute
- Skip actual installation (mgr.InstallWithOptions)

### Step 3: Add --dry-run flag to update command
Modify update.go:
- Add `var dryRun bool` flag
- Pass to install logic

### Step 4: Format dry-run output
Output format:
```
Would install: <tool>@<version>
  Dependencies: <list or none>
  Actions:
    1. <action>: <key params>
    2. ...
```

## File Changes

| File | Change |
|------|--------|
| `internal/executor/executor.go` | Add DryRun method |
| `cmd/tsuku/install.go` | Add --dry-run flag, pass to logic |
| `cmd/tsuku/update.go` | Add --dry-run flag |

## Testing Strategy
- Unit test for DryRun method output
- Integration test: verify no filesystem changes with --dry-run
- CI: existing tests should pass

## Success Criteria
1. `tsuku install tool --dry-run` shows planned actions
2. `tsuku update tool --dry-run` shows planned actions
3. No filesystem changes when --dry-run is set
4. All existing tests pass
