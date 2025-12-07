package actions

import "github.com/tsukumogami/tsuku/internal/recipe"

// ResolvedDeps contains the resolved dependencies for a recipe.
// Keys are dependency names, values are version constraints ("latest" for implicit).
type ResolvedDeps struct {
	InstallTime map[string]string // Needed during tsuku install
	Runtime     map[string]string // Needed when tool runs
}

// ResolveDependencies collects dependencies from a recipe by examining
// each step's action and merging with step-level extensions.
//
// The resolution process:
// 1. For each step, get implicit deps from ActionDependencies registry
// 2. Merge step-level extra_dependencies into install-time deps
// 3. Merge step-level extra_runtime_dependencies into runtime deps
//
// Note: Recipe-level overrides and transitive resolution are handled separately.
func ResolveDependencies(r *recipe.Recipe) ResolvedDeps {
	result := ResolvedDeps{
		InstallTime: make(map[string]string),
		Runtime:     make(map[string]string),
	}

	for _, step := range r.Steps {
		// Get implicit deps from action registry
		actionDeps := GetActionDeps(step.Action)

		// Add install-time deps from action
		for _, dep := range actionDeps.InstallTime {
			result.InstallTime[dep] = "latest"
		}

		// Add runtime deps from action
		for _, dep := range actionDeps.Runtime {
			result.Runtime[dep] = "latest"
		}

		// Merge step-level extra_dependencies
		if extraDeps := getStringSliceParam(step.Params, "extra_dependencies"); extraDeps != nil {
			for _, dep := range extraDeps {
				name, version := parseDependency(dep)
				result.InstallTime[name] = version
			}
		}

		// Merge step-level extra_runtime_dependencies
		if extraRuntimeDeps := getStringSliceParam(step.Params, "extra_runtime_dependencies"); extraRuntimeDeps != nil {
			for _, dep := range extraRuntimeDeps {
				name, version := parseDependency(dep)
				result.Runtime[name] = version
			}
		}
	}

	return result
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
