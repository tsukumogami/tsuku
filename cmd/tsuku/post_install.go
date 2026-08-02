package main

import (
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
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
