package version

import (
	"fmt"
	"strings"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// FactoryValidator implements recipe.VersionValidator using the provider factory.
// This enables the recipe package to validate version configuration without
// maintaining duplicate lists of known sources.
type FactoryValidator struct {
	factory *ProviderFactory
}

// NewFactoryValidator creates a validator backed by the given factory.
func NewFactoryValidator(factory *ProviderFactory) *FactoryValidator {
	return &FactoryValidator{factory: factory}
}

// CanResolveVersion returns true if a version provider can be created for this recipe.
func (v *FactoryValidator) CanResolveVersion(r *recipe.Recipe) bool {
	for _, strategy := range v.factory.strategies {
		if strategy.CanHandle(r) {
			return true
		}
	}
	return false
}

// KnownSources returns the list of known version source values.
// These are the sources that have explicit strategies in the factory.
func (v *FactoryValidator) KnownSources() []string {
	return []string{
		"github_releases",
		"github_tags",
		"pypi",
		"crates_io",
		"npm",
		"rubygems",
		"nixpkgs",
		"go_toolchain",
		"goproxy",
		"metacpan",
		"homebrew",
		"hashicorp",
		"nodejs_dist",
		"manual",
	}
}

// ValidateVersionConfig performs detailed validation of version configuration.
// It checks if the recipe has a valid version source configuration.
func (v *FactoryValidator) ValidateVersionConfig(r *recipe.Recipe) error {
	// Check if any strategy can handle this recipe
	if v.CanResolveVersion(r) {
		return nil
	}

	// No strategy matched - provide helpful error message
	if r.Version.Source == "" {
		// Check if there's a potential inferrable action
		for _, step := range r.Steps {
			switch step.Action {
			case "npm_install", "pipx_install", "cargo_install", "gem_install",
				"cpan_install", "go_install", "github_archive", "github_file":
				// There's an action that could infer version, but it's missing required params
				return fmt.Errorf("action '%s' could infer version source but may be missing required parameters", step.Action)
			case "require_system":
				// require_system doesn't need version source - it detects version directly
				return nil
			}
		}
		return fmt.Errorf("no version source configured (add [version] section with source field or github_repo)")
	}

	// There's a source but it didn't match any strategy
	source := r.Version.Source
	if idx := strings.Index(source, ":"); idx != -1 {
		source = source[:idx]
	}

	if !isValidSourceName(source) {
		return fmt.Errorf("invalid version source name: %s", r.Version.Source)
	}

	// Source is valid format but not recognized - might be a custom source
	// that requires version-sources.toml in the registry
	return nil
}

// defaultFactory is the singleton factory used for registration
var defaultFactory = NewProviderFactory()

// init registers the FactoryValidator with the recipe package at startup.
// This enables the recipe package to validate version configuration without
// importing the version package (breaking the circular dependency).
func init() {
	recipe.SetVersionValidator(NewFactoryValidator(defaultFactory))
}
