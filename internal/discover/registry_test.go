package discover

import (
	"os"
	"path/filepath"
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
