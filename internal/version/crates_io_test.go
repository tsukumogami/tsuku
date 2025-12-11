package version

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestIsValidCratesIOPackageName tests crate name validation
func TestIsValidCratesIOPackageName(t *testing.T) {
	tests := []struct {
		name      string
		crateName string
		wantValid bool
	}{
		// Valid crate names
		{"simple lowercase", "serde", true},
		{"with hyphen", "cargo-audit", true},
		{"with underscore", "tokio_util", true},
		{"mixed case", "TokioUtil", true},
		{"with numbers", "serde2", true},
		{"single letter", "a", true},

		// Invalid crate names
		{"empty string", "", false},
		{"starts with number", "123abc", false},
		{"starts with hyphen", "-cargo", false},
		{"starts with underscore", "_cargo", false},
		{"contains space", "cargo audit", false},
		{"contains special char @", "cargo@audit", false},
		{"contains dot", "cargo.audit", false},
		{"contains slash", "cargo/audit", false},
		{"too long", strings.Repeat("a", 65), false}, // 65 chars
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidCratesIOPackageName(tt.crateName)
			if got != tt.wantValid {
				t.Errorf("isValidCratesIOPackageName(%q) = %v, want %v", tt.crateName, got, tt.wantValid)
			}
		})
	}
}

// TestListCratesIOVersions_ValidCrate tests successful version listing
func TestListCratesIOVersions_ValidCrate(t *testing.T) {
	// Create mock crates.io server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request path contains the crate name
		if !strings.Contains(r.URL.Path, "/cargo-audit/") {
			t.Errorf("Expected path to contain '/cargo-audit/', got %s", r.URL.Path)
		}

		// Verify User-Agent is set (required by crates.io)
		if r.Header.Get("User-Agent") == "" {
			t.Error("Expected User-Agent header to be set")
		}

		// Return mock crates.io response
		response := cratesIOVersionsResponse{
			Versions: []cratesIOVersion{
				{Num: "0.18.4", Yanked: false},
				{Num: "0.18.3", Yanked: false},
				{Num: "0.18.2", Yanked: true}, // Yanked version should be excluded
				{Num: "0.17.0", Yanked: false},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := New(WithCratesIORegistry(server.URL))
	ctx := context.Background()
	versions, err := resolver.ListCratesIOVersions(ctx, "cargo-audit")

	if err != nil {
		t.Fatalf("ListCratesIOVersions failed: %v", err)
	}

	// Should have 3 versions (yanked excluded)
	if len(versions) != 3 {
		t.Errorf("Expected 3 versions (excluding yanked), got %d", len(versions))
	}

	// Verify versions are sorted (newest first)
	if versions[0] != "0.18.4" {
		t.Errorf("Expected first version to be 0.18.4, got %s", versions[0])
	}
}

// TestListCratesIOVersions_InvalidCrateName tests validation error
func TestListCratesIOVersions_InvalidCrateName(t *testing.T) {
	resolver := New()
	ctx := context.Background()

	invalidNames := []string{
		"",
		"123abc",                // Starts with number
		"my@crate",              // Invalid char
		"my crate",              // Space
		strings.Repeat("a", 65), // Too long
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			_, err := resolver.ListCratesIOVersions(ctx, name)
			if err == nil {
				t.Errorf("ListCratesIOVersions(%q) expected error for invalid name, got nil", name)
			}
			if !strings.Contains(err.Error(), "invalid crate name") {
				t.Errorf("ListCratesIOVersions(%q) error = %v, want 'invalid crate name'", name, err)
			}
		})
	}
}

// TestListCratesIOVersions_CrateNotFound tests 404 error handling
func TestListCratesIOVersions_CrateNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"detail":"Not found"}]}`))
	}))
	defer server.Close()

	resolver := New(WithCratesIORegistry(server.URL))
	ctx := context.Background()
	_, err := resolver.ListCratesIOVersions(ctx, "nonexistent-crate")

	if err == nil {
		t.Fatal("Expected error for 404, got nil")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestListCratesIOVersions_RateLimitExceeded tests 429 error handling
func TestListCratesIOVersions_RateLimitExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"errors":[{"detail":"Rate limited"}]}`))
	}))
	defer server.Close()

	resolver := New(WithCratesIORegistry(server.URL))
	ctx := context.Background()
	_, err := resolver.ListCratesIOVersions(ctx, "test-crate")

	if err == nil {
		t.Fatal("Expected error for 429, got nil")
	}

	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("Expected 'rate limit' error, got: %v", err)
	}
}

// TestListCratesIOVersions_MalformedJSON tests JSON parsing error handling
func TestListCratesIOVersions_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"versions": invalid json`))
	}))
	defer server.Close()

	resolver := New(WithCratesIORegistry(server.URL))
	ctx := context.Background()
	_, err := resolver.ListCratesIOVersions(ctx, "test-crate")

	if err == nil {
		t.Fatal("Expected error for malformed JSON, got nil")
	}

	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("Expected 'parse' error, got: %v", err)
	}
}

// TestListCratesIOVersions_WrongContentType tests Content-Type validation
func TestListCratesIOVersions_WrongContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html>Error</html>`))
	}))
	defer server.Close()

	resolver := New(WithCratesIORegistry(server.URL))
	ctx := context.Background()
	_, err := resolver.ListCratesIOVersions(ctx, "test-crate")

	if err == nil {
		t.Fatal("Expected error for wrong Content-Type, got nil")
	}

	if !strings.Contains(err.Error(), "content-type") {
		t.Errorf("Expected 'content-type' error, got: %v", err)
	}
}

// TestListCratesIOVersions_VersionSorting tests that versions are sorted correctly
func TestListCratesIOVersions_VersionSorting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return versions in non-sorted order
		response := cratesIOVersionsResponse{
			Versions: []cratesIOVersion{
				{Num: "1.0.0", Yanked: false},
				{Num: "2.5.0", Yanked: false},
				{Num: "1.9.0", Yanked: false},
				{Num: "2.0.0", Yanked: false},
				{Num: "10.0.0", Yanked: false},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := New(WithCratesIORegistry(server.URL))
	ctx := context.Background()
	versions, err := resolver.ListCratesIOVersions(ctx, "test-crate")

	if err != nil {
		t.Fatalf("ListCratesIOVersions failed: %v", err)
	}

	// Should be sorted newest first: 10.0.0, 2.5.0, 2.0.0, 1.9.0, 1.0.0
	expectedOrder := []string{"10.0.0", "2.5.0", "2.0.0", "1.9.0", "1.0.0"}
	if len(versions) != len(expectedOrder) {
		t.Fatalf("Expected %d versions, got %d", len(expectedOrder), len(versions))
	}

	for i, expected := range expectedOrder {
		if versions[i] != expected {
			t.Errorf("versions[%d] = %s, want %s", i, versions[i], expected)
		}
	}
}

// TestCratesIOProvider_ResolveVersion_ExactMatch tests exact version matching
func TestCratesIOProvider_ResolveVersion_ExactMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := cratesIOVersionsResponse{
			Versions: []cratesIOVersion{
				{Num: "0.18.4", Yanked: false},
				{Num: "0.18.3", Yanked: false},
				{Num: "0.17.0", Yanked: false},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := New(WithCratesIORegistry(server.URL))
	provider := NewCratesIOProvider(resolver, "cargo-audit")
	ctx := context.Background()

	info, err := provider.ResolveVersion(ctx, "0.18.3")
	if err != nil {
		t.Fatalf("ResolveVersion failed: %v", err)
	}

	if info.Version != "0.18.3" {
		t.Errorf("Expected version 0.18.3, got %s", info.Version)
	}
}

// TestCratesIOProvider_ResolveVersion_FuzzyMatch tests fuzzy version matching
func TestCratesIOProvider_ResolveVersion_FuzzyMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := cratesIOVersionsResponse{
			Versions: []cratesIOVersion{
				{Num: "0.18.4", Yanked: false},
				{Num: "0.18.3", Yanked: false},
				{Num: "0.17.5", Yanked: false},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := New(WithCratesIORegistry(server.URL))
	provider := NewCratesIOProvider(resolver, "cargo-audit")
	ctx := context.Background()

	// "0.18" should match "0.18.4" (newest 0.18.x)
	info, err := provider.ResolveVersion(ctx, "0.18")
	if err != nil {
		t.Fatalf("ResolveVersion failed: %v", err)
	}

	if info.Version != "0.18.4" {
		t.Errorf("Expected version 0.18.4 for fuzzy match '0.18', got %s", info.Version)
	}
}

// TestCratesIOProvider_ResolveVersion_NotFound tests version not found
func TestCratesIOProvider_ResolveVersion_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := cratesIOVersionsResponse{
			Versions: []cratesIOVersion{
				{Num: "0.18.4", Yanked: false},
				{Num: "0.18.3", Yanked: false},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := New(WithCratesIORegistry(server.URL))
	provider := NewCratesIOProvider(resolver, "cargo-audit")
	ctx := context.Background()

	// "0.19" should not match any version
	_, err := provider.ResolveVersion(ctx, "0.19")
	if err == nil {
		t.Fatal("Expected error for non-existent version, got nil")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestCratesIOProvider_ResolveLatest tests resolving latest version
func TestCratesIOProvider_ResolveLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := cratesIOVersionsResponse{
			Versions: []cratesIOVersion{
				{Num: "0.18.4", Yanked: false},
				{Num: "0.19.0", Yanked: true}, // Latest but yanked
				{Num: "0.18.3", Yanked: false},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := New(WithCratesIORegistry(server.URL))
	provider := NewCratesIOProvider(resolver, "cargo-audit")
	ctx := context.Background()

	info, err := provider.ResolveLatest(ctx)
	if err != nil {
		t.Fatalf("ResolveLatest failed: %v", err)
	}

	// Should return 0.18.4, not 0.19.0 (yanked)
	if info.Version != "0.18.4" {
		t.Errorf("Expected version 0.18.4, got %s", info.Version)
	}
}

// TestCratesIOProvider_SourceDescription tests source description
func TestCratesIOProvider_SourceDescription(t *testing.T) {
	resolver := New()
	provider := NewCratesIOProvider(resolver, "cargo-audit")

	desc := provider.SourceDescription()
	expected := "crates.io:cargo-audit"

	if desc != expected {
		t.Errorf("SourceDescription() = %s, want %s", desc, expected)
	}
}
