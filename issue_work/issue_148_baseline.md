# Issue 148 Baseline

## Environment
- Date: 2025-11-30
- Branch: fix/148-install-binaries-verification
- Base commit: e7eb48b58fa924ab103508e6fab2538f513260b9

## Test Results
- Total: 17 packages
- Passed: All
- Failed: 0

## Build Status
Pass - no warnings

## Pre-existing Issues
None - all tests pass, build succeeds

## Issue Details

### Problem
The verification enforcement for directory-mode installs only applies to composite
actions (`github_archive`, `download_archive`). A recipe could bypass verification
by calling `install_binaries` directly with `install_mode="directory"`.

### Required Fix
Add verification check to `install_binaries` action itself (defense in depth).
When `install_mode` is `directory` or `directory_wrapped`, require a non-empty
`[verify]` section.

### Risk Level
MEDIUM - requires manual recipe crafting, but violates defense-in-depth principle.

## Existing Verification Pattern

The pattern to copy from `composites.go`:

```go
verifyCmd := strings.TrimSpace(ctx.Recipe.Verify.Command)
if (installMode == "directory" || installMode == "directory_wrapped") && verifyCmd == "" {
    return fmt.Errorf("recipes with install_mode='%s' must include a [verify] section...", installMode)
}
```

This check exists in:
- `DownloadArchiveAction.Execute()` (line 47)
- `GitHubArchiveAction.Execute()` (line 190)

Target file: `internal/actions/install_binaries.go`
