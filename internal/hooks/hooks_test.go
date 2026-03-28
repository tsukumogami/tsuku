package hooks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/hooks"
)

// TestWriteHookFiles verifies that WriteHookFiles writes all hook files
// (command-not-found and activation) to the target directory with 0644
// permissions (scenario-9).
func TestWriteHookFiles(t *testing.T) {
	dir := t.TempDir()

	if err := hooks.WriteHookFiles(dir); err != nil {
		t.Fatalf("WriteHookFiles returned error: %v", err)
	}

	expected := []string{
		"tsuku.bash", "tsuku.zsh", "tsuku.fish",
		"tsuku-activate.bash", "tsuku-activate.zsh", "tsuku-activate.fish",
	}
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

	expected := []string{
		"tsuku.bash", "tsuku.zsh", "tsuku.fish",
		"tsuku-activate.bash", "tsuku-activate.zsh", "tsuku-activate.fish",
	}
	for _, name := range expected {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s to exist after second write, got: %v", name, err)
		}
	}
}

// TestHookFilesEmbedded verifies that all hook files are accessible via the
// embedded FS.
func TestHookFilesEmbedded(t *testing.T) {
	expected := []string{
		"tsuku.bash", "tsuku.zsh", "tsuku.fish",
		"tsuku-activate.bash", "tsuku-activate.zsh", "tsuku-activate.fish",
	}
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

// TestActivateBashContent verifies the bash activation hook calls hook-env
// and registers on PROMPT_COMMAND.
func TestActivateBashContent(t *testing.T) {
	data, err := hooks.HookFiles.ReadFile("tsuku-activate.bash")
	if err != nil {
		t.Fatalf("read tsuku-activate.bash: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "_tsuku_hook()") {
		t.Error("bash activation hook missing _tsuku_hook function definition")
	}
	if !strings.Contains(content, `tsuku hook-env bash`) {
		t.Error("bash activation hook missing 'tsuku hook-env bash' call")
	}
	if !strings.Contains(content, "PROMPT_COMMAND") {
		t.Error("bash activation hook missing PROMPT_COMMAND registration")
	}
}

// TestActivateZshContent verifies the zsh activation hook calls hook-env
// and registers on precmd_functions.
func TestActivateZshContent(t *testing.T) {
	data, err := hooks.HookFiles.ReadFile("tsuku-activate.zsh")
	if err != nil {
		t.Fatalf("read tsuku-activate.zsh: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "_tsuku_hook()") {
		t.Error("zsh activation hook missing _tsuku_hook function definition")
	}
	if !strings.Contains(content, `tsuku hook-env zsh`) {
		t.Error("zsh activation hook missing 'tsuku hook-env zsh' call")
	}
	if !strings.Contains(content, "precmd_functions") {
		t.Error("zsh activation hook missing precmd_functions registration")
	}
}

// TestActivateFishContent verifies the fish activation hook calls hook-env
// and registers on fish_prompt.
func TestActivateFishContent(t *testing.T) {
	data, err := hooks.HookFiles.ReadFile("tsuku-activate.fish")
	if err != nil {
		t.Fatalf("read tsuku-activate.fish: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "_tsuku_hook") {
		t.Error("fish activation hook missing _tsuku_hook function")
	}
	if !strings.Contains(content, "tsuku hook-env fish") {
		t.Error("fish activation hook missing 'tsuku hook-env fish' call")
	}
	if !strings.Contains(content, "fish_prompt") {
		t.Error("fish activation hook missing fish_prompt event registration")
	}
}
