package datamigration

import (
	"os"
	"path/filepath"
	"testing"
)

// mkdir creates a directory, failing the test rather than returning an error.
func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
}

// write creates a file with the given contents, creating parents as needed.
func write(t *testing.T, path, contents string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// read returns a file's contents, failing the test if it is missing.
func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(data)
}

func TestMerge(t *testing.T) {
	tests := []struct {
		name string
		// setup populates src and dst and returns the entries to merge.
		setup func(t *testing.T, src, dst string) []string
		// check asserts on the outcome.
		check func(t *testing.T, src, dst string, result Result)
	}{
		{
			name: "moves an entry the destination does not have",
			setup: func(t *testing.T, src, dst string) []string {
				write(t, filepath.Join(src, "versions", "node", "v22", "bin", "node"), "binary")
				return []string{"versions"}
			},
			check: func(t *testing.T, src, dst string, result Result) {
				if got := read(t, filepath.Join(dst, "versions", "node", "v22", "bin", "node")); got != "binary" {
					t.Errorf("moved file contents = %q, want %q", got, "binary")
				}
				if _, err := os.Lstat(filepath.Join(src, "versions")); !os.IsNotExist(err) {
					t.Errorf("source entry still present after move: %v", err)
				}
				if !result.Merged() {
					t.Error("Merged() = false, want true")
				}
			},
		},
		{
			name: "merges directories present on both sides without overwriting",
			setup: func(t *testing.T, src, dst string) []string {
				write(t, filepath.Join(src, "versions", "node", "v20", "bin", "node"), "from-src")
				write(t, filepath.Join(dst, "versions", "node", "v22", "bin", "node"), "from-dst")
				return []string{"versions"}
			},
			check: func(t *testing.T, src, dst string, result Result) {
				if got := read(t, filepath.Join(dst, "versions", "node", "v20", "bin", "node")); got != "from-src" {
					t.Errorf("merged-in file = %q, want %q", got, "from-src")
				}
				if got := read(t, filepath.Join(dst, "versions", "node", "v22", "bin", "node")); got != "from-dst" {
					t.Errorf("pre-existing file = %q, want %q", got, "from-dst")
				}
			},
		},
		{
			name: "leaves a conflicting file in place and never overwrites",
			setup: func(t *testing.T, src, dst string) []string {
				write(t, filepath.Join(src, "default-packages"), "from-src")
				write(t, filepath.Join(dst, "default-packages"), "from-dst")
				return []string{"default-packages"}
			},
			check: func(t *testing.T, src, dst string, result Result) {
				if got := read(t, filepath.Join(dst, "default-packages")); got != "from-dst" {
					t.Errorf("destination was overwritten: got %q, want %q", got, "from-dst")
				}
				if got := read(t, filepath.Join(src, "default-packages")); got != "from-src" {
					t.Errorf("source was destroyed: got %q, want %q", got, "from-src")
				}
				if len(result.Conflicts) != 1 {
					t.Fatalf("Conflicts = %d, want 1", len(result.Conflicts))
				}
			},
		},
		{
			// The invariant that keeps this from being a move-anything primitive. Under
			// os.Stat a symlink to a directory looks like a directory, and the merge
			// would descend into whatever it points at and empty that instead.
			name: "treats a symlink on the source side as a conflict, never following it",
			setup: func(t *testing.T, src, dst string) []string {
				outside := filepath.Join(t.TempDir(), "elsewhere")
				write(t, filepath.Join(outside, "private.txt"), "not-yours")
				if err := os.Symlink(outside, filepath.Join(src, "versions")); err != nil {
					t.Fatalf("Symlink: %v", err)
				}
				mkdir(t, filepath.Join(dst, "versions"))
				return []string{"versions"}
			},
			check: func(t *testing.T, src, dst string, result Result) {
				link, err := os.Readlink(filepath.Join(src, "versions"))
				if err != nil {
					t.Fatalf("source symlink was consumed: %v", err)
				}
				if _, err := os.Stat(filepath.Join(link, "private.txt")); err != nil {
					t.Errorf("the symlink target was raided: %v", err)
				}
				if _, err := os.Stat(filepath.Join(dst, "versions", "private.txt")); !os.IsNotExist(err) {
					t.Error("contents of the symlink target were moved into the destination")
				}
				if len(result.Conflicts) != 1 {
					t.Fatalf("Conflicts = %d, want 1", len(result.Conflicts))
				}
			},
		},
		{
			name: "treats a symlink on the destination side as a conflict, never following it",
			setup: func(t *testing.T, src, dst string) []string {
				outside := filepath.Join(t.TempDir(), "elsewhere")
				mkdir(t, outside)
				mkdir(t, dst)
				if err := os.Symlink(outside, filepath.Join(dst, "versions")); err != nil {
					t.Fatalf("Symlink: %v", err)
				}
				write(t, filepath.Join(src, "versions", "node", "v22", "bin", "node"), "binary")
				return []string{"versions"}
			},
			check: func(t *testing.T, src, dst string, result Result) {
				if got := read(t, filepath.Join(src, "versions", "node", "v22", "bin", "node")); got != "binary" {
					t.Errorf("source was consumed: got %q", got)
				}
				link, err := os.Readlink(filepath.Join(dst, "versions"))
				if err != nil {
					t.Fatalf("destination symlink was replaced: %v", err)
				}
				entries, err := os.ReadDir(link)
				if err != nil {
					t.Fatalf("ReadDir(%s): %v", link, err)
				}
				if len(entries) != 0 {
					t.Errorf("data was written through the destination symlink: %d entries", len(entries))
				}
				if len(result.Conflicts) != 1 {
					t.Fatalf("Conflicts = %d, want 1", len(result.Conflicts))
				}
			},
		},
		{
			name: "is a no-op when the source does not exist",
			setup: func(t *testing.T, src, dst string) []string {
				if err := os.RemoveAll(src); err != nil {
					t.Fatalf("RemoveAll: %v", err)
				}
				return []string{"versions"}
			},
			check: func(t *testing.T, src, dst string, result Result) {
				if result.Merged() || len(result.Conflicts) != 0 {
					t.Errorf("expected an empty result, got %+v", result)
				}
			},
		},
		{
			// The prune that runs after recursing into a merged directory must not be
			// able to discard whatever the merge declined to move. os.Remove refuses a
			// non-empty directory; os.RemoveAll would not, and would take the conflicted
			// file with it.
			name: "keeps a conflicted file when pruning the merged directory",
			setup: func(t *testing.T, src, dst string) []string {
				write(t, filepath.Join(src, "versions", "moved.txt"), "from-src")
				write(t, filepath.Join(src, "versions", "clash.txt"), "src-copy")
				write(t, filepath.Join(dst, "versions", "clash.txt"), "dst-copy")
				return []string{"versions"}
			},
			check: func(t *testing.T, src, dst string, result Result) {
				if got := read(t, filepath.Join(src, "versions", "clash.txt")); got != "src-copy" {
					t.Errorf("the conflicted source file was destroyed by the prune: %q", got)
				}
				if got := read(t, filepath.Join(dst, "versions", "clash.txt")); got != "dst-copy" {
					t.Errorf("the destination was overwritten: %q", got)
				}
				if got := read(t, filepath.Join(dst, "versions", "moved.txt")); got != "from-src" {
					t.Errorf("the non-conflicting file did not move: %q", got)
				}
				if len(result.Conflicts) != 1 {
					t.Fatalf("Conflicts = %d, want 1", len(result.Conflicts))
				}
			},
		},
		{
			name: "never removes its own source directory",
			setup: func(t *testing.T, src, dst string) []string {
				write(t, filepath.Join(src, "versions", "keep"), "x")
				write(t, filepath.Join(src, "not-listed.txt"), "tsuku's own file")
				return []string{"versions"}
			},
			check: func(t *testing.T, src, dst string, result Result) {
				if _, err := os.Stat(src); err != nil {
					t.Errorf("source directory was removed: %v", err)
				}
				if got := read(t, filepath.Join(src, "not-listed.txt")); got != "tsuku's own file" {
					t.Errorf("an unlisted entry was touched: %q", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			src := filepath.Join(root, "src")
			dst := filepath.Join(root, "dst")
			mkdir(t, src)

			entries := tt.setup(t, src, dst)
			result, err := Merge(src, dst, entries)
			if err != nil {
				t.Fatalf("Merge: %v", err)
			}
			tt.check(t, src, dst, result)
		})
	}
}

func TestMergeIsIdempotent(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	write(t, filepath.Join(src, "versions", "node", "v22", "bin", "node"), "binary")

	if _, err := Merge(src, dst, []string{"versions"}); err != nil {
		t.Fatalf("first Merge: %v", err)
	}

	// The second run is the one that matters: the migration is re-entered on every
	// install, so a second pass over an already-migrated tree has to be a no-op rather
	// than a source of conflicts.
	second, err := Merge(src, dst, []string{"versions"})
	if err != nil {
		t.Fatalf("second Merge: %v", err)
	}
	if second.Merged() {
		t.Errorf("second run moved %v, want nothing", second.Moved)
	}
	if len(second.Conflicts) != 0 {
		t.Errorf("second run reported conflicts: %+v", second.Conflicts)
	}
	if got := read(t, filepath.Join(dst, "versions", "node", "v22", "bin", "node")); got != "binary" {
		t.Errorf("data after second run = %q, want %q", got, "binary")
	}
}

func TestDirEntriesIgnoresFilesAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "versions"))
	mkdir(t, filepath.Join(dir, "alias"))
	write(t, filepath.Join(dir, "nvm@0.40.3.bash"), "export X=1")
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(dir, "linked")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	names, err := DirEntries(dir)
	if err != nil {
		t.Fatalf("DirEntries: %v", err)
	}

	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	if !got["versions"] || !got["alias"] {
		t.Errorf("DirEntries = %v, want it to include versions and alias", names)
	}
	if got["nvm@0.40.3.bash"] {
		t.Error("DirEntries included a file; tsuku's own shell.d writes are files")
	}
	if got["linked"] {
		t.Error("DirEntries included a symlink to a directory")
	}
}
