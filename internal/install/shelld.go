package install

import (
	"strings"

	"github.com/tsukumogami/tsuku/internal/shellenv"
)

// shellDPrefix is the $TSUKU_HOME-relative directory every shell fragment
// lives in. Cleanup paths use forward slashes regardless of platform.
const shellDPrefix = "share/shell.d/"

// BuildShellDSelection projects every installed tool's recorded cleanup actions
// onto share/shell.d: the active version's fragments land in Active, every
// version's fragments land in Known. Pure -- it reads state and nothing else.
//
// The projection is keyed on recorded paths rather than on a naming convention,
// so a legacy record (share/shell.d/nvm.bash) and a version-keyed one
// (share/shell.d/nvm@0.40.7.bash) are indistinguishable to it. A legacy file
// whose version is active is in Active and is sourced exactly as before.
func BuildShellDSelection(state *State) shellenv.ShellDSelection {
	sel := shellenv.ShellDSelection{
		Active: make(map[string]string),
		Known:  make(map[string]string),
	}
	if state == nil {
		return sel
	}

	for _, ts := range state.Installed {
		activeVersion := ts.ActiveVersion
		if activeVersion == "" {
			activeVersion = ts.Version // legacy state files
		}
		for version, vs := range ts.Versions {
			for _, ca := range vs.CleanupActions {
				if !strings.HasPrefix(ca.Path, shellDPrefix) {
					continue
				}
				sel.Known[ca.Path] = ca.ContentHash
				if version == activeVersion {
					sel.Active[ca.Path] = ca.ContentHash
				}
			}
		}
	}

	return sel
}

// ShellDSelection loads state and projects it. Callers that already hold a
// state snapshot should use BuildShellDSelection directly; this exists so the
// lifecycle hooks cannot forget to rebuild the projection after a state write.
//
// A read failure yields the zero selection, which excludes nothing. That is the
// pre-version-key behavior: a cache that sources a fragment it should not is
// recoverable, one that drops a working integration is not.
func (m *Manager) ShellDSelection() shellenv.ShellDSelection {
	state, err := m.state.Load()
	if err != nil {
		return shellenv.ShellDSelection{}
	}
	return BuildShellDSelection(state)
}

// shellsRecordedFor returns the shells any version of the tool records a
// shell.d fragment for. Switching the active version changes which fragment
// belongs in the cache for exactly these shells.
func shellsRecordedFor(ts ToolState) map[string]bool {
	shells := make(map[string]bool)
	for _, vs := range ts.Versions {
		for _, ca := range vs.CleanupActions {
			if shell := shellFromCleanupPath(ca.Path); shell != "" {
				shells[shell] = true
			}
		}
	}
	return shells
}

// rebuildShellCachesForTool rebuilds the caches of every shell the named tool
// has a recorded fragment for, against current state. Call it after the state
// write that changes the active version -- the selection is derived from state,
// so a rebuild that runs first sees the old world.
func (m *Manager) rebuildShellCachesForTool(name string, extraShells map[string]bool) {
	state, err := m.state.Load()
	if err != nil {
		return
	}

	shells := make(map[string]bool, len(extraShells))
	for shell := range extraShells {
		shells[shell] = true
	}
	if ts, ok := state.Installed[name]; ok {
		for shell := range shellsRecordedFor(ts) {
			shells[shell] = true
		}
	}
	if len(shells) == 0 {
		return
	}

	m.rebuildShellCaches(shells, BuildShellDSelection(state))
}
