package main

import (
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/datamigration"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/shellenv"
)

// warnFunc reports a non-fatal problem. The two install paths print through
// different machinery -- a progress reporter and plain stdout -- so the shared
// helper takes the printer rather than picking one.
type warnFunc func(format string, args ...interface{})

// finishPostInstall records the cleanup actions a post-install phase produced
// and rebuilds the shell caches those actions touched.
//
// Both the recipe-driven install and `tsuku install --plan` need exactly this,
// and they had already drifted: the plan path rebuilt the cache but never
// recorded anything, so its shell.d files were orphaned -- invisible to remove,
// to doctor's hash check, and to the active-version projection.
//
// Recording comes first. The projection that decides which fragments belong in
// the cache is read back out of state, so a rebuild that runs before the write
// cannot see the files this install just produced.
func finishPostInstall(cfg *config.Config, mgr *install.Manager, toolName string, cleanup []actions.CleanupAction, warnf warnFunc) {
	// The migration itself ran inside set_env, in the same process that rewrote the
	// export. All that is left here is to tell the user if any of it did not land.
	if toolName == datamigration.NvmTool {
		reportPendingDataMigration(cfg)
	}

	if len(cleanup) == 0 {
		return
	}

	if err := mgr.RecordCleanup(toolName, convertCleanupActions(cleanup)); err != nil {
		warnf("failed to record cleanup actions: %v", err)
	}

	affectedShells := make(map[string]bool)
	for _, ca := range cleanup {
		if shell := install.ShellFromCleanupPath(ca.Path); shell != "" {
			affectedShells[shell] = true
		}
	}
	if len(affectedShells) == 0 {
		return
	}

	selection := mgr.ShellDSelection()
	for shell := range affectedShells {
		if err := shellenv.RebuildShellCache(cfg.HomeDir, shell, selection); err != nil {
			warnf("failed to rebuild shell cache for %s: %v", shell, err)
		}
	}
}

// activeCleanupPaths returns the cleanup paths recorded by the tool's active
// version. Other versions' paths describe files that exist but are not the ones
// the user's shell sources.
func activeCleanupPaths(ts *install.ToolState) []string {
	if ts == nil {
		return nil
	}
	activeVersion := ts.ActiveVersion
	if activeVersion == "" {
		activeVersion = ts.Version // legacy state files
	}
	vs, ok := ts.Versions[activeVersion]
	if !ok {
		return nil
	}
	paths := make([]string, 0, len(vs.CleanupActions))
	for _, ca := range vs.CleanupActions {
		paths = append(paths, ca.Path)
	}
	return paths
}

// reportPendingDataMigration writes a notice when a tool's data could not be moved out
// of a legacy location.
//
// Detection is by shape rather than by anything the migration handed back, so this needs
// no state threaded out of the action layer -- and no notices directory on the execution
// context, which would mean widening the most-constructed struct in the tree and adding
// an actions -> notices dependency for one tool.
//
// Nothing is written on success. A migration that worked has nothing to tell anyone:
// `nvm ls` lists what it always did.
//
// The notice is keyed on its own name rather than the tool's, because WriteNotice writes
// <Tool>.json and the install that triggers this writes notices/nvm.json twice in the
// same tick. It carries a non-empty Error deliberately: that is what keeps it out of
// renderBackgroundSuccess, which drops Messages entirely, and out of the sweep that
// deletes error-free notices.
func reportPendingDataMigration(cfg *config.Config) {
	sources := datamigration.FindNvmSources(cfg.HomeDir)
	if len(sources) == 0 {
		return
	}

	dataDir := datamigration.NvmDataDir(cfg.HomeDir)
	messages := []string{
		"Your Node versions and npm packages are still in:",
	}
	for _, s := range sources {
		messages = append(messages, "  "+s.Dir)
	}
	messages = append(messages,
		"They belong in "+dataDir+".",
		"Run 'tsuku doctor --fix' to retry, or move them by hand with mv.",
	)

	_ = notices.WriteNotice(cfg.HomeDir, &notices.Notice{
		Tool:     dataMigrationNoticeName,
		Kind:     notices.KindDataMigration,
		Error:    "nvm data was not moved to " + dataDir,
		Messages: messages,
	})
}

// dataMigrationNoticeName keys the migration notice. Not a tool name: notices are
// stored as <Tool>.json, and an install writes the tool's own notice twice in the same
// tick, which would clobber this one.
const dataMigrationNoticeName = "nvm-data-migration"
