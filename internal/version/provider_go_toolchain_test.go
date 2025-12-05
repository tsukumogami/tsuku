package version

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// TestNormalizeGoToolchainVersion tests the version normalization function
func TestNormalizeGoToolchainVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"go1.23.4", "1.23.4"},
		{"go1.22.0", "1.22.0"},
		{"go1.21", "1.21"},
		{"1.23.4", "1.23.4"},       // Already normalized
		{"", ""},                   // Empty
		{"golang1.23", "lang1.23"}, // Non-standard prefix
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeGoToolchainVersion(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeGoToolchainVersion(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestResolveGoToolchain_ValidResponse tests successful version resolution
func TestResolveGoToolchain_ValidResponse(t *testing.T) {
	// Create mock go.dev/dl server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify it's the correct endpoint
		if r.URL.Path != "/dl/" || r.URL.Query().Get("mode") != "json" {
			t.Errorf("Expected /dl/?mode=json, got %s", r.URL.String())
		}

		response := []goRelease{
			{Version: "go1.23.4", Stable: true},
			{Version: "go1.22.8", Stable: true},
			{Version: "go1.21.0", Stable: true},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	ctx := context.Background()

	info, err := resolver.ResolveGoToolchain(ctx)
	if err != nil {
		t.Fatalf("ResolveGoToolchain failed: %v", err)
	}

	if info.Version != "1.23.4" {
		t.Errorf("Expected version 1.23.4, got %s", info.Version)
	}

	if info.Tag != "1.23.4" {
		t.Errorf("Expected tag 1.23.4, got %s", info.Tag)
	}
}

// TestResolveGoToolchain_OnlyStableVersions tests that unstable versions are skipped
func TestResolveGoToolchain_OnlyStableVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []goRelease{
			{Version: "go1.24rc1", Stable: false},   // RC version
			{Version: "go1.23beta1", Stable: false}, // Beta version
			{Version: "go1.23.4", Stable: true},     // Stable version
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	ctx := context.Background()

	info, err := resolver.ResolveGoToolchain(ctx)
	if err != nil {
		t.Fatalf("ResolveGoToolchain failed: %v", err)
	}

	// Should skip unstable versions and return the first stable one
	if info.Version != "1.23.4" {
		t.Errorf("Expected version 1.23.4 (first stable), got %s", info.Version)
	}
}

// TestResolveGoToolchain_NoStableVersions tests error when no stable versions exist
func TestResolveGoToolchain_NoStableVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []goRelease{
			{Version: "go1.24rc1", Stable: false},
			{Version: "go1.23beta1", Stable: false},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	ctx := context.Background()

	_, err := resolver.ResolveGoToolchain(ctx)
	if err == nil {
		t.Fatal("Expected error when no stable versions exist, got nil")
	}

	if !strings.Contains(err.Error(), "no stable Go releases") {
		t.Errorf("Expected 'no stable Go releases' error, got: %v", err)
	}
}

// TestListGoToolchainVersions_ValidResponse tests successful version listing
func TestListGoToolchainVersions_ValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []goRelease{
			{Version: "go1.23.4", Stable: true},
			{Version: "go1.24rc1", Stable: false},
			{Version: "go1.22.8", Stable: true},
			{Version: "go1.21.0", Stable: true},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	ctx := context.Background()

	versions, err := resolver.ListGoToolchainVersions(ctx)
	if err != nil {
		t.Fatalf("ListGoToolchainVersions failed: %v", err)
	}

	// Should only include stable versions
	if len(versions) != 3 {
		t.Errorf("Expected 3 stable versions, got %d", len(versions))
	}

	// Should maintain order (newest first)
	expectedOrder := []string{"1.23.4", "1.22.8", "1.21.0"}
	for i, v := range versions {
		if v != expectedOrder[i] {
			t.Errorf("Version at index %d: expected %s, got %s", i, expectedOrder[i], v)
		}
	}
}

// TestListGoToolchainVersions_EmptyResponse tests handling of empty response
func TestListGoToolchainVersions_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	ctx := context.Background()

	versions, err := resolver.ListGoToolchainVersions(ctx)
	if err != nil {
		t.Fatalf("ListGoToolchainVersions failed: %v", err)
	}

	if len(versions) != 0 {
		t.Errorf("Expected empty list, got %d versions", len(versions))
	}
}

// TestResolveGoToolchain_HTTPError tests error handling for non-200 status
func TestResolveGoToolchain_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	ctx := context.Background()

	_, err := resolver.ResolveGoToolchain(ctx)
	if err == nil {
		t.Fatal("Expected error for 500 status, got nil")
	}

	// Error should contain status code
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Expected error to contain status code 500, got: %v", err)
	}
}

// TestResolveGoToolchain_MalformedJSON tests error handling for invalid JSON
func TestResolveGoToolchain_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"version": invalid json`))
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	ctx := context.Background()

	_, err := resolver.ResolveGoToolchain(ctx)
	if err == nil {
		t.Fatal("Expected error for malformed JSON, got nil")
	}

	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("Expected 'parse' error, got: %v", err)
	}
}

// TestListGoToolchainVersions_HTTPError tests error handling for non-200 status in list
func TestListGoToolchainVersions_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	ctx := context.Background()

	_, err := resolver.ListGoToolchainVersions(ctx)
	if err == nil {
		t.Fatal("Expected error for 503 status, got nil")
	}

	if !strings.Contains(err.Error(), "503") {
		t.Errorf("Expected error to contain status code 503, got: %v", err)
	}
}

// TestListGoToolchainVersions_MalformedJSON tests error handling for invalid JSON in list
func TestListGoToolchainVersions_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json at all`))
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	ctx := context.Background()

	_, err := resolver.ListGoToolchainVersions(ctx)
	if err == nil {
		t.Fatal("Expected error for malformed JSON, got nil")
	}

	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("Expected 'parse' error, got: %v", err)
	}
}

// TestGoToolchainProvider_ResolveVersion tests specific version resolution via provider
func TestGoToolchainProvider_ResolveVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []goRelease{
			{Version: "go1.23.4", Stable: true},
			{Version: "go1.23.3", Stable: true},
			{Version: "go1.22.8", Stable: true},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	provider := NewGoToolchainProvider(resolver)
	ctx := context.Background()

	// Test exact version match
	info, err := provider.ResolveVersion(ctx, "1.23.4")
	if err != nil {
		t.Fatalf("ResolveVersion failed for exact match: %v", err)
	}
	if info.Version != "1.23.4" {
		t.Errorf("Expected 1.23.4, got %s", info.Version)
	}

	// Test fuzzy version match (1.23 -> 1.23.4)
	info, err = provider.ResolveVersion(ctx, "1.23")
	if err != nil {
		t.Fatalf("ResolveVersion failed for fuzzy match: %v", err)
	}
	if info.Version != "1.23.4" {
		t.Errorf("Expected 1.23.4 for fuzzy match, got %s", info.Version)
	}

	// Test non-existent version
	_, err = provider.ResolveVersion(ctx, "1.99.0")
	if err == nil {
		t.Fatal("Expected error for non-existent version, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestGoToolchainProvider_ListVersions tests version listing via provider
func TestGoToolchainProvider_ListVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []goRelease{
			{Version: "go1.23.4", Stable: true},
			{Version: "go1.22.8", Stable: true},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	provider := NewGoToolchainProvider(resolver)
	ctx := context.Background()

	versions, err := provider.ListVersions(ctx)
	if err != nil {
		t.Fatalf("ListVersions failed: %v", err)
	}

	if len(versions) != 2 {
		t.Errorf("Expected 2 versions, got %d", len(versions))
	}
}

// TestGoToolchainProvider_ResolveLatest tests latest version resolution via provider
func TestGoToolchainProvider_ResolveLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []goRelease{
			{Version: "go1.23.4", Stable: true},
			{Version: "go1.22.8", Stable: true},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	provider := NewGoToolchainProvider(resolver)
	ctx := context.Background()

	info, err := provider.ResolveLatest(ctx)
	if err != nil {
		t.Fatalf("ResolveLatest failed: %v", err)
	}

	if info.Version != "1.23.4" {
		t.Errorf("Expected 1.23.4, got %s", info.Version)
	}
}

// TestGoToolchainProvider_SourceDescription tests the source description
func TestGoToolchainProvider_SourceDescription(t *testing.T) {
	resolver := New()
	provider := NewGoToolchainProvider(resolver)

	desc := provider.SourceDescription()
	if desc != "go.dev/dl" {
		t.Errorf("Expected source description 'go.dev/dl', got %s", desc)
	}
}

// TestGoToolchainSourceStrategy_CanHandle tests the factory strategy
func TestGoToolchainSourceStrategy_CanHandle(t *testing.T) {
	strategy := &GoToolchainSourceStrategy{}

	tests := []struct {
		source   string
		expected bool
	}{
		{"go_toolchain", true},
		{"go", false},
		{"golang", false},
		{"", false},
		{"pypi", false},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			r := &recipe.Recipe{
				Version: recipe.VersionSection{Source: tt.source},
			}
			got := strategy.CanHandle(r)
			if got != tt.expected {
				t.Errorf("CanHandle(source=%q) = %v, want %v", tt.source, got, tt.expected)
			}
		})
	}
}

// TestGoToolchainSourceStrategy_Create tests the factory creates correct provider
func TestGoToolchainSourceStrategy_Create(t *testing.T) {
	strategy := &GoToolchainSourceStrategy{}
	resolver := New()
	r := &recipe.Recipe{
		Version: recipe.VersionSection{Source: "go_toolchain"},
	}

	provider, err := strategy.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if provider == nil {
		t.Fatal("Expected provider, got nil")
	}

	// Verify it's the right type
	_, ok := provider.(*GoToolchainProvider)
	if !ok {
		t.Errorf("Expected *GoToolchainProvider, got %T", provider)
	}
}

// TestGoToolchainSourceStrategy_Priority tests the strategy priority
func TestGoToolchainSourceStrategy_Priority(t *testing.T) {
	strategy := &GoToolchainSourceStrategy{}

	if strategy.Priority() != PriorityKnownRegistry {
		t.Errorf("Expected priority %d, got %d", PriorityKnownRegistry, strategy.Priority())
	}
}

// TestResolveGoToolchain_EmptyVersionString tests handling of empty version strings
func TestResolveGoToolchain_EmptyVersionString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []goRelease{
			{Version: "", Stable: true},         // Empty version
			{Version: "go1.23.4", Stable: true}, // Valid version
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	ctx := context.Background()

	info, err := resolver.ResolveGoToolchain(ctx)
	if err != nil {
		t.Fatalf("ResolveGoToolchain failed: %v", err)
	}

	// Should skip the empty version and return the valid one
	if info.Version != "1.23.4" {
		t.Errorf("Expected version 1.23.4, got %s", info.Version)
	}
}

// TestListGoToolchainVersions_SkipsEmptyVersions tests that empty versions are filtered out
func TestListGoToolchainVersions_SkipsEmptyVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []goRelease{
			{Version: "", Stable: true},         // Empty version - should be skipped
			{Version: "go1.23.4", Stable: true}, // Valid version
			{Version: "go", Stable: true},       // Just "go" - normalizes to empty string
			{Version: "go1.22.8", Stable: true}, // Valid version
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithGoDevURL(server.URL)
	ctx := context.Background()

	versions, err := resolver.ListGoToolchainVersions(ctx)
	if err != nil {
		t.Fatalf("ListGoToolchainVersions failed: %v", err)
	}

	// Should only include valid versions (empty ones filtered out)
	if len(versions) != 2 {
		t.Errorf("Expected 2 valid versions, got %d: %v", len(versions), versions)
	}
}
