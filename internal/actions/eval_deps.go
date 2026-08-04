package actions

import (
	"os"
	"path/filepath"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

// GetEvalDeps returns the eval-time dependencies for an action.
// Returns nil if the action has no eval-time dependencies.
func GetEvalDeps(action string) []string {
	act := Get(action)
	if act == nil {
		return nil
	}
	deps := act.Dependencies()
	return deps.EvalTime
}

// CheckEvalDeps checks which eval-time dependencies are not installed.
// It asks what state records under $TSUKU_HOME for each one.
// Returns a list of missing dependency names.
func CheckEvalDeps(deps []string) []string {
	if len(deps) == 0 {
		return nil
	}

	tsukuHome := resolveTsukuHome()
	if tsukuHome == "" {
		// Can't determine the tsuku home directory, assume all deps are missing
		return deps
	}

	return checkEvalDepsInHome(deps, tsukuHome)
}

// checkEvalDepsInHome checks which dependencies are missing from the given
// tsuku home directory.
func checkEvalDepsInHome(deps []string, tsukuHome string) []string {
	// The home comes from resolveTsukuHome, the same place GetToolsDir gets
	// it, rather than from config.DefaultConfig. This check gates the
	// resolvers that go looking for the binary afterwards -- ResolveGo,
	// ResolveCargo, ResolvePythonStandalone -- and they all read GetToolsDir,
	// so answering from a different directory than they read would put the
	// check and the thing it guards out of step. Both of them ignore
	// config.DefaultHomeOverride, which is a real divergence from the rest of
	// the CLI and is tracked as issue #2501; fixing it here alone would only
	// widen the gap.
	//
	// HomeDir locates state.json and ToolsDir locates the version
	// directories. Those two fields are all this reads.
	cfg := &config.Config{
		HomeDir:  tsukuHome,
		ToolsDir: filepath.Join(tsukuHome, "tools"),
	}

	mgr := install.New(cfg)

	var missing []string
	for _, dep := range deps {
		if !isToolInstalled(mgr, cfg, dep) {
			missing = append(missing, dep)
		}
	}
	return missing
}

// isToolInstalled reports whether some version of a tool is installed.
//
// The versions come from state. They must not come from the names of the
// directories under $TSUKU_HOME/tools: those look like "<tool>-<version>" but
// cannot be taken apart again, because the registry ships dozens of pairs
// where one tool's name prefixes another's -- go/go-task, rust/rust-analyzer,
// git/git-lfs. Strip "go-" off "go-task-3.44.0" and the remainder,
// "task-3.44.0", is a perfectly good version string, so no lexical rule
// separates another tool's installation from one of this tool's versions.
// Matching the prefix reported go installed when the user had only go-task,
// and plan generation went off to decompose against a Go toolchain that was
// never there instead of offering to install one.
//
// State says which versions belong to the tool. The directory check says one
// of them is still on disk, which is what the decomposer about to run the
// binary needs to be true.
func isToolInstalled(mgr *install.Manager, cfg *config.Config, name string) bool {
	versions, err := mgr.InstalledVersions(name)
	if err != nil {
		return false
	}

	for _, version := range versions {
		// Lstat, not Stat, and IsDir on top of it: that is what the ReadDir
		// scan this replaced reported, since DirEntry.IsDir does not follow
		// symlinks either. Nothing tsuku installs puts a symlink here -- the
		// per-tool symlinks live under tools/current -- so following one
		// would only widen what counts as installed.
		if info, err := os.Lstat(cfg.ToolDir(name, version)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}
