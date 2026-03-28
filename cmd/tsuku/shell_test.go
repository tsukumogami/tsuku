package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
)

// shellSetupProject creates a temp directory with a .tsuku.toml and optionally
// creates tool bin directories under a fake $TSUKU_HOME.
func shellSetupProject(t *testing.T, tomlContent string, installedTools map[string]string) (projectDir string, cfg *config.Config) {
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

func TestRunShell_ActivationOutput(t *testing.T) {
	toml := `
[tools]
go = "1.22"
`
	projectDir, cfg := shellSetupProject(t, toml, map[string]string{"go": "1.22"})

	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	output, err := runShell(projectDir, "", "bash", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output == "" {
		t.Fatal("expected non-empty output")
	}

	// Should contain export statements for PATH, _TSUKU_DIR, and _TSUKU_PREV_PATH.
	if !strings.Contains(output, "export PATH=") {
		t.Errorf("missing PATH export in:\n%s", output)
	}
	if !strings.Contains(output, "export _TSUKU_DIR=") {
		t.Errorf("missing _TSUKU_DIR export in:\n%s", output)
	}
	if !strings.Contains(output, "export _TSUKU_PREV_PATH=") {
		t.Errorf("missing _TSUKU_PREV_PATH export in:\n%s", output)
	}

	// PATH should include the tool bin dir.
	goBin := cfg.ToolBinDir("go", "1.22")
	if !strings.Contains(output, goBin) {
		t.Errorf("PATH should contain go bin dir %q, got:\n%s", goBin, output)
	}
}

func TestRunShell_NoConfigReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		ToolsDir: filepath.Join(t.TempDir(), "tools"),
	}

	// Prevent LoadProjectConfig from walking up to find a real config.
	t.Setenv("HOME", dir)

	output, err := runShell(dir, "", "bash", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "" {
		t.Errorf("expected empty output when no .tsuku.toml, got %q", output)
	}
}

func TestRunShell_RepeatedInvocation(t *testing.T) {
	toml := `
[tools]
go = "1.22"
`
	projectDir, cfg := shellSetupProject(t, toml, map[string]string{"go": "1.22"})

	originalPath := "/usr/bin:/bin"
	t.Setenv("PATH", originalPath)
	t.Setenv("HOME", filepath.Dir(projectDir))

	// First activation.
	output1, err := runShell(projectDir, "", "bash", cfg)
	if err != nil {
		t.Fatalf("first activation error: %v", err)
	}

	// Second activation with prevPath set (simulates re-running in same shell).
	output2, err := runShell(projectDir, originalPath, "bash", cfg)
	if err != nil {
		t.Fatalf("second activation error: %v", err)
	}

	// Both should produce output.
	if output1 == "" || output2 == "" {
		t.Fatal("both activations should produce output")
	}

	// Both should use the original PATH as base, so _TSUKU_PREV_PATH should
	// be the same.
	if !strings.Contains(output2, originalPath) {
		t.Errorf("second activation should use prevPath as base, got:\n%s", output2)
	}
}

func TestRunShell_FishOutput(t *testing.T) {
	toml := `
[tools]
go = "1.22"
`
	projectDir, cfg := shellSetupProject(t, toml, map[string]string{"go": "1.22"})

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	output, err := runShell(projectDir, "", "fish", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "set -gx PATH") {
		t.Errorf("expected fish syntax, got:\n%s", output)
	}
	if strings.Contains(output, "export") {
		t.Errorf("fish output should not contain 'export':\n%s", output)
	}
}

func TestDetectShell_FlagTakesPrecedence(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")

	got := detectShell("fish")
	if got != "fish" {
		t.Errorf("detectShell(\"fish\") = %q, want \"fish\"", got)
	}
}

func TestDetectShell_FallsBackToEnv(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/zsh")

	got := detectShell("")
	if got != "zsh" {
		t.Errorf("detectShell(\"\") with SHELL=/usr/bin/zsh = %q, want \"zsh\"", got)
	}
}

func TestDetectShell_DefaultsBash(t *testing.T) {
	t.Setenv("SHELL", "")

	got := detectShell("")
	if got != "bash" {
		t.Errorf("detectShell(\"\") with no SHELL = %q, want \"bash\"", got)
	}
}
