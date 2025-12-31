# Issue 764 Implementation Plan

## Goal
Add CLI capability to verify system dependencies (require_command actions) are satisfied for a recipe.

## Approach
Add a new subcommand `tsuku verify-deps <recipe>` that:
1. Loads a recipe and its installation plan
2. Filters the plan for the current target platform
3. Finds all `require_command` steps
4. Executes each to verify the command exists (with optional version check)
5. Reports pass/fail for each
6. Returns exit code 0 if all pass, non-zero otherwise

## Why a new command instead of modifying verify
The existing `verify` command verifies an *installed tool's* integrity (binary checksums, symlinks). This is a different concern - we're verifying *system dependencies* before/after manual installation. A separate command avoids confusion.

## Files to Modify/Create

### New File: `cmd/tsuku/verify_deps.go`
- New `verifyDepsCmd` cobra command
- Uses existing `RequireCommandAction.Execute()` logic
- Filters plan using `executor.FilterPlan()` for current platform
- Collects and reports results

### Update: `docs/REFERENCE.md` (if exists) or create user docs
- Document the new command

## Implementation Details

### Command Structure
```
tsuku verify-deps <recipe>
  --json    Output in JSON format
```

### Algorithm
1. Load recipe via loader.Get(recipeName)
2. Resolve dependencies to get full step list
3. Filter plan by current target using platform.DetectTarget()
4. Find all steps where action = "require_command"
5. For each step, run RequireCommandAction.Execute() or inline logic
6. Collect results (pass/fail/version info)
7. Print results (colorized terminal or JSON)
8. Exit with appropriate code

### Exit Codes
- 0: All require_command checks passed
- 1: One or more checks failed
- Use existing ExitDependencyFailed constant

## Tests

### Unit Tests: `cmd/tsuku/verify_deps_test.go`
- Test with recipe containing require_command steps
- Test with recipe without require_command steps
- Test JSON output format
- Test exit codes

### Integration-style Tests
- Use mock commands in test PATH
- Verify version checking works

## User Documentation
- Add section to README or help text explaining the command
- Examples of usage
