# Issue 15 Implementation Summary

## Changes Made

### Modified Files
- `internal/executor/executor.go`: Added DryRun method and formatActionDescription helper
- `cmd/tsuku/install.go`: Added --dry-run flag and runDryRun function
- `cmd/tsuku/update.go`: Added --dry-run flag

## Features
- `tsuku install <tool> --dry-run` shows planned actions without executing
- `tsuku update <tool> --dry-run` shows planned update without executing
- Output format:
  ```
  Would install: <tool>@<version>
    Dependencies: <list or none>
    Actions:
      1. <action>: <description>
      2. ...
    Verification: <command>
  ```
- No filesystem changes when --dry-run is set

## Action Descriptions
The formatActionDescription function provides useful details for each action:
- download: shows URL
- extract: shows source file
- install_binaries: lists binary names
- chmod: shows file and mode
- cargo/npm/pipx/gem_install: shows package name
- run_command: shows command (truncated if long)

## Testing
- All unit tests pass
- Manual verification: flags show in --help output
- CI: existing tests should pass (no behavior change for normal operations)
