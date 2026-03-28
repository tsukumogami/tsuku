package shellenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
)

// setupProject creates a temp directory with a .tsuku.toml and optionally
// creates tool bin directories under a fake $TSUKU_HOME.
func setupProject(t *testing.T, tomlContent string, installedTools map[string]string) (projectDir string, cfg *config.Config) {
	t.Helper()

	projectDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".tsuku.toml"), []byte(tomlContent), 0644); err != nil {
		t.Fatal(err)
	}

	tsukuHome := t.TempDir()
	cfg = &config.Config{
		HomeDir:  tsukuHome,
		ToolsDir: filepath.Join(tsukuHome, "tools"),
	}

	for name, version := range installedTools {
		binDir := cfg.ToolBinDir(name, version)
		if err := os.MkdirAll(binDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	return projectDir, cfg
}

func TestComputeActivation_ProjectFound(t *testing.T) {
	toml := `
[tools]
go = "1.22"
node = "20.16.0"
`
	projectDir, cfg := setupProject(t, toml, map[string]string{
		"go":   "1.22",
		"node": "20.16.0",
	})

	t.Setenv("PATH", "/usr/bin:/bin")
	// Prevent LoadProjectConfig from stopping at $HOME ceiling.
	t.Setenv("HOME", filepath.Dir(projectDir))

	result, err := ComputeActivation(projectDir, "", "", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected activation result, got nil")
	}
	if !result.Active {
		t.Error("expected Active=true")
	}
	if result.Dir != projectDir {
		t.Errorf("Dir = %q, want %q", result.Dir, projectDir)
	}
	if result.PrevPath != "/usr/bin:/bin" {
		t.Errorf("PrevPath = %q, want %q", result.PrevPath, "/usr/bin:/bin")
	}
	if len(result.Skipped) != 0 {
		t.Errorf("Skipped = %v, want empty", result.Skipped)
	}

	// PATH should contain both tool bin dirs prepended to the original.
	goBin := cfg.ToolBinDir("go", "1.22")
	nodeBin := cfg.ToolBinDir("node", "20.16.0")
	if !strings.Contains(result.PATH, goBin) {
		t.Errorf("PATH missing go bin dir %q", goBin)
	}
	if !strings.Contains(result.PATH, nodeBin) {
		t.Errorf("PATH missing node bin dir %q", nodeBin)
	}
	if !strings.HasSuffix(result.PATH, ":/usr/bin:/bin") {
		t.Errorf("PATH should end with original PATH, got %q", result.PATH)
	}
}

func TestComputeActivation_SameDirectory(t *testing.T) {
	toml := `
[tools]
go = "1.22"
`
	projectDir, cfg := setupProject(t, toml, map[string]string{"go": "1.22"})

	// When cwd == curDir, should return nil (no-op).
	result, err := ComputeActivation(projectDir, "/usr/bin", projectDir, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for same directory, got %+v", result)
	}
}

func TestComputeActivation_NoConfig(t *testing.T) {
	// Directory without .tsuku.toml.
	dir := t.TempDir()
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		ToolsDir: filepath.Join(t.TempDir(), "tools"),
	}

	// Prevent walking up to find a real config.
	t.Setenv("HOME", dir)

	result, err := ComputeActivation(dir, "", "", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil when no .tsuku.toml, got %+v", result)
	}
}

func TestComputeActivation_SkippedTools(t *testing.T) {
	toml := `
[tools]
go = "1.22"
node = "20.16.0"
python = "3.12"
`
	// Only install go; node and python are missing.
	projectDir, cfg := setupProject(t, toml, map[string]string{
		"go": "1.22",
	})

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	result, err := ComputeActivation(projectDir, "", "", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected activation result, got nil")
	}

	// node and python should be skipped (sorted).
	if len(result.Skipped) != 2 {
		t.Fatalf("Skipped = %v, want 2 entries", result.Skipped)
	}
	if result.Skipped[0] != "node" || result.Skipped[1] != "python" {
		t.Errorf("Skipped = %v, want [node python]", result.Skipped)
	}

	// PATH should still contain the installed go bin dir.
	goBin := cfg.ToolBinDir("go", "1.22")
	if !strings.Contains(result.PATH, goBin) {
		t.Errorf("PATH missing go bin dir %q", goBin)
	}
}

func TestComputeActivation_EmptyVersion(t *testing.T) {
	toml := `
[tools]
jq = ""
`
	projectDir, cfg := setupProject(t, toml, nil)

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	result, err := ComputeActivation(projectDir, "", "", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected activation result, got nil")
	}
	// jq with empty version should be skipped.
	if len(result.Skipped) != 1 || result.Skipped[0] != "jq" {
		t.Errorf("Skipped = %v, want [jq]", result.Skipped)
	}
}

func TestComputeActivation_UsesPrevPath(t *testing.T) {
	toml := `
[tools]
go = "1.22"
`
	projectDir, cfg := setupProject(t, toml, map[string]string{"go": "1.22"})

	t.Setenv("PATH", "/something/modified:/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	// When prevPath is provided, it should be used as the base instead of $PATH.
	result, err := ComputeActivation(projectDir, "/original/bin:/usr/bin", "", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected activation result, got nil")
	}
	if result.PrevPath != "/original/bin:/usr/bin" {
		t.Errorf("PrevPath = %q, want %q", result.PrevPath, "/original/bin:/usr/bin")
	}
	if !strings.HasSuffix(result.PATH, ":/original/bin:/usr/bin") {
		t.Errorf("PATH should use prevPath as base, got %q", result.PATH)
	}
}

func TestFormatExports_Bash(t *testing.T) {
	result := &ActivationResult{
		PATH:     "/tools/go-1.22/bin:/usr/bin",
		Dir:      "/home/user/project",
		PrevPath: "/usr/bin",
		Active:   true,
	}

	output := FormatExports(result, "bash")

	if !strings.Contains(output, `export PATH="/tools/go-1.22/bin:/usr/bin"`) {
		t.Errorf("missing PATH export in:\n%s", output)
	}
	if !strings.Contains(output, `export _TSUKU_DIR="/home/user/project"`) {
		t.Errorf("missing _TSUKU_DIR export in:\n%s", output)
	}
	if !strings.Contains(output, `export _TSUKU_PREV_PATH="/usr/bin"`) {
		t.Errorf("missing _TSUKU_PREV_PATH export in:\n%s", output)
	}
}

func TestFormatExports_Zsh(t *testing.T) {
	result := &ActivationResult{
		PATH:     "/tools/go-1.22/bin:/usr/bin",
		Dir:      "/home/user/project",
		PrevPath: "/usr/bin",
		Active:   true,
	}

	output := FormatExports(result, "zsh")

	// zsh uses the same syntax as bash.
	if !strings.Contains(output, "export PATH=") {
		t.Errorf("missing export keyword in:\n%s", output)
	}
}

func TestFormatExports_Fish(t *testing.T) {
	result := &ActivationResult{
		PATH:     "/tools/go-1.22/bin:/usr/bin",
		Dir:      "/home/user/project",
		PrevPath: "/usr/bin",
		Active:   true,
	}

	output := FormatExports(result, "fish")

	if !strings.Contains(output, "set -gx PATH") {
		t.Errorf("missing fish PATH in:\n%s", output)
	}
	if !strings.Contains(output, "set -gx _TSUKU_DIR") {
		t.Errorf("missing fish _TSUKU_DIR in:\n%s", output)
	}
	if !strings.Contains(output, "set -gx _TSUKU_PREV_PATH") {
		t.Errorf("missing fish _TSUKU_PREV_PATH in:\n%s", output)
	}
	// Should not contain "export".
	if strings.Contains(output, "export") {
		t.Errorf("fish output should not contain 'export':\n%s", output)
	}
}

func TestFormatExports_Nil(t *testing.T) {
	output := FormatExports(nil, "bash")
	if output != "" {
		t.Errorf("expected empty output for nil result, got %q", output)
	}
}
