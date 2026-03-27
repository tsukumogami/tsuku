package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolRequirement_StringShorthand(t *testing.T) {
	content := `
[tools]
node = "20.16.0"
go = "1.22"
`
	result := loadFromString(t, content)
	if result.Config.Tools["node"].Version != "20.16.0" {
		t.Errorf("node version = %q, want %q", result.Config.Tools["node"].Version, "20.16.0")
	}
	if result.Config.Tools["go"].Version != "1.22" {
		t.Errorf("go version = %q, want %q", result.Config.Tools["go"].Version, "1.22")
	}
}

func TestToolRequirement_InlineTable(t *testing.T) {
	content := `
[tools]
python = { version = "3.12" }
`
	result := loadFromString(t, content)
	if result.Config.Tools["python"].Version != "3.12" {
		t.Errorf("python version = %q, want %q", result.Config.Tools["python"].Version, "3.12")
	}
}

func TestToolRequirement_EmptyVersion(t *testing.T) {
	content := `
[tools]
jq = ""
`
	result := loadFromString(t, content)
	if result.Config.Tools["jq"].Version != "" {
		t.Errorf("jq version = %q, want empty", result.Config.Tools["jq"].Version)
	}
}

func TestToolRequirement_LatestVersion(t *testing.T) {
	content := `
[tools]
ripgrep = "latest"
`
	result := loadFromString(t, content)
	if result.Config.Tools["ripgrep"].Version != "latest" {
		t.Errorf("ripgrep version = %q, want %q", result.Config.Tools["ripgrep"].Version, "latest")
	}
}

func TestLoadProjectConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	result, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for missing config, got %+v", result)
	}
}

func TestLoadProjectConfig_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "this is not valid toml [[[")

	_, err := LoadProjectConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid TOML, got nil")
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error should mention parsing, got: %v", err)
	}
}

func TestLoadProjectConfig_MaxToolsExceeded(t *testing.T) {
	dir := t.TempDir()

	var b strings.Builder
	b.WriteString("[tools]\n")
	for i := 0; i <= MaxTools; i++ {
		b.WriteString("tool")
		b.WriteString(strings.Repeat("x", 4)) // pad name
		b.WriteString(string(rune('a'+i%26)) + string(rune('a'+i/26%26)) + string(rune('a'+i/676%26)))
		b.WriteString(" = \"1.0\"\n")
	}
	writeConfig(t, dir, b.String())

	_, err := LoadProjectConfig(dir)
	if err == nil {
		t.Fatal("expected error for exceeding MaxTools, got nil")
	}
	if !strings.Contains(err.Error(), "maximum") {
		t.Errorf("error should mention maximum, got: %v", err)
	}
}

func TestLoadProjectConfig_ParentTraversal(t *testing.T) {
	// Create structure: root/sub/deep
	// Place config at root level.
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	deep := filepath.Join(sub, "deep")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}

	writeConfig(t, root, "[tools]\nnode = \"18.0.0\"\n")

	result, err := LoadProjectConfig(deep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected config from parent traversal, got nil")
	}
	if result.Dir != root {
		t.Errorf("config dir = %q, want %q", result.Dir, root)
	}
	if result.Config.Tools["node"].Version != "18.0.0" {
		t.Errorf("node version = %q, want %q", result.Config.Tools["node"].Version, "18.0.0")
	}
}

func TestLoadProjectConfig_CeilingPathsStopTraversal(t *testing.T) {
	// Create structure: root/sub/deep
	// Place config at root, set ceiling at sub.
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	deep := filepath.Join(sub, "deep")
	if err := os.MkdirAll(deep, 0755); err != nil {
		t.Fatal(err)
	}

	writeConfig(t, root, "[tools]\nnode = \"18.0.0\"\n")

	t.Setenv(EnvCeilingPaths, sub)

	result, err := LoadProjectConfig(deep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (ceiling should stop traversal), got config at %q", result.Dir)
	}
}

func TestLoadProjectConfig_SymlinkResolution(t *testing.T) {
	// Create real dir with config, then symlink to it.
	realDir := t.TempDir()
	writeConfig(t, realDir, "[tools]\ngo = \"1.22\"\n")

	linkParent := t.TempDir()
	linkPath := filepath.Join(linkParent, "linked")
	if err := os.Symlink(realDir, linkPath); err != nil {
		t.Skip("symlinks not supported on this platform")
	}

	result, err := LoadProjectConfig(linkPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected config via symlink resolution, got nil")
	}
	if result.Config.Tools["go"].Version != "1.22" {
		t.Errorf("go version = %q, want %q", result.Config.Tools["go"].Version, "1.22")
	}
}

func TestFindProjectDir_Found(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "[tools]\nnode = \"20.0.0\"\n")

	got := FindProjectDir(dir)
	if got != dir {
		t.Errorf("FindProjectDir = %q, want %q", got, dir)
	}
}

func TestFindProjectDir_NotFound(t *testing.T) {
	dir := t.TempDir()
	got := FindProjectDir(dir)
	if got != "" {
		t.Errorf("FindProjectDir = %q, want empty", got)
	}
}

func TestLoadProjectConfig_MixedFormats(t *testing.T) {
	content := `
[tools]
node = "20.16.0"
python = { version = "3.12" }
ripgrep = "latest"
jq = ""
`
	result := loadFromString(t, content)
	tools := result.Config.Tools
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}
	if tools["node"].Version != "20.16.0" {
		t.Errorf("node = %q", tools["node"].Version)
	}
	if tools["python"].Version != "3.12" {
		t.Errorf("python = %q", tools["python"].Version)
	}
	if tools["ripgrep"].Version != "latest" {
		t.Errorf("ripgrep = %q", tools["ripgrep"].Version)
	}
	if tools["jq"].Version != "" {
		t.Errorf("jq = %q", tools["jq"].Version)
	}
}

func TestLoadProjectConfig_EmptyToolsSection(t *testing.T) {
	content := "[tools]\n"
	result := loadFromString(t, content)
	if len(result.Config.Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(result.Config.Tools))
	}
}

func TestLoadProjectConfig_ConfigResultFields(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "[tools]\nnode = \"18.0.0\"\n")

	result, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != filepath.Join(dir, ConfigFileName) {
		t.Errorf("Path = %q, want %q", result.Path, filepath.Join(dir, ConfigFileName))
	}
	if result.Dir != dir {
		t.Errorf("Dir = %q, want %q", result.Dir, dir)
	}
}

// helpers

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func loadFromString(t *testing.T, content string) *ConfigResult {
	t.Helper()
	dir := t.TempDir()
	writeConfig(t, dir, content)
	result, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("LoadProjectConfig failed: %v", err)
	}
	if result == nil {
		t.Fatal("LoadProjectConfig returned nil")
	}
	return result
}
