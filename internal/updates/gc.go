package updates

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/log"
)

// VersionStore is the slice of install.Manager that garbage collection needs.
//
// It is what makes reclamation safe. Directory names under $TSUKU_HOME/tools
// look like "<tool>-<version>", but they cannot be taken apart again: tool
// names collide, and the registry ships dozens of pairs where one name prefixes
// another -- git/git-lfs, docker/docker-compose, helm/helm-docs. Strip "git-"
// from "git-lfs-3.5.0" and the remainder, "lfs-3.5.0", is a perfectly good
// version string, so no lexical rule separates another tool's installation from
// one of this tool's versions. Asking state which versions a tool has, and
// rebuilding the directory name from that pair, keeps the name out of the
// delete decision entirely.
//
// Removing the directory is also only half the job. The version's shell.d
// fragments live outside it, and its VersionState keeps claiming them, which
// both leaks the files and holds open the "still referenced by another version"
// skip for a version that no longer exists.
type VersionStore interface {
	// InstalledVersions returns the versions state records for a tool. A tool
	// that state has no record of yields no versions and no error.
	InstalledVersions(tool string) ([]string, error)
	// ReapVersion runs a version's cleanup actions and drops its VersionState.
	//
	// It is called while the version's directory still exists, and it must be:
	// reconciling first means a failure leaves the version installed rather
	// than recorded but absent.
	//
	// It refuses a version that state does not record, and it refuses the
	// active version. Both refusals are load-bearing -- they are the second
	// lock on a door this function is the first lock on -- so an
	// implementation that answers differently is not a drop-in.
	ReapVersion(tool, version string) error
}

// GarbageCollectVersions removes old version directories for a tool that are
// past the retention period. It protects the active version and the previous
// version (rollback target). The now parameter enables clock injection for tests.
//
// Only versions the store records are candidates. A directory that state has no
// record of stays put: deleting it would mean deleting on the strength of a
// filesystem name, which is how this function used to remove other people's
// tools. Reclaiming those needs a pass that can prove a directory belongs to no
// installed tool at all, which is a different job with a different safety
// argument.
func GarbageCollectVersions(store VersionStore, toolsDir, toolName, activeVersion, previousVersion string, retention time.Duration, now time.Time) error {
	if store == nil {
		return fmt.Errorf("garbage collection needs a version store to tell it what it may reclaim")
	}

	versions, err := store.InstalledVersions(toolName)
	if err != nil {
		return fmt.Errorf("read installed versions for %s: %w", toolName, err)
	}

	for _, version := range versions {
		// Never delete the active version
		if version == activeVersion {
			continue
		}

		// Never delete the rollback target
		if previousVersion != "" && version == previousVersion {
			continue
		}

		// The version now arrives from state rather than from a directory
		// listing, so it reaches the path join as data. Anything that could
		// steer os.RemoveAll out of the tools directory does not get there.
		//
		// install's validator, not version's. They disagree, and the rule that
		// has to apply here is the one that let the directory be created:
		// version.ValidateVersionString has a stricter charset, so using it
		// would silently stop reclaiming versions tsuku itself installed.
		if err := install.ValidateVersionString(version); err != nil {
			log.Default().Debug("gc: skip unusable version", "tool", toolName, "version", version, "error", err)
			continue
		}

		// Same layout as config.ToolDir, which is the canonical builder --
		// change one and this has to change with it. This function is handed a
		// tools directory rather than a *config.Config, so it cannot call it.
		dirPath := filepath.Join(toolsDir, toolName+"-"+version)

		// Lstat, not Stat: a symlink here is not a version directory this
		// function installed, and its target's mtime is not this version's age.
		//
		// A version state records with no directory on disk is left recorded.
		// Reconciling that is a repair, and a transient stat failure must not
		// be able to trigger one.
		info, err := os.Lstat(dirPath)
		if err != nil || !info.IsDir() {
			log.Default().Debug("gc: skip, not a version directory", "tool", toolName, "version", version, "path", dirPath, "error", err)
			continue
		}

		// Check age against retention period
		age := now.Sub(info.ModTime())
		if age < retention {
			continue
		}

		// Reconcile state before the directory goes, so a failure leaves the
		// version installed rather than recorded but absent.
		if err := store.ReapVersion(toolName, version); err != nil {
			log.Default().Debug("gc: reap version state", "tool", toolName, "version", version, "error", err)
			continue
		}

		// Remove the old version directory
		if err := os.RemoveAll(dirPath); err != nil {
			log.Default().Debug("gc: remove old version", "tool", toolName, "version", version, "error", err)
			continue
		}
		log.Default().Debug("gc: removed old version", "tool", toolName, "version", version, "age", age)
	}

	return nil
}
