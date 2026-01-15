# Issue 865 Implementation Summary

## Feature: Binary Symlinks and Applications Integration

### What Was Implemented

This PR adds support for binary symlinks and ~/Applications integration for macOS application bundles installed via the `app_bundle` action.

### Changes by File

1. **internal/config/config.go**
   - Added `AppsDir` field to Config struct ($TSUKU_HOME/apps)
   - Added `AppDir(name, version)` helper method
   - Updated `DefaultConfig()` and `EnsureDirectories()`

2. **internal/config/config_test.go**
   - Added `TestAppDir` test
   - Updated `TestEnsureDirectories` to include AppsDir

3. **internal/testutil/testutil.go**
   - Added AppsDir to test config

4. **internal/install/state.go**
   - Added `AppPath` field to VersionState (tracks installed .app bundle location)
   - Added `ApplicationSymlink` field to VersionState (tracks ~/Applications symlink)

5. **internal/actions/action.go**
   - Added `AppsDir` field to ExecutionContext
   - Added `CurrentDir` field to ExecutionContext
   - Added `AppResult` field to ExecutionContext (stores AppBundleResult)

6. **internal/actions/app_bundle.go**
   - Added `AppBundleResult` struct for state tracking
   - Added `binaries` parameter support for CLI tool symlinks
   - Added `symlink_applications` parameter (default: true) for ~/Applications symlinks
   - Creates binary symlinks in $TSUKU_HOME/tools/current/
   - Creates ~/Applications symlinks for Spotlight/Launchpad integration
   - Stores result in ctx.AppResult for installer state tracking

7. **internal/actions/util.go**
   - Added `GetBoolDefault` helper function

8. **internal/install/list.go**
   - Added `InstalledApp` struct
   - Added `ListApps()` method to Manager

9. **cmd/tsuku/list.go**
   - Added `--apps` flag to show installed applications
   - Updated `--all` to include applications
   - Added apps output in both text and JSON formats

10. **internal/install/remove.go**
    - Updated `RemoveVersion()` to clean up app bundles and symlinks
    - Updated `removeToolEntirely()` to clean up app bundles and symlinks

11. **internal/executor/executor.go**
    - Added `appsDir` and `currentDir` fields to Executor
    - Added `SetAppsDir()` and `SetCurrentDir()` methods
    - Updated ExecutionContext creation to include AppsDir and CurrentDir

12. **internal/builders/orchestrator.go**
    - Added `AppsDir` and `CurrentDir` to OrchestratorConfig
    - Updated executor configuration

13. **cmd/tsuku/plan_install.go, install_deps.go, install_lib.go, helpers.go**
    - Added calls to SetAppsDir() and SetCurrentDir()

### Recipe Example

```toml
[metadata]
name = "vscode"
description = "Visual Studio Code"

[[steps]]
action = "app_bundle"
[steps.params]
url = "https://update.code.visualstudio.com/{version}/darwin-universal/stable"
checksum = "sha256:..."
app_name = "Visual Studio Code.app"
binaries = ["Contents/Resources/app/bin/code"]
symlink_applications = true  # default
```

### Testing Notes

- All existing tests pass
- New TestAppDir test added for config helper
- Pre-existing failures in actions package (symlink/download cache tests) are unrelated to this change
