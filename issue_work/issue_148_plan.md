# Issue 148 Implementation Plan

## Problem

The verification enforcement for directory-mode installs only applies to composite
actions (`github_archive`, `download_archive`). A recipe could bypass verification
by calling `install_binaries` directly with `install_mode="directory"`.

## Solution

Add the same verification check from `composites.go` to `install_binaries.go`
for defense in depth.

## Implementation Steps

### 1. Add verification check to install_binaries.go

Location: `internal/actions/install_binaries.go`, in `Execute()` method

Add after parsing install_mode (line 43), before the switch statement:

```go
// Enforce verification for directory-based installs (defense in depth)
verifyCmd := strings.TrimSpace(ctx.Recipe.Verify.Command)
if (installMode == "directory" || installMode == "directory_wrapped") && verifyCmd == "" {
    return fmt.Errorf("recipes with install_mode='%s' must include a [verify] section with a command to ensure the installation works correctly", installMode)
}
```

### 2. Add tests for new behavior

Location: `internal/actions/install_binaries_test.go`

Add test function similar to `TestGitHubArchiveAction_VerificationEnforcement`:

Test cases:
- "binaries" mode without verify (allowed)
- "binaries" mode with verify (allowed)
- "directory" mode without verify (blocked)
- "directory" mode with verify (allowed)
- "directory_wrapped" mode without verify (blocked)
- "directory_wrapped" mode with verify (allowed)

## Files to Modify

1. `internal/actions/install_binaries.go` - Add verification check
2. `internal/actions/install_binaries_test.go` - Add tests

## Acceptance Criteria

- [ ] install_binaries action rejects directory-mode installs without verification
- [ ] install_binaries action rejects directory_wrapped-mode installs without verification
- [ ] install_binaries action allows binaries-mode installs without verification
- [ ] All existing tests pass
- [ ] New tests added for verification enforcement
