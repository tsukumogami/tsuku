package hook_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/hook"
)

// shareHooksDir returns a temp directory containing the hook files for use in tests.
// It relies on Install writing hook files to shareHooksDir as a side effect.
func makeShareHooksDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

// TestInstallBashMarkerInsertion verifies that hook install writes the two-line
// marker block into ~/.bashrc (scenario-10).
func TestInstallBashMarkerInsertion(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := makeShareHooksDir(t)

	if err := hook.Install("bash", homeDir, shareHooksDir); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	rcFile := filepath.Join(homeDir, ".bashrc")
	data, err := os.ReadFile(rcFile)
	if err != nil {
		t.Fatalf("could not read .bashrc: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "# tsuku hook") {
		t.Errorf(".bashrc missing marker comment; got:\n%s", content)
	}
	if !strings.Contains(content, `. "$TSUKU_HOME/share/hooks/tsuku.bash"`) {
		t.Errorf(".bashrc missing source line; got:\n%s", content)
	}
}

// TestInstallZshMarkerInsertion verifies that hook install writes the two-line
// marker block into ~/.zshrc (scenario-11).
func TestInstallZshMarkerInsertion(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := makeShareHooksDir(t)

	if err := hook.Install("zsh", homeDir, shareHooksDir); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	rcFile := filepath.Join(homeDir, ".zshrc")
	data, err := os.ReadFile(rcFile)
	if err != nil {
		t.Fatalf("could not read .zshrc: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "# tsuku hook") {
		t.Errorf(".zshrc missing marker comment; got:\n%s", content)
	}
	if !strings.Contains(content, `. "$TSUKU_HOME/share/hooks/tsuku.zsh"`) {
		t.Errorf(".zshrc missing source line; got:\n%s", content)
	}
}

// TestInstallIdempotencyBash verifies that running hook install twice for bash
// does not produce duplicate marker blocks (scenario-12).
func TestInstallIdempotencyBash(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := makeShareHooksDir(t)

	for i := 0; i < 2; i++ {
		if err := hook.Install("bash", homeDir, shareHooksDir); err != nil {
			t.Fatalf("Install call %d returned error: %v", i+1, err)
		}
	}

	rcFile := filepath.Join(homeDir, ".bashrc")
	data, err := os.ReadFile(rcFile)
	if err != nil {
		t.Fatalf("could not read .bashrc: %v", err)
	}

	count := strings.Count(string(data), "# tsuku hook")
	if count != 1 {
		t.Errorf("expected exactly 1 marker block, got %d; content:\n%s", count, string(data))
	}
}

// TestInstallIdempotencyZsh verifies that running hook install twice for zsh
// does not produce duplicate marker blocks (scenario-12).
func TestInstallIdempotencyZsh(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := makeShareHooksDir(t)

	for i := 0; i < 2; i++ {
		if err := hook.Install("zsh", homeDir, shareHooksDir); err != nil {
			t.Fatalf("Install call %d returned error: %v", i+1, err)
		}
	}

	rcFile := filepath.Join(homeDir, ".zshrc")
	data, err := os.ReadFile(rcFile)
	if err != nil {
		t.Fatalf("could not read .zshrc: %v", err)
	}

	count := strings.Count(string(data), "# tsuku hook")
	if count != 1 {
		t.Errorf("expected exactly 1 marker block, got %d; content:\n%s", count, string(data))
	}
}

// TestInstallAtomicWrite verifies that the rc file is not left in a bad state
// if interrupted — we check that the file is valid after install (scenario-13).
// We verify this by checking the temp file is removed and the rc file is correct.
func TestInstallAtomicWrite(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := makeShareHooksDir(t)

	if err := hook.Install("bash", homeDir, shareHooksDir); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	// No temp files should remain in the home directory.
	entries, err := os.ReadDir(homeDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tsuku-hook-") {
			t.Errorf("temp file not cleaned up: %s", e.Name())
		}
	}

	// The rc file must be correctly written.
	rcFile := filepath.Join(homeDir, ".bashrc")
	data, err := os.ReadFile(rcFile)
	if err != nil {
		t.Fatalf("could not read .bashrc: %v", err)
	}
	if !strings.Contains(string(data), "# tsuku hook") {
		t.Errorf(".bashrc missing marker after install")
	}
}

// TestUninstallRemovesExactBlock verifies that hook uninstall removes exactly
// the two-line marker block and leaves other content intact (scenario-14).
func TestUninstallRemovesExactBlock(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := makeShareHooksDir(t)

	// Pre-populate .bashrc with some existing content.
	rcFile := filepath.Join(homeDir, ".bashrc")
	existing := "# existing config\nexport PATH=$PATH:/usr/local/bin\n"
	if err := os.WriteFile(rcFile, []byte(existing), 0644); err != nil {
		t.Fatalf("write existing .bashrc: %v", err)
	}

	// Install hook.
	if err := hook.Install("bash", homeDir, shareHooksDir); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	// Uninstall hook.
	if err := hook.Uninstall("bash", homeDir); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}

	data, err := os.ReadFile(rcFile)
	if err != nil {
		t.Fatalf("could not read .bashrc after uninstall: %v", err)
	}
	content := string(data)

	// Marker must be gone.
	if strings.Contains(content, "# tsuku hook") {
		t.Errorf("marker comment still present after uninstall; content:\n%s", content)
	}
	if strings.Contains(content, "tsuku.bash") {
		t.Errorf("source line still present after uninstall; content:\n%s", content)
	}

	// Existing content must be preserved.
	if !strings.Contains(content, "# existing config") {
		t.Errorf("existing config was removed; content:\n%s", content)
	}
	if !strings.Contains(content, "export PATH=$PATH:/usr/local/bin") {
		t.Errorf("existing PATH line was removed; content:\n%s", content)
	}
}

// TestUninstallIdempotent verifies that running hook uninstall twice does not
// error (scenario-14).
func TestUninstallIdempotent(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := makeShareHooksDir(t)

	if err := hook.Install("bash", homeDir, shareHooksDir); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := hook.Uninstall("bash", homeDir); err != nil {
			t.Fatalf("Uninstall call %d returned error: %v", i+1, err)
		}
	}
}

// TestUninstallNoFile verifies that uninstall with no rc file present does
// not error (idempotent, scenario-14).
func TestUninstallNoFile(t *testing.T) {
	homeDir := t.TempDir()
	if err := hook.Uninstall("bash", homeDir); err != nil {
		t.Fatalf("Uninstall on missing file returned error: %v", err)
	}
}

// TestStatusDetection verifies that Status returns the correct installed state
// before and after install/uninstall (scenario-15).
func TestStatusDetection(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := makeShareHooksDir(t)

	// Before install: not installed.
	for _, shell := range []string{"bash", "zsh"} {
		installed, err := hook.Status(shell, homeDir)
		if err != nil {
			t.Fatalf("Status(%s) before install returned error: %v", shell, err)
		}
		if installed {
			t.Errorf("Status(%s) expected false before install, got true", shell)
		}
	}

	// Install bash.
	if err := hook.Install("bash", homeDir, shareHooksDir); err != nil {
		t.Fatalf("Install(bash) returned error: %v", err)
	}

	installed, err := hook.Status("bash", homeDir)
	if err != nil {
		t.Fatalf("Status(bash) after install returned error: %v", err)
	}
	if !installed {
		t.Errorf("Status(bash) expected true after install, got false")
	}

	// Zsh still not installed.
	installed, err = hook.Status("zsh", homeDir)
	if err != nil {
		t.Fatalf("Status(zsh) returned error: %v", err)
	}
	if installed {
		t.Errorf("Status(zsh) expected false (not installed), got true")
	}

	// After uninstall: not installed again.
	if err := hook.Uninstall("bash", homeDir); err != nil {
		t.Fatalf("Uninstall(bash) returned error: %v", err)
	}
	installed, err = hook.Status("bash", homeDir)
	if err != nil {
		t.Fatalf("Status(bash) after uninstall returned error: %v", err)
	}
	if installed {
		t.Errorf("Status(bash) expected false after uninstall, got true")
	}
}

// TestInstallFish verifies that hook install for fish writes the hook file to
// ~/.config/fish/conf.d/tsuku.fish (scenario-16).
func TestInstallFish(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := makeShareHooksDir(t)

	if err := hook.Install("fish", homeDir, shareHooksDir); err != nil {
		t.Fatalf("Install(fish) returned error: %v", err)
	}

	dest := filepath.Join(homeDir, ".config", "fish", "conf.d", "tsuku.fish")
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("expected tsuku.fish to exist: %v", err)
	}
	if info.Mode().Perm() != 0644 {
		t.Errorf("expected tsuku.fish permissions 0644, got %04o", info.Mode().Perm())
	}
}

// TestInstallFishIdempotent verifies that installing fish hook twice does not error
// and does not corrupt the file (scenario-16).
func TestInstallFishIdempotent(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := makeShareHooksDir(t)

	for i := 0; i < 2; i++ {
		if err := hook.Install("fish", homeDir, shareHooksDir); err != nil {
			t.Fatalf("Install(fish) call %d returned error: %v", i+1, err)
		}
	}
}

// TestUninstallFish verifies that hook uninstall for fish removes the conf.d
// file (scenario-17).
func TestUninstallFish(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := makeShareHooksDir(t)

	if err := hook.Install("fish", homeDir, shareHooksDir); err != nil {
		t.Fatalf("Install(fish) returned error: %v", err)
	}

	if err := hook.Uninstall("fish", homeDir); err != nil {
		t.Fatalf("Uninstall(fish) returned error: %v", err)
	}

	dest := filepath.Join(homeDir, ".config", "fish", "conf.d", "tsuku.fish")
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("expected tsuku.fish to be removed, but it still exists (or stat error: %v)", err)
	}
}

// TestStatusFish verifies Status for fish (scenario-15).
func TestStatusFish(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := makeShareHooksDir(t)

	installed, err := hook.Status("fish", homeDir)
	if err != nil {
		t.Fatalf("Status(fish) before install: %v", err)
	}
	if installed {
		t.Errorf("Status(fish) expected false before install")
	}

	if err := hook.Install("fish", homeDir, shareHooksDir); err != nil {
		t.Fatalf("Install(fish): %v", err)
	}

	installed, err = hook.Status("fish", homeDir)
	if err != nil {
		t.Fatalf("Status(fish) after install: %v", err)
	}
	if !installed {
		t.Errorf("Status(fish) expected true after install")
	}
}

// TestInstallHookFilePermissions verifies that hook files written to shareHooksDir
// have 0644 permissions.
func TestInstallHookFilePermissions(t *testing.T) {
	homeDir := t.TempDir()
	shareHooksDir := makeShareHooksDir(t)

	if err := hook.Install("bash", homeDir, shareHooksDir); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	for _, name := range []string{"tsuku.bash", "tsuku.zsh", "tsuku.fish"} {
		path := filepath.Join(shareHooksDir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
			continue
		}
		if info.Mode().Perm() != 0644 {
			t.Errorf("expected %s to have permissions 0644, got %04o", name, info.Mode().Perm())
		}
	}
}
