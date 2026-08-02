package updates

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/log"
)

// VersionReaper is the slice of install.Manager that garbage collection needs.
// Removing a version directory is only half the job: the version's shell.d
// fragments live outside the tool directory, and its VersionState keeps
// claiming them, which both leaks the files and holds open the
// "still referenced by another version" skip for a version that no longer
// exists. Pass nil to skip state reconciliation.
type VersionReaper interface {
	ReapVersion(tool, version string) error
}

// GarbageCollectVersions removes old version directories for a tool that are
// past the retention period. It protects the active version and the previous
// version (rollback target). The now parameter enables clock injection for tests.
func GarbageCollectVersions(reaper VersionReaper, toolsDir, toolName, activeVersion, previousVersion string, retention time.Duration, now time.Time) error {
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read tools directory: %w", err)
	}

	prefix := toolName + "-"
	activeDir := toolName + "-" + activeVersion
	previousDir := toolName + "-" + previousVersion

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Only consider directories matching this tool's naming pattern
		if !strings.HasPrefix(name, prefix) {
			continue
		}

		// Never delete the active version
		if name == activeDir {
			continue
		}

		// Never delete the rollback target
		if previousVersion != "" && name == previousDir {
			continue
		}

		// Check age against retention period
		info, err := entry.Info()
		if err != nil {
			continue
		}

		age := now.Sub(info.ModTime())
		if age < retention {
			continue
		}

		// Reconcile state before the directory goes, so a failure leaves the
		// version installed rather than recorded but absent.
		version := strings.TrimPrefix(name, prefix)
		if reaper != nil {
			if err := reaper.ReapVersion(toolName, version); err != nil {
				log.Default().Debug("gc: reap version state", "tool", toolName, "version", version, "error", err)
				continue
			}
		}

		// Remove the old version directory
		dirPath := toolsDir + "/" + name
		if err := os.RemoveAll(dirPath); err != nil {
			log.Default().Debug("gc: remove old version", "tool", toolName, "dir", name, "error", err)
			continue
		}
		log.Default().Debug("gc: removed old version", "tool", toolName, "dir", name, "age", age)
	}

	return nil
}
