package version

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveHomebrew_Success(t *testing.T) {
	// Mock Homebrew API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/formula/libyaml.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "libyaml",
			"full_name": "libyaml",
			"versions": {
				"stable": "0.2.5",
				"head": null,
				"bottle": true
			},
			"revision": 0,
			"deprecated": false,
			"disabled": false,
			"versioned_formulae": []
		}`))
	}))
	defer server.Close()

	resolver := &Resolver{
		httpClient:          server.Client(),
		homebrewRegistryURL: server.URL,
	}

	info, err := resolver.ResolveHomebrew(context.Background(), "libyaml")
	if err != nil {
		t.Fatalf("ResolveHomebrew failed: %v", err)
	}

	if info.Version != "0.2.5" {
		t.Errorf("expected version '0.2.5', got '%s'", info.Version)
	}
	if info.Tag != "0.2.5" {
		t.Errorf("expected tag '0.2.5', got '%s'", info.Tag)
	}
}

func TestResolveHomebrew_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	resolver := &Resolver{
		httpClient:          server.Client(),
		homebrewRegistryURL: server.URL,
	}

	_, err := resolver.ResolveHomebrew(context.Background(), "nonexistent-formula")
	if err == nil {
		t.Fatal("expected error for non-existent formula")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Fatalf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeNotFound {
		t.Errorf("expected ErrTypeNotFound, got %v", resolverErr.Type)
	}
}

func TestResolveHomebrew_DisabledFormula(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "oldformula",
			"versions": {
				"stable": "1.0.0"
			},
			"deprecated": false,
			"disabled": true
		}`))
	}))
	defer server.Close()

	resolver := &Resolver{
		httpClient:          server.Client(),
		homebrewRegistryURL: server.URL,
	}

	_, err := resolver.ResolveHomebrew(context.Background(), "oldformula")
	if err == nil {
		t.Fatal("expected error for disabled formula")
	}
}

func TestResolveHomebrew_InvalidFormula(t *testing.T) {
	resolver := &Resolver{
		httpClient: &http.Client{},
	}

	tests := []struct {
		name    string
		formula string
	}{
		{"empty", ""},
		{"path traversal", "../etc/passwd"},
		{"shell injection", "formula;rm -rf /"},
		{"too long", string(make([]byte, 200))},
		{"starts with dash", "-formula"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolver.ResolveHomebrew(context.Background(), tt.formula)
			if err == nil {
				t.Errorf("expected error for invalid formula name: %s", tt.formula)
			}
		})
	}
}

func TestListHomebrewVersions_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "openssl",
			"versions": {
				"stable": "3.2.0"
			},
			"versioned_formulae": ["openssl@3.0", "openssl@1.1"]
		}`))
	}))
	defer server.Close()

	resolver := &Resolver{
		httpClient:          server.Client(),
		homebrewRegistryURL: server.URL,
	}

	versions, err := resolver.ListHomebrewVersions(context.Background(), "openssl")
	if err != nil {
		t.Fatalf("ListHomebrewVersions failed: %v", err)
	}

	// Should include stable version and versions from versioned formulae
	if len(versions) != 3 {
		t.Errorf("expected 3 versions, got %d: %v", len(versions), versions)
	}

	// Versions should be sorted newest first
	expected := []string{"3.2.0", "3.0", "1.1"}
	for i, v := range expected {
		if i >= len(versions) {
			t.Errorf("missing version at index %d", i)
			continue
		}
		if versions[i] != v {
			t.Errorf("expected version '%s' at index %d, got '%s'", v, i, versions[i])
		}
	}
}

func TestListHomebrewVersions_NoVersionedFormulae(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "libyaml",
			"versions": {
				"stable": "0.2.5"
			},
			"versioned_formulae": []
		}`))
	}))
	defer server.Close()

	resolver := &Resolver{
		httpClient:          server.Client(),
		homebrewRegistryURL: server.URL,
	}

	versions, err := resolver.ListHomebrewVersions(context.Background(), "libyaml")
	if err != nil {
		t.Fatalf("ListHomebrewVersions failed: %v", err)
	}

	if len(versions) != 1 {
		t.Errorf("expected 1 version, got %d: %v", len(versions), versions)
	}
	if versions[0] != "0.2.5" {
		t.Errorf("expected '0.2.5', got '%s'", versions[0])
	}
}

func TestHomebrewProvider_ResolveLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "libyaml",
			"versions": {"stable": "0.2.5"},
			"versioned_formulae": []
		}`))
	}))
	defer server.Close()

	resolver := &Resolver{
		httpClient:          server.Client(),
		homebrewRegistryURL: server.URL,
	}

	provider := NewHomebrewProvider(resolver, "libyaml")

	info, err := provider.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest failed: %v", err)
	}
	if info.Version != "0.2.5" {
		t.Errorf("expected version '0.2.5', got '%s'", info.Version)
	}
}

func TestHomebrewProvider_ResolveVersion_Exact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "libyaml",
			"versions": {"stable": "0.2.5"},
			"versioned_formulae": []
		}`))
	}))
	defer server.Close()

	resolver := &Resolver{
		httpClient:          server.Client(),
		homebrewRegistryURL: server.URL,
	}

	provider := NewHomebrewProvider(resolver, "libyaml")

	info, err := provider.ResolveVersion(context.Background(), "0.2.5")
	if err != nil {
		t.Fatalf("ResolveVersion failed: %v", err)
	}
	if info.Version != "0.2.5" {
		t.Errorf("expected version '0.2.5', got '%s'", info.Version)
	}
}

func TestHomebrewProvider_ResolveVersion_Fuzzy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "libyaml",
			"versions": {"stable": "0.2.5"},
			"versioned_formulae": []
		}`))
	}))
	defer server.Close()

	resolver := &Resolver{
		httpClient:          server.Client(),
		homebrewRegistryURL: server.URL,
	}

	provider := NewHomebrewProvider(resolver, "libyaml")

	// Fuzzy match: "0.2" should match "0.2.5"
	info, err := provider.ResolveVersion(context.Background(), "0.2")
	if err != nil {
		t.Fatalf("ResolveVersion fuzzy failed: %v", err)
	}
	if info.Version != "0.2.5" {
		t.Errorf("expected version '0.2.5', got '%s'", info.Version)
	}
}

func TestHomebrewProvider_ResolveVersion_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "libyaml",
			"versions": {"stable": "0.2.5"},
			"versioned_formulae": []
		}`))
	}))
	defer server.Close()

	resolver := &Resolver{
		httpClient:          server.Client(),
		homebrewRegistryURL: server.URL,
	}

	provider := NewHomebrewProvider(resolver, "libyaml")

	_, err := provider.ResolveVersion(context.Background(), "9.9.9")
	if err == nil {
		t.Error("expected error for non-existent version")
	}
}

func TestHomebrewProvider_SourceDescription(t *testing.T) {
	provider := NewHomebrewProvider(nil, "libyaml")
	desc := provider.SourceDescription()
	if desc != "Homebrew:libyaml" {
		t.Errorf("expected 'Homebrew:libyaml', got '%s'", desc)
	}
}

func TestIsValidHomebrewFormula(t *testing.T) {
	tests := []struct {
		name    string
		formula string
		valid   bool
	}{
		{"simple", "libyaml", true},
		{"with dash", "lib-yaml", true},
		{"with underscore", "lib_yaml", true},
		{"versioned formula", "openssl@3.0", true},
		{"with dots", "node.js", true},
		{"empty", "", false},
		{"path traversal dots", "..", false},
		{"path separator", "lib/yaml", false},
		{"backslash", "lib\\yaml", false},
		{"starts with dash", "-libyaml", false},
		{"uppercase", "LibYaml", false},
		{"shell chars", "lib;yaml", false},
		{"space", "lib yaml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidHomebrewFormula(tt.formula)
			if result != tt.valid {
				t.Errorf("isValidHomebrewFormula(%q) = %v, want %v", tt.formula, result, tt.valid)
			}
		})
	}
}

func TestResolveHomebrew_NetworkError(t *testing.T) {
	// Create a server that closes immediately
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("connection refused")
	}))
	server.Close() // Close immediately to simulate network error

	resolver := &Resolver{
		httpClient:          &http.Client{},
		homebrewRegistryURL: server.URL,
	}

	_, err := resolver.ResolveHomebrew(context.Background(), "libyaml")
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestResolveHomebrew_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	resolver := &Resolver{
		httpClient:          server.Client(),
		homebrewRegistryURL: server.URL,
	}

	_, err := resolver.ResolveHomebrew(context.Background(), "libyaml")
	if err == nil {
		t.Fatal("expected parsing error")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Fatalf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeParsing {
		t.Errorf("expected ErrTypeParsing, got %v", resolverErr.Type)
	}
}

func TestResolveHomebrew_NoStableVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "headonly",
			"versions": {
				"stable": "",
				"head": "HEAD"
			}
		}`))
	}))
	defer server.Close()

	resolver := &Resolver{
		httpClient:          server.Client(),
		homebrewRegistryURL: server.URL,
	}

	_, err := resolver.ResolveHomebrew(context.Background(), "headonly")
	if err == nil {
		t.Fatal("expected error for formula without stable version")
	}
}
