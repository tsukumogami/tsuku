package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/installevents"
	"github.com/tsukumogami/tsuku/internal/shellenv"
)

// Remove removes an installed tool (legacy method - removes active version only).
// Source is read from ctx via installevents.SourceFromContext; callers must
// wrap ctx with installevents.WithSource before invoking.
//
// Deprecated: Use RemoveVersion or RemoveAllVersions instead.
func (m *Manager) Remove(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	src := installevents.SourceFromContext(ctx)

	// 1. Find installed version
	tools, err := m.List()
	if err != nil {
		return fmt.Errorf("failed to list installed tools: %w", err)
	}

	var version string
	for _, tool := range tools {
		if tool.Name == name {
			version = tool.Version
			break
		}
	}

	if version == "" {
		err := fmt.Errorf("tool %s is not installed", name)
		m.publishRemoveOutcome(name, "", "", src, err)
		return err
	}

	// 2. Remove tool directory
	toolDir := m.config.ToolDir(name, version)
	if err := os.RemoveAll(toolDir); err != nil {
		err = fmt.Errorf("failed to remove tool directory: %w", err)
		m.publishRemoveOutcome(name, version, "", src, err)
		return err
	}

	// 3. Remove symlink if it points to this tool
	symlinkPath := m.config.CurrentSymlink(name)
	if _, err := os.Lstat(symlinkPath); err == nil {
		if err := os.Remove(symlinkPath); err != nil {
			err = fmt.Errorf("failed to remove symlink: %w", err)
			m.publishRemoveOutcome(name, version, "", src, err)
			return err
		}
	}

	m.publishRemoveOutcome(name, version, "", src, nil)
	return nil
}

// publishRemoveOutcome publishes Removed (success) or RemoveFailed
// (failure) on m.bus. publish-after-state invariant: callers invoke
// this AFTER state-mutating work has either committed or failed.
func (m *Manager) publishRemoveOutcome(name, version, activeAfter string, src installevents.Source, err error) {
	if m.bus == nil {
		return
	}
	now := time.Now()
	if err != nil {
		m.bus.Publish(installevents.RemoveFailed{
			Tool:             name,
			AttemptedVersion: version,
			Err:              err,
			Source:           src,
			Timestamp:        now,
		})
		return
	}
	m.bus.Publish(installevents.Removed{
		Tool:        name,
		Version:     version,
		ActiveAfter: activeAfter,
		Source:      src,
		Timestamp:   now,
	})
}

// RemoveVersion removes a specific version of a tool.
// If the removed version was active, switches to the most recently installed remaining version.
// If this was the last version, removes the tool entirely from state.
// Source is read from ctx via installevents.SourceFromContext; callers must
// wrap ctx with installevents.WithSource before invoking.
func (m *Manager) RemoveVersion(ctx context.Context, name, version string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	src := installevents.SourceFromContext(ctx)
	defer func() {
		// publish-after-state: state mutations above have either committed
		// or failed before this defer runs. ActiveAfter is recomputed
		// from current state so a successful Remove that switched the
		// active version reports the new active.
		var activeAfter string
		if err == nil {
			if ts, _ := m.state.GetToolState(name); ts != nil {
				activeAfter = ts.ActiveVersion
			}
		}
		m.publishRemoveOutcome(name, version, activeAfter, src, err)
	}()

	// Validate version string to prevent path traversal attacks
	if err := ValidateVersionString(version); err != nil {
		return fmt.Errorf("invalid version: %w", err)
	}

	// Get tool state
	toolState, err := m.state.GetToolState(name)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}
	if toolState == nil {
		return fmt.Errorf("tool %q is not installed", name)
	}

	// Check if version exists
	versionState, exists := toolState.Versions[version]
	if !exists {
		return m.versionNotInstalledError(name, version, toolState)
	}

	// Execute cleanup actions before removing the directory.
	// Collect other versions' cleanup paths so we don't delete shared resources.
	// Shell caches are rebuilt at the end of this function rather than here:
	// removing the active version promotes another one, and a rebuild that runs
	// before the promotion sees the pre-promotion world.
	affectedShells := m.executeCleanupActions(ctx, name, version, versionState.CleanupActions, toolState)

	// Remove version directory
	toolDir := m.config.ToolDir(name, version)
	if err := os.RemoveAll(toolDir); err != nil {
		return fmt.Errorf("failed to remove tool directory: %w", err)
	}

	// Remove .app bundle and ~/Applications symlink if this was an app
	if versionState.AppPath != "" {
		_ = os.RemoveAll(versionState.AppPath)
	}
	if versionState.ApplicationSymlink != "" {
		_ = os.Remove(versionState.ApplicationSymlink)
	}

	// Check if this was the last version
	if len(toolState.Versions) == 1 {
		// Last version - remove entire tool
		if err := m.removeToolEntirely(name, toolState); err != nil {
			return err
		}
		m.rebuildShellCachesForTool(name, affectedShells)
		return nil
	}

	// Track if we need to switch active version
	wasActive := toolState.ActiveVersion == version
	var newActiveVersion string

	// Update state - remove version from map
	err = m.state.UpdateTool(name, func(ts *ToolState) {
		delete(ts.Versions, version)
		if wasActive {
			// Switch to most recently installed remaining version
			newActiveVersion = getMostRecentVersion(ts.Versions)
			ts.ActiveVersion = newActiveVersion
			// Update legacy fields
			ts.Version = newActiveVersion
			if vs, ok := ts.Versions[newActiveVersion]; ok {
				ts.Binaries = vs.Binaries
			}
		}
	})
	if err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}

	// If active version was removed, update symlinks to point to new active version
	if wasActive && newActiveVersion != "" {
		// Reload state to get binaries for new active version
		toolState, err = m.state.GetToolState(name)
		if err != nil {
			return fmt.Errorf("failed to reload state: %w", err)
		}
		binaries := toolState.Versions[newActiveVersion].Binaries
		if len(binaries) == 0 {
			binaries = []string{filepath.Join("bin", name)}
		}
		if err := m.createSymlinksForBinaries(ctx, name, newActiveVersion, binaries); err != nil {
			return fmt.Errorf("failed to update symlinks: %w", err)
		}
	}

	// Rebuild against the promoted world. This covers both the shells whose
	// files were just deleted and the shells the newly active version owns a
	// fragment for -- the promotion is what makes that fragment the one to
	// source, and nothing else would notice.
	m.rebuildShellCachesForTool(name, affectedShells)

	return nil
}

// RemoveAllVersions removes all versions of a tool.
// Source is read from ctx via installevents.SourceFromContext; callers must
// wrap ctx with installevents.WithSource before invoking.
func (m *Manager) RemoveAllVersions(ctx context.Context, name string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	src := installevents.SourceFromContext(ctx)
	defer func() {
		// publish-after-state: state has either been fully cleared or
		// left intact. ActiveAfter is empty on success (tool fully gone).
		m.publishRemoveOutcome(name, "", "", src, err)
	}()
	// Get tool state
	toolState, err := m.state.GetToolState(name)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}
	if toolState == nil {
		return fmt.Errorf("tool %q is not installed", name)
	}

	// Execute cleanup actions for all versions. When removing all versions,
	// no cross-version safety check is needed -- everything goes.
	allShells := make(map[string]bool)
	for _, vs := range toolState.Versions {
		for _, ca := range vs.CleanupActions {
			m.executeSingleCleanup(ca)
			if shell := shellFromCleanupPath(ca.Path); shell != "" {
				allShells[shell] = true
			}
		}
	}

	// Remove all version directories
	for version := range toolState.Versions {
		toolDir := m.config.ToolDir(name, version)
		if err := os.RemoveAll(toolDir); err != nil {
			return fmt.Errorf("failed to remove version %s: %w", version, err)
		}
	}

	// Remove symlinks and state
	if err := m.removeToolEntirely(name, toolState); err != nil {
		return err
	}

	// Rebuild shell cache for affected shells, against state with the tool gone
	m.rebuildShellCaches(allShells, m.ShellDSelection())
	return nil
}

// removeToolEntirely removes all symlinks and state for a tool.
func (m *Manager) removeToolEntirely(name string, toolState *ToolState) error {
	// Collect all binaries to remove symlinks
	binaries := make(map[string]bool)

	// From legacy field
	for _, b := range toolState.Binaries {
		binaries[filepath.Base(b)] = true
	}

	// From each version
	for _, vs := range toolState.Versions {
		for _, b := range vs.Binaries {
			binaries[filepath.Base(b)] = true
		}

		// Remove .app bundles and ~/Applications symlinks
		if vs.AppPath != "" {
			_ = os.RemoveAll(vs.AppPath) // Remove the .app bundle
		}
		if vs.ApplicationSymlink != "" {
			_ = os.Remove(vs.ApplicationSymlink) // Remove ~/Applications symlink
		}
	}

	// Fallback to tool name if no binaries found
	if len(binaries) == 0 {
		binaries[name] = true
	}

	// Remove symlinks
	for binaryName := range binaries {
		symlinkPath := m.config.CurrentSymlink(binaryName)
		_ = os.Remove(symlinkPath) // Ignore errors - symlink may not exist
	}

	// Remove from state
	return m.state.RemoveTool(name)
}

// getMostRecentVersion returns the version with the most recent InstalledAt time.
func getMostRecentVersion(versions map[string]VersionState) string {
	var mostRecent string
	var mostRecentTime time.Time

	// Sort keys for deterministic behavior when timestamps are equal
	versionKeys := make([]string, 0, len(versions))
	for v := range versions {
		versionKeys = append(versionKeys, v)
	}
	sort.Strings(versionKeys)

	for _, v := range versionKeys {
		state := versions[v]
		if mostRecent == "" || state.InstalledAt.After(mostRecentTime) {
			mostRecent = v
			mostRecentTime = state.InstalledAt
		}
	}
	return mostRecent
}

// executeCleanupActions runs cleanup actions for a version being removed.
// It skips any cleanup path that another version of the same tool also references
// (multi-version safety). Returns the set of shell names affected for cache rebuild.
// ctx is accepted for future cancellation hooks.
func (m *Manager) executeCleanupActions(ctx context.Context, name, version string, actions []CleanupAction, toolState *ToolState) map[string]bool {
	_ = ctx
	if len(actions) == 0 {
		return nil
	}

	// Build set of cleanup paths owned by other versions of this tool.
	otherPaths := make(map[string]bool)
	for v, vs := range toolState.Versions {
		if v == version {
			continue
		}
		for _, ca := range vs.CleanupActions {
			otherPaths[ca.Path] = true
		}
	}

	affectedShells := make(map[string]bool)
	for _, ca := range actions {
		if shell := shellFromCleanupPath(ca.Path); shell != "" {
			// Mark the shell affected whether or not the file is deleted. A
			// retained path still changes which fragment belongs in the cache,
			// because the version that recorded it is going away.
			affectedShells[shell] = true
		}
		if otherPaths[ca.Path] {
			// Another version still references this path -- skip.
			fmt.Printf("   Cleanup: skipping %s (still referenced by another version)\n", ca.Path)
			continue
		}
		m.executeSingleCleanup(ca)
	}
	return affectedShells
}

// ReapVersion drops a version whose directory has already been deleted out of
// band -- garbage collection removes tool directories directly. It runs that
// version's cleanup actions (skipping anything another version still records),
// removes its VersionState, and rebuilds the affected caches.
//
// It refuses to touch the active version. Callers that delete directories are
// expected to protect it themselves; this is the second lock on the door.
//
// A version that is not in state, or a tool that is not, is a no-op: the
// directory is gone either way and there is nothing left to reconcile.
func (m *Manager) ReapVersion(name, version string) error {
	toolState, err := m.state.GetToolState(name)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}
	if toolState == nil {
		return nil
	}
	if toolState.ActiveVersion == version {
		return fmt.Errorf("refusing to reap the active version %s of %s", version, name)
	}
	versionState, exists := toolState.Versions[version]
	if !exists {
		return nil
	}

	affectedShells := m.executeCleanupActions(context.Background(), name, version, versionState.CleanupActions, toolState)

	if err := m.state.UpdateTool(name, func(ts *ToolState) {
		delete(ts.Versions, version)
		if ts.PreviousVersion == version {
			ts.PreviousVersion = ""
		}
	}); err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}

	m.rebuildShellCachesForTool(name, affectedShells)
	return nil
}

// executeSingleCleanup performs one cleanup action. Failures log a warning
// and never block removal.
func (m *Manager) executeSingleCleanup(ca CleanupAction) {
	absPath := filepath.Join(m.config.HomeDir, ca.Path)

	var err error
	switch ca.Action {
	case "delete_file":
		err = os.Remove(absPath)
	case "delete_dir":
		err = os.RemoveAll(absPath)
	default:
		fmt.Printf("   Warning: unknown cleanup action %q for %s\n", ca.Action, ca.Path)
		return
	}

	if err != nil && !os.IsNotExist(err) {
		fmt.Printf("   Warning: cleanup %s %s failed: %v\n", ca.Action, ca.Path, err)
	}
}

// ShellFromCleanupPath extracts the shell extension from a shell.d cleanup path.
// Returns "" if the path doesn't look like a shell.d file.
func ShellFromCleanupPath(path string) string {
	return shellFromCleanupPath(path)
}

// shellFromCleanupPath is the unexported implementation.
func shellFromCleanupPath(path string) string {
	if !strings.HasPrefix(path, shellDPrefix) {
		return ""
	}
	ext := filepath.Ext(path)
	if ext == "" {
		return ""
	}
	return ext[1:] // strip leading dot
}

// TargetFromCleanupPath extracts the target a shell.d cleanup path was written
// for, dropping the version key and the shell suffix. Returns "" if the path
// doesn't look like a shell.d file.
//
// Comparing two versions' fragments has to go through this: version-keyed
// filenames mean the same target has a different path in every version, so a
// comparison keyed on the raw path sees two unrelated files.
func TargetFromCleanupPath(path string) string {
	shell := shellFromCleanupPath(path)
	if shell == "" {
		return ""
	}
	return shellenv.DisplayName(strings.TrimPrefix(path, shellDPrefix), shell)
}

// rebuildShellCaches rebuilds shell init caches for the given set of shells.
// sel must be projected from state as it stands after the caller's last write,
// or the rebuild sources the version that was active a moment ago.
func (m *Manager) rebuildShellCaches(shells map[string]bool, sel shellenv.ShellDSelection) {
	for shell := range shells {
		if err := shellenv.RebuildShellCache(m.config.HomeDir, shell, sel); err != nil {
			fmt.Printf("   Warning: failed to rebuild shell cache for %s: %v\n", shell, err)
		}
	}
}
