package version

import (
	"context"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestCaskProvider_ResolveLatest(t *testing.T) {
	resolver := New()
	provider := NewCaskProvider(resolver, "iterm2")

	info, err := provider.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest() error = %v", err)
	}

	// Verify version
	if info.Version == "" {
		t.Error("ResolveLatest() returned empty version")
	}

	// Verify metadata contains expected fields
	if info.Metadata == nil {
		t.Fatal("ResolveLatest() returned nil metadata")
	}

	url, ok := info.Metadata["url"]
	if !ok || url == "" {
		t.Error("Metadata missing 'url' field")
	}

	checksum, ok := info.Metadata["checksum"]
	if !ok || checksum == "" {
		t.Error("Metadata missing 'checksum' field")
	}
	if !strings.HasPrefix(checksum, "sha256:") {
		t.Errorf("Checksum should start with 'sha256:', got %q", checksum)
	}
}

func TestCaskProvider_ResolveVersion(t *testing.T) {
	resolver := New()
	provider := NewCaskProvider(resolver, "iterm2")

	// Get the hardcoded version first
	latest, err := provider.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest() error = %v", err)
	}

	// Test exact version match
	info, err := provider.ResolveVersion(context.Background(), latest.Version)
	if err != nil {
		t.Fatalf("ResolveVersion() error = %v", err)
	}

	if info.Version != latest.Version {
		t.Errorf("ResolveVersion() version = %q, want %q", info.Version, latest.Version)
	}

	// Test non-existent version
	_, err = provider.ResolveVersion(context.Background(), "99.99.99")
	if err == nil {
		t.Error("ResolveVersion() expected error for non-existent version")
	}
}

func TestCaskProvider_ResolveLatest_UnknownCask(t *testing.T) {
	resolver := New()
	provider := NewCaskProvider(resolver, "unknown-cask-that-does-not-exist")

	_, err := provider.ResolveLatest(context.Background())
	if err == nil {
		t.Error("ResolveLatest() expected error for unknown cask")
	}
}

func TestCaskProvider_SourceDescription(t *testing.T) {
	resolver := New()
	provider := NewCaskProvider(resolver, "iterm2")

	desc := provider.SourceDescription()
	if desc != "Cask:iterm2" {
		t.Errorf("SourceDescription() = %q, want %q", desc, "Cask:iterm2")
	}
}

func TestCaskProvider_Interface(t *testing.T) {
	resolver := New()
	provider := NewCaskProvider(resolver, "iterm2")

	// Verify it implements VersionResolver
	var _ VersionResolver = provider
}

func TestCaskSourceStrategy_CanHandle(t *testing.T) {
	tests := []struct {
		name   string
		source string
		cask   string
		want   bool
	}{
		{"cask source with cask name", "cask", "iterm2", true},
		{"cask source without cask name", "cask", "", false},
		{"different source", "homebrew", "iterm2", false},
		{"empty source", "", "iterm2", false},
	}

	strategy := &CaskSourceStrategy{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recipe.Recipe{
				Version: recipe.VersionSection{
					Source: tt.source,
					Cask:   tt.cask,
				},
			}
			if got := strategy.CanHandle(r); got != tt.want {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want)
			}
		})
	}
}
