package actions

import "github.com/tsukumogami/tsuku/internal/recipe"

// ResolvedDeps contains the resolved dependencies for a recipe.
// Keys are dependency names, values are version constraints ("latest" for implicit).
type ResolvedDeps struct {
	InstallTime map[string]string // Needed during tsuku install
	Runtime     map[string]string // Needed when tool runs
}

// ResolveDependencies collects dependencies from a recipe by examining
// each step's action and merging with step-level and recipe-level overrides.
//
// The resolution process follows precedence rules:
// 1. For each step:
//   - If step has "dependencies", use those (replaces action implicit)
//   - Otherwise, use action's InstallTime deps + step's extra_dependencies
//   - If step has "runtime_dependencies", use those (replaces action implicit)
//   - Otherwise, use action's Runtime deps + step's extra_runtime_dependencies
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
	result := ResolvedDeps{
		InstallTime: make(map[string]string),
		Runtime:     make(map[string]string),
	}

	// Phase 1: Collect from steps
	for _, step := range r.Steps {
		actionDeps := GetActionDeps(step.Action)

		// Install-time: step replace OR (action implicit + step extend)
		if stepDeps := getStringSliceParam(step.Params, "dependencies"); stepDeps != nil {
			// Step-level replace: use only what's declared
			for _, dep := range stepDeps {
				name, version := parseDependency(dep)
				result.InstallTime[name] = version
			}
		} else {
			// Action implicit
			for _, dep := range actionDeps.InstallTime {
				result.InstallTime[dep] = "latest"
			}
			// Step-level extend
			if extraDeps := getStringSliceParam(step.Params, "extra_dependencies"); extraDeps != nil {
				for _, dep := range extraDeps {
					name, version := parseDependency(dep)
					result.InstallTime[name] = version
				}
			}
		}

		// Runtime: step replace OR (action implicit + step extend)
		if stepRuntimeDeps := getStringSliceParam(step.Params, "runtime_dependencies"); stepRuntimeDeps != nil {
			// Step-level replace: use only what's declared
			for _, dep := range stepRuntimeDeps {
				name, version := parseDependency(dep)
				result.Runtime[name] = version
			}
		} else {
			// Action implicit
			for _, dep := range actionDeps.Runtime {
				result.Runtime[dep] = "latest"
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
