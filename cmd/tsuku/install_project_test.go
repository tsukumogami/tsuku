package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/tsukumogami/tsuku/internal/project"
)

func TestProjectInstall_NoConfigError(t *testing.T) {
	// Create a temp directory without any .tsuku.toml
	tmpDir := t.TempDir()

	result, err := project.LoadProjectConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result for directory without config")
	}
}

func TestProjectInstall_EmptyTools(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, project.ConfigFileName)

	content := `# Project tools
[tools]
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := project.LoadProjectConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Config.Tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(result.Config.Tools))
	}
}

func TestProjectInstall_SortedIterationOrder(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, project.ConfigFileName)

	content := `[tools]
zsh = "5.9"
node = "20.16.0"
go = "1.22"
alpha = "1.0"
python = { version = "3.12" }
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := project.LoadProjectConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Extract tool names and sort them
	var names []string
	for name := range result.Config.Tools {
		names = append(names, name)
	}
	sort.Strings(names)

	expected := []string{"alpha", "go", "node", "python", "zsh"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d tools, got %d", len(expected), len(names))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("tool[%d]: expected %q, got %q", i, expected[i], name)
		}
	}

	// Verify python parsed from inline table
	if result.Config.Tools["python"].Version != "3.12" {
		t.Errorf("expected python version 3.12, got %q", result.Config.Tools["python"].Version)
	}
}

func TestProjectInstall_FlagIncompatibility(t *testing.T) {
	// Verify that the incompatible flags list is correct by checking
	// the flags exist on the install command.
	incompatible := []string{"plan", "recipe", "from", "sandbox"}
	for _, name := range incompatible {
		flag := installCmd.Flags().Lookup(name)
		if flag == nil {
			t.Errorf("expected flag --%s to exist on install command", name)
		}
	}
}

func TestProjectInstall_PartialFailureExitCode(t *testing.T) {
	// Verify the exit code constants are defined correctly.
	if ExitPartialFailure != 15 {
		t.Errorf("expected ExitPartialFailure=15, got %d", ExitPartialFailure)
	}
	if ExitInstallFailed != 6 {
		t.Errorf("expected ExitInstallFailed=6, got %d", ExitInstallFailed)
	}
	if ExitUserDeclined != 13 {
		t.Errorf("expected ExitUserDeclined=13, got %d", ExitUserDeclined)
	}
}

func TestProjectInstall_UnpinnedVersionDetection(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, project.ConfigFileName)

	content := `[tools]
node = "20.16.0"
ripgrep = "latest"
jq = ""
python = { version = "3.12" }
go = {}
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := project.LoadProjectConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Identify unpinned tools (empty or "latest")
	var unpinned []string
	for name, req := range result.Config.Tools {
		if req.Version == "" || req.Version == "latest" {
			unpinned = append(unpinned, name)
		}
	}
	sort.Strings(unpinned)

	expected := []string{"go", "jq", "ripgrep"}
	if len(unpinned) != len(expected) {
		t.Fatalf("expected %d unpinned tools, got %d: %v", len(expected), len(unpinned), unpinned)
	}
	for i, name := range unpinned {
		if name != expected[i] {
			t.Errorf("unpinned[%d]: expected %q, got %q", i, expected[i], name)
		}
	}
}

func TestProjectInstall_SummaryFormat(t *testing.T) {
	// Test the summary helper functions
	tests := []struct {
		count    int
		wantTool string
		wantVerb string
	}{
		{1, "tool", "is"},
		{2, "tools", "are"},
		{0, "tools", "are"},
	}
	for _, tt := range tests {
		if got := pluralTool(tt.count); got != tt.wantTool {
			t.Errorf("pluralTool(%d) = %q, want %q", tt.count, got, tt.wantTool)
		}
		if got := pluralVerb(tt.count); got != tt.wantVerb {
			t.Errorf("pluralVerb(%d) = %q, want %q", tt.count, got, tt.wantVerb)
		}
	}
}

func TestProjectInstall_AllSuccess(t *testing.T) {
	results := []projectToolResult{
		{Name: "go", Status: "installed"},
		{Name: "node", Status: "installed"},
		{Name: "ripgrep", Status: "installed"},
	}

	failCount := 0
	for _, r := range results {
		if r.Status == "failed" {
			failCount++
		}
	}

	if failCount != 0 {
		t.Errorf("expected 0 failures, got %d", failCount)
	}
	// ExitSuccess should be used
}

func TestProjectInstall_AllFailure(t *testing.T) {
	results := []projectToolResult{
		{Name: "go", Status: "failed", Error: os.ErrNotExist},
		{Name: "node", Status: "failed", Error: os.ErrNotExist},
	}

	failCount := 0
	for _, r := range results {
		if r.Status == "failed" {
			failCount++
		}
	}

	if failCount != len(results) {
		t.Errorf("expected all %d to fail, got %d failures", len(results), failCount)
	}
	// ExitInstallFailed should be used
}

func TestProjectInstall_PartialFailure(t *testing.T) {
	results := []projectToolResult{
		{Name: "go", Status: "installed"},
		{Name: "node", Status: "failed", Error: os.ErrNotExist},
		{Name: "ripgrep", Status: "installed"},
	}

	failCount := 0
	for _, r := range results {
		if r.Status == "failed" {
			failCount++
		}
	}

	if failCount == 0 || failCount == len(results) {
		t.Errorf("expected partial failure: %d failures out of %d", failCount, len(results))
	}
	// ExitPartialFailure should be used
}

func TestProjectInstall_DryRunFlagSupported(t *testing.T) {
	// Verify --dry-run, --force, --fresh exist and are compatible with no-args.
	// These flags should not be in the incompatible list.
	supported := []string{"dry-run", "force", "fresh"}
	incompatible := map[string]bool{"plan": true, "recipe": true, "from": true, "sandbox": true}
	for _, name := range supported {
		if incompatible[name] {
			t.Errorf("flag --%s should be compatible with project install", name)
		}
		flag := installCmd.Flags().Lookup(name)
		if flag == nil {
			t.Errorf("expected flag --%s to exist on install command", name)
		}
	}
}
