package main

import (
	"github.com/tsukumogami/tsuku/internal/index"
	"github.com/tsukumogami/tsuku/internal/install"
)

// stateReaderAdapter adapts *install.StateManager to the index.StateReader
// interface. internal/index must not import internal/install; this adapter
// lives at the cmd/ layer where both packages are visible.
type stateReaderAdapter struct {
	mgr *install.StateManager
}

// AllTools loads the full installed state and converts each install.ToolState
// to index.ToolInfo.
func (a *stateReaderAdapter) AllTools() (map[string]index.ToolInfo, error) {
	state, err := a.mgr.Load()
	if err != nil {
		return nil, err
	}

	result := make(map[string]index.ToolInfo, len(state.Installed))
	for name, ts := range state.Installed {
		info := index.ToolInfo{
			ActiveVersion: ts.ActiveVersion,
			Source:        ts.Source,
			Versions:      make(map[string]index.VersionInfo, len(ts.Versions)),
		}
		// ts.Binaries (top-level ToolState field) is not used here because it is
		// deprecated. state.migrateToMultiVersion() ensures that Versions[v].Binaries
		// is always populated on load, so we read binary paths from the per-version
		// entries below.
		for ver, vs := range ts.Versions {
			info.Versions[ver] = index.VersionInfo{
				Binaries: vs.Binaries,
			}
		}
		result[name] = info
	}
	return result, nil
}
