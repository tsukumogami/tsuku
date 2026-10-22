// Package realpath compares filesystem paths in one space: the one with every
// symlink resolved.
//
// A $TSUKU_HOME can be reached through a symlink: ~/.tsuku linked to another
// volume, or a home under macOS's /var or /tmp, which link into /private. A
// check that resolves one side of a comparison and not the other then reads
// every path under that home as foreign. Resolving both sides with Resolve
// and comparing with Within keeps the two in the same space, whether or not
// the home has a link in it.
package realpath

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Resolve returns path with every symlink in it resolved, like
// filepath.EvalSymlinks, except that the path doesn't have to exist yet.
//
// The longest leading part of path that exists is resolved, and the
// components below it are appended unchanged; they can't be links, since
// they don't exist. That covers a directory an install is about to create,
// such as $TSUKU_HOME/tools/<name>-<version>/bin before the tool lands.
//
// A component that exists as a symlink whose target doesn't is an error, not
// a component to append as written: the link could be pointed anywhere
// later, so where it leads can't be known now.
func Resolve(path string) (string, error) {
	clean := filepath.Clean(path)
	existing := clean
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(existing)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if _, lerr := os.Lstat(existing); lerr == nil {
			return "", fmt.Errorf("resolving %s: %s is a symlink to a path that does not exist", path, existing)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", err
		}
		missing = append(missing, filepath.Base(existing))
		existing = parent
	}
}

// Within reports whether path is dir or sits under it. It compares the two
// as given, so both should come from Resolve.
//
// The trailing separator is what keeps a sibling from matching:
// /home/u/.tsuku-backup is not under /home/u/.tsuku.
func Within(path, dir string) bool {
	path = filepath.Clean(path)
	dir = filepath.Clean(dir)
	if path == dir {
		return true
	}
	if !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	return strings.HasPrefix(path, dir)
}
