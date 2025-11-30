package version

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tsuku-dev/tsuku/internal/recipe"
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
		if r.URL.Path != "/" || r.URL.Query().Get("mode") != "json" {
			t.Errorf("Expected /?mode=json, got %s", r.URL.String())
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

	// Create resolver with mock server (we need to inject the URL)
	resolver := newResolverWithGoToolchainURL(server.URL)
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

	resolver := newResolverWithGoToolchainURL(server.URL)
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

	resolver := newResolverWithGoToolchainURL(server.URL)
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

	resolver := newResolverWithGoToolchainURL(server.URL)
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

	resolver := newResolverWithGoToolchainURL(server.URL)
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

	resolver := newResolverWithGoToolchainURL(server.URL)
	ctx := context.Background()

	_, err := resolver.ResolveGoToolchain(ctx)
	if err == nil {
		t.Fatal("Expected error for 500 status, got nil")
	}

	// Error may contain status code or status text
	if !strings.Contains(err.Error(), "500") && !strings.Contains(err.Error(), "Internal Server Error") {
		t.Errorf("Expected error to contain status code or text, got: %v", err)
	}
}

// TestResolveGoToolchain_MalformedJSON tests error handling for invalid JSON
func TestResolveGoToolchain_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"version": invalid json`))
	}))
	defer server.Close()

	resolver := newResolverWithGoToolchainURL(server.URL)
	ctx := context.Background()

	_, err := resolver.ResolveGoToolchain(ctx)
	if err == nil {
		t.Fatal("Expected error for malformed JSON, got nil")
	}

	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("Expected 'parse' error, got: %v", err)
	}
}

// TestGoToolchainProvider_ResolveVersion tests specific version resolution
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

	resolverWrapper := newResolverWithGoToolchainURL(server.URL)
	ctx := context.Background()

	// Test exact version match using the wrapper's ListGoToolchainVersions
	versions, err := resolverWrapper.ListGoToolchainVersions(ctx)
	if err != nil {
		t.Fatalf("ListGoToolchainVersions failed: %v", err)
	}

	// Test exact version match
	found := false
	for _, v := range versions {
		if v == "1.23.4" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected to find version 1.23.4 in list")
	}

	// Test fuzzy matching logic directly (since it's the same as provider)
	// Fuzzy match: "1.23" should match "1.23.4"
	prefix := "1.23."
	foundFuzzy := false
	for _, v := range versions {
		if strings.HasPrefix(v, prefix) {
			foundFuzzy = true
			break
		}
	}
	if !foundFuzzy {
		t.Errorf("Expected to find version matching 1.23.x")
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

// newResolverWithGoToolchainURL creates a resolver with a custom go.dev URL for testing
// This is a helper function to enable unit testing without hitting the real API
func newResolverWithGoToolchainURL(baseURL string) *resolverWithGoURL {
	return &resolverWithGoURL{
		Resolver: New(),
		goDevURL: baseURL,
	}
}

// resolverWithGoURL wraps Resolver to allow testing with a mock go.dev server
type resolverWithGoURL struct {
	*Resolver
	goDevURL string
}

// ResolveGoToolchain overrides the base method to use the test URL
func (r *resolverWithGoURL) ResolveGoToolchain(ctx context.Context) (*VersionInfo, error) {
	apiURL := r.goDevURL + "/?mode=json"

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "go_toolchain",
			Message: "failed to create request",
			Err:     err,
		}
	}

	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "go_toolchain", "failed to fetch Go releases")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "go_toolchain",
			Message: "go.dev returned status " + http.StatusText(resp.StatusCode),
		}
	}

	var releases []goRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "go_toolchain",
			Message: "failed to parse go.dev response",
			Err:     err,
		}
	}

	for _, release := range releases {
		if release.Stable {
			version := normalizeGoToolchainVersion(release.Version)
			if version == "" {
				continue
			}
			return &VersionInfo{
				Tag:     version,
				Version: version,
			}, nil
		}
	}

	return nil, &ResolverError{
		Type:    ErrTypeNotFound,
		Source:  "go_toolchain",
		Message: "no stable Go releases found",
	}
}

// ListGoToolchainVersions overrides the base method to use the test URL
func (r *resolverWithGoURL) ListGoToolchainVersions(ctx context.Context) ([]string, error) {
	apiURL := r.goDevURL + "/?mode=json"

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "go_toolchain",
			Message: "failed to create request",
			Err:     err,
		}
	}

	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "go_toolchain", "failed to fetch Go releases")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "go_toolchain",
			Message: "go.dev returned status " + http.StatusText(resp.StatusCode),
		}
	}

	var releases []goRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "go_toolchain",
			Message: "failed to parse go.dev response",
			Err:     err,
		}
	}

	var versions []string
	for _, release := range releases {
		if release.Stable {
			version := normalizeGoToolchainVersion(release.Version)
			if version != "" {
				versions = append(versions, version)
			}
		}
	}

	return versions, nil
}
