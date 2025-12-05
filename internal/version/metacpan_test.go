package version

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestIsValidMetaCPANDistribution(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid distribution names
		{"simple", "App-Ack", true},
		{"single word", "Moose", true},
		{"multi part", "Perl-Critic", true},
		{"long name", "MooseX-Types-DateTime-MoreCoercions", true},
		{"with numbers", "File-Slurp-9000", true},
		{"lowercase", "app-ack", true},

		// Invalid distribution names
		{"empty", "", false},
		{"module name", "App::Ack", false},
		{"starts with hyphen", "-App-Ack", false},
		{"starts with number", "9App", false},
		{"contains slash", "App/Ack", false},
		{"contains dot", "App.Ack", false},
		{"too long", "A" + string(make([]byte, 128)), false},
		{"consecutive hyphens", "App--Ack", false},
		{"ends with hyphen", "App-Ack-", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidMetaCPANDistribution(tt.input)
			if result != tt.expected {
				t.Errorf("isValidMetaCPANDistribution(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeModuleToDistribution(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"App::Ack", "App-Ack"},
		{"Perl::Critic", "Perl-Critic"},
		{"File::Slurp", "File-Slurp"},
		{"MooseX::Types::DateTime", "MooseX-Types-DateTime"},
		{"Moose", "Moose"}, // Already a distribution name
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeModuleToDistribution(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeModuleToDistribution(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestResolveMetaCPAN(t *testing.T) {
	// Create mock MetaCPAN API server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check path
		if r.URL.Path != "/release/App-Ack" {
			http.NotFound(w, r)
			return
		}

		// Check headers
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("expected Accept header application/json, got %s", r.Header.Get("Accept"))
		}

		if r.Header.Get("User-Agent") == "" {
			t.Error("expected User-Agent header to be set")
		}

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		release := metacpanRelease{
			Distribution: "App-Ack",
			Version:      "3.7.0",
			Author:       "PETDANCE",
			DownloadURL:  "https://cpan.metacpan.org/authors/id/P/PE/PETDANCE/ack-v3.7.0.tar.gz",
			Status:       "latest",
		}
		_ = json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	// Create resolver with mock server
	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	info, err := resolver.ResolveMetaCPAN(ctx, "App-Ack")
	if err != nil {
		t.Fatalf("ResolveMetaCPAN failed: %v", err)
	}

	if info.Version != "3.7.0" {
		t.Errorf("expected version 3.7.0, got %s", info.Version)
	}
}

func TestListMetaCPANVersions(t *testing.T) {
	// Create mock MetaCPAN API server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check path
		if r.URL.Path != "/release/_search" {
			http.NotFound(w, r)
			return
		}

		// Check method
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		// Check headers
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		response := metacpanSearchResponse{
			Hits: struct {
				Hits []struct {
					Source metacpanRelease `json:"_source"`
				} `json:"hits"`
			}{
				Hits: []struct {
					Source metacpanRelease `json:"_source"`
				}{
					{Source: metacpanRelease{Version: "3.7.0", Status: "latest"}},
					{Source: metacpanRelease{Version: "3.6.0", Status: "cpan"}},
					{Source: metacpanRelease{Version: "3.5.0", Status: "cpan"}},
					{Source: metacpanRelease{Version: "3.4.0", Status: "cpan"}},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create resolver with mock server
	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	versions, err := resolver.ListMetaCPANVersions(ctx, "App-Ack")
	if err != nil {
		t.Fatalf("ListMetaCPANVersions failed: %v", err)
	}

	// Should have 4 versions
	if len(versions) != 4 {
		t.Errorf("expected 4 versions, got %d: %v", len(versions), versions)
	}

	// Should be sorted newest first
	if versions[0] != "3.7.0" {
		t.Errorf("expected first version to be 3.7.0, got %s", versions[0])
	}
}

func TestResolveMetaCPAN_NotFound(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	_, err := resolver.ResolveMetaCPAN(ctx, "Nonexistent-Distribution")
	if err == nil {
		t.Error("expected error for nonexistent distribution")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeNotFound {
		t.Errorf("expected ErrTypeNotFound, got %v", resolverErr.Type)
	}
}

func TestResolveMetaCPAN_RateLimit(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	_, err := resolver.ResolveMetaCPAN(ctx, "App-Ack")
	if err == nil {
		t.Error("expected error for rate limit")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeRateLimit {
		t.Errorf("expected ErrTypeRateLimit, got %v", resolverErr.Type)
	}
}

func TestResolveMetaCPAN_InvalidDistributionName(t *testing.T) {
	resolver := New()
	ctx := context.Background()

	// Test invalid distribution name
	_, err := resolver.ResolveMetaCPAN(ctx, "invalid;name")
	if err == nil {
		t.Error("expected error for invalid distribution name")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeValidation {
		t.Errorf("expected ErrTypeValidation, got %v", resolverErr.Type)
	}
}

func TestResolveMetaCPAN_ModuleNameSuggestion(t *testing.T) {
	resolver := New()
	ctx := context.Background()

	// Test module name (should suggest conversion)
	_, err := resolver.ResolveMetaCPAN(ctx, "App::Ack")
	if err == nil {
		t.Error("expected error for module name")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeValidation {
		t.Errorf("expected ErrTypeValidation, got %v", resolverErr.Type)
	}

	// Error message should suggest App-Ack
	if resolverErr.Message == "" {
		t.Error("expected error message")
	}
}

func TestResolveMetaCPAN_HTTPSEnforcement(t *testing.T) {
	// Create a resolver with HTTP URL (should fail)
	resolver := NewWithMetaCPANRegistry("http://fastapi.metacpan.org/v1")

	ctx := context.Background()
	_, err := resolver.ResolveMetaCPAN(ctx, "App-Ack")
	if err == nil {
		t.Error("expected error for HTTP URL")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeValidation {
		t.Errorf("expected ErrTypeValidation for HTTPS enforcement, got %v", resolverErr.Type)
	}
}

func TestResolveMetaCPAN_InvalidContentType(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>Not JSON</html>"))
	}))
	defer server.Close()

	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	_, err := resolver.ResolveMetaCPAN(ctx, "App-Ack")
	if err == nil {
		t.Error("expected error for invalid content-type")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeParsing {
		t.Errorf("expected ErrTypeParsing, got %v", resolverErr.Type)
	}
}

func TestMetaCPANProvider_ResolveVersion(t *testing.T) {
	// Create mock server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := metacpanSearchResponse{
			Hits: struct {
				Hits []struct {
					Source metacpanRelease `json:"_source"`
				} `json:"hits"`
			}{
				Hits: []struct {
					Source metacpanRelease `json:"_source"`
				}{
					{Source: metacpanRelease{Version: "3.7.0"}},
					{Source: metacpanRelease{Version: "3.6.0"}},
					{Source: metacpanRelease{Version: "3.5.0"}},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()
	provider := NewMetaCPANProvider(resolver, "App-Ack")

	ctx := context.Background()

	// Test exact match
	info, err := provider.ResolveVersion(ctx, "3.6.0")
	if err != nil {
		t.Fatalf("ResolveVersion failed: %v", err)
	}
	if info.Version != "3.6.0" {
		t.Errorf("expected version 3.6.0, got %s", info.Version)
	}

	// Test fuzzy match
	info, err = provider.ResolveVersion(ctx, "3.7")
	if err != nil {
		t.Fatalf("ResolveVersion fuzzy failed: %v", err)
	}
	if info.Version != "3.7.0" {
		t.Errorf("expected fuzzy version 3.7.0, got %s", info.Version)
	}

	// Test not found
	_, err = provider.ResolveVersion(ctx, "9.9.9")
	if err == nil {
		t.Error("expected error for nonexistent version")
	}
}

func TestMetaCPANProvider_SourceDescription(t *testing.T) {
	resolver := New()
	provider := NewMetaCPANProvider(resolver, "App-Ack")

	desc := provider.SourceDescription()
	if desc != "metacpan:App-Ack" {
		t.Errorf("expected 'metacpan:App-Ack', got %s", desc)
	}
}

func TestMetaCPANSourceStrategy_CanHandle(t *testing.T) {
	strategy := &MetaCPANSourceStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "metacpan source with cpan_install",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "metacpan"},
				Steps: []recipe.Step{
					{
						Action: "cpan_install",
						Params: map[string]interface{}{"distribution": "App-Ack"},
					},
				},
			},
			expected: true,
		},
		{
			name: "metacpan source without cpan_install",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "metacpan"},
				Steps:   []recipe.Step{},
			},
			expected: false,
		},
		{
			name: "other source",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "npm"},
				Steps: []recipe.Step{
					{
						Action: "cpan_install",
						Params: map[string]interface{}{"distribution": "App-Ack"},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestInferredMetaCPANStrategy_CanHandle(t *testing.T) {
	strategy := &InferredMetaCPANStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "with cpan_install action",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{
						Action: "cpan_install",
						Params: map[string]interface{}{"distribution": "App-Ack"},
					},
				},
			},
			expected: true,
		},
		{
			name: "without cpan_install action",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{
						Action: "download_archive",
						Params: map[string]interface{}{"url": "https://example.com/file.tar.gz"},
					},
				},
			},
			expected: false,
		},
		{
			name: "cpan_install without distribution param",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{
						Action: "cpan_install",
						Params: map[string]interface{}{},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := strategy.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestListMetaCPANVersions_DeduplicatesVersions(t *testing.T) {
	// Create mock server that returns duplicate versions
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := metacpanSearchResponse{
			Hits: struct {
				Hits []struct {
					Source metacpanRelease `json:"_source"`
				} `json:"hits"`
			}{
				Hits: []struct {
					Source metacpanRelease `json:"_source"`
				}{
					{Source: metacpanRelease{Version: "3.7.0"}},
					{Source: metacpanRelease{Version: "3.7.0"}}, // Duplicate
					{Source: metacpanRelease{Version: "3.6.0"}},
					{Source: metacpanRelease{Version: "3.6.0"}}, // Duplicate
				},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	versions, err := resolver.ListMetaCPANVersions(ctx, "App-Ack")
	if err != nil {
		t.Fatalf("ListMetaCPANVersions failed: %v", err)
	}

	// Should have 2 unique versions
	if len(versions) != 2 {
		t.Errorf("expected 2 unique versions, got %d: %v", len(versions), versions)
	}
}

func TestMetaCPANProvider_ResolveLatest(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		release := metacpanRelease{
			Distribution: "App-Ack",
			Version:      "3.7.0",
			Author:       "PETDANCE",
			Status:       "latest",
		}
		_ = json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()
	provider := NewMetaCPANProvider(resolver, "App-Ack")

	ctx := context.Background()
	info, err := provider.ResolveLatest(ctx)
	if err != nil {
		t.Fatalf("ResolveLatest failed: %v", err)
	}

	if info.Version != "3.7.0" {
		t.Errorf("expected version 3.7.0, got %s", info.Version)
	}
}

func TestListMetaCPANVersions_InvalidDistributionName(t *testing.T) {
	resolver := New()
	ctx := context.Background()

	// Test invalid distribution name
	_, err := resolver.ListMetaCPANVersions(ctx, "invalid;name")
	if err == nil {
		t.Error("expected error for invalid distribution name")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeValidation {
		t.Errorf("expected ErrTypeValidation, got %v", resolverErr.Type)
	}
}

func TestListMetaCPANVersions_ModuleNameSuggestion(t *testing.T) {
	resolver := New()
	ctx := context.Background()

	// Test module name (should suggest conversion)
	_, err := resolver.ListMetaCPANVersions(ctx, "App::Ack")
	if err == nil {
		t.Error("expected error for module name")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeValidation {
		t.Errorf("expected ErrTypeValidation, got %v", resolverErr.Type)
	}

	// Error message should suggest App-Ack
	if resolverErr.Message == "" {
		t.Error("expected error message with suggestion")
	}
}

func TestListMetaCPANVersions_HTTPSEnforcement(t *testing.T) {
	// Create a resolver with HTTP URL (should fail)
	resolver := NewWithMetaCPANRegistry("http://fastapi.metacpan.org/v1")

	ctx := context.Background()
	_, err := resolver.ListMetaCPANVersions(ctx, "App-Ack")
	if err == nil {
		t.Error("expected error for HTTP URL")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeValidation {
		t.Errorf("expected ErrTypeValidation for HTTPS enforcement, got %v", resolverErr.Type)
	}
}

func TestListMetaCPANVersions_NotFound(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	_, err := resolver.ListMetaCPANVersions(ctx, "Nonexistent-Distribution")
	if err == nil {
		t.Error("expected error for nonexistent distribution")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeNotFound {
		t.Errorf("expected ErrTypeNotFound, got %v", resolverErr.Type)
	}
}

func TestListMetaCPANVersions_RateLimit(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	_, err := resolver.ListMetaCPANVersions(ctx, "App-Ack")
	if err == nil {
		t.Error("expected error for rate limit")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeRateLimit {
		t.Errorf("expected ErrTypeRateLimit, got %v", resolverErr.Type)
	}
}

func TestListMetaCPANVersions_InvalidContentType(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>Not JSON</html>"))
	}))
	defer server.Close()

	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	_, err := resolver.ListMetaCPANVersions(ctx, "App-Ack")
	if err == nil {
		t.Error("expected error for invalid content-type")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeParsing {
		t.Errorf("expected ErrTypeParsing, got %v", resolverErr.Type)
	}
}

func TestListMetaCPANVersions_ServerError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	_, err := resolver.ListMetaCPANVersions(ctx, "App-Ack")
	if err == nil {
		t.Error("expected error for server error")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeNetwork {
		t.Errorf("expected ErrTypeNetwork, got %v", resolverErr.Type)
	}
}

func TestResolveMetaCPAN_EmptyVersion(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		release := metacpanRelease{
			Distribution: "App-Ack",
			Version:      "", // Empty version
			Author:       "PETDANCE",
		}
		_ = json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	_, err := resolver.ResolveMetaCPAN(ctx, "App-Ack")
	if err == nil {
		t.Error("expected error for empty version")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeNotFound {
		t.Errorf("expected ErrTypeNotFound, got %v", resolverErr.Type)
	}
}

func TestResolveMetaCPAN_ServerError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	resolver := NewWithMetaCPANRegistry(server.URL)
	resolver.httpClient = server.Client()

	ctx := context.Background()
	_, err := resolver.ResolveMetaCPAN(ctx, "App-Ack")
	if err == nil {
		t.Error("expected error for server error")
	}

	resolverErr, ok := err.(*ResolverError)
	if !ok {
		t.Errorf("expected ResolverError, got %T", err)
	}
	if resolverErr.Type != ErrTypeNetwork {
		t.Errorf("expected ErrTypeNetwork, got %v", resolverErr.Type)
	}
}

func TestMetaCPANSourceStrategy_Create(t *testing.T) {
	strategy := &MetaCPANSourceStrategy{}
	resolver := New()

	r := &recipe.Recipe{
		Version: recipe.VersionSection{Source: "metacpan"},
		Steps: []recipe.Step{
			{
				Action: "cpan_install",
				Params: map[string]interface{}{"distribution": "App-Ack"},
			},
		},
	}

	provider, err := strategy.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if provider.SourceDescription() != "metacpan:App-Ack" {
		t.Errorf("expected 'metacpan:App-Ack', got %s", provider.SourceDescription())
	}
}

func TestMetaCPANSourceStrategy_Create_NoDistribution(t *testing.T) {
	strategy := &MetaCPANSourceStrategy{}
	resolver := New()

	r := &recipe.Recipe{
		Version: recipe.VersionSection{Source: "metacpan"},
		Steps: []recipe.Step{
			{
				Action: "cpan_install",
				Params: map[string]interface{}{}, // No distribution
			},
		},
	}

	_, err := strategy.Create(resolver, r)
	if err == nil {
		t.Error("expected error when no distribution found")
	}
}

func TestInferredMetaCPANStrategy_Create(t *testing.T) {
	strategy := &InferredMetaCPANStrategy{}
	resolver := New()

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "cpan_install",
				Params: map[string]interface{}{"distribution": "Perl-Critic"},
			},
		},
	}

	provider, err := strategy.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if provider.SourceDescription() != "metacpan:Perl-Critic" {
		t.Errorf("expected 'metacpan:Perl-Critic', got %s", provider.SourceDescription())
	}
}

func TestInferredMetaCPANStrategy_Create_NoDistribution(t *testing.T) {
	strategy := &InferredMetaCPANStrategy{}
	resolver := New()

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "cpan_install",
				Params: map[string]interface{}{}, // No distribution
			},
		},
	}

	_, err := strategy.Create(resolver, r)
	if err == nil {
		t.Error("expected error when no distribution found")
	}
}
