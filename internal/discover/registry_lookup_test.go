package discover

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTestEntry(t *testing.T, dir, name string, entry RegistryEntry) {
	t.Helper()
	relPath := RegistryEntryPath(name)
	fullPath := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryLookup_Hit(t *testing.T) {
	dir := t.TempDir()
	writeTestEntry(t, dir, "bat", RegistryEntry{Builder: "github", Source: "sharkdp/bat"})

	lookup, err := NewRegistryLookup(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := lookup.Resolve(context.Background(), "bat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Builder != "github" {
		t.Errorf("got builder %q, want %q", result.Builder, "github")
	}
	if result.Source != "sharkdp/bat" {
		t.Errorf("got source %q, want %q", result.Source, "sharkdp/bat")
	}
	if result.Confidence != ConfidenceRegistry {
		t.Errorf("got confidence %q, want %q", result.Confidence, ConfidenceRegistry)
	}
}

func TestRegistryLookup_Miss(t *testing.T) {
	dir := t.TempDir()
	lookup, _ := NewRegistryLookup(dir)

	result, err := lookup.Resolve(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestNewRegistryLookup_EmptyDir(t *testing.T) {
	_, err := NewRegistryLookup("")
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}
