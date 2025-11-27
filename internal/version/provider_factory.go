package version

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/tsuku-dev/tsuku/internal/recipe"
)

// Priority levels for strategy evaluation (higher = evaluated first)
// These constants document the rationale for priority ordering and make the system self-documenting.
const (
	// PriorityKnownRegistry: Known package registries (pypi, npm) that need special handling
	// These have highest priority because they match specific source values and need registry-specific APIs
	// Examples: source="pypi" → PyPIProvider (not CustomProvider), source="npm" → NpmProvider
	PriorityKnownRegistry = 100

	// PriorityExplicitHint: Explicit hint fields like github_repo, pypi_package
	// These are explicit configuration but lower priority than known registries
	PriorityExplicitHint = 90

	// PriorityExplicitSource: User specified [version] source = "..." (generic custom sources)
	// Lower than known registries because it's a catch-all for any source value
	// Falls back to CustomProvider that looks up the source name in the registry
	PriorityExplicitSource = 80

	// PriorityInferred: Inferred from installation actions (backward compatibility)
	// Lowest priority to allow all explicit configuration to override
	PriorityInferred = 10
)

// ProviderStrategy defines how to create a provider from a recipe.
// This enables the Open/Closed Principle: adding PyPI, cargo, etc. doesn't require
// modifying ProviderFromRecipe().
type ProviderStrategy interface {
	// CanHandle returns true if this strategy can create a provider for the recipe
	CanHandle(r *recipe.Recipe) bool

	// Create creates the provider
	Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error)

	// Priority determines evaluation order (higher = evaluated first)
	Priority() int
}

// ProviderFactory manages provider strategies
type ProviderFactory struct {
	strategies []ProviderStrategy
}

// NewProviderFactory creates a factory with default strategies
func NewProviderFactory() *ProviderFactory {
	f := &ProviderFactory{}

	// Register strategies (priority order determined by Priority() methods)
	f.Register(&PyPISourceStrategy{})        // PriorityKnownRegistry (100) - intercepts source="pypi"
	f.Register(&CratesIOSourceStrategy{})    // PriorityKnownRegistry (100) - intercepts source="crates_io"
	f.Register(&RubyGemsSourceStrategy{})    // PriorityKnownRegistry (100) - intercepts source="rubygems"
	f.Register(&NixpkgsSourceStrategy{})     // PriorityKnownRegistry (100) - intercepts source="nixpkgs"
	f.Register(&GitHubRepoStrategy{})        // PriorityExplicitHint (90)
	f.Register(&ExplicitSourceStrategy{})    // PriorityExplicitSource (80) - catch-all for custom sources
	f.Register(&InferredNpmStrategy{})       // PriorityInferred (10)
	f.Register(&InferredPyPIStrategy{})      // PriorityInferred (10)
	f.Register(&InferredCratesIOStrategy{})  // PriorityInferred (10)
	f.Register(&InferredRubyGemsStrategy{})  // PriorityInferred (10)
	f.Register(&InferredGitHubStrategy{})    // PriorityInferred (10)

	return f
}

// Register adds a new strategy
func (f *ProviderFactory) Register(strategy ProviderStrategy) {
	f.strategies = append(f.strategies, strategy)

	// Sort by priority (descending)
	sort.Slice(f.strategies, func(i, j int) bool {
		return f.strategies[i].Priority() > f.strategies[j].Priority()
	})
}

// ProviderFromRecipe creates the appropriate VersionProvider for a recipe.
// This is the SINGLE SOURCE OF TRUTH for version resolution routing.
func (f *ProviderFactory) ProviderFromRecipe(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, strategy := range f.strategies {
		if strategy.CanHandle(r) {
			return strategy.Create(resolver, r)
		}
	}

	return nil, fmt.Errorf("no version source configured for recipe %s (add [version] section)", r.Metadata.Name)
}

// --- Concrete Strategies ---

// ExplicitSourceStrategy handles recipes with explicit [version] source = "..."
type ExplicitSourceStrategy struct{}

func (s *ExplicitSourceStrategy) Priority() int { return PriorityExplicitSource }

func (s *ExplicitSourceStrategy) CanHandle(r *recipe.Recipe) bool {
	return r.Version.Source != ""
}

func (s *ExplicitSourceStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	// Validate source name (prevent injection)
	if !isValidSourceName(r.Version.Source) {
		return nil, fmt.Errorf("invalid source name: %s", r.Version.Source)
	}

	return NewCustomProvider(resolver, r.Version.Source), nil
}

// PyPISourceStrategy handles recipes with [version] source = "pypi"
// This intercepts source="pypi" to use PyPIProvider instead of generic CustomProvider
type PyPISourceStrategy struct{}

func (s *PyPISourceStrategy) Priority() int { return PriorityKnownRegistry }

func (s *PyPISourceStrategy) CanHandle(r *recipe.Recipe) bool {
	if r.Version.Source != "pypi" {
		return false
	}
	// Must have pipx_install action with package name
	for _, step := range r.Steps {
		if step.Action == "pipx_install" {
			if _, ok := step.Params["package"].(string); ok {
				return true
			}
		}
	}
	return false
}

func (s *PyPISourceStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, step := range r.Steps {
		if step.Action == "pipx_install" {
			if pkg, ok := step.Params["package"].(string); ok {
				return NewPyPIProvider(resolver, pkg), nil
			}
		}
	}
	return nil, fmt.Errorf("no PyPI package found in pipx_install steps")
}

// GitHubRepoStrategy handles recipes with [version] github_repo = "..."
type GitHubRepoStrategy struct{}

func (s *GitHubRepoStrategy) Priority() int { return PriorityExplicitHint }

func (s *GitHubRepoStrategy) CanHandle(r *recipe.Recipe) bool {
	return r.Version.GitHubRepo != ""
}

func (s *GitHubRepoStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	if r.Version.TagPrefix != "" {
		return NewGitHubProviderWithPrefix(resolver, r.Version.GitHubRepo, r.Version.TagPrefix), nil
	}
	return NewGitHubProvider(resolver, r.Version.GitHubRepo), nil
}

// InferredGitHubStrategy infers GitHub from github_archive/github_file actions (DEPRECATED)
type InferredGitHubStrategy struct{}

func (s *InferredGitHubStrategy) Priority() int { return PriorityInferred } // Low priority

func (s *InferredGitHubStrategy) CanHandle(r *recipe.Recipe) bool {
	for _, step := range r.Steps {
		if step.Action == "github_archive" || step.Action == "github_file" {
			if _, ok := step.Params["repo"].(string); ok {
				return true
			}
		}
	}
	return false
}

func (s *InferredGitHubStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, step := range r.Steps {
		if step.Action == "github_archive" || step.Action == "github_file" {
			if repo, ok := step.Params["repo"].(string); ok {
				return NewGitHubProvider(resolver, repo), nil
			}
		}
	}
	return nil, fmt.Errorf("no GitHub repo found in steps")
}

// InferredNpmStrategy infers npm from npm_install action (DEPRECATED)
type InferredNpmStrategy struct{}

func (s *InferredNpmStrategy) Priority() int { return PriorityInferred }

func (s *InferredNpmStrategy) CanHandle(r *recipe.Recipe) bool {
	for _, step := range r.Steps {
		if step.Action == "npm_install" {
			if _, ok := step.Params["package"].(string); ok {
				return true
			}
		}
	}
	return false
}

func (s *InferredNpmStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, step := range r.Steps {
		if step.Action == "npm_install" {
			if pkg, ok := step.Params["package"].(string); ok {
				return NewNpmProvider(resolver, pkg), nil
			}
		}
	}
	return nil, fmt.Errorf("no npm package found in steps")
}

// InferredPyPIStrategy infers PyPI from pipx_install action
type InferredPyPIStrategy struct{}

func (s *InferredPyPIStrategy) Priority() int { return PriorityInferred } // Low priority

func (s *InferredPyPIStrategy) CanHandle(r *recipe.Recipe) bool {
	for _, step := range r.Steps {
		if step.Action == "pipx_install" {
			if _, ok := step.Params["package"].(string); ok {
				return true
			}
		}
	}
	return false
}

func (s *InferredPyPIStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, step := range r.Steps {
		if step.Action == "pipx_install" {
			if pkg, ok := step.Params["package"].(string); ok {
				return NewPyPIProvider(resolver, pkg), nil
			}
		}
	}
	return nil, fmt.Errorf("no PyPI package found in pipx_install steps")
}

// CratesIOSourceStrategy handles recipes with [version] source = "crates_io"
// This intercepts source="crates_io" to use CratesIOProvider instead of generic CustomProvider
type CratesIOSourceStrategy struct{}

func (s *CratesIOSourceStrategy) Priority() int { return PriorityKnownRegistry }

func (s *CratesIOSourceStrategy) CanHandle(r *recipe.Recipe) bool {
	if r.Version.Source != "crates_io" {
		return false
	}
	// Must have cargo_install action with crate name
	for _, step := range r.Steps {
		if step.Action == "cargo_install" {
			if _, ok := step.Params["crate"].(string); ok {
				return true
			}
		}
	}
	return false
}

func (s *CratesIOSourceStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, step := range r.Steps {
		if step.Action == "cargo_install" {
			if crate, ok := step.Params["crate"].(string); ok {
				return NewCratesIOProvider(resolver, crate), nil
			}
		}
	}
	return nil, fmt.Errorf("no crate found in cargo_install steps")
}

// InferredCratesIOStrategy infers crates.io from cargo_install action
type InferredCratesIOStrategy struct{}

func (s *InferredCratesIOStrategy) Priority() int { return PriorityInferred } // Low priority

func (s *InferredCratesIOStrategy) CanHandle(r *recipe.Recipe) bool {
	for _, step := range r.Steps {
		if step.Action == "cargo_install" {
			if _, ok := step.Params["crate"].(string); ok {
				return true
			}
		}
	}
	return false
}

func (s *InferredCratesIOStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, step := range r.Steps {
		if step.Action == "cargo_install" {
			if crate, ok := step.Params["crate"].(string); ok {
				return NewCratesIOProvider(resolver, crate), nil
			}
		}
	}
	return nil, fmt.Errorf("no crate found in cargo_install steps")
}

// RubyGemsSourceStrategy handles recipes with [version] source = "rubygems"
// This intercepts source="rubygems" to use RubyGemsProvider instead of generic CustomProvider
type RubyGemsSourceStrategy struct{}

func (s *RubyGemsSourceStrategy) Priority() int { return PriorityKnownRegistry }

func (s *RubyGemsSourceStrategy) CanHandle(r *recipe.Recipe) bool {
	if r.Version.Source != "rubygems" {
		return false
	}
	// Must have gem_install action with gem name
	for _, step := range r.Steps {
		if step.Action == "gem_install" {
			if _, ok := step.Params["gem"].(string); ok {
				return true
			}
		}
	}
	return false
}

func (s *RubyGemsSourceStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, step := range r.Steps {
		if step.Action == "gem_install" {
			if gem, ok := step.Params["gem"].(string); ok {
				return NewRubyGemsProvider(resolver, gem), nil
			}
		}
	}
	return nil, fmt.Errorf("no gem found in gem_install steps")
}

// InferredRubyGemsStrategy infers RubyGems from gem_install action
type InferredRubyGemsStrategy struct{}

func (s *InferredRubyGemsStrategy) Priority() int { return PriorityInferred } // Low priority

func (s *InferredRubyGemsStrategy) CanHandle(r *recipe.Recipe) bool {
	for _, step := range r.Steps {
		if step.Action == "gem_install" {
			if _, ok := step.Params["gem"].(string); ok {
				return true
			}
		}
	}
	return false
}

func (s *InferredRubyGemsStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, step := range r.Steps {
		if step.Action == "gem_install" {
			if gem, ok := step.Params["gem"].(string); ok {
				return NewRubyGemsProvider(resolver, gem), nil
			}
		}
	}
	return nil, fmt.Errorf("no gem found in gem_install steps")
}

// NixpkgsSourceStrategy handles recipes with [version] source = "nixpkgs"
// This intercepts source="nixpkgs" to use NixpkgsProvider for channel-based versioning
type NixpkgsSourceStrategy struct{}

func (s *NixpkgsSourceStrategy) Priority() int { return PriorityKnownRegistry }

func (s *NixpkgsSourceStrategy) CanHandle(r *recipe.Recipe) bool {
	return r.Version.Source == "nixpkgs"
}

func (s *NixpkgsSourceStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	return NewNixpkgsProvider(resolver), nil
}

// Pre-compile regex for performance (avoid compiling on every call)
var sourceNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// isValidSourceName validates custom source names to prevent injection
func isValidSourceName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	// Allow alphanumeric, hyphens, underscores only
	return sourceNameRegex.MatchString(name)
}
