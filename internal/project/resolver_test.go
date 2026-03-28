package project

import (
	"context"
	"errors"
	"testing"

	"github.com/tsukumogami/tsuku/internal/index"
)

func TestResolver_CommandInIndexAndConfig(t *testing.T) {
	cfg := &ConfigResult{
		Config: &ProjectConfig{
			Tools: map[string]ToolRequirement{
				"jq": {Version: "1.7.1"},
			},
		},
	}
	lookup := func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}

	r := NewResolver(cfg, lookup)
	version, ok, err := r.ProjectVersionFor(context.Background(), "jq")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for command in config")
	}
	if version != "1.7.1" {
		t.Errorf("version = %q, want %q", version, "1.7.1")
	}
}

func TestResolver_CommandInIndexButNotConfig(t *testing.T) {
	cfg := &ConfigResult{
		Config: &ProjectConfig{
			Tools: map[string]ToolRequirement{
				"ripgrep": {Version: "14.0.0"},
			},
		},
	}
	lookup := func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}

	r := NewResolver(cfg, lookup)
	version, ok, err := r.ProjectVersionFor(context.Background(), "jq")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for command not in config")
	}
	if version != "" {
		t.Errorf("version = %q, want empty", version)
	}
}

func TestResolver_CommandNotInIndex(t *testing.T) {
	cfg := &ConfigResult{
		Config: &ProjectConfig{
			Tools: map[string]ToolRequirement{
				"jq": {Version: "1.7.1"},
			},
		},
	}
	lookup := func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return nil, nil
	}

	r := NewResolver(cfg, lookup)
	version, ok, err := r.ProjectVersionFor(context.Background(), "unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for command not in index")
	}
	if version != "" {
		t.Errorf("version = %q, want empty", version)
	}
}

func TestResolver_NilConfig(t *testing.T) {
	lookup := func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		t.Fatal("lookup should not be called when config is nil")
		return nil, nil
	}

	r := NewResolver(nil, lookup)
	version, ok, err := r.ProjectVersionFor(context.Background(), "jq")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for nil config")
	}
	if version != "" {
		t.Errorf("version = %q, want empty", version)
	}
}

func TestResolver_LookupErrorPropagation(t *testing.T) {
	cfg := &ConfigResult{
		Config: &ProjectConfig{
			Tools: map[string]ToolRequirement{
				"jq": {Version: "1.7.1"},
			},
		},
	}
	lookupErr := errors.New("index corrupted")
	lookup := func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return nil, lookupErr
	}

	r := NewResolver(cfg, lookup)
	_, _, err := r.ProjectVersionFor(context.Background(), "jq")
	if !errors.Is(err, lookupErr) {
		t.Fatalf("expected lookup error to propagate, got %v", err)
	}
}

func TestResolver_MultipleMatchesFirstConfigWins(t *testing.T) {
	cfg := &ConfigResult{
		Config: &ProjectConfig{
			Tools: map[string]ToolRequirement{
				"jq-alt": {Version: "2.0.0"},
			},
		},
	}
	lookup := func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{
			{Recipe: "jq", Command: "jq"},
			{Recipe: "jq-alt", Command: "jq"},
		}, nil
	}

	r := NewResolver(cfg, lookup)
	version, ok, err := r.ProjectVersionFor(context.Background(), "jq")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for command matching second recipe in config")
	}
	if version != "2.0.0" {
		t.Errorf("version = %q, want %q", version, "2.0.0")
	}
}

func TestResolver_EmptyVersionInConfig(t *testing.T) {
	cfg := &ConfigResult{
		Config: &ProjectConfig{
			Tools: map[string]ToolRequirement{
				"jq": {Version: ""},
			},
		},
	}
	lookup := func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}

	r := NewResolver(cfg, lookup)
	version, ok, err := r.ProjectVersionFor(context.Background(), "jq")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for recipe in config even with empty version")
	}
	if version != "" {
		t.Errorf("version = %q, want empty (use latest)", version)
	}
}
