package shellenv

import (
	"path/filepath"
	"sort"
	"strings"
)

// ShellDSelection tells the cache builder which recorded shell.d files belong to a
// tool's active version. A file in Known but not Active belongs to an installed-but-
// inactive version and is excluded. A file in neither is unrecorded and is included
// unchanged, which is the behavior on every release to date.
//
// The narrowness is deliberate. Code paths that write shell.d without recording a
// cleanup action still exist -- library installs run no post-install phase, and a
// library pulled in as a dependency has nowhere to record -- so a stricter "source
// only what state records" rule would silently disable a working shell integration:
// no error, no log line, just a tool that has vanished from the user's shell. Any
// fragment written by an older tsuku is in the same position.
type ShellDSelection struct {
	Active map[string]string // $TSUKU_HOME-relative path -> recorded SHA-256
	Known  map[string]string // every recorded shell.d path, active or not
}

// firstSelection collapses the variadic-optional parameter every consumer takes.
// Passing nothing yields the zero selection, which excludes nothing and verifies
// no hashes.
func firstSelection(selection []ShellDSelection) ShellDSelection {
	if len(selection) == 0 {
		return ShellDSelection{}
	}
	return selection[0]
}

// excludes reports whether a shell.d file belongs to an installed-but-inactive
// version and must be kept out of the cache. relPath is $TSUKU_HOME-relative.
func (s ShellDSelection) excludes(relPath string) bool {
	if _, known := s.Known[relPath]; !known {
		return false
	}
	_, active := s.Active[relPath]
	return !active
}

// hashFor returns the recorded SHA-256 for a shell.d file, or "" when the file
// has no recorded hash and must be included without verification.
func (s ShellDSelection) hashFor(relPath string) string {
	return s.Active[relPath]
}

// shellDRelPath builds the $TSUKU_HOME-relative key for a shell.d basename.
// It must agree with the paths actions record, which use forward slashes.
func shellDRelPath(name string) string {
	return "share/shell.d/" + name
}

// DisplayName turns a shell.d basename into the name shown to the user and
// written into the cache's "# tsuku:" comment. Fragments are version-keyed as
// <target>@<version>.<shell>, and neither half of that key is worth showing:
// the user asked about nvm, not nvm@0.40.6.
func DisplayName(fileName, shell string) string {
	name := strings.TrimSuffix(fileName, "."+shell)
	if at := strings.Index(name, "@"); at >= 0 {
		name = name[:at]
	}
	return name
}

// ShellsFromPaths maps recorded shell.d cleanup paths to the shells they serve,
// sorted and deduplicated. Callers hold the state; this is the filename half of
// the question "which shells does this tool integrate with".
func ShellsFromPaths(paths []string) []string {
	seen := make(map[string]bool)
	for _, p := range paths {
		if !strings.HasPrefix(p, "share/shell.d/") {
			continue
		}
		ext := filepath.Ext(p)
		if ext == "" {
			continue
		}
		shell := ext[1:]
		if shell == "bash" || shell == "zsh" {
			seen[shell] = true
		}
	}
	var shells []string
	for s := range seen {
		shells = append(shells, s)
	}
	sort.Strings(shells)
	return shells
}
