package discover

import (
	"context"
	"testing"
)

func TestRegistryLookup_Hit(t *testing.T) {
	reg, _ := ParseRegistry([]byte(`{
		"schema_version": 1,
		"tools": {"bat": {"builder": "github", "source": "sharkdp/bat"}}
	}`))
	lookup, err := NewRegistryLookup(reg)
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
	reg, _ := ParseRegistry([]byte(`{"schema_version": 1, "tools": {}}`))
	lookup, _ := NewRegistryLookup(reg)

	result, err := lookup.Resolve(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestNewRegistryLookup_NilRegistry(t *testing.T) {
	_, err := NewRegistryLookup(nil)
	if err == nil {
		t.Fatal("expected error for nil registry")
	}
}
