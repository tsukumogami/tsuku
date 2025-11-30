# Issue #19 Implementation Plan

## Overview
Define distinct exit codes for different error types to enable scripts to distinguish between failure modes.

## Exit Code Definitions

| Code | Constant | Meaning |
|------|----------|---------|
| 0 | ExitSuccess | Success |
| 1 | ExitGeneral | General error |
| 2 | ExitUsage | Invalid arguments / usage error |
| 3 | ExitRecipeNotFound | Recipe not found |
| 4 | ExitVersionNotFound | Version not found |
| 5 | ExitNetwork | Network error |
| 6 | ExitInstallFailed | Installation failed |
| 7 | ExitVerifyFailed | Verification failed |
| 8 | ExitDependencyFailed | Dependency resolution failed |

## Implementation Strategy

### 1. Create exit codes package
Create new file `cmd/tsuku/exitcodes.go`:
- Define constants for each exit code
- Create `exitWithCode(code int)` helper function
- Replace `os.Exit(1)` calls with appropriate exit codes

### 2. Categorize existing exit calls
Based on error context in each file:
- **ExitUsage (2)**: Cobra handles this automatically
- **ExitRecipeNotFound (3)**: Recipe/tool not found errors
- **ExitVersionNotFound (4)**: Version resolution failures
- **ExitNetwork (5)**: API/download failures
- **ExitInstallFailed (6)**: Installation step failures
- **ExitVerifyFailed (7)**: Verification command failures
- **ExitDependencyFailed (8)**: Dependency resolution failures
- **ExitGeneral (1)**: Config errors, state errors, other

### 3. Files to Update
- `cmd/tsuku/exitcodes.go` - NEW: Define exit codes
- `cmd/tsuku/main.go` - General errors, config errors
- `cmd/tsuku/install.go` - Recipe not found, installation failed
- `cmd/tsuku/verify.go` - Verification failed
- `cmd/tsuku/versions.go` - Version not found
- `cmd/tsuku/update.go` - Similar to install
- `cmd/tsuku/remove.go` - General errors
- `cmd/tsuku/list.go` - Config/state errors
- `cmd/tsuku/recipes.go` - Registry errors
- `cmd/tsuku/outdated.go` - Config/state errors
- `cmd/tsuku/search.go` - Config errors
- `cmd/tsuku/create.go` - Ecosystem/package errors
- `cmd/tsuku/config.go` - Config errors
- `cmd/tsuku/update_registry.go` - Cache errors
- `cmd/tsuku/helpers.go` - JSON encoding errors

### 4. Approach
1. Create exitcodes.go with constants and helper
2. Update each file to use specific exit codes
3. Keep os.Exit(1) as fallback for truly general errors
4. Run tests to ensure no regressions

### 5. Documentation
Exit codes will be self-documenting through the constants.
No need for help output changes per the "avoid over-engineering" principle.

## File Changes Summary
- 1 new file: `cmd/tsuku/exitcodes.go`
- 13 files to update with specific exit codes
