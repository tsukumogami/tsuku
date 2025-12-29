package version

import (
	"context"
	"testing"
	"time"
)

// TestFossilTimelineProvider_Integration tests the provider against the real SQLite timeline.
// This is a long-running test that makes HTTP requests to sqlite.org.
// Run with: go test -v -run TestFossilTimelineProvider_Integration ./internal/version/
func TestFossilTimelineProvider_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	resolver := New()
	provider := NewFossilTimelineProvider(resolver, "https://sqlite.org/src", "sqlite")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test ListVersions
	t.Run("ListVersions", func(t *testing.T) {
		versions, err := provider.ListVersions(ctx)
		if err != nil {
			t.Fatalf("ListVersions failed: %v", err)
		}
		if len(versions) == 0 {
			t.Fatal("ListVersions returned no versions")
		}
		t.Logf("Found %d versions, first 5: %v", len(versions), versions[:min(5, len(versions))])
	})

	// Test ResolveLatest
	t.Run("ResolveLatest", func(t *testing.T) {
		info, err := provider.ResolveLatest(ctx)
		if err != nil {
			t.Fatalf("ResolveLatest failed: %v", err)
		}
		if info.Version == "" {
			t.Fatal("ResolveLatest returned empty version")
		}
		if info.Tag == "" {
			t.Fatal("ResolveLatest returned empty tag")
		}
		t.Logf("Latest version: %s (tag: %s)", info.Version, info.Tag)
	})

	// Test TarballURL
	t.Run("TarballURL", func(t *testing.T) {
		url := provider.TarballURL("3.51.1")
		expected := "https://sqlite.org/src/tarball/version-3.51.1/sqlite.tar.gz"
		if url != expected {
			t.Errorf("TarballURL = %q, want %q", url, expected)
		}
	})
}
