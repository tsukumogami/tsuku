package version

import (
	"testing"

	"github.com/tsuku-dev/tsuku/internal/recipe"
)

func TestNewProviderFactory(t *testing.T) {
	factory := NewProviderFactory()
	if factory == nil {
		t.Fatal("NewProviderFactory() returned nil")
	}
	if len(factory.strategies) == 0 {
		t.Error("NewProviderFactory() should register default strategies")
	}
}

func TestIsValidSourceName_Validation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid names
		{"simple", "github", true},
		{"with hyphen", "my-source", true},
		{"with underscore", "my_source", true},
		{"alphanumeric", "source123", true},
		{"mixed case", "MySource", true},

		// Invalid names
		{"empty", "", false},
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false}, // 65 chars
		{"with space", "my source", false},
		{"with special char", "my@source", false},
		{"with slash", "my/source", false},
		{"with dot", "my.source", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidSourceName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidSourceName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExplicitSourceStrategy_Priority(t *testing.T) {
	s := &ExplicitSourceStrategy{}
	if s.Priority() != PriorityExplicitSource {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityExplicitSource)
	}
}

func TestExplicitSourceStrategy_CanHandle(t *testing.T) {
	s := &ExplicitSourceStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "with source",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "custom-source"},
			},
			expected: true,
		},
		{
			name: "without source",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: ""},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExplicitSourceStrategy_Create_InvalidName(t *testing.T) {
	s := &ExplicitSourceStrategy{}
	resolver := New()
	r := &recipe.Recipe{
		Version: recipe.VersionSection{Source: "invalid@source"},
	}

	_, err := s.Create(resolver, r)
	if err == nil {
		t.Error("Create() should fail for invalid source name")
	}
}

func TestGitHubRepoStrategy_Priority(t *testing.T) {
	s := &GitHubRepoStrategy{}
	if s.Priority() != PriorityExplicitHint {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityExplicitHint)
	}
}

func TestGitHubRepoStrategy_CanHandle(t *testing.T) {
	s := &GitHubRepoStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "with github_repo",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{GitHubRepo: "owner/repo"},
			},
			expected: true,
		},
		{
			name: "without github_repo",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{GitHubRepo: ""},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGitHubRepoStrategy_Create(t *testing.T) {
	s := &GitHubRepoStrategy{}
	resolver := New()

	tests := []struct {
		name      string
		recipe    *recipe.Recipe
		expectNil bool
	}{
		{
			name: "without prefix",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{GitHubRepo: "owner/repo"},
			},
			expectNil: false,
		},
		{
			name: "with prefix",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{
					GitHubRepo: "owner/repo",
					TagPrefix:  "v",
				},
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := s.Create(resolver, tt.recipe)
			if err != nil {
				t.Errorf("Create() error = %v", err)
			}
			if (provider == nil) != tt.expectNil {
				t.Errorf("Create() returned nil = %v, want %v", provider == nil, tt.expectNil)
			}
		})
	}
}

func TestInferredGitHubStrategy_Priority(t *testing.T) {
	s := &InferredGitHubStrategy{}
	if s.Priority() != PriorityInferred {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityInferred)
	}
}

func TestInferredGitHubStrategy_CanHandle(t *testing.T) {
	s := &InferredGitHubStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "with github_archive action",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "github_archive", Params: map[string]interface{}{"repo": "owner/repo"}},
				},
			},
			expected: true,
		},
		{
			name: "with github_file action",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "github_file", Params: map[string]interface{}{"repo": "owner/repo"}},
				},
			},
			expected: true,
		},
		{
			name: "without github action",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "download", Params: map[string]interface{}{"url": "https://example.com"}},
				},
			},
			expected: false,
		},
		{
			name: "github action without repo",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "github_archive", Params: map[string]interface{}{}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestInferredNpmStrategy_Priority(t *testing.T) {
	s := &InferredNpmStrategy{}
	if s.Priority() != PriorityInferred {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityInferred)
	}
}

func TestInferredNpmStrategy_CanHandle(t *testing.T) {
	s := &InferredNpmStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "with npm_install action",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "npm_install", Params: map[string]interface{}{"package": "express"}},
				},
			},
			expected: true,
		},
		{
			name: "without npm action",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "download", Params: map[string]interface{}{}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestInferredPyPIStrategy_Priority(t *testing.T) {
	s := &InferredPyPIStrategy{}
	if s.Priority() != PriorityInferred {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityInferred)
	}
}

func TestInferredCratesIOStrategy_Priority(t *testing.T) {
	s := &InferredCratesIOStrategy{}
	if s.Priority() != PriorityInferred {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityInferred)
	}
}

func TestInferredRubyGemsStrategy_Priority(t *testing.T) {
	s := &InferredRubyGemsStrategy{}
	if s.Priority() != PriorityInferred {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityInferred)
	}
}

func TestPyPISourceStrategy_Priority(t *testing.T) {
	s := &PyPISourceStrategy{}
	if s.Priority() != PriorityKnownRegistry {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityKnownRegistry)
	}
}

func TestCratesIOSourceStrategy_Priority(t *testing.T) {
	s := &CratesIOSourceStrategy{}
	if s.Priority() != PriorityKnownRegistry {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityKnownRegistry)
	}
}

func TestRubyGemsSourceStrategy_Priority(t *testing.T) {
	s := &RubyGemsSourceStrategy{}
	if s.Priority() != PriorityKnownRegistry {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityKnownRegistry)
	}
}

func TestNixpkgsSourceStrategy_Priority(t *testing.T) {
	s := &NixpkgsSourceStrategy{}
	if s.Priority() != PriorityKnownRegistry {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityKnownRegistry)
	}
}

func TestNixpkgsSourceStrategy_CanHandle(t *testing.T) {
	s := &NixpkgsSourceStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "with nixpkgs source",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "nixpkgs"},
			},
			expected: true,
		},
		{
			name: "with other source",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "other"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestProviderFactory_ProviderFromRecipe_NoSource(t *testing.T) {
	factory := NewProviderFactory()
	resolver := New()

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
	}

	_, err := factory.ProviderFromRecipe(resolver, r)
	if err == nil {
		t.Error("ProviderFromRecipe() should fail when no source configured")
	}
}

func TestProviderFactory_ProviderFromRecipe_GitHubRepo(t *testing.T) {
	factory := NewProviderFactory()
	resolver := New()

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Version:  recipe.VersionSection{GitHubRepo: "owner/repo"},
	}

	provider, err := factory.ProviderFromRecipe(resolver, r)
	if err != nil {
		t.Fatalf("ProviderFromRecipe() error = %v", err)
	}
	if provider == nil {
		t.Error("ProviderFromRecipe() returned nil provider")
	}
}

func TestProviderFactory_Register(t *testing.T) {
	factory := &ProviderFactory{}
	initialCount := len(factory.strategies)

	factory.Register(&ExplicitSourceStrategy{})

	if len(factory.strategies) != initialCount+1 {
		t.Errorf("Register() did not add strategy, count = %d, want %d", len(factory.strategies), initialCount+1)
	}
}

func TestCustomProvider_SourceDescription_Factory(t *testing.T) {
	resolver := New()
	provider := NewCustomProvider(resolver, "my-source")

	desc := provider.SourceDescription()
	expected := "custom:my-source"

	if desc != expected {
		t.Errorf("SourceDescription() = %q, want %q", desc, expected)
	}
}

func TestPyPISourceStrategy_CanHandle(t *testing.T) {
	s := &PyPISourceStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "pypi source with pipx_install action",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "pypi"},
				Steps: []recipe.Step{
					{Action: "pipx_install", Params: map[string]interface{}{"package": "black"}},
				},
			},
			expected: true,
		},
		{
			name: "pypi source without pipx_install",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "pypi"},
				Steps:   []recipe.Step{},
			},
			expected: false,
		},
		{
			name: "non-pypi source",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "github"},
				Steps: []recipe.Step{
					{Action: "pipx_install", Params: map[string]interface{}{"package": "black"}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCratesIOSourceStrategy_CanHandle(t *testing.T) {
	s := &CratesIOSourceStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "crates_io source with cargo_install action",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "crates_io"},
				Steps: []recipe.Step{
					{Action: "cargo_install", Params: map[string]interface{}{"crate": "ripgrep"}},
				},
			},
			expected: true,
		},
		{
			name: "crates_io source without cargo_install",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "crates_io"},
				Steps:   []recipe.Step{},
			},
			expected: false,
		},
		{
			name: "non-crates_io source",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "github"},
				Steps: []recipe.Step{
					{Action: "cargo_install", Params: map[string]interface{}{"crate": "ripgrep"}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRubyGemsSourceStrategy_CanHandle(t *testing.T) {
	s := &RubyGemsSourceStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "rubygems source with gem_install action",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "rubygems"},
				Steps: []recipe.Step{
					{Action: "gem_install", Params: map[string]interface{}{"gem": "rails"}},
				},
			},
			expected: true,
		},
		{
			name: "rubygems source without gem_install",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "rubygems"},
				Steps:   []recipe.Step{},
			},
			expected: false,
		},
		{
			name: "non-rubygems source",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "github"},
				Steps: []recipe.Step{
					{Action: "gem_install", Params: map[string]interface{}{"gem": "rails"}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestInferredPyPIStrategy_CanHandle(t *testing.T) {
	s := &InferredPyPIStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "with pipx_install action",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "pipx_install", Params: map[string]interface{}{"package": "black"}},
				},
			},
			expected: true,
		},
		{
			name: "without pipx_install action",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "download", Params: map[string]interface{}{}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestInferredCratesIOStrategy_CanHandle(t *testing.T) {
	s := &InferredCratesIOStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "with cargo_install action",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "cargo_install", Params: map[string]interface{}{"crate": "ripgrep"}},
				},
			},
			expected: true,
		},
		{
			name: "without cargo_install action",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "download", Params: map[string]interface{}{}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestInferredRubyGemsStrategy_CanHandle(t *testing.T) {
	s := &InferredRubyGemsStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "with gem_install action",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "gem_install", Params: map[string]interface{}{"gem": "rails"}},
				},
			},
			expected: true,
		},
		{
			name: "without gem_install action",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "download", Params: map[string]interface{}{}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExplicitSourceStrategy_Create(t *testing.T) {
	resolver := New()
	s := &ExplicitSourceStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Version:  recipe.VersionSection{Source: "nodejs_dist"},
	}

	provider, err := s.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if provider == nil {
		t.Fatal("Create() returned nil provider")
	}

	// Verify it's a CustomProvider
	customProvider, ok := provider.(*CustomProvider)
	if !ok {
		t.Errorf("Create() returned %T, want *CustomProvider", provider)
	}

	if customProvider != nil && customProvider.SourceDescription() != "custom:nodejs_dist" {
		t.Errorf("SourceDescription() = %q, want %q", customProvider.SourceDescription(), "custom:nodejs_dist")
	}
}

func TestExplicitSourceStrategy_Create_InvalidSource(t *testing.T) {
	resolver := New()
	s := &ExplicitSourceStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Version:  recipe.VersionSection{Source: "../invalid"},
	}

	_, err := s.Create(resolver, r)
	if err == nil {
		t.Error("Create() should fail for invalid source name")
	}
}

func TestInferredGitHubStrategy_Create(t *testing.T) {
	resolver := New()
	s := &InferredGitHubStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Steps: []recipe.Step{
			{
				Action: "github_archive",
				Params: map[string]interface{}{
					"repo": "owner/repo",
				},
			},
		},
	}

	provider, err := s.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if provider == nil {
		t.Fatal("Create() returned nil provider")
	}
}

func TestInferredGitHubStrategy_Create_NoRepo(t *testing.T) {
	resolver := New()
	s := &InferredGitHubStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Steps: []recipe.Step{
			{
				Action: "github_archive",
				Params: map[string]interface{}{}, // Missing repo
			},
		},
	}

	_, err := s.Create(resolver, r)
	if err == nil {
		t.Error("Create() should fail when repo is missing")
	}
}

func TestInferredPyPIStrategy_Create(t *testing.T) {
	resolver := New()
	s := &InferredPyPIStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Steps: []recipe.Step{
			{
				Action: "pipx_install",
				Params: map[string]interface{}{
					"package": "black",
				},
			},
		},
	}

	provider, err := s.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if provider == nil {
		t.Fatal("Create() returned nil provider")
	}
}

func TestInferredCratesIOStrategy_Create(t *testing.T) {
	resolver := New()
	s := &InferredCratesIOStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Steps: []recipe.Step{
			{
				Action: "cargo_install",
				Params: map[string]interface{}{
					"crate": "ripgrep",
				},
			},
		},
	}

	provider, err := s.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if provider == nil {
		t.Fatal("Create() returned nil provider")
	}
}

func TestInferredRubyGemsStrategy_Create(t *testing.T) {
	resolver := New()
	s := &InferredRubyGemsStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Steps: []recipe.Step{
			{
				Action: "gem_install",
				Params: map[string]interface{}{
					"gem": "rails",
				},
			},
		},
	}

	provider, err := s.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if provider == nil {
		t.Fatal("Create() returned nil provider")
	}
}

func TestPyPISourceStrategy_Create(t *testing.T) {
	resolver := New()
	s := &PyPISourceStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Version:  recipe.VersionSection{Source: "pypi"},
		Steps: []recipe.Step{
			{
				Action: "pipx_install",
				Params: map[string]interface{}{
					"package": "black",
				},
			},
		},
	}

	provider, err := s.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if provider == nil {
		t.Fatal("Create() returned nil provider")
	}
}

func TestCratesIOSourceStrategy_Create(t *testing.T) {
	resolver := New()
	s := &CratesIOSourceStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Version:  recipe.VersionSection{Source: "crates_io"},
		Steps: []recipe.Step{
			{
				Action: "cargo_install",
				Params: map[string]interface{}{
					"crate": "ripgrep",
				},
			},
		},
	}

	provider, err := s.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if provider == nil {
		t.Fatal("Create() returned nil provider")
	}
}

func TestRubyGemsSourceStrategy_Create(t *testing.T) {
	resolver := New()
	s := &RubyGemsSourceStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Version:  recipe.VersionSection{Source: "rubygems"},
		Steps: []recipe.Step{
			{
				Action: "gem_install",
				Params: map[string]interface{}{
					"gem": "rails",
				},
			},
		},
	}

	provider, err := s.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if provider == nil {
		t.Fatal("Create() returned nil provider")
	}
}

func TestProviderFactory_ProviderFromRecipe_NoMatchingStrategy(t *testing.T) {
	factory := NewProviderFactory()
	resolver := New()

	// Recipe with no version source and no package manager actions
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{"url": "https://example.com/file.tar.gz"},
			},
		},
	}

	_, err := factory.ProviderFromRecipe(resolver, r)
	if err == nil {
		t.Error("ProviderFromRecipe() should fail when no strategy matches")
	}
}

func TestNixpkgsSourceStrategy_Create(t *testing.T) {
	resolver := New()
	s := &NixpkgsSourceStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Version:  recipe.VersionSection{Source: "nixpkgs"},
		Steps: []recipe.Step{
			{
				Action: "nix_install",
				Params: map[string]interface{}{
					"package": "hello",
				},
			},
		},
	}

	provider, err := s.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if provider == nil {
		t.Fatal("Create() returned nil provider")
	}
}

func TestProviderFactory_ProviderFromRecipe_WithExplicitSource(t *testing.T) {
	factory := NewProviderFactory()
	resolver := New()

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Version:  recipe.VersionSection{Source: "nodejs_dist"},
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{"url": "https://example.com/file.tar.gz"},
			},
		},
	}

	provider, err := factory.ProviderFromRecipe(resolver, r)
	if err != nil {
		t.Fatalf("ProviderFromRecipe() error = %v", err)
	}

	if provider == nil {
		t.Fatal("ProviderFromRecipe() returned nil provider")
	}

	// Verify it's the custom provider
	customProvider, ok := provider.(*CustomProvider)
	if !ok {
		t.Errorf("ProviderFromRecipe() returned %T, want *CustomProvider", provider)
	}
	if customProvider != nil && customProvider.SourceDescription() != "custom:nodejs_dist" {
		t.Errorf("SourceDescription() = %q, want %q", customProvider.SourceDescription(), "custom:nodejs_dist")
	}
}

func TestInferredPyPIStrategy_Create_NoPackage(t *testing.T) {
	resolver := New()
	s := &InferredPyPIStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Steps: []recipe.Step{
			{
				Action: "pipx_install",
				Params: map[string]interface{}{}, // Missing package
			},
		},
	}

	_, err := s.Create(resolver, r)
	if err == nil {
		t.Error("Create() should fail when package is missing")
	}
}

func TestInferredCratesIOStrategy_Create_NoCrate(t *testing.T) {
	resolver := New()
	s := &InferredCratesIOStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Steps: []recipe.Step{
			{
				Action: "cargo_install",
				Params: map[string]interface{}{}, // Missing crate
			},
		},
	}

	_, err := s.Create(resolver, r)
	if err == nil {
		t.Error("Create() should fail when crate is missing")
	}
}

func TestInferredRubyGemsStrategy_Create_NoGem(t *testing.T) {
	resolver := New()
	s := &InferredRubyGemsStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Steps: []recipe.Step{
			{
				Action: "gem_install",
				Params: map[string]interface{}{}, // Missing gem
			},
		},
	}

	_, err := s.Create(resolver, r)
	if err == nil {
		t.Error("Create() should fail when gem is missing")
	}
}

func TestNpmSourceStrategy_Priority(t *testing.T) {
	s := &NpmSourceStrategy{}
	if s.Priority() != PriorityKnownRegistry {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityKnownRegistry)
	}
}

func TestNpmSourceStrategy_CanHandle(t *testing.T) {
	s := &NpmSourceStrategy{}

	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name: "npm source with npm_install action",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "npm"},
				Steps: []recipe.Step{
					{Action: "npm_install", Params: map[string]interface{}{"package": "prettier"}},
				},
			},
			expected: true,
		},
		{
			name: "npm source without npm_install",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "npm"},
				Steps:   []recipe.Step{},
			},
			expected: false,
		},
		{
			name: "non-npm source",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "github"},
				Steps: []recipe.Step{
					{Action: "npm_install", Params: map[string]interface{}{"package": "prettier"}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.CanHandle(tt.recipe)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNpmSourceStrategy_Create(t *testing.T) {
	resolver := New()
	s := &NpmSourceStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Version:  recipe.VersionSection{Source: "npm"},
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]interface{}{
					"package": "prettier",
				},
			},
		},
	}

	provider, err := s.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if provider == nil {
		t.Fatal("Create() returned nil provider")
	}
}

func TestNpmSourceStrategy_Create_NoPackage(t *testing.T) {
	resolver := New()
	s := &NpmSourceStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Version:  recipe.VersionSection{Source: "npm"},
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]interface{}{}, // Missing package
			},
		},
	}

	_, err := s.Create(resolver, r)
	if err == nil {
		t.Error("Create() should fail when package is missing")
	}
}

func TestInferredNpmStrategy_Create(t *testing.T) {
	resolver := New()
	s := &InferredNpmStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]interface{}{
					"package": "prettier",
				},
			},
		},
	}

	provider, err := s.Create(resolver, r)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if provider == nil {
		t.Fatal("Create() returned nil provider")
	}
}

func TestInferredNpmStrategy_Create_NoPackage(t *testing.T) {
	resolver := New()
	s := &InferredNpmStrategy{}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "test-tool"},
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]interface{}{}, // Missing package
			},
		},
	}

	_, err := s.Create(resolver, r)
	if err == nil {
		t.Error("Create() should fail when package is missing")
	}
}
