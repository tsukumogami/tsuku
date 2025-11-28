package version

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsuku-dev/tsuku/internal/recipe"
)

// --- ProviderFactory Tests ---

func TestProviderFromRecipe_ExplicitSource(t *testing.T) {
	factory := NewProviderFactory()
	resolver := New()

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Version: recipe.VersionSection{
			Source: "custom_source",
		},
	}

	provider, err := factory.ProviderFromRecipe(resolver, r)
	if err != nil {
		t.Fatalf("ProviderFromRecipe failed: %v", err)
	}

	// Should create CustomProvider
	if _, ok := provider.(*CustomProvider); !ok {
		t.Errorf("Expected *CustomProvider, got %T", provider)
	}

	// Verify source description
	desc := provider.SourceDescription()
	expected := "custom:custom_source"
	if desc != expected {
		t.Errorf("SourceDescription() = %q, want %q", desc, expected)
	}
}

func TestProviderFromRecipe_GitHubRepo(t *testing.T) {
	factory := NewProviderFactory()
	resolver := New()

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Version: recipe.VersionSection{
			GitHubRepo: "rust-lang/rust",
		},
	}

	provider, err := factory.ProviderFromRecipe(resolver, r)
	if err != nil {
		t.Fatalf("ProviderFromRecipe failed: %v", err)
	}

	// Should create GitHubProvider
	if _, ok := provider.(*GitHubProvider); !ok {
		t.Errorf("Expected *GitHubProvider, got %T", provider)
	}

	// Verify source description
	desc := provider.SourceDescription()
	expected := "GitHub:rust-lang/rust"
	if desc != expected {
		t.Errorf("SourceDescription() = %q, want %q", desc, expected)
	}
}

func TestProviderFromRecipe_InferredGitHub(t *testing.T) {
	factory := NewProviderFactory()
	resolver := New()

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Steps: []recipe.Step{
			{
				Action: "github_archive",
				Params: map[string]interface{}{
					"repo": "neovim/neovim",
				},
			},
		},
	}

	provider, err := factory.ProviderFromRecipe(resolver, r)
	if err != nil {
		t.Fatalf("ProviderFromRecipe failed: %v", err)
	}

	// Should create GitHubProvider
	if _, ok := provider.(*GitHubProvider); !ok {
		t.Errorf("Expected *GitHubProvider, got %T", provider)
	}

	// Verify source description
	desc := provider.SourceDescription()
	expected := "GitHub:neovim/neovim"
	if desc != expected {
		t.Errorf("SourceDescription() = %q, want %q", desc, expected)
	}
}

func TestProviderFromRecipe_InferredNpm(t *testing.T) {
	factory := NewProviderFactory()
	resolver := New()

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]interface{}{
					"package": "turbo",
				},
			},
		},
	}

	provider, err := factory.ProviderFromRecipe(resolver, r)
	if err != nil {
		t.Fatalf("ProviderFromRecipe failed: %v", err)
	}

	// Should create NpmProvider
	if _, ok := provider.(*NpmProvider); !ok {
		t.Errorf("Expected *NpmProvider, got %T", provider)
	}

	// Verify source description
	desc := provider.SourceDescription()
	expected := "npm:turbo"
	if desc != expected {
		t.Errorf("SourceDescription() = %q, want %q", desc, expected)
	}
}

func TestProviderFromRecipe_NoSource(t *testing.T) {
	factory := NewProviderFactory()
	resolver := New()

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		// No version section, no steps with version info
	}

	_, err := factory.ProviderFromRecipe(resolver, r)
	if err == nil {
		t.Fatal("Expected error for recipe without version source")
	}

	expectedError := "no version source configured"
	if err.Error() != "no version source configured for recipe test-tool (add [version] section)" {
		t.Errorf("Error = %q, want to contain %q", err.Error(), expectedError)
	}
}

func TestProviderFromRecipe_Priority(t *testing.T) {
	// Test that explicit source beats inferred GitHub
	factory := NewProviderFactory()
	resolver := New()

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Version: recipe.VersionSection{
			Source: "custom_source", // Explicit source (priority 100)
		},
		Steps: []recipe.Step{
			{
				Action: "github_archive", // This should be ignored
				Params: map[string]interface{}{
					"repo": "neovim/neovim",
				},
			},
		},
	}

	provider, err := factory.ProviderFromRecipe(resolver, r)
	if err != nil {
		t.Fatalf("ProviderFromRecipe failed: %v", err)
	}

	// Should create CustomProvider, NOT GitHubProvider
	if _, ok := provider.(*CustomProvider); !ok {
		t.Errorf("Expected *CustomProvider (explicit source should win), got %T", provider)
	}
}

// --- GitHubProvider Tests ---

func TestGitHubProvider_Interface(t *testing.T) {
	resolver := New()
	provider := NewGitHubProvider(resolver, "rust-lang/rust")

	// Verify it implements VersionResolver
	var _ VersionResolver = provider

	// Verify it implements VersionLister
	var _ VersionLister = provider
}

func TestGitHubProvider_SourceDescription(t *testing.T) {
	resolver := New()
	provider := NewGitHubProvider(resolver, "rust-lang/rust")

	desc := provider.SourceDescription()
	expected := "GitHub:rust-lang/rust"
	if desc != expected {
		t.Errorf("SourceDescription() = %q, want %q", desc, expected)
	}
}

// --- NpmProvider Tests ---

func TestNpmProvider_Interface(t *testing.T) {
	resolver := New()
	provider := NewNpmProvider(resolver, "turbo")

	// Verify it implements VersionResolver
	var _ VersionResolver = provider

	// Verify it implements VersionLister
	var _ VersionLister = provider
}

func TestNpmProvider_SourceDescription(t *testing.T) {
	resolver := New()
	provider := NewNpmProvider(resolver, "turbo")

	desc := provider.SourceDescription()
	expected := "npm:turbo"
	if desc != expected {
		t.Errorf("SourceDescription() = %q, want %q", desc, expected)
	}
}

func TestNpmProvider_ResolveVersion_ExactMatch(t *testing.T) {
	// Create mock npm registry server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"name": "turbo",
			"versions": map[string]interface{}{
				"2.0.0": map[string]interface{}{"name": "turbo"},
				"1.5.0": map[string]interface{}{"name": "turbo"},
				"1.2.3": map[string]interface{}{"name": "turbo"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithNpmRegistry(server.URL)
	provider := NewNpmProvider(resolver, "turbo")

	ctx := context.Background()
	versionInfo, err := provider.ResolveVersion(ctx, "1.5.0")

	if err != nil {
		t.Fatalf("ResolveVersion failed: %v", err)
	}

	if versionInfo.Version != "1.5.0" {
		t.Errorf("Version = %q, want %q", versionInfo.Version, "1.5.0")
	}
	if versionInfo.Tag != "1.5.0" {
		t.Errorf("Tag = %q, want %q", versionInfo.Tag, "1.5.0")
	}
}

func TestNpmProvider_ResolveVersion_FuzzyMatch(t *testing.T) {
	// Create mock npm registry server with versions that test the fuzzy matching logic
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"name": "test-package",
			"versions": map[string]interface{}{
				"2.0.0":  map[string]interface{}{"name": "test-package"},
				"1.20.0": map[string]interface{}{"name": "test-package"}, // Should NOT match "1.2"
				"1.2.5":  map[string]interface{}{"name": "test-package"}, // Should match "1.2"
				"1.2.3":  map[string]interface{}{"name": "test-package"}, // Should match "1.2"
				"1.2.0":  map[string]interface{}{"name": "test-package"}, // Should match "1.2"
				"1.0.0":  map[string]interface{}{"name": "test-package"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithNpmRegistry(server.URL)
	provider := NewNpmProvider(resolver, "test-package")

	ctx := context.Background()

	// Test "1.2" fuzzy matching
	// Should match "1.2.5" (newest 1.2.x version) and NOT "1.20.0"
	versionInfo, err := provider.ResolveVersion(ctx, "1.2")

	if err != nil {
		t.Fatalf("ResolveVersion failed: %v", err)
	}

	// The implementation returns the first match from the sorted list
	// Versions are sorted newest first, so we should get 1.2.5
	expected := "1.2.5"
	if versionInfo.Version != expected {
		t.Errorf("Fuzzy match for '1.2': got version %q, want %q", versionInfo.Version, expected)
		t.Errorf("This indicates the fuzzy matching may have incorrectly matched '1.20.0' instead of '1.2.x'")
	}

	// Test "1" fuzzy matching - should match "1.20.0" (newest 1.x version)
	versionInfo2, err := provider.ResolveVersion(ctx, "1")
	if err != nil {
		t.Fatalf("ResolveVersion failed for '1': %v", err)
	}

	expected2 := "1.20.0"
	if versionInfo2.Version != expected2 {
		t.Errorf("Fuzzy match for '1': got version %q, want %q", versionInfo2.Version, expected2)
	}
}

func TestNpmProvider_ResolveVersion_FuzzyMatchEdgeCases(t *testing.T) {
	tests := []struct {
		name              string
		requestVersion    string
		availableVersions []string
		expectedMatch     string
		shouldMatch       bool
	}{
		{
			name:              "1.2 should match 1.2.3 not 1.20.0",
			requestVersion:    "1.2",
			availableVersions: []string{"2.0.0", "1.20.0", "1.2.3", "1.2.0", "1.0.0"},
			expectedMatch:     "1.2.3",
			shouldMatch:       true,
		},
		{
			name:              "1 should match 1.20.0",
			requestVersion:    "1",
			availableVersions: []string{"2.0.0", "1.20.0", "1.2.3", "1.0.0"},
			expectedMatch:     "1.20.0",
			shouldMatch:       true,
		},
		{
			name:              "1.2.3 exact match",
			requestVersion:    "1.2.3",
			availableVersions: []string{"1.2.3", "1.2.0"},
			expectedMatch:     "1.2.3",
			shouldMatch:       true,
		},
		{
			name:              "10 should not match 1.0.0",
			requestVersion:    "10",
			availableVersions: []string{"1.0.0", "2.0.0"},
			expectedMatch:     "",
			shouldMatch:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				versions := make(map[string]interface{})
				for _, v := range tt.availableVersions {
					versions[v] = map[string]interface{}{"name": "test-package"}
				}

				response := map[string]interface{}{
					"name":     "test-package",
					"versions": versions,
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			resolver := NewWithNpmRegistry(server.URL)
			provider := NewNpmProvider(resolver, "test-package")

			ctx := context.Background()
			versionInfo, err := provider.ResolveVersion(ctx, tt.requestVersion)

			if tt.shouldMatch {
				if err != nil {
					t.Fatalf("ResolveVersion failed: %v", err)
				}
				if versionInfo.Version != tt.expectedMatch {
					t.Errorf("Version = %q, want %q", versionInfo.Version, tt.expectedMatch)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error for non-matching version %q, got version %q", tt.requestVersion, versionInfo.Version)
				}
			}
		})
	}
}

func TestNpmProvider_ResolveVersion_NotFound(t *testing.T) {
	// Create mock npm registry server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"name": "turbo",
			"versions": map[string]interface{}{
				"2.0.0": map[string]interface{}{"name": "turbo"},
				"1.5.0": map[string]interface{}{"name": "turbo"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithNpmRegistry(server.URL)
	provider := NewNpmProvider(resolver, "turbo")

	ctx := context.Background()
	_, err := provider.ResolveVersion(ctx, "99.99.99")

	if err == nil {
		t.Fatal("Expected error for non-existent version")
	}

	expectedError := "version 99.99.99 not found"
	if err.Error() != "version 99.99.99 not found for npm package turbo" {
		t.Errorf("Error message should contain %q, got: %v", expectedError, err)
	}
}

// --- CustomProvider Tests ---

func TestCustomProvider_Interface(t *testing.T) {
	resolver := New()
	provider := NewCustomProvider(resolver, "nodejs_dist")

	// Verify it implements VersionResolver
	var _ VersionResolver = provider

	// Verify it does NOT implement VersionLister (should fail compilation if it does)
	// We test this by checking if the interface assertion fails at runtime
	if _, ok := interface{}(provider).(VersionLister); ok {
		t.Error("CustomProvider should NOT implement VersionLister interface")
	}
}

func TestCustomProvider_SourceDescription(t *testing.T) {
	resolver := New()
	provider := NewCustomProvider(resolver, "nodejs_dist")

	desc := provider.SourceDescription()
	expected := "custom:nodejs_dist"
	if desc != expected {
		t.Errorf("SourceDescription() = %q, want %q", desc, expected)
	}
}

// --- Strategy Tests ---

func TestExplicitSourceStrategy_InvalidSourceName(t *testing.T) {
	factory := NewProviderFactory()
	resolver := New()

	invalidNames := []string{
		"invalid name",           // spaces
		"invalid@name",           // special chars
		"invalid/name",           // slashes
		"",                       // empty
		string(make([]byte, 65)), // too long
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			r := &recipe.Recipe{
				Metadata: recipe.MetadataSection{
					Name: "test-tool",
				},
				Version: recipe.VersionSection{
					Source: name,
				},
			}

			_, err := factory.ProviderFromRecipe(resolver, r)
			if err == nil {
				t.Errorf("Expected error for invalid source name %q", name)
			}
		})
	}
}

func TestIsValidSourceName(t *testing.T) {
	tests := []struct {
		name       string
		sourceName string
		wantValid  bool
	}{
		{"valid simple", "nodejs_dist", true},
		{"valid with hyphen", "node-dist", true},
		{"valid with underscore", "node_dist", true},
		{"valid alphanumeric", "nodejs18", true},
		{"invalid empty", "", false},
		{"invalid too long", string(make([]byte, 65)), false},
		{"invalid spaces", "node dist", false},
		{"invalid special chars", "node@dist", false},
		{"invalid slash", "node/dist", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidSourceName(tt.sourceName)
			if got != tt.wantValid {
				t.Errorf("isValidSourceName(%q) = %v, want %v", tt.sourceName, got, tt.wantValid)
			}
		})
	}
}
