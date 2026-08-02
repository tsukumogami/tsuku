package datamigration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/config"
)

// NvmTool is the recipe name whose data this file migrates.
const NvmTool = "nvm"

// nvmDataEntries is what nvm keeps under $NVM_DIR on the user's behalf.
//
// An explicit list rather than "everything that is not a program file". The versioned
// tool directory holds both halves, and deciding by exclusion would classify any file
// dropped between two nvm releases as user data — emptying a directory that garbage
// collection deliberately keeps as the rollback target, and depositing a stale program
// file into the one tree tsuku promises never to touch.
var nvmDataEntries = []string{"versions", "alias", ".cache", "default-packages", "current"}

// nvmDataMarkers are the entries whose presence means a directory is being used as an
// nvm data root.
var nvmDataMarkers = []string{"versions", "alias", ".cache"}

// NvmDataDir returns the stable data root for nvm under the given $TSUKU_HOME.
func NvmDataDir(tsukuHome string) string {
	return filepath.Join(tsukuHome, config.DataDirName, NvmTool)
}

// Source is one legacy location holding nvm data that has not been migrated.
type Source struct {
	// Dir is the directory holding the data.
	Dir string
	// Entries is what should move out of it.
	Entries []string
}

// FindNvmSources returns the legacy locations still holding nvm data, most urgent first.
//
// Two exist, for two different reasons.
//
// Before set_env took effect, NVM_DIR was never exported, so nvm.sh self-located to the
// directory it was sourced from and Node versions piled up in share/shell.d. Nothing
// garbage-collects that tree, so this data is stranded rather than doomed.
//
// After set_env began working, NVM_DIR named the versioned tool directory, which is
// reclaimed. That data is on a clock, so it is returned first and wins any collision.
//
// Versioned directories are found by globbing rather than by reading state, so a
// directory whose state entry has already been dropped is still rescued. The glob is
// only a candidate filter: a directory is a source only if it actually holds nvm data.
func FindNvmSources(tsukuHome string) []Source {
	var sources []Source

	toolsDir := filepath.Join(tsukuHome, "tools")
	matches, _ := filepath.Glob(filepath.Join(toolsDir, NvmTool+"-*"))
	for _, dir := range matches {
		if hasNvmData(dir) {
			sources = append(sources, Source{Dir: dir, Entries: nvmDataEntries})
		}
	}

	shellD := filepath.Join(tsukuHome, "share", "shell.d")
	if hasNvmData(shellD) {
		// Everything here that is a directory is the user's; see DirEntries.
		if names, err := DirEntries(shellD); err == nil && len(names) > 0 {
			sources = append(sources, Source{Dir: shellD, Entries: names})
		}
	}

	return sources
}

// hasNvmData reports whether dir looks like it has been used as an nvm data root.
func hasNvmData(dir string) bool {
	for _, marker := range nvmDataMarkers {
		info, err := os.Lstat(filepath.Join(dir, marker))
		if err == nil && info.Mode().IsDir() {
			return true
		}
	}
	return false
}

// MigrateNvm moves any nvm data found in a legacy location into the stable data root.
//
// Safe to call when there is nothing to do, and safe to call again after a partial run:
// it detects the shape on disk rather than tracking a migration flag, which is how this
// codebase does its other migrations.
//
// Callers must only invoke this at a moment when the exported NVM_DIR either already
// names the stable root or is about to, in the same process. Moving the data while the
// user's shell still points at the old location produces exactly the failure this
// migration exists to prevent.
func MigrateNvm(tsukuHome string) (Result, error) {
	dst := NvmDataDir(tsukuHome)
	var combined Result

	for _, source := range FindNvmSources(tsukuHome) {
		if source.Dir == dst {
			continue
		}
		result, err := Merge(source.Dir, dst, source.Entries)
		combined.Moved = append(combined.Moved, result.Moved...)
		combined.Conflicts = append(combined.Conflicts, result.Conflicts...)
		if err != nil {
			return combined, err
		}
	}

	return combined, nil
}

// ConflictReport renders conflicts as a user-facing message, or "" when there are none.
func ConflictReport(dst string, conflicts []Conflict) string {
	if len(conflicts) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Some nvm data could not be moved to %s:\n", dst)
	for _, c := range conflicts {
		fmt.Fprintf(&b, "  %s -- %s\n", c.Path, c.Reason)
	}
	// Not `mv src/* dst/`: that skips dotfiles, losing .cache silently, and nests
	// versions/versions when the destination already has one.
	b.WriteString("Move the listed paths into that directory to finish, then run 'tsuku doctor'.")
	return b.String()
}
