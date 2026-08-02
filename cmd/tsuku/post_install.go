package main

import (
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/shellenv"
)

// warnFunc reports a non-fatal problem. The two install paths print through
// different machinery -- a progress reporter and plain stdout -- so the shared
// helper takes the printer rather than picking one.
type warnFunc func(format string, args ...interface{})

// finishPostInstall records the cleanup actions a post-install phase produced
// against the named version, and rebuilds the shell caches those actions
// touched.
//
// Both the recipe-driven install and `tsuku install --plan` need exactly this,
// and they had already drifted: the plan path rebuilt the cache but never
// recorded anything, so its shell.d files were orphaned -- invisible to remove,
// to doctor's hash check, and to the active-version projection.
//
// The version is explicit rather than resolved from state because dependency
// installs also come through here, and a dependency's version is not
// necessarily the tool's active one.
//
// Recording comes first. The projection that decides which fragments belong in
// the cache is read back out of state, so a rebuild that runs before the write
// cannot see the files this install just produced.
func finishPostInstall(cfg *config.Config, mgr *install.Manager, toolName, version string, cleanup []actions.CleanupAction, warnf warnFunc) {
	if len(cleanup) == 0 {
		return
	}

	if err := mgr.RecordCleanupForVersion(toolName, version, convertCleanupActions(cleanup)); err != nil {
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

// recordDependencyInstalls persists what the executor's own dependency installs
// left behind.
//
// A dependency installed by Executor.installSingleDependency never reaches
// install.Manager, so before this ran it had no state entry at all: nothing knew
// the tool was there, and the shell.d fragment its steps wrote was invisible to
// remove, to doctor and to the active-version projection. Giving the dependency
// its own entry and recording against that entry -- rather than against the tool
// that pulled it in -- is what makes `tsuku remove <dep>` able to take its own
// files with it. It also keeps the projection honest: the fragment is named
// after the dependency and keyed on the dependency's version, so the state that
// decides whether it belongs in the cache has to be the dependency's too.
//
// parent names the tool whose install pulled the dependency in, and is recorded
// as a RequiredBy edge so removing the dependency warns first.
func recordDependencyInstalls(
	cfg *config.Config,
	mgr *install.Manager,
	parent string,
	deps []executor.DependencyInstall,
	warnf warnFunc,
) {
	for _, dep := range deps {
		if dep.RecipeType == "library" {
			// Libraries live under $TSUKU_HOME/libs and are tracked in
			// state.Libs, which has no cleanup-action field. Nothing in the
			// registry ships a library recipe that writes outside its own
			// directory today -- set_env is rejected in library recipes by the
			// validator -- so warn rather than invent a home for it.
			if len(dep.CleanupActions) > 0 {
				warnf("library dependency %s@%s wrote %d file(s) outside its install directory; "+
					"tsuku cannot record those yet, so removing it will leave them behind",
					dep.Tool, dep.Version, len(dep.CleanupActions))
			}
			continue
		}

		if err := mgr.RecordDependencyInstall(dep.Tool, dep.Version, parent, dep.Binaries); err != nil {
			warnf("failed to record dependency %s@%s: %v", dep.Tool, dep.Version, err)
			continue
		}

		finishPostInstall(cfg, mgr, dep.Tool, dep.Version, dep.CleanupActions, warnf)
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
