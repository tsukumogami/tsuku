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

// TestEncodeModulePath tests the module path encoding function
func TestEncodeModulePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"github.com/user/repo", "github.com/user/repo"},    // No uppercase
		{"github.com/User/Repo", "github.com/!user/!repo"},  // Uppercase in user/repo
		{"github.com/GoLang/Go", "github.com/!go!lang/!go"}, // Multiple uppercase
		{"UPPER", "!u!p!p!e!r"},                             // All uppercase
		{"MixedCase", "!mixed!case"},                        // Mixed case
		{"", ""},                                            // Empty
		{"lowercase", "lowercase"},                          // All lowercase
		{"github.com/golangci/golangci-lint", "github.com/golangci/golangci-lint"}, // Real module path
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := encodeModulePath(tt.input)
			if got != tt.expected {
				t.Errorf("encodeModulePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// goProxyLatestResponse represents the JSON response from /@latest endpoint
type goProxyLatestResponse struct {
	Version string `json:"Version"`
	Time    string `json:"Time"`
}

// TestResolveGoProxy_ValidResponse tests successful version resolution
func TestResolveGoProxy_ValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify it's the correct endpoint
		if !strings.HasSuffix(r.URL.Path, "/@latest") {
			t.Errorf("Expected path ending with /@latest, got %s", r.URL.Path)
		}

		response := goProxyLatestResponse{
			Version: "v1.64.8",
			Time:    "2025-01-15T10:00:00Z",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := New(WithGoProxyURL(server.URL))
	ctx := context.Background()

	info, err := resolver.ResolveGoProxy(ctx, "github.com/golangci/golangci-lint")
	if err != nil {
		t.Fatalf("ResolveGoProxy failed: %v", err)
	}

	if info.Version != "1.64.8" {
		t.Errorf("Expected version 1.64.8, got %s", info.Version)
	}

	if info.Tag != "v1.64.8" {
		t.Errorf("Expected tag v1.64.8, got %s", info.Tag)
	}
}

// TestResolveGoProxy_EncodesModulePath tests that uppercase letters are encoded
func TestResolveGoProxy_EncodesModulePath(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path

		response := goProxyLatestResponse{
			Version: "v1.0.0",
			Time:    "2025-01-15T10:00:00Z",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := New(WithGoProxyURL(server.URL))
	ctx := context.Background()

	_, err := resolver.ResolveGoProxy(ctx, "github.com/User/Repo")
	if err != nil {
		t.Fatalf("ResolveGoProxy failed: %v", err)
	}

	// Path should have encoded uppercase letters
	expectedPath := "/github.com/!user/!repo/@latest"
	if receivedPath != expectedPath {
		t.Errorf("Expected path %q, got %q", expectedPath, receivedPath)
	}
}

// TestResolveGoProxy_HTTPError tests error handling for non-200 status
func TestResolveGoProxy_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	resolver := New(WithGoProxyURL(server.URL))
	ctx := context.Background()

	_, err := resolver.ResolveGoProxy(ctx, "github.com/nonexistent/module")
	if err == nil {
		t.Fatal("Expected error for 404 status, got nil")
	}

	// 404 returns a user-friendly "not found" message
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected error to contain 'not found', got: %v", err)
	}
}

// TestResolveGoProxy_MalformedJSON tests error handling for invalid JSON
func TestResolveGoProxy_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Version": invalid json`))
	}))
	defer server.Close()

	resolver := New(WithGoProxyURL(server.URL))
	ctx := context.Background()

	_, err := resolver.ResolveGoProxy(ctx, "github.com/test/module")
	if err == nil {
		t.Fatal("Expected error for malformed JSON, got nil")
	}

	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("Expected 'parse' error, got: %v", err)
	}
}

// TestListGoProxyVersions_ValidResponse tests successful version listing
func TestListGoProxyVersions_ValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify it's the correct endpoint
		if !strings.HasSuffix(r.URL.Path, "/@v/list") {
			t.Errorf("Expected path ending with /@v/list, got %s", r.URL.Path)
		}

		// Return newline-separated version list
		_, _ = w.Write([]byte("v1.64.8\nv1.64.7\nv1.63.4\nv1.62.0\n"))
	}))
	defer server.Close()

	resolver := New(WithGoProxyURL(server.URL))
	ctx := context.Background()

	versions, err := resolver.ListGoProxyVersions(ctx, "github.com/golangci/golangci-lint")
	if err != nil {
		t.Fatalf("ListGoProxyVersions failed: %v", err)
	}

	if len(versions) != 4 {
		t.Errorf("Expected 4 versions, got %d", len(versions))
	}

	// Should maintain order (newest first after sorting)
	expectedOrder := []string{"v1.64.8", "v1.64.7", "v1.63.4", "v1.62.0"}
	for i, v := range versions {
		if v != expectedOrder[i] {
			t.Errorf("Version at index %d: expected %s, got %s", i, expectedOrder[i], v)
		}
	}
}

// TestListGoProxyVersions_EmptyResponse tests handling of empty response
func TestListGoProxyVersions_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Empty response (no versions)
		_, _ = w.Write([]byte(""))
	}))
	defer server.Close()

	resolver := New(WithGoProxyURL(server.URL))
	ctx := context.Background()

	versions, err := resolver.ListGoProxyVersions(ctx, "github.com/test/module")
	if err != nil {
		t.Fatalf("ListGoProxyVersions failed: %v", err)
	}

	if len(versions) != 0 {
		t.Errorf("Expected empty list, got %d versions", len(versions))
	}
}

// TestListGoProxyVersions_HTTPError tests error handling for non-200 status
func TestListGoProxyVersions_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	resolver := New(WithGoProxyURL(server.URL))
	ctx := context.Background()

	_, err := resolver.ListGoProxyVersions(ctx, "github.com/test/module")
	if err == nil {
		t.Fatal("Expected error for 503 status, got nil")
	}

	if !strings.Contains(err.Error(), "503") {
		t.Errorf("Expected error to contain status code 503, got: %v", err)
	}
}

// TestListGoProxyVersions_EncodesModulePath tests that uppercase letters are encoded
func TestListGoProxyVersions_EncodesModulePath(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		_, _ = w.Write([]byte("v1.0.0\n"))
	}))
	defer server.Close()

	resolver := New(WithGoProxyURL(server.URL))
	ctx := context.Background()

	_, err := resolver.ListGoProxyVersions(ctx, "github.com/User/Repo")
	if err != nil {
		t.Fatalf("ListGoProxyVersions failed: %v", err)
	}

	expectedPath := "/github.com/!user/!repo/@v/list"
	if receivedPath != expectedPath {
		t.Errorf("Expected path %q, got %q", expectedPath, receivedPath)
	}
}

// TestGoProxyProvider_ResolveLatest tests latest version resolution via provider
func TestGoProxyProvider_ResolveLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := goProxyLatestResponse{
			Version: "v1.64.8",
			Time:    "2025-01-15T10:00:00Z",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := New(WithGoProxyURL(server.URL))
	provider := NewGoProxyProvider(resolver, "github.com/golangci/golangci-lint")
	ctx := context.Background()

	info, err := provider.ResolveLatest(ctx)
	if err != nil {
		t.Fatalf("ResolveLatest failed: %v", err)
	}

	if info.Version != "1.64.8" {
		t.Errorf("Expected version 1.64.8, got %s", info.Version)
	}
}

// TestGoProxyProvider_ListVersions tests version listing via provider
func TestGoProxyProvider_ListVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("v1.64.8\nv1.64.7\n"))
	}))
	defer server.Close()

	resolver := New(WithGoProxyURL(server.URL))
	provider := NewGoProxyProvider(resolver, "github.com/golangci/golangci-lint")
	ctx := context.Background()

	versions, err := provider.ListVersions(ctx)
	if err != nil {
		t.Fatalf("ListVersions failed: %v", err)
	}

	if len(versions) != 2 {
		t.Errorf("Expected 2 versions, got %d", len(versions))
	}
}

// TestGoProxyProvider_ResolveVersion tests specific version resolution via provider
func TestGoProxyProvider_ResolveVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return version list for validation
		_, _ = w.Write([]byte("v1.64.8\nv1.64.7\nv1.63.4\n"))
	}))
	defer server.Close()

	resolver := New(WithGoProxyURL(server.URL))
	provider := NewGoProxyProvider(resolver, "github.com/golangci/golangci-lint")
	ctx := context.Background()

	// Test exact version match with v prefix
	info, err := provider.ResolveVersion(ctx, "v1.64.8")
	if err != nil {
		t.Fatalf("ResolveVersion failed for exact match: %v", err)
	}
	if info.Version != "1.64.8" {
		t.Errorf("Expected 1.64.8, got %s", info.Version)
	}
	if info.Tag != "v1.64.8" {
		t.Errorf("Expected tag v1.64.8, got %s", info.Tag)
	}

	// Test version without v prefix (should be normalized)
	info, err = provider.ResolveVersion(ctx, "1.64.7")
	if err != nil {
		t.Fatalf("ResolveVersion failed for version without prefix: %v", err)
	}
	if info.Version != "1.64.7" {
		t.Errorf("Expected 1.64.7, got %s", info.Version)
	}

	// Test non-existent version
	_, err = provider.ResolveVersion(ctx, "v9.99.99")
	if err == nil {
		t.Fatal("Expected error for non-existent version, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestGoProxyProvider_SourceDescription tests the source description
func TestGoProxyProvider_SourceDescription(t *testing.T) {
	resolver := New()
	provider := NewGoProxyProvider(resolver, "github.com/test/module")

	desc := provider.SourceDescription()
	if desc != "proxy.golang.org" {
		t.Errorf("Expected source description 'proxy.golang.org', got %s", desc)
	}
}

// TestGoProxySourceStrategy_CanHandle tests the factory strategy
func TestGoProxySourceStrategy_CanHandle(t *testing.T) {
	strategy := &GoProxySourceStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "goproxy with go_install action",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "goproxy"},
				Steps: []recipe.Step{{
					Action: "go_install",
					Params: map[string]interface{}{"module": "github.com/golangci/golangci-lint"},
				}},
			},
			expected: true,
		},
		{
			name: "goproxy without go_install action",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "goproxy"},
				Steps:   []recipe.Step{},
			},
			expected: false,
		},
		{
			name: "goproxy with go_install but no module param",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "goproxy"},
				Steps: []recipe.Step{{
					Action: "go_install",
					Params: map[string]interface{}{},
				}},
			},
			expected: false,
		},
		{
			name: "different source with go_install",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "pypi"},
				Steps: []recipe.Step{{
					Action: "go_install",
					Params: map[string]interface{}{"module": "github.com/test/test"},
				}},
			},
			expected: false,
		},
		{
			name: "empty source",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: ""},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strategy.CanHandle(tt.recipe)
			if got != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestGoProxySourceStrategy_Create tests the factory creates correct provider
func TestGoProxySourceStrategy_Create(t *testing.T) {
	strategy := &GoProxySourceStrategy{}
	resolver := New()
	r := &recipe.Recipe{
		Version: recipe.VersionSection{Source: "goproxy"},
		Steps: []recipe.Step{{
			Action: "go_install",
			Params: map[string]interface{}{"module": "github.com/golangci/golangci-lint"},
		}},
	}

	provider, err := strategy.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if provider == nil {
		t.Fatal("Expected provider, got nil")
	}

	// Verify it's the right type
	goProxyProvider, ok := provider.(*GoProxyProvider)
	if !ok {
		t.Errorf("Expected *GoProxyProvider, got %T", provider)
	}

	// Verify module path was extracted correctly
	if goProxyProvider.modulePath != "github.com/golangci/golangci-lint" {
		t.Errorf("Expected module path 'github.com/golangci/golangci-lint', got %s", goProxyProvider.modulePath)
	}
}

// TestGoProxySourceStrategy_Create_NoGoInstall tests error when no go_install step
func TestGoProxySourceStrategy_Create_NoGoInstall(t *testing.T) {
	strategy := &GoProxySourceStrategy{}
	resolver := New()
	r := &recipe.Recipe{
		Version: recipe.VersionSection{Source: "goproxy"},
		Steps:   []recipe.Step{},
	}

	_, err := strategy.Create(resolver, r)
	if err == nil {
		t.Fatal("Expected error for missing go_install step, got nil")
	}

	if !strings.Contains(err.Error(), "no Go module found") {
		t.Errorf("Expected 'no Go module found' error, got: %v", err)
	}
}

// TestGoProxySourceStrategy_Priority tests the strategy priority
func TestGoProxySourceStrategy_Priority(t *testing.T) {
	strategy := &GoProxySourceStrategy{}

	if strategy.Priority() != PriorityKnownRegistry {
		t.Errorf("Expected priority %d, got %d", PriorityKnownRegistry, strategy.Priority())
	}
}

// TestGoProxySourceStrategy_Create_WithVersionModule tests that Version.Module takes precedence
func TestGoProxySourceStrategy_Create_WithVersionModule(t *testing.T) {
	strategy := &GoProxySourceStrategy{}
	resolver := New()

	// Recipe where version module differs from install module
	// This is the case for tools like staticcheck where:
	// - Version lookup: honnef.co/go/tools
	// - Install path: honnef.co/go/tools/cmd/staticcheck
	r := &recipe.Recipe{
		Version: recipe.VersionSection{
			Source: "goproxy",
			Module: "honnef.co/go/tools", // Version resolution uses parent module
		},
		Steps: []recipe.Step{{
			Action: "go_install",
			Params: map[string]interface{}{"module": "honnef.co/go/tools/cmd/staticcheck"}, // Install uses subpackage
		}},
	}

	provider, err := strategy.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if provider == nil {
		t.Fatal("Expected provider, got nil")
	}

	// Verify it uses Version.Module, not the step param
	goProxyProvider, ok := provider.(*GoProxyProvider)
	if !ok {
		t.Errorf("Expected *GoProxyProvider, got %T", provider)
	}

	// The provider should use the version section module, not the step module
	if goProxyProvider.modulePath != "honnef.co/go/tools" {
		t.Errorf("Expected module path 'honnef.co/go/tools', got %s", goProxyProvider.modulePath)
	}
}
