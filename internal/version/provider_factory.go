package version

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/tsukumogami/tsuku/internal/recipe"
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
	f.Register(&NpmSourceStrategy{})         // PriorityKnownRegistry (100) - intercepts source="npm"
	f.Register(&NixpkgsSourceStrategy{})     // PriorityKnownRegistry (100) - intercepts source="nixpkgs"
	f.Register(&GoToolchainSourceStrategy{}) // PriorityKnownRegistry (100) - intercepts source="go_toolchain"
	f.Register(&GoProxySourceStrategy{})     // PriorityKnownRegistry (100) - intercepts source="goproxy"
	f.Register(&MetaCPANSourceStrategy{})    // PriorityKnownRegistry (100) - intercepts source="metacpan"
	f.Register(&HomebrewSourceStrategy{})    // PriorityKnownRegistry (100) - intercepts source="homebrew"
	f.Register(&GitHubRepoStrategy{})        // PriorityExplicitHint (90)
	f.Register(&ExplicitSourceStrategy{})    // PriorityExplicitSource (80) - catch-all for custom sources
	f.Register(&InferredNpmStrategy{})       // PriorityInferred (10)
	f.Register(&InferredPyPIStrategy{})      // PriorityInferred (10)
	f.Register(&InferredCratesIOStrategy{})  // PriorityInferred (10)
	f.Register(&InferredRubyGemsStrategy{})  // PriorityInferred (10)
	f.Register(&InferredMetaCPANStrategy{})  // PriorityInferred (10)
	f.Register(&InferredGitHubStrategy{})    // PriorityInferred (10)
	f.Register(&InferredGoProxyStrategy{})   // PriorityInferred (10)
	f.Register(&InferredFossilStrategy{})    // PriorityInferred (10)
	f.Register(&InferredHomebrewStrategy{})  // PriorityInferred (10)

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

// NpmSourceStrategy handles recipes with [version] source = "npm"
// This intercepts source="npm" to use NpmProvider instead of generic CustomProvider
type NpmSourceStrategy struct{}

func (s *NpmSourceStrategy) Priority() int { return PriorityKnownRegistry }

func (s *NpmSourceStrategy) CanHandle(r *recipe.Recipe) bool {
	if r.Version.Source != "npm" {
		return false
	}
	// Must have npm_install action with package name
	for _, step := range r.Steps {
		if step.Action == "npm_install" {
			if _, ok := step.Params["package"].(string); ok {
				return true
			}
		}
	}
	return false
}

func (s *NpmSourceStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, step := range r.Steps {
		if step.Action == "npm_install" {
			if pkg, ok := step.Params["package"].(string); ok {
				return NewNpmProvider(resolver, pkg), nil
			}
		}
	}
	return nil, fmt.Errorf("no npm package found in npm_install steps")
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

// GoToolchainSourceStrategy handles recipes with [version] source = "go_toolchain"
// This intercepts source="go_toolchain" to use GoToolchainProvider for Go toolchain versioning
type GoToolchainSourceStrategy struct{}

func (s *GoToolchainSourceStrategy) Priority() int { return PriorityKnownRegistry }

func (s *GoToolchainSourceStrategy) CanHandle(r *recipe.Recipe) bool {
	return r.Version.Source == "go_toolchain"
}

func (s *GoToolchainSourceStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	return NewGoToolchainProvider(resolver), nil
}

// GoProxySourceStrategy handles recipes with [version] source = "goproxy"
// This intercepts source="goproxy" to use GoProxyProvider for Go module versioning
type GoProxySourceStrategy struct{}

func (s *GoProxySourceStrategy) Priority() int { return PriorityKnownRegistry }

func (s *GoProxySourceStrategy) CanHandle(r *recipe.Recipe) bool {
	if r.Version.Source != "goproxy" {
		return false
	}
	// Must have go_install action with module path
	for _, step := range r.Steps {
		if step.Action == "go_install" {
			if _, ok := step.Params["module"].(string); ok {
				return true
			}
		}
	}
	return false
}

func (s *GoProxySourceStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	// First, check [version] section for explicit module path
	// This allows recipes to specify a different module for version resolution
	// than the install path (e.g., honnef.co/go/tools vs honnef.co/go/tools/cmd/staticcheck)
	if r.Version.Module != "" {
		return NewGoProxyProvider(resolver, r.Version.Module), nil
	}

	// Fall back to module from go_install step params
	for _, step := range r.Steps {
		if step.Action == "go_install" {
			if module, ok := step.Params["module"].(string); ok {
				return NewGoProxyProvider(resolver, module), nil
			}
		}
	}
	return nil, fmt.Errorf("no Go module found in go_install steps")
}

// MetaCPANSourceStrategy handles recipes with [version] source = "metacpan"
// This intercepts source="metacpan" to use MetaCPANProvider instead of generic CustomProvider
type MetaCPANSourceStrategy struct{}

func (s *MetaCPANSourceStrategy) Priority() int { return PriorityKnownRegistry }

func (s *MetaCPANSourceStrategy) CanHandle(r *recipe.Recipe) bool {
	if r.Version.Source != "metacpan" {
		return false
	}
	// Must have cpan_install action with distribution name
	for _, step := range r.Steps {
		if step.Action == "cpan_install" {
			if _, ok := step.Params["distribution"].(string); ok {
				return true
			}
		}
	}
	return false
}

func (s *MetaCPANSourceStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, step := range r.Steps {
		if step.Action == "cpan_install" {
			if dist, ok := step.Params["distribution"].(string); ok {
				return NewMetaCPANProvider(resolver, dist), nil
			}
		}
	}
	return nil, fmt.Errorf("no distribution found in cpan_install steps")
}

// InferredMetaCPANStrategy infers MetaCPAN from cpan_install action
type InferredMetaCPANStrategy struct{}

func (s *InferredMetaCPANStrategy) Priority() int { return PriorityInferred } // Low priority

func (s *InferredMetaCPANStrategy) CanHandle(r *recipe.Recipe) bool {
	for _, step := range r.Steps {
		if step.Action == "cpan_install" {
			if _, ok := step.Params["distribution"].(string); ok {
				return true
			}
		}
	}
	return false
}

func (s *InferredMetaCPANStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, step := range r.Steps {
		if step.Action == "cpan_install" {
			if dist, ok := step.Params["distribution"].(string); ok {
				return NewMetaCPANProvider(resolver, dist), nil
			}
		}
	}
	return nil, fmt.Errorf("no distribution found in cpan_install steps")
}

// HomebrewSourceStrategy handles recipes with [version] source = "homebrew"
// This intercepts source="homebrew" to use HomebrewProvider for library version resolution
type HomebrewSourceStrategy struct{}

func (s *HomebrewSourceStrategy) Priority() int { return PriorityKnownRegistry }

func (s *HomebrewSourceStrategy) CanHandle(r *recipe.Recipe) bool {
	if r.Version.Source != "homebrew" {
		return false
	}
	// Must have formula specified in version section
	return r.Version.Formula != ""
}

func (s *HomebrewSourceStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	if r.Version.Formula == "" {
		return nil, fmt.Errorf("no formula specified for Homebrew version source")
	}
	return NewHomebrewProvider(resolver, r.Version.Formula), nil
}

// InferredGoProxyStrategy infers goproxy from go_install action
type InferredGoProxyStrategy struct{}

func (s *InferredGoProxyStrategy) Priority() int { return PriorityInferred }

func (s *InferredGoProxyStrategy) CanHandle(r *recipe.Recipe) bool {
	for _, step := range r.Steps {
		if step.Action == "go_install" {
			if _, ok := step.Params["module"].(string); ok {
				return true
			}
		}
	}
	return false
}

func (s *InferredGoProxyStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	// Use Recipe.Version.Module if explicitly set (for edge cases where
	// pattern-based inference doesn't work)
	if r.Version.Module != "" {
		return NewGoProxyProvider(resolver, r.Version.Module), nil
	}

	// Get module from go_install step and try pattern-based inference
	for _, step := range r.Steps {
		if step.Action == "go_install" {
			if installPath, ok := step.Params["module"].(string); ok {
				versionModule := InferGoVersionModule(installPath)
				return NewGoProxyProvider(resolver, versionModule), nil
			}
		}
	}
	return nil, fmt.Errorf("no Go module found in go_install steps")
}

// InferGoVersionModule extracts the version module path from a Go install path.
// It handles two common patterns where the install path differs from the version module:
//
// Pattern 1 (GitHub): github.com/<owner>/<repo>/deeper/path
//
//	→ Returns github.com/<owner>/<repo>
//
// Pattern 2 (/cmd/ convention): some.url/path/cmd/tool
//
//	→ Returns some.url/path
//
// If no pattern matches, returns the install path unchanged.
func InferGoVersionModule(installPath string) string {
	// Pattern 1: GitHub repos - github.com/<owner>/<repo>[/...]
	if len(installPath) > 11 && installPath[:11] == "github.com/" {
		// Find the third slash (after owner/repo)
		slashCount := 0
		for i := 0; i < len(installPath); i++ {
			if installPath[i] == '/' {
				slashCount++
				if slashCount == 3 {
					return installPath[:i]
				}
			}
		}
		// No third slash means it's already github.com/owner/repo
		return installPath
	}

	// Pattern 2: /cmd/ convention - extract everything before /cmd/
	cmdIndex := -1
	for i := 0; i <= len(installPath)-5; i++ {
		if installPath[i:i+5] == "/cmd/" {
			cmdIndex = i
			break
		}
	}
	if cmdIndex > 0 {
		return installPath[:cmdIndex]
	}

	// No pattern matched - use install path as-is
	return installPath
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

// InferredFossilStrategy infers Fossil provider from fossil_archive action
type InferredFossilStrategy struct{}

func (s *InferredFossilStrategy) Priority() int { return PriorityInferred }

func (s *InferredFossilStrategy) CanHandle(r *recipe.Recipe) bool {
	for _, step := range r.Steps {
		if step.Action == "fossil_archive" {
			if _, ok := step.Params["repo"].(string); ok {
				if _, ok := step.Params["project_name"].(string); ok {
					return true
				}
			}
		}
	}
	return false
}

func (s *InferredFossilStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, step := range r.Steps {
		if step.Action == "fossil_archive" {
			repo, ok := step.Params["repo"].(string)
			if !ok {
				continue
			}
			projectName, ok := step.Params["project_name"].(string)
			if !ok {
				continue
			}

			// Get optional configuration from action params
			tagPrefix, _ := step.Params["tag_prefix"].(string)
			versionSeparator, _ := step.Params["version_separator"].(string)
			timelineTag, _ := step.Params["timeline_tag"].(string)

			return NewFossilTimelineProviderWithOptions(
				resolver, repo, projectName,
				tagPrefix, versionSeparator, timelineTag,
			), nil
		}
	}
	return nil, fmt.Errorf("no Fossil repository found in fossil_archive steps")
}

// InferredHomebrewStrategy infers homebrew from homebrew action
type InferredHomebrewStrategy struct{}

func (s *InferredHomebrewStrategy) Priority() int { return PriorityInferred }

func (s *InferredHomebrewStrategy) CanHandle(r *recipe.Recipe) bool {
	for _, step := range r.Steps {
		if step.Action == "homebrew" {
			return true
		}
	}
	return false
}

func (s *InferredHomebrewStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	for _, step := range r.Steps {
		if step.Action == "homebrew" {
			// Use formula from step params if specified, otherwise use recipe name
			formula, ok := step.Params["formula"].(string)
			if !ok || formula == "" {
				formula = r.Metadata.Name
			}
			return NewHomebrewProvider(resolver, formula), nil
		}
	}
	return nil, fmt.Errorf("no homebrew action found in steps")
}
