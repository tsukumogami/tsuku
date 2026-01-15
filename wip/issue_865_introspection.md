# Issue 865 Introspection

## Context Reviewed
- Design doc: `docs/designs/DESIGN-cask-support.md`
- Sibling issues reviewed: #862 (walking skeleton - CLOSED), #863 (cask version provider - CLOSED/MERGED via #882), #864 (DMG extraction - CLOSED), #873 (tap cache - CLOSED), #882 (full cask provider - MERGED), #883 (tap provider - MERGED)
- Prior patterns identified:
  - `AppsDir` field added to `ExecutionContext` in `internal/actions/action.go`
  - `app_bundle.go` implements ZIP and DMG extraction but NOT binaries/symlinks
  - Current `remove.go` handles tool symlinks via `removeToolEntirely()` but has no awareness of apps or `~/Applications`
  - Current `list.go` has `--all` (libraries) and `--show-system-dependencies` but no `--apps` flag
  - State tracking uses `ToolState` with `Binaries` field but no `IsApp` or app-specific fields

## Gap Analysis

### Minor Gaps

1. **Follow existing symlink pattern**: The `createSymlinksForBinaries()` method in `internal/install/manager.go` establishes the pattern for symlink creation. The `binaries` parameter implementation should follow this pattern.

2. **DMG extraction fully implemented**: #864 was completed - `extractDMG()` function exists in `app_bundle.go` with proper `hdiutil` handling. Issue #865 can build on this directly.

3. **AppsDir already configured**: The `ExecutionContext` already has `AppsDir` field (from #862). No additional context changes needed.

4. **IMPLEMENTATION_CONTEXT.md exists**: A `wip/IMPLEMENTATION_CONTEXT.md` file was already created with relevant design context for this issue.

### Moderate Gaps

1. **State tracking for apps not specified**: The issue mentions `tsuku list --apps` but doesn't specify how apps are distinguished from tools in state. Need to decide:
   - Option A: Add `IsApp bool` field to `ToolState`
   - Option B: Use presence of app path (`$TSUKU_HOME/apps/`) as indicator
   - Option C: Add separate `Apps` map in `State` (parallel to `Libs`)

   **Proposed amendment**: Use Option B (path-based detection) for simplicity - apps are identified by having an installed path under `$TSUKU_HOME/apps/`. This avoids state migration and matches how libraries are currently tracked.

2. **Remove cleanup for ~/Applications symlink**: The issue mentions "Handle removal: `tsuku remove` cleans up all symlinks (bin and Applications)" but doesn't specify how to track which applications have `~/Applications` symlinks. Need to decide:
   - Option A: Track symlink path in `VersionState`
   - Option B: Assume symlink exists and attempt removal (ignore errors)
   - Option C: Derive symlink name from recipe's `app_name` parameter (stored in state)

   **Proposed amendment**: Use Option A - add `ApplicationSymlink string` field to `VersionState` to track the exact path created. This enables reliable cleanup.

3. **Version switching for app symlinks**: The issue mentions "Handle version switching: updating to new version updates symlinks to point to new .app" but the current `app_bundle` action returns after copying the `.app` - there's no post-install hook to update version symlinks. Need to clarify how version switching integrates with the install flow.

   **Proposed amendment**: Add symlink management to `app_bundle.Execute()` directly (like how `install_binaries` action creates symlinks). The action should:
   - Create `~/Applications/<app_name>` pointing to the installed `.app`
   - Create `$TSUKU_HOME/bin/<binary>` symlinks for each entry in `binaries`
   - Record created symlinks in the execution result for state tracking

### Major Gaps

None identified. The issue acceptance criteria align with the design and prior implementation work.

## Recommendation
**Proceed** with moderate amendments

The issue is implementable, but three moderate gaps need confirmation before starting implementation:

1. How to identify apps in `tsuku list --apps` (recommend path-based detection)
2. How to track `~/Applications` symlinks for removal (recommend `ApplicationSymlink` field in state)
3. Where symlink creation happens (recommend in `app_bundle.Execute()` directly)

## Proposed Amendments

Based on review of completed work in this milestone:

1. **App identification**: Apps are identified by having installed versions under `$TSUKU_HOME/apps/` (no new state field needed)

2. **Symlink tracking**: Add `ApplicationSymlink string` field to `VersionState` to track `~/Applications` symlinks for cleanup

3. **Symlink creation location**: Implement binary and application symlinks within `app_bundle.Execute()` - this keeps all app installation logic in one place

4. **Remove handling**: Extend `removeToolEntirely()` to check for and remove `~/Applications` symlinks when removing apps

5. **State recording**: The `app_bundle` action needs to communicate installed binaries and symlinks back to the installer for state tracking. Follow the pattern used by other actions that create binaries.

## Files to Modify

Based on analysis:
- `internal/actions/app_bundle.go` - Add `binaries` and `symlink_applications` parameter handling
- `internal/install/state.go` - Add `ApplicationSymlink` field to `VersionState`
- `internal/install/remove.go` - Handle `~/Applications` symlink cleanup
- `internal/install/list.go` - Add `ListApps()` method and `AppsDir` support
- `cmd/tsuku/list.go` - Add `--apps` flag
- Tests for all modified files
