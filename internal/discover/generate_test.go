package discover

import (
	"encoding/json"
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

	output := filepath.Join(dir, "discovery.json")
	result, err := Generate(GenerateConfig{
		SeedsDir: seedsDir,
		Output:   output,
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

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var reg registryFile
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if reg.SchemaVersion != 2 {
		t.Errorf("schema_version = %d, want 2", reg.SchemaVersion)
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

	output := filepath.Join(dir, "discovery.json")
	result, err := Generate(GenerateConfig{
		SeedsDir:  seedsDir,
		QueueFile: queuePath,
		Output:    output,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("total = %d, want 2", result.Total)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var reg registryFile
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatalf("parse output: %v", err)
	}

	if reg.Tools["jq"].Builder != "github" {
		t.Errorf("jq builder = %q, want github", reg.Tools["jq"].Builder)
	}
	if reg.Tools["gh"].Builder != "homebrew" {
		t.Errorf("gh builder = %q, want homebrew", reg.Tools["gh"].Builder)
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

	output := filepath.Join(dir, "discovery.json")
	result, err := Generate(GenerateConfig{
		SeedsDir:   seedsDir,
		Output:     output,
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

func TestGenerate_SortedOutput(t *testing.T) {
	dir := t.TempDir()
	seedsDir := filepath.Join(dir, "seeds")
	if err := os.Mkdir(seedsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedsDir, "tools.json"), []byte(`{
		"category": "test",
		"entries": [
			{"name": "zsh", "builder": "homebrew", "source": "zsh"},
			{"name": "awk", "builder": "homebrew", "source": "awk"}
		]
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "discovery.json")
	if _, err := Generate(GenerateConfig{SeedsDir: seedsDir, Output: output}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var reg registryFile
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if len(reg.Tools) != 2 {
		t.Errorf("tools = %d, want 2", len(reg.Tools))
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
	path := filepath.Join(dir, "discovery.json")
	if err := os.WriteFile(path, []byte(`{
		"schema_version": 1,
		"tools": {
			"good": {"builder": "github", "source": "owner/good"},
			"bad": {"builder": "github", "source": "owner/bad"}
		}
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	validators := map[string]Validator{
		"github": &conditionalValidator{
			allow: map[string]bool{"owner/good": true},
		},
	}

	result, err := ValidateExisting(path, validators)
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
