package version

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestFactoryValidator_CanResolveVersion_WithGitHubRepo(t *testing.T) {
	factory := NewProviderFactory()
	validator := NewFactoryValidator(factory)

	r := &recipe.Recipe{
		Version: recipe.VersionSection{
			GitHubRepo: "owner/repo",
		},
	}

	if !validator.CanResolveVersion(r) {
		t.Error("expected CanResolveVersion to return true for recipe with github_repo")
	}
}

func TestFactoryValidator_CanResolveVersion_WithExplicitSource(t *testing.T) {
	factory := NewProviderFactory()
	validator := NewFactoryValidator(factory)

	r := &recipe.Recipe{
		Version: recipe.VersionSection{
			Source: "manual",
		},
	}

	if !validator.CanResolveVersion(r) {
		t.Error("expected CanResolveVersion to return true for recipe with explicit source")
	}
}

func TestFactoryValidator_CanResolveVersion_WithInferredPyPI(t *testing.T) {
	factory := NewProviderFactory()
	validator := NewFactoryValidator(factory)

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "pipx_install",
				Params: map[string]interface{}{"package": "ruff"},
			},
		},
	}

	if !validator.CanResolveVersion(r) {
		t.Error("expected CanResolveVersion to return true for recipe with pipx_install")
	}
}

func TestFactoryValidator_CanResolveVersion_NoSource(t *testing.T) {
	factory := NewProviderFactory()
	validator := NewFactoryValidator(factory)

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{"url": "http://example.com"},
			},
		},
	}

	if validator.CanResolveVersion(r) {
		t.Error("expected CanResolveVersion to return false for recipe without version source")
	}
}

func TestFactoryValidator_ValidateVersionConfig_Valid(t *testing.T) {
	factory := NewProviderFactory()
	validator := NewFactoryValidator(factory)

	r := &recipe.Recipe{
		Version: recipe.VersionSection{
			GitHubRepo: "owner/repo",
		},
	}

	err := validator.ValidateVersionConfig(r)
	if err != nil {
		t.Errorf("expected no error for valid config, got: %v", err)
	}
}

func TestFactoryValidator_ValidateVersionConfig_NoSource(t *testing.T) {
	factory := NewProviderFactory()
	validator := NewFactoryValidator(factory)

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{"url": "http://example.com"},
			},
		},
	}

	err := validator.ValidateVersionConfig(r)
	if err == nil {
		t.Error("expected error for recipe without version source")
	}
}

func TestFactoryValidator_ValidateVersionConfig_RequireSystem(t *testing.T) {
	factory := NewProviderFactory()
	validator := NewFactoryValidator(factory)

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "require_system",
				Params: map[string]interface{}{"command": "docker"},
			},
		},
	}

	err := validator.ValidateVersionConfig(r)
	if err != nil {
		t.Errorf("expected no error for require_system recipe, got: %v", err)
	}
}

func TestFactoryValidator_ValidateVersionConfig_InferrableButMissingParams(t *testing.T) {
	factory := NewProviderFactory()
	validator := NewFactoryValidator(factory)

	// npm_install without package param
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]interface{}{},
			},
		},
	}

	err := validator.ValidateVersionConfig(r)
	if err == nil {
		t.Error("expected error for npm_install without package param")
	}
	if err.Error() != "action 'npm_install' could infer version source but may be missing required parameters" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFactoryValidator_KnownSources(t *testing.T) {
	factory := NewProviderFactory()
	validator := NewFactoryValidator(factory)

	sources := validator.KnownSources()

	// Should include common sources
	expected := map[string]bool{
		"pypi":         true,
		"npm":          true,
		"crates_io":    true,
		"rubygems":     true,
		"homebrew":     true,
		"go_toolchain": true,
	}

	sourceMap := make(map[string]bool)
	for _, s := range sources {
		sourceMap[s] = true
	}

	for exp := range expected {
		if !sourceMap[exp] {
			t.Errorf("expected source '%s' in KnownSources", exp)
		}
	}
}

func TestFactoryValidator_Registration(t *testing.T) {
	// The init() function should have registered the validator
	validator := recipe.GetVersionValidator()
	if validator == nil {
		t.Error("expected version validator to be registered")
	}

	// Verify it's actually a FactoryValidator by testing behavior
	r := &recipe.Recipe{
		Version: recipe.VersionSection{
			GitHubRepo: "owner/repo",
		},
	}

	if !validator.CanResolveVersion(r) {
		t.Error("registered validator should be able to resolve github_repo")
	}
}
