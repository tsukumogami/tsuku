package discover

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerate_SeedsOnly(t *testing.T) {
	dir := t.TempDir()
	seedsDir := filepath.Join(dir, "seeds")
	if err := os.Mkdir(seedsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedsDir, "tools.json"), []byte(`{
		"category": "test",
		"entries": [
			{"name": "bat", "builder": "github", "source": "sharkdp/bat"},
			{"name": "fd", "builder": "github", "source": "sharkdp/fd"}
		]
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(dir, "discovery")
	result, err := Generate(GenerateConfig{
		SeedsDir:  seedsDir,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("total = %d, want 2", result.Total)
	}
	if result.Valid != 2 {
		t.Errorf("valid = %d, want 2", result.Valid)
	}

	// Verify individual files exist
	for _, name := range []string{"bat", "fd"} {
		path := filepath.Join(outputDir, RegistryEntryPath(name))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file for %s at %s: %v", name, path, err)
		}
	}

	// Verify we can load the directory back
	reg, err := LoadRegistryDir(outputDir)
	if err != nil {
		t.Fatalf("LoadRegistryDir: %v", err)
	}
	if len(reg.Tools) != 2 {
		t.Errorf("tools count = %d, want 2", len(reg.Tools))
	}
}

func TestGenerate_MergeQueueAndSeeds(t *testing.T) {
	dir := t.TempDir()

	queuePath := filepath.Join(dir, "queue.json")
	if err := os.WriteFile(queuePath, []byte(`{
		"schema_version": 1,
		"packages": [
			{"id": "homebrew:jq", "source": "homebrew", "name": "jq"},
			{"id": "homebrew:gh", "source": "homebrew", "name": "gh"}
		]
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	seedsDir := filepath.Join(dir, "seeds")
	if err := os.Mkdir(seedsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedsDir, "overrides.json"), []byte(`{
		"category": "overrides",
		"entries": [
			{"name": "jq", "builder": "github", "source": "jqlang/jq"}
		]
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(dir, "discovery")
	result, err := Generate(GenerateConfig{
		SeedsDir:  seedsDir,
		QueueFile: queuePath,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("total = %d, want 2", result.Total)
	}

	// Load and verify jq uses github builder (seed overrides queue)
	entry, err := LoadRegistryEntry(outputDir, "jq")
	if err != nil {
		t.Fatalf("load jq: %v", err)
	}
	if entry.Builder != "github" {
		t.Errorf("jq builder = %q, want github", entry.Builder)
	}

	entry, err = LoadRegistryEntry(outputDir, "gh")
	if err != nil {
		t.Fatalf("load gh: %v", err)
	}
	if entry.Builder != "homebrew" {
		t.Errorf("gh builder = %q, want homebrew", entry.Builder)
	}
}

func TestGenerate_WithValidation(t *testing.T) {
	dir := t.TempDir()
	seedsDir := filepath.Join(dir, "seeds")
	if err := os.Mkdir(seedsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedsDir, "tools.json"), []byte(`{
		"category": "test",
		"entries": [
			{"name": "good", "builder": "github", "source": "owner/good"},
			{"name": "bad", "builder": "github", "source": "owner/bad"}
		]
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	validators := map[string]Validator{
		"github": &conditionalValidator{
			allow: map[string]bool{"owner/good": true},
		},
	}

	outputDir := filepath.Join(dir, "discovery")
	result, err := Generate(GenerateConfig{
		SeedsDir:   seedsDir,
		OutputDir:  outputDir,
		Validators: validators,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid != 1 {
		t.Errorf("valid = %d, want 1", result.Valid)
	}
	if len(result.Failures) != 1 {
		t.Errorf("failures = %d, want 1", len(result.Failures))
	}
}

func TestGenerate_EmptyInputError(t *testing.T) {
	_, err := Generate(GenerateConfig{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestValidateExisting(t *testing.T) {
	dir := t.TempDir()
	// Write entries as individual files
	for name, entry := range map[string]RegistryEntry{
		"good": {Builder: "github", Source: "owner/good"},
		"bad":  {Builder: "github", Source: "owner/bad"},
	} {
		writeTestEntry(t, dir, name, entry)
	}

	validators := map[string]Validator{
		"github": &conditionalValidator{
			allow: map[string]bool{"owner/good": true},
		},
	}

	result, err := ValidateExisting(dir, validators)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("total = %d, want 2", result.Total)
	}
	if result.Valid != 1 {
		t.Errorf("valid = %d, want 1", result.Valid)
	}
}

func TestRegistryEntryPath(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"bat", filepath.Join("b", "ba", "bat.json")},
		{"ripgrep", filepath.Join("r", "ri", "ripgrep.json")},
		{"fd", filepath.Join("f", "fd", "fd.json")},
		{"a", filepath.Join("a", "_", "a.json")},
	}
	for _, tt := range tests {
		got := RegistryEntryPath(tt.name)
		if got != tt.want {
			t.Errorf("RegistryEntryPath(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestLoadRegistryEntry(t *testing.T) {
	dir := t.TempDir()
	writeTestEntry(t, dir, "bat", RegistryEntry{
		Builder:        "github",
		Source:         "sharkdp/bat",
		Disambiguation: true,
	})

	entry, err := LoadRegistryEntry(dir, "bat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.Builder != "github" {
		t.Errorf("builder = %q, want github", entry.Builder)
	}
	if !entry.Disambiguation {
		t.Error("expected disambiguation = true")
	}
}

func TestLoadRegistryEntry_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadRegistryEntry(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing entry")
	}
}

func TestLoadRegistryDir(t *testing.T) {
	dir := t.TempDir()
	writeTestEntry(t, dir, "bat", RegistryEntry{Builder: "github", Source: "sharkdp/bat"})
	writeTestEntry(t, dir, "fd", RegistryEntry{Builder: "github", Source: "sharkdp/fd"})

	reg, err := LoadRegistryDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reg.Tools) != 2 {
		t.Errorf("tools = %d, want 2", len(reg.Tools))
	}
	entry, ok := reg.Lookup("bat")
	if !ok {
		t.Fatal("expected bat in registry")
	}
	if entry.Source != "sharkdp/bat" {
		t.Errorf("bat source = %q, want sharkdp/bat", entry.Source)
	}
}

// Test helper
type conditionalValidator struct {
	allow map[string]bool
}

func (v *conditionalValidator) Validate(entry SeedEntry) error {
	if v.allow[entry.Source] {
		return nil
	}
	return fmt.Errorf("source %q not allowed", entry.Source)
}
