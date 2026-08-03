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
// names collide, and the registry ships 59 pairs where one name prefixes
// another. Strip "git-" from "git-lfs-3.5.0" and the remainder, "lfs-3.5.0",
// is a perfectly good version string, so no lexical rule separates another
// tool's installation from one of this tool's versions. Asking state which
// versions a tool has, and rebuilding the directory name from that pair, keeps
// the name out of the delete decision entirely.
//
// Removing the directory is also only half the job. The version's shell.d
// fragments live outside it, and its VersionState keeps claiming them, which
// both leaks the files and holds open the "still referenced by another version"
// skip for a version that no longer exists.
type VersionStore interface {
	// InstalledVersions returns the versions state records for a tool. A tool
	// state has no record of yields no versions and no error.
	InstalledVersions(tool string) ([]string, error)
	// ReapVersion runs a version's cleanup actions and drops its VersionState.
	// It refuses a version state does not record.
	ReapVersion(tool, version string) error
}

// GarbageCollectVersions removes old version directories for a tool that are
// past the retention period. It protects the active version and the previous
// version (rollback target). The now parameter enables clock injection for tests.
//
// Only versions the store records are candidates. A directory state has no
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
		if err := install.ValidateVersionString(version); err != nil {
			log.Default().Debug("gc: skip unusable version", "tool", toolName, "version", version, "error", err)
			continue
		}

		// Lstat, not Stat: a symlink here is not a version directory this
		// function installed, and its target's mtime is not this version's age.
		dirPath := filepath.Join(toolsDir, toolName+"-"+version)
		info, err := os.Lstat(dirPath)
		if err != nil || !info.IsDir() {
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
