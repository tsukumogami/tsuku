package hooks_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/hooks"
)

// TestWriteHookFiles verifies that WriteHookFiles writes all three hook files
// to the target directory with 0644 permissions (scenario-9).
func TestWriteHookFiles(t *testing.T) {
	dir := t.TempDir()

	if err := hooks.WriteHookFiles(dir); err != nil {
		t.Fatalf("WriteHookFiles returned error: %v", err)
	}

	expected := []string{"tsuku.bash", "tsuku.zsh", "tsuku.fish"}
	for _, name := range expected {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected file %s to exist, got error: %v", name, err)
			continue
		}
		if info.IsDir() {
			t.Errorf("expected %s to be a file, not a directory", name)
			continue
		}
		// Verify 0644 permissions.
		mode := info.Mode().Perm()
		if mode != 0644 {
			t.Errorf("expected %s to have permissions 0644, got %04o", name, mode)
		}
		// Verify the file is non-empty.
		if info.Size() == 0 {
			t.Errorf("expected %s to be non-empty", name)
		}
	}
}

// TestWriteHookFilesIdempotent verifies that calling WriteHookFiles twice
// does not produce errors and overwrites the files correctly.
func TestWriteHookFilesIdempotent(t *testing.T) {
	dir := t.TempDir()

	if err := hooks.WriteHookFiles(dir); err != nil {
		t.Fatalf("first WriteHookFiles returned error: %v", err)
	}
	if err := hooks.WriteHookFiles(dir); err != nil {
		t.Fatalf("second WriteHookFiles returned error: %v", err)
	}

	expected := []string{"tsuku.bash", "tsuku.zsh", "tsuku.fish"}
	for _, name := range expected {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s to exist after second write, got: %v", name, err)
		}
	}
}

// TestHookFilesEmbedded verifies that all three hook files are accessible
// via the embedded FS.
func TestHookFilesEmbedded(t *testing.T) {
	expected := []string{"tsuku.bash", "tsuku.zsh", "tsuku.fish"}
	for _, name := range expected {
		data, err := hooks.HookFiles.ReadFile(name)
		if err != nil {
			t.Errorf("expected embedded file %s to be readable, got: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("expected embedded file %s to be non-empty", name)
		}
	}
}
