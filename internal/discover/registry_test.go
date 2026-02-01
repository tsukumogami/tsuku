package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRegistry_Valid(t *testing.T) {
	data := []byte(`{
		"schema_version": 1,
		"tools": {
			"kubectl": {"builder": "github", "source": "kubernetes/kubernetes", "binary": "kubectl"},
			"ripgrep": {"builder": "github", "source": "BurntSushi/ripgrep"}
		}
	}`)
	reg, err := ParseRegistry(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reg.Tools) != 2 {
		t.Errorf("got %d tools, want 2", len(reg.Tools))
	}
}

func TestParseRegistry_WrongVersion(t *testing.T) {
	data := []byte(`{"schema_version": 99, "tools": {}}`)
	_, err := ParseRegistry(data)
	if err == nil {
		t.Fatal("expected error for unsupported schema version")
	}
}

func TestParseRegistry_Malformed(t *testing.T) {
	_, err := ParseRegistry([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestRegistry_LookupCaseInsensitive(t *testing.T) {
	data := []byte(`{
		"schema_version": 1,
		"tools": {
			"kubectl": {"builder": "github", "source": "kubernetes/kubernetes"}
		}
	}`)
	reg, err := ParseRegistry(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, name := range []string{"kubectl", "Kubectl", "KUBECTL"} {
		entry, ok := reg.Lookup(name)
		if !ok {
			t.Errorf("lookup %q: expected hit", name)
			continue
		}
		if entry.Builder != "github" {
			t.Errorf("lookup %q: got builder %q, want %q", name, entry.Builder, "github")
		}
	}
}

func TestRegistry_LookupMiss(t *testing.T) {
	data := []byte(`{"schema_version": 1, "tools": {}}`)
	reg, err := ParseRegistry(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok := reg.Lookup("nonexistent")
	if ok {
		t.Error("expected miss for nonexistent tool")
	}
}

func TestParseRegistry_EmptyBuilder(t *testing.T) {
	data := []byte(`{"schema_version": 1, "tools": {"kubectl": {"builder": "", "source": "kubernetes/kubernetes"}}}`)
	_, err := ParseRegistry(data)
	if err == nil {
		t.Fatal("expected error for empty builder")
	}
	if !strings.Contains(err.Error(), "kubectl") || !strings.Contains(err.Error(), "builder") {
		t.Errorf("error should mention tool name and field: %v", err)
	}
}

func TestParseRegistry_MissingBuilder(t *testing.T) {
	data := []byte(`{"schema_version": 1, "tools": {"kubectl": {"source": "kubernetes/kubernetes"}}}`)
	_, err := ParseRegistry(data)
	if err == nil {
		t.Fatal("expected error for missing builder")
	}
	if !strings.Contains(err.Error(), "builder") {
		t.Errorf("error should mention builder field: %v", err)
	}
}

func TestParseRegistry_EmptySource(t *testing.T) {
	data := []byte(`{"schema_version": 1, "tools": {"kubectl": {"builder": "github", "source": ""}}}`)
	_, err := ParseRegistry(data)
	if err == nil {
		t.Fatal("expected error for empty source")
	}
	if !strings.Contains(err.Error(), "kubectl") || !strings.Contains(err.Error(), "source") {
		t.Errorf("error should mention tool name and field: %v", err)
	}
}

func TestParseRegistry_MissingSource(t *testing.T) {
	data := []byte(`{"schema_version": 1, "tools": {"kubectl": {"builder": "github"}}}`)
	_, err := ParseRegistry(data)
	if err == nil {
		t.Fatal("expected error for missing source")
	}
	if !strings.Contains(err.Error(), "source") {
		t.Errorf("error should mention source field: %v", err)
	}
}

func TestParseRegistry_OptionalBinaryAllowed(t *testing.T) {
	data := []byte(`{"schema_version": 1, "tools": {
		"kubectl": {"builder": "github", "source": "kubernetes/kubernetes", "binary": "kubectl"},
		"ripgrep": {"builder": "github", "source": "BurntSushi/ripgrep"}
	}}`)
	reg, err := ParseRegistry(data)
	if err != nil {
		t.Fatalf("optional binary field should not cause error: %v", err)
	}
	if len(reg.Tools) != 2 {
		t.Errorf("got %d tools, want 2", len(reg.Tools))
	}
}

func TestParseRegistry_OptionalMetadata(t *testing.T) {
	data := []byte(`{
		"schema_version": 1,
		"tools": {
			"ripgrep": {
				"builder": "github",
				"source": "BurntSushi/ripgrep",
				"description": "A fast search tool",
				"homepage": "https://github.com/BurntSushi/ripgrep",
				"repo": "BurntSushi/ripgrep"
			},
			"bat": {
				"builder": "github",
				"source": "sharkdp/bat",
				"disambiguation": true
			}
		}
	}`)
	reg, err := ParseRegistry(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reg.Tools) != 2 {
		t.Errorf("got %d tools, want 2", len(reg.Tools))
	}
	rg := reg.Tools["ripgrep"]
	if rg.Description != "A fast search tool" {
		t.Errorf("description = %q, want %q", rg.Description, "A fast search tool")
	}
	bat := reg.Tools["bat"]
	if !bat.Disambiguation {
		t.Error("bat.Disambiguation should be true")
	}
}

func TestParseRegistry_MetadataOptional(t *testing.T) {
	// Entries without metadata fields should work fine
	data := []byte(`{
		"schema_version": 1,
		"tools": {
			"jq": {"builder": "homebrew", "source": "jq"}
		}
	}`)
	reg, err := ParseRegistry(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.Tools["jq"].Description != "" {
		t.Error("expected empty description for minimal entry")
	}
}

func TestParseRegistry_LookupWithMetadata(t *testing.T) {
	data := []byte(`{
		"schema_version": 1,
		"tools": {
			"ripgrep": {"builder": "github", "source": "BurntSushi/ripgrep", "description": "fast grep"}
		}
	}`)
	reg, err := ParseRegistry(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entry, ok := reg.Lookup("ripgrep")
	if !ok {
		t.Fatal("expected hit")
	}
	if entry.Builder != "github" || entry.Source != "BurntSushi/ripgrep" {
		t.Errorf("lookup returned wrong entry: %+v", entry)
	}
}

func TestParseRegistry_Version2Rejected(t *testing.T) {
	data := []byte(`{"schema_version": 2, "tools": {}}`)
	_, err := ParseRegistry(data)
	if err == nil {
		t.Fatal("expected error for schema version 2")
	}
}

func TestLoadRegistry_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "discovery.json")
	data := []byte(`{"schema_version": 1, "tools": {"jq": {"builder": "github", "source": "jqlang/jq"}}}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	reg, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entry, ok := reg.Lookup("jq")
	if !ok {
		t.Fatal("expected hit for jq")
	}
	if entry.Source != "jqlang/jq" {
		t.Errorf("got source %q, want %q", entry.Source, "jqlang/jq")
	}
}
