package discover

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/builders"
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

func TestGenerate_MetadataWrittenToFiles(t *testing.T) {
	dir := t.TempDir()
	seedsDir := filepath.Join(dir, "seeds")
	if err := os.Mkdir(seedsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedsDir, "tools.json"), []byte(`{
		"category": "test",
		"entries": [
			{"name": "bat", "builder": "github", "source": "sharkdp/bat"},
			{"name": "jq", "builder": "homebrew", "source": "jq"}
		]
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(dir, "discovery")
	result, err := Generate(GenerateConfig{
		SeedsDir:  seedsDir,
		OutputDir: outputDir,
		Validators: map[string]Validator{
			"github": &enrichingValidator{meta: &EntryMetadata{
				Description: "A cat clone with wings",
				Homepage:    "https://github.com/sharkdp/bat",
				Repo:        "https://github.com/sharkdp/bat",
			}},
			"homebrew": &enrichingValidator{meta: &EntryMetadata{
				Description: "Lightweight JSON processor",
				Homepage:    "https://jqlang.github.io/jq/",
			}},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Valid != 2 {
		t.Fatalf("expected 2 valid, got %d", result.Valid)
	}

	// Verify bat entry has metadata
	batEntry, err := LoadRegistryEntry(outputDir, "bat")
	if err != nil {
		t.Fatalf("load bat: %v", err)
	}
	if batEntry.Description != "A cat clone with wings" {
		t.Errorf("bat description = %q, want 'A cat clone with wings'", batEntry.Description)
	}
	if batEntry.Homepage != "https://github.com/sharkdp/bat" {
		t.Errorf("bat homepage = %q", batEntry.Homepage)
	}
	if batEntry.Repo != "https://github.com/sharkdp/bat" {
		t.Errorf("bat repo = %q", batEntry.Repo)
	}

	// Verify jq entry has metadata (no repo for homebrew)
	jqEntry, err := LoadRegistryEntry(outputDir, "jq")
	if err != nil {
		t.Fatalf("load jq: %v", err)
	}
	if jqEntry.Description != "Lightweight JSON processor" {
		t.Errorf("jq description = %q", jqEntry.Description)
	}
	if jqEntry.Homepage != "https://jqlang.github.io/jq/" {
		t.Errorf("jq homepage = %q", jqEntry.Homepage)
	}
}

func TestProbeAndFilter_EnrichesAndRejects(t *testing.T) {
	// Use "crates.io" as builder name to match QualityFilter thresholds
	// (100 downloads OR 5 versions).
	entries := []SeedEntry{
		{Name: "ripgrep", Builder: "crates.io", Source: "ripgrep"},
		{Name: "squatter", Builder: "crates.io", Source: "squatter"},
		{Name: "no-prober", Builder: "other", Source: "whatever"},
	}

	probers := map[string]builders.EcosystemProber{
		"crates.io": &multiResultProber{
			name: "crates.io",
			results: map[string]*builders.ProbeResult{
				"ripgrep":  {Source: "ripgrep", Downloads: 500, VersionCount: 20, HasRepository: true},
				"squatter": {Source: "squatter", Downloads: 5, VersionCount: 1, HasRepository: false},
			},
		},
	}

	accepted, probed, rejections := ProbeAndFilter(context.Background(), entries, probers, false)

	if probed != 2 {
		t.Errorf("probed = %d, want 2", probed)
	}
	if len(rejections) != 1 {
		t.Fatalf("rejections = %d, want 1", len(rejections))
	}
	if rejections[0].Entry.Name != "squatter" {
		t.Errorf("rejected entry = %q, want squatter", rejections[0].Entry.Name)
	}
	if len(accepted) != 2 {
		t.Fatalf("accepted = %d, want 2", len(accepted))
	}

	// Check that ripgrep was enriched
	var rg SeedEntry
	for _, e := range accepted {
		if e.Name == "ripgrep" {
			rg = e
		}
	}
	if rg.Downloads != 500 {
		t.Errorf("ripgrep Downloads = %d, want 500", rg.Downloads)
	}
	if rg.VersionCount != 20 {
		t.Errorf("ripgrep VersionCount = %d, want 20", rg.VersionCount)
	}
	if !rg.HasRepository {
		t.Error("ripgrep HasRepository = false, want true")
	}

	// Check that no-prober entry passed through unchanged
	var np SeedEntry
	for _, e := range accepted {
		if e.Name == "no-prober" {
			np = e
		}
	}
	if np.Downloads != 0 || np.HasRepository {
		t.Error("no-prober entry should be unchanged")
	}
}

func TestProbeAndFilter_ProbeFailurePassesThrough(t *testing.T) {
	entries := []SeedEntry{
		{Name: "missing", Builder: "crates.io", Source: "missing"},
	}
	probers := map[string]builders.EcosystemProber{
		"crates.io": &multiResultProber{
			name:    "crates.io",
			results: map[string]*builders.ProbeResult{}, // returns nil for "missing"
		},
	}

	accepted, probed, rejections := ProbeAndFilter(context.Background(), entries, probers, false)
	if probed != 0 {
		t.Errorf("probed = %d, want 0 (nil result)", probed)
	}
	if len(rejections) != 0 {
		t.Errorf("rejections = %d, want 0", len(rejections))
	}
	if len(accepted) != 1 {
		t.Errorf("accepted = %d, want 1 (pass through)", len(accepted))
	}
}

func TestGenerate_QualityFieldsWrittenToFiles(t *testing.T) {
	dir := t.TempDir()
	seedsDir := filepath.Join(dir, "seeds")
	if err := os.Mkdir(seedsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedsDir, "tools.json"), []byte(`{
		"category": "test",
		"entries": [
			{"name": "good-tool", "builder": "homebrew", "source": "good-tool"}
		]
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(dir, "discovery")
	result, err := Generate(GenerateConfig{
		SeedsDir:  seedsDir,
		OutputDir: outputDir,
		Probers: map[string]builders.EcosystemProber{
			"homebrew": &multiResultProber{
				name: "homebrew",
				results: map[string]*builders.ProbeResult{
					"good-tool": {Source: "good-tool", Downloads: 1000, VersionCount: 10, HasRepository: true},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Probed != 1 {
		t.Errorf("probed = %d, want 1", result.Probed)
	}

	entry, err := LoadRegistryEntry(outputDir, "good-tool")
	if err != nil {
		t.Fatalf("load good-tool: %v", err)
	}
	if entry.Downloads != 1000 {
		t.Errorf("Downloads = %d, want 1000", entry.Downloads)
	}
	if entry.VersionCount != 10 {
		t.Errorf("VersionCount = %d, want 10", entry.VersionCount)
	}
	if !entry.HasRepository {
		t.Error("HasRepository = false, want true")
	}
}

// Test helpers

// multiResultProber implements builders.EcosystemProber with per-name results.
type multiResultProber struct {
	name    string
	results map[string]*builders.ProbeResult
}

func (m *multiResultProber) Name() string { return m.name }

func (m *multiResultProber) Probe(_ context.Context, name string) (*builders.ProbeResult, error) {
	r, ok := m.results[name]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (m *multiResultProber) RequiresLLM() bool { return false }
func (m *multiResultProber) CanBuild(_ context.Context, _ builders.BuildRequest) (bool, error) {
	return false, nil
}
func (m *multiResultProber) NewSession(_ context.Context, _ builders.BuildRequest, _ *builders.SessionOptions) (builders.BuildSession, error) {
	return nil, nil
}

type enrichingValidator struct {
	meta *EntryMetadata
}

func (v *enrichingValidator) Validate(entry SeedEntry) (*EntryMetadata, error) {
	return v.meta, nil
}

type conditionalValidator struct {
	allow map[string]bool
}

func (v *conditionalValidator) Validate(entry SeedEntry) (*EntryMetadata, error) {
	if v.allow[entry.Source] {
		return nil, nil
	}
	return nil, fmt.Errorf("source %q not allowed", entry.Source)
}
