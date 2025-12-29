# Issue 732 - Completion Summary

## Issue
Fix circular dependency false positive when installing git-source

## Root Cause
The circular dependency check in `installWithDependencies` (install_deps.go:226-230) happened BEFORE checking if a tool was already installed (lines 251-291). This caused shared dependencies to be incorrectly flagged as circular.

Example scenario:
- `git-source` depends on: `curl`, `openssl`
- `curl` depends on: `openssl`
- After `curl` installed `openssl`, when `git-source` tried to install `openssl`, it was already in the visited map and triggered a false positive

## Solution
Reordered the logic in `installWithDependencies` to:
1. Check if tool is already installed FIRST
2. If installed and this is a dependency install, update state and return early WITHOUT marking as visited
3. Only then check for circular dependencies and mark as visited

This allows shared dependencies to be recognized as already installed without triggering circular dependency detection.

## Changes Made

### Code Changes
- **cmd/tsuku/install_deps.go** (installWithDependencies function)
  - Moved "already installed" check before circular dependency check
  - Updated early return logic to skip marking visited for already-installed dependencies
  - Only mark tools as visited when they're about to be processed

### Tests Added
- **cmd/tsuku/dependency_test.go**
  - Added `TestDependencyResolution_SharedDependency`
  - Tests the exact scenario from issue #732: git-source → curl → openssl
  - Verifies shared dependencies don't trigger false positives

## Test Results
- All existing tests continue to pass (23 packages)
- New test passes, validating the fix
- `TestCircularDependencyDetection` still correctly detects actual circular dependencies

## Verification
The fix ensures:
- Shared dependencies are recognized as already installed
- Actual circular dependencies (A → B → A) are still detected
- No regression in dependency resolution logic

## Commits
1. `53e51a2` - chore: document issue 732 baseline
2. `f4b5e8d` - chore: add implementation plan for issue 732
3. `a8c9f2b` - fix: reorder dependency checks to prevent false positives
4. `2225d10` - test: add shared dependency resolution test

## Next Steps
- Create PR with fix
- Re-enable git-source test in Build Essentials workflow (if disabled)
- Monitor CI to ensure all checks pass
