// Package datamigration relocates a tool's user data between roots.
//
// It exists because tsuku used to hand some tools a data root it later reclaimed, and
// the data has to be moved to the stable root without any window in which it could be
// lost. The merge below is deliberately incapable of destroying anything: the worst
// outcome it can produce is the same bytes present in two places.
//
// This is a leaf package (stdlib and internal/config only) because both internal/actions
// and internal/install need it and neither imports the other. merge.go is general;
// nvm.go holds the one tool's predicates and is the part to delete once the affected
// releases have aged out.
package datamigration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Conflict is one entry the merge declined to move.
type Conflict struct {
	// Path is the source path that stayed where it was.
	Path string
	// Reason says why, in terms a user can act on.
	Reason string
}

// Result reports what a merge did.
type Result struct {
	// Moved is the source paths that were relocated.
	Moved []string
	// Conflicts is the entries left in place, with reasons.
	Conflicts []Conflict
}

// Merged reports whether anything actually moved.
func (r Result) Merged() bool { return len(r.Moved) > 0 }

// Merge moves the named entries from src into dst.
//
// The algorithm has no deletion primitive and never overwrites. For each entry: rename
// it when nothing occupies the destination; recurse when both sides are real
// directories; otherwise leave it alone and record a conflict. It never removes src
// itself — callers know what that directory is and whether pruning it is safe, and for
// at least one caller src is a directory tsuku relies on.
//
// Every stat is os.Lstat and every type test is lstat-flavored, so a symlink is never
// treated as a directory to descend into. That is load-bearing rather than fastidious:
// under os.Stat a symlink at src/versions pointing anywhere at all would make this
// function enumerate that directory and rename its contents into dst, which turns a
// migration into a move-anything primitive. A symlink on either side is a conflict.
//
// There is deliberately no copy fallback. If a rename fails — EXDEV across a mount
// point, a permission problem — the entry is reported and left alone. Copying instead
// would be worse than doing nothing: a recursive copy here has no hard-link tracking, so
// a Node estate whose package store shares inodes can multiply in size and run the disk
// out, and a copy that dies mid-tree leaves two partial trees with nothing recording
// which is authoritative. `mv` handles the cross-device case properly; the honest move
// is to say so and let the user run it.
func Merge(src, dst string, entries []string) (Result, error) {
	var result Result

	srcInfo, err := os.Lstat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, err
	}
	if !srcInfo.Mode().IsDir() {
		return result, fmt.Errorf("%s is not a directory", src)
	}

	if err := os.MkdirAll(dst, 0700); err != nil {
		return result, fmt.Errorf("creating %s: %w", dst, err)
	}

	for _, name := range entries {
		if err := mergeEntry(filepath.Join(src, name), filepath.Join(dst, name), &result); err != nil {
			return result, err
		}
	}

	return result, nil
}

// DirEntries returns the names of every entry directly under dir that is a real
// directory, ignoring symlinks.
//
// One population's data sits loose in a directory tsuku also writes files into. tsuku's
// writers there only ever write named files, and every reader of that directory skips
// entries that are directories — so "is a directory" identifies what is not tsuku's
// without needing a list of names to look for.
func DirEntries(dir string) ([]string, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, item := range items {
		// os.ReadDir's DirEntry.IsDir is already lstat-flavored: a symlink to a
		// directory reports false, which is what we want.
		if item.IsDir() {
			names = append(names, item.Name())
		}
	}
	return names, nil
}

// mergeEntry moves one path, recursing into directories present on both sides.
func mergeEntry(src, dst string, result *Result) error {
	srcInfo, err := os.Lstat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	dstInfo, err := os.Lstat(dst)
	switch {
	case err != nil && os.IsNotExist(err):
		if renameErr := os.Rename(src, dst); renameErr != nil {
			result.Conflicts = append(result.Conflicts, Conflict{
				Path:   src,
				Reason: renameReason(renameErr),
			})
			return nil
		}
		result.Moved = append(result.Moved, src)
		return nil

	case err != nil:
		return err
	}

	// Both sides exist. Only two real directories can be merged; anything else --
	// including a symlink standing in for a directory -- is left alone.
	if !srcInfo.Mode().IsDir() || !dstInfo.Mode().IsDir() {
		result.Conflicts = append(result.Conflicts, Conflict{
			Path:   src,
			Reason: fmt.Sprintf("%s already exists and cannot be merged safely", dst),
		})
		return nil
	}

	children, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := mergeEntry(filepath.Join(src, child.Name()), filepath.Join(dst, child.Name()), result); err != nil {
			return err
		}
	}

	// Prune the source directory only if the merge emptied it. os.Remove refuses a
	// non-empty directory, so this can never discard anything left behind.
	_ = os.Remove(src)
	return nil
}

// renameReason explains a failed rename in terms of what the user should do.
func renameReason(err error) string {
	if errors.Is(err, os.ErrPermission) {
		return fmt.Sprintf("could not be moved (%v); check permissions and move it by hand", err)
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) && linkErr.Err != nil {
		// EXDEV and friends. `mv` copies correctly across devices, including hard links.
		return fmt.Sprintf("could not be moved (%v); move it by hand with mv", linkErr.Err)
	}
	return fmt.Sprintf("could not be moved (%v)", err)
}
