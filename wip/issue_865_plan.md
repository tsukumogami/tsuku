# Issue 865 Implementation Plan

## Summary

Add binary symlinks and ~/Applications integration for cask-based applications. This completes the cask installation experience by exposing CLI tools to PATH and enabling macOS Spotlight/Launchpad discovery.

## Files to Modify

| File | Change Type | Purpose |
|------|-------------|---------|
| `internal/config/config.go` | Modify | Add `AppsDir` field |
| `internal/config/config_test.go` | Modify | Test new AppsDir field |
| `internal/testutil/testutil.go` | Modify | Add AppsDir to test config |
| `internal/install/state.go` | Modify | Add `AppPath` and `ApplicationSymlink` to VersionState |
| `internal/actions/app_bundle.go` | Modify | Add binaries and symlink_applications support |
| `internal/actions/app_bundle_test.go` | New | Add tests for new parameters |
| `internal/install/list.go` | Modify | Add ListApps() method |
| `internal/install/list_test.go` | Modify | Add tests for ListApps |
| `internal/install/remove.go` | Modify | Clean up ~/Applications symlinks on remove |
| `internal/install/remove_test.go` | Modify | Add tests for app removal |
| `cmd/tsuku/list.go` | Modify | Add --apps flag |

## Implementation Steps

### Step 1: Add AppsDir to Config

Add `AppsDir` field to Config struct for consistent path management:

```go
// In config.go Config struct:
AppsDir string // $TSUKU_HOME/apps (macOS application bundles)

// In DefaultConfig():
AppsDir: filepath.Join(tsukuHome, "apps"),

// In EnsureDirectories():
c.AppsDir,
```

Update testutil.NewTestConfig() to include AppsDir.

### Step 2: Extend VersionState for App Tracking

Add fields to track app-specific state:

```go
// In state.go VersionState struct:
AppPath            string `json:"app_path,omitempty"`             // Path to installed .app bundle (e.g., $TSUKU_HOME/apps/foo-1.0.app)
ApplicationSymlink string `json:"application_symlink,omitempty"`  // Path to ~/Applications symlink if created
```

This enables:
- Identifying apps via presence of `AppPath`
- Reliable cleanup of ~/Applications symlinks on removal

### Step 3: Implement Binary Symlinks in app_bundle

Modify `app_bundle.Execute()` to:

1. Parse `binaries` parameter ([]string of paths within .app bundle)
2. For each binary path, create symlink in `$TSUKU_HOME/tools/current/`:
   - Target: `$TSUKU_HOME/apps/<name>-<version>.app/<binary-path>`
   - Link: `$TSUKU_HOME/tools/current/<binary-basename>`

Example:
```
binaries = ["Contents/Resources/app/bin/code"]
→ Symlink: ~/.tsuku/tools/current/code → ~/.tsuku/apps/visual-studio-code-1.85.0.app/Contents/Resources/app/bin/code
```

### Step 4: Implement ~/Applications Symlink

Add optional `symlink_applications` parameter (default: true):

1. If enabled, create symlink in ~/Applications:
   - Target: `$TSUKU_HOME/apps/<name>-<version>.app`
   - Link: `~/Applications/<app_name>` (from parameter)

2. Track symlink path in execution result for state recording

### Step 5: Update ExecutionResult for App State

app_bundle needs to communicate created symlinks back to the installer:

```go
// Extend ExecutionResult or add new struct:
type AppInstallResult struct {
    AppPath            string   // Installed .app path
    ApplicationSymlink string   // ~/Applications symlink path (if created)
    Binaries           []string // Symlinked binary names
}
```

### Step 6: Implement ListApps() in Manager

Add method to list only applications:

```go
func (m *Manager) ListApps() ([]InstalledApp, error) {
    state, err := m.state.Load()
    // Filter: only tools where any version has non-empty AppPath
    // Return: name, version, app path, is_active
}
```

### Step 7: Add --apps Flag to CLI

Modify `cmd/tsuku/list.go`:

```go
showApps, _ := cmd.Flags().GetBool("apps")
if showApps {
    apps, err = mgr.ListApps()
    // Display apps only
}
```

### Step 8: Handle App Removal

Extend `removeToolEntirely()` to:

1. Check each version's `ApplicationSymlink`
2. If non-empty, remove the ~/Applications symlink
3. Also remove app directory from `$TSUKU_HOME/apps/`

### Step 9: Handle Version Switching

When switching active app version:

1. Update binary symlinks to point to new version's binaries
2. Update ~/Applications symlink to point to new version's .app

## Design Decisions

1. **Symlink location for binaries**: Use existing `$TSUKU_HOME/tools/current/` to keep all tool binaries in one place. Apps are stored separately in `$TSUKU_HOME/apps/` but their CLI tools appear alongside other tools.

2. **State tracking**: Use `AppPath` field presence to identify apps rather than a separate `IsApp` bool - more informative and avoids state migration.

3. **~/Applications default**: Enable by default (`symlink_applications = true`) since this matches user expectations for macOS apps.

4. **Error handling**: ~/Applications symlink failure should warn but not fail the install - the app is still usable without Spotlight integration.

## Testing Strategy

1. **Unit tests for app_bundle**: Test binary extraction from params, symlink creation logic
2. **Unit tests for list**: Test ListApps() filtering
3. **Unit tests for remove**: Test ~/Applications cleanup
4. **Integration**: Manual test with a real cask recipe

## Risks and Mitigations

1. **Risk**: Binary paths within .app may not exist
   - **Mitigation**: Validate binary paths exist before creating symlinks

2. **Risk**: ~/Applications permission denied
   - **Mitigation**: Warn and continue; don't fail the install

3. **Risk**: Symlink name collisions
   - **Mitigation**: Check for existing symlinks, overwrite if managed by tsuku
