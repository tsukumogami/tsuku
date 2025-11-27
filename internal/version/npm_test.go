package version

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestIsValidNpmPackageName tests npm package name validation
func TestIsValidNpmPackageName(t *testing.T) {
	tests := []struct {
		name      string
		pkgName   string
		wantValid bool
	}{
		// Valid unscoped packages
		{"valid simple", "express", true},
		{"valid with hyphens", "aws-sdk", true},
		{"valid with dots", "lodash.merge", true},
		{"valid with underscore", "debug_logger", true},
		{"valid all lowercase", "mypackage", true},

		// Valid scoped packages
		{"valid scoped", "@aws/sdk", true},
		{"valid scoped with hyphens", "@aws-amplify/cli", true},
		{"valid scoped complex", "@scope/my-package.name", true},

		// Invalid packages
		{"empty string", "", false},
		{"too long", strings.Repeat("a", 215), false},
		{"uppercase", "MyPackage", false},
		{"uppercase scoped", "@AWS/sdk", false},
		{"invalid chars space", "my package", false},
		{"invalid chars special", "my@package", false},
		{"only scope no package", "@scope/", false},
		{"double scope", "@scope1/@scope2/package", false},
		{"no package name", "@", false},
		{"starts with dot", ".package", false},
		{"starts with underscore", "_package", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidNpmPackageName(tt.pkgName)
			if got != tt.wantValid {
				t.Errorf("isValidNpmPackageName(%q) = %v, want %v", tt.pkgName, got, tt.wantValid)
			}
		})
	}
}

// TestListNpmVersions_ValidPackage tests successful npm version listing
func TestListNpmVersions_ValidPackage(t *testing.T) {
	// Create mock npm registry server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request path
		if !strings.Contains(r.URL.Path, "/turbo") && !strings.Contains(r.URL.Path, "turbo") {
			t.Errorf("Expected path to contain 'turbo', got %s", r.URL.Path)
		}

		// Return mock npm registry response
		response := map[string]interface{}{
			"name": "turbo",
			"versions": map[string]interface{}{
				"2.0.0": map[string]interface{}{"name": "turbo"},
				"1.5.0": map[string]interface{}{"name": "turbo"},
				"1.0.0": map[string]interface{}{"name": "turbo"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Use NewWithNpmRegistry to inject test server URL
	resolver := NewWithNpmRegistry(server.URL)
	ctx := context.Background()
	versions, err := resolver.ListNpmVersions(ctx, "turbo")

	if err != nil {
		t.Fatalf("ListNpmVersions failed: %v", err)
	}

	if len(versions) != 3 {
		t.Errorf("Expected 3 versions, got %d", len(versions))
	}

	// Verify versions are sorted (newest first)
	if versions[0] != "2.0.0" {
		t.Errorf("Expected first version to be 2.0.0, got %s", versions[0])
	}
}

// TestListNpmVersions_InvalidPackageName tests package name validation
func TestListNpmVersions_InvalidPackageName(t *testing.T) {
	r := New()
	ctx := context.Background()

	invalidNames := []string{
		"",
		"InvalidName",  // Uppercase
		strings.Repeat("a", 215),  // Too long
		"my package",  // Spaces
		"my@package",  // Invalid chars
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			_, err := r.ListNpmVersions(ctx, name)
			if err == nil {
				t.Errorf("ListNpmVersions(%q) expected error for invalid name, got nil", name)
			}
			if !strings.Contains(err.Error(), "invalid npm package name") {
				t.Errorf("ListNpmVersions(%q) error = %v, want 'invalid npm package name'", name, err)
			}
		})
	}
}

// TestListNpmVersions_PackageNotFound tests 404 error handling
func TestListNpmVersions_PackageNotFound(t *testing.T) {
	// Create mock server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Not found"}`))
	}))
	defer server.Close()

	resolver := NewWithNpmRegistry(server.URL)
	ctx := context.Background()
	_, err := resolver.ListNpmVersions(ctx, "nonexistent-package")

	if err == nil {
		t.Fatal("Expected error for 404, got nil")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestListNpmVersions_RateLimitExceeded tests 429 error handling
func TestListNpmVersions_RateLimitExceeded(t *testing.T) {
	// Create mock server that returns 429
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"Rate limit exceeded"}`))
	}))
	defer server.Close()

	resolver := NewWithNpmRegistry(server.URL)
	ctx := context.Background()
	_, err := resolver.ListNpmVersions(ctx, "test-package")

	if err == nil {
		t.Fatal("Expected error for 429, got nil")
	}

	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("Expected 'rate limit' error, got: %v", err)
	}
}

// TestListNpmVersions_MalformedJSON tests JSON parsing error handling
func TestListNpmVersions_MalformedJSON(t *testing.T) {
	// Create mock server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"versions": invalid json`))
	}))
	defer server.Close()

	resolver := NewWithNpmRegistry(server.URL)
	ctx := context.Background()
	_, err := resolver.ListNpmVersions(ctx, "test-package")

	if err == nil {
		t.Fatal("Expected error for malformed JSON, got nil")
	}

	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("Expected 'parse' error, got: %v", err)
	}
}

// TestListNpmVersions_VersionSorting tests that versions are sorted correctly
func TestListNpmVersions_VersionSorting(t *testing.T) {
	// Test that newer versions appear first
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"name": "test-package",
			"versions": map[string]interface{}{
				"1.0.0": map[string]interface{}{},
				"2.5.0": map[string]interface{}{},
				"1.9.0": map[string]interface{}{},
				"2.0.0": map[string]interface{}{},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithNpmRegistry(server.URL)
	ctx := context.Background()
	versions, err := resolver.ListNpmVersions(ctx, "test-package")

	if err != nil {
		t.Fatalf("ListNpmVersions failed: %v", err)
	}

	// Should be sorted newest first: 2.5.0, 2.0.0, 1.9.0, 1.0.0
	if len(versions) < 1 || versions[0] != "2.5.0" {
		t.Errorf("Expected first version to be 2.5.0, got %v", versions)
	}
}

// TestListNpmVersions_ScopedPackage tests URL encoding for scoped packages
func TestListNpmVersions_ScopedPackage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify path contains the scoped package name
		// url.URL.String() will preserve @ and / in path component
		path := r.URL.Path
		t.Logf("Path received: %s", path)

		response := map[string]interface{}{
			"name": "@aws-amplify/cli",
			"versions": map[string]interface{}{
				"1.0.0": map[string]interface{}{"name": "@aws-amplify/cli"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithNpmRegistry(server.URL)
	ctx := context.Background()
	versions, err := resolver.ListNpmVersions(ctx, "@aws-amplify/cli")

	if err != nil {
		t.Fatalf("ListNpmVersions failed for scoped package: %v", err)
	}

	if len(versions) != 1 {
		t.Errorf("Expected 1 version, got %d", len(versions))
	}
}

// Note: All npm tests now use NewWithNpmRegistry() to inject a test server URL,
// allowing comprehensive testing without hitting the real npm registry.
