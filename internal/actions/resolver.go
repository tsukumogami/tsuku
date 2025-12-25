package actions

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// MaxTransitiveDepth is the maximum depth for transitive dependency resolution.
// This prevents infinite loops from undetected cycles and limits resolution time.
const MaxTransitiveDepth = 10

// Errors for transitive dependency resolution
var (
	ErrCyclicDependency = errors.New("cyclic dependency detected")
	ErrMaxDepthExceeded = errors.New("maximum dependency depth exceeded")
)

// RecipeLoader is an interface for loading recipes by name.
// This abstraction allows testing without a real registry.
type RecipeLoader interface {
	GetWithContext(ctx context.Context, name string) (*recipe.Recipe, error)
}

// ResolvedDeps contains the resolved dependencies for a recipe.
// Keys are dependency names, values are version constraints ("latest" for implicit).
type ResolvedDeps struct {
	InstallTime map[string]string // Needed during tsuku install
	Runtime     map[string]string // Needed when tool runs
}

// ResolveDependencies collects dependencies from a recipe by examining
// each step's action and merging with step-level and recipe-level overrides.
// Uses runtime.GOOS for platform-specific dependency resolution.
//
// The resolution process follows precedence rules:
// 1. For each step:
//   - If step has "dependencies", use those (replaces action implicit)
//   - Otherwise, use action's InstallTime deps + platform-specific deps + step's extra_dependencies
//   - If step has "runtime_dependencies", use those (replaces action implicit)
//   - Otherwise, use action's Runtime deps + platform-specific deps + step's extra_runtime_dependencies
//
// 2. Recipe-level replace (if set, overrides everything from steps):
//   - Dependencies replaces all install deps
//   - RuntimeDependencies replaces all runtime deps
//
// 3. Recipe-level extend (adds to current set):
//   - ExtraDependencies adds to install deps
//   - ExtraRuntimeDependencies adds to runtime deps
//
// Note: Transitive resolution is handled separately.
func ResolveDependencies(r *recipe.Recipe) ResolvedDeps {
	return ResolveDependenciesForPlatform(r, runtime.GOOS)
}

// ResolveDependenciesForPlatform resolves dependencies for a specific target OS.
// This allows testing platform-specific behavior without mocking runtime.GOOS.
func ResolveDependenciesForPlatform(r *recipe.Recipe, targetOS string) ResolvedDeps {
	result := ResolvedDeps{
		InstallTime: make(map[string]string),
		Runtime:     make(map[string]string),
	}

	// Phase 1: Collect from steps
	for _, step := range r.Steps {
		actionDeps := GetActionDeps(step.Action)
		// TODO(#644): Aggregate dependencies from primitive actions when step.Action is decomposable.
		// Currently only collects dependencies declared directly on the composite action.
		// Should check if action implements Decomposable, decompose it, and recursively
		// collect dependencies from all primitive actions in the decomposition tree.

		// Install-time: step replace OR (action implicit + platform-specific + step extend)
		if stepDeps := getStringSliceParam(step.Params, "dependencies"); stepDeps != nil {
			// Step-level replace: use only what's declared
			for _, dep := range stepDeps {
				name, version := parseDependency(dep)
				result.InstallTime[name] = version
			}
		} else {
			// Action implicit (cross-platform)
			for _, dep := range actionDeps.InstallTime {
				// Skip self-dependencies to prevent circular loops
				// (e.g., patchelf uses homebrew which depends on patchelf)
				if dep != r.Metadata.Name {
					result.InstallTime[dep] = "latest"
				}
			}
			// Platform-specific install deps
			for _, dep := range getPlatformInstallDeps(actionDeps, targetOS) {
				if dep != r.Metadata.Name {
					result.InstallTime[dep] = "latest"
				}
			}
			// Step-level extend
			if extraDeps := getStringSliceParam(step.Params, "extra_dependencies"); extraDeps != nil {
				for _, dep := range extraDeps {
					name, version := parseDependency(dep)
					result.InstallTime[name] = version
				}
			}
		}

		// Runtime: step replace OR (action implicit + platform-specific + step extend)
		if stepRuntimeDeps := getStringSliceParam(step.Params, "runtime_dependencies"); stepRuntimeDeps != nil {
			// Step-level replace: use only what's declared
			for _, dep := range stepRuntimeDeps {
				name, version := parseDependency(dep)
				result.Runtime[name] = version
			}
		} else {
			// Action implicit (cross-platform)
			for _, dep := range actionDeps.Runtime {
				// Skip self-dependencies to prevent circular loops
				if dep != r.Metadata.Name {
					result.Runtime[dep] = "latest"
				}
			}
			// Platform-specific runtime deps
			for _, dep := range getPlatformRuntimeDeps(actionDeps, targetOS) {
				if dep != r.Metadata.Name {
					result.Runtime[dep] = "latest"
				}
			}
			// Step-level extend
			if extraRuntimeDeps := getStringSliceParam(step.Params, "extra_runtime_dependencies"); extraRuntimeDeps != nil {
				for _, dep := range extraRuntimeDeps {
					name, version := parseDependency(dep)
					result.Runtime[name] = version
				}
			}
		}
	}

	// Phase 2: Recipe-level replace (if set, overrides everything above)
	if len(r.Metadata.Dependencies) > 0 {
		result.InstallTime = make(map[string]string) // Clear
		for _, dep := range r.Metadata.Dependencies {
			name, version := parseDependency(dep)
			result.InstallTime[name] = version
		}
	}

	if len(r.Metadata.RuntimeDependencies) > 0 {
		result.Runtime = make(map[string]string) // Clear
		for _, dep := range r.Metadata.RuntimeDependencies {
			name, version := parseDependency(dep)
			result.Runtime[name] = version
		}
	}

	// Phase 3: Recipe-level extend (adds to current set)
	for _, dep := range r.Metadata.ExtraDependencies {
		name, version := parseDependency(dep)
		result.InstallTime[name] = version
	}

	for _, dep := range r.Metadata.ExtraRuntimeDependencies {
		name, version := parseDependency(dep)
		result.Runtime[name] = version
	}

	return result
}

// getPlatformInstallDeps returns platform-specific install-time dependencies
// for the given target OS.
func getPlatformInstallDeps(deps ActionDeps, targetOS string) []string {
	switch targetOS {
	case "linux":
		return deps.LinuxInstallTime
	case "darwin":
		return deps.DarwinInstallTime
	default:
		return nil
	}
}

// getPlatformRuntimeDeps returns platform-specific runtime dependencies
// for the given target OS.
func getPlatformRuntimeDeps(deps ActionDeps, targetOS string) []string {
	switch targetOS {
	case "linux":
		return deps.LinuxRuntime
	case "darwin":
		return deps.DarwinRuntime
	default:
		return nil
	}
}

// getStringSliceParam extracts a []string from step params.
// Returns nil if the param doesn't exist or isn't a string slice.
func getStringSliceParam(params map[string]interface{}, key string) []string {
	val, ok := params[key]
	if !ok {
		return nil
	}

	// Handle []interface{} (common from TOML parsing)
	if slice, ok := val.([]interface{}); ok {
		result := make([]string, 0, len(slice))
		for _, item := range slice {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}

	// Handle []string directly
	if slice, ok := val.([]string); ok {
		return slice
	}

	return nil
}

// parseDependency parses a dependency string into name and version.
// Supports formats: "name", "name@version"
// Returns "latest" as version if no version specified.
func parseDependency(dep string) (name, version string) {
	for i := 0; i < len(dep); i++ {
		if dep[i] == '@' {
			return dep[:i], dep[i+1:]
		}
	}
	return dep, "latest"
}

// ResolveTransitive expands dependencies transitively by loading each dependency's
// recipe and resolving its dependencies recursively.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - loader: RecipeLoader to fetch dependency recipes
//   - deps: Direct dependencies to expand (from ResolveDependencies)
//   - rootName: Name of the root recipe (used for cycle detection)
//
// The function:
//   - Resolves each dependency's own dependencies recursively
//   - Detects cycles and returns ErrCyclicDependency with the cycle path
//   - Enforces MaxTransitiveDepth to prevent excessive recursion
//   - Preserves version constraints (first encountered wins)
//
// Returns the fully expanded dependencies or an error if a cycle is detected
// or max depth is exceeded.
func ResolveTransitive(
	ctx context.Context,
	loader RecipeLoader,
	deps ResolvedDeps,
	rootName string,
) (ResolvedDeps, error) {
	result := ResolvedDeps{
		InstallTime: make(map[string]string),
		Runtime:     make(map[string]string),
	}

	// Copy direct deps to result
	for name, version := range deps.InstallTime {
		result.InstallTime[name] = version
	}
	for name, version := range deps.Runtime {
		result.Runtime[name] = version
	}

	// Resolve install-time deps transitively
	installVisited := make(map[string]bool)
	if err := resolveTransitiveSet(ctx, loader, result.InstallTime, []string{rootName}, 0, installVisited, false); err != nil {
		return ResolvedDeps{}, err
	}

	// Resolve runtime deps transitively
	runtimeVisited := make(map[string]bool)
	if err := resolveTransitiveSet(ctx, loader, result.Runtime, []string{rootName}, 0, runtimeVisited, true); err != nil {
		return ResolvedDeps{}, err
	}

	return result, nil
}

// resolveTransitiveSet recursively resolves a set of dependencies.
// It modifies the deps map in place, adding transitive dependencies.
// The path slice tracks the current resolution path for cycle detection.
// The visited map tracks already-processed deps to avoid redundant work.
// The forRuntime flag indicates whether to use Runtime or InstallTime deps.
func resolveTransitiveSet(
	ctx context.Context,
	loader RecipeLoader,
	deps map[string]string,
	path []string,
	depth int,
	visited map[string]bool,
	forRuntime bool,
) error {
	if depth >= MaxTransitiveDepth {
		return fmt.Errorf("%w: exceeded depth %d at path %s",
			ErrMaxDepthExceeded, MaxTransitiveDepth, strings.Join(path, " -> "))
	}

	// Get current deps to process (snapshot to avoid modifying while iterating)
	toProcess := make([]string, 0, len(deps))
	for name := range deps {
		toProcess = append(toProcess, name)
	}

	for _, depName := range toProcess {
		// Check for cycle BEFORE checking visited (self-cycle: A -> A)
		for _, ancestor := range path {
			if ancestor == depName {
				cyclePath := append(path, depName)
				return fmt.Errorf("%w: %s", ErrCyclicDependency, strings.Join(cyclePath, " -> "))
			}
		}

		// Skip if already visited (processed in a different branch)
		if visited[depName] {
			continue
		}

		// Mark as visited
		visited[depName] = true

		// Load the dependency's recipe
		depRecipe, err := loader.GetWithContext(ctx, depName)
		if err != nil {
			// Dependency recipe not found - skip (may be a system dep or not in registry)
			continue
		}

		// Resolve the dependency's own dependencies
		depDeps := ResolveDependencies(depRecipe)

		// Select the appropriate dep set based on forRuntime flag
		var sourceDeps map[string]string
		if forRuntime {
			sourceDeps = depDeps.Runtime
		} else {
			sourceDeps = depDeps.InstallTime
		}

		// Collect new transitive deps (not yet in our deps map)
		// Also check for self-reference cycle
		newDeps := make(map[string]string)
		for transName, transVersion := range sourceDeps {
			// Check for self-reference (A depends on A)
			if transName == depName {
				cyclePath := append(path, depName, transName)
				return fmt.Errorf("%w: %s", ErrCyclicDependency, strings.Join(cyclePath, " -> "))
			}
			// Check for cycle back to an ancestor in the path
			for _, ancestor := range path {
				if transName == ancestor {
					cyclePath := append(path, depName, transName)
					return fmt.Errorf("%w: %s", ErrCyclicDependency, strings.Join(cyclePath, " -> "))
				}
			}
			if _, exists := deps[transName]; !exists {
				deps[transName] = transVersion
				newDeps[transName] = transVersion
			}
		}

		// Only recurse if there are new deps to process
		if len(newDeps) > 0 {
			newPath := append(path, depName)
			if err := resolveTransitiveSet(ctx, loader, newDeps, newPath, depth+1, visited, forRuntime); err != nil {
				return err
			}
		}
	}

	return nil
}
