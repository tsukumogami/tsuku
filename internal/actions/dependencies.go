package actions

import (
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// GetActionDeps returns the dependencies for an action by name.
// Returns an empty ActionDeps if the action is not found.
func GetActionDeps(actionName string) ActionDeps {
	act := Get(actionName)
	if act == nil {
		return ActionDeps{}
	}
	return act.Dependencies()
}

// ShadowedDep represents a dependency that is declared explicitly but already
// inherited from primitive actions.
type ShadowedDep struct {
	Name   string // Dependency name
	Source string // Action that provides this dependency
}

// DetectShadowedDeps compares recipe-level and step-level declared dependencies
// with dependencies automatically inherited from primitive actions.
// Returns a list of shadowed dependencies with information about where they're inherited from.
func DetectShadowedDeps(r *recipe.Recipe) []ShadowedDep {
	var shadowed []ShadowedDep

	// Build a map of all aggregated dependencies (inherited from primitives)
	aggregatedInstall := make(map[string]string) // dep name -> source action
	aggregatedRuntime := make(map[string]string)

	for _, step := range r.Steps {
		// Get aggregated deps from this step's action
		aggregated := aggregatePrimitiveDeps(step.Action, step.Params)

		// Track which action provides each dep
		for _, dep := range aggregated.InstallTime {
			if _, exists := aggregatedInstall[dep]; !exists {
				aggregatedInstall[dep] = step.Action
			}
		}
		for _, dep := range aggregated.Runtime {
			if _, exists := aggregatedRuntime[dep]; !exists {
				aggregatedRuntime[dep] = step.Action
			}
		}
	}

	// Check recipe-level dependencies for shadowing
	for _, declaredDep := range r.Metadata.Dependencies {
		name, _ := parseDependency(declaredDep)
		if sourceAction, exists := aggregatedInstall[name]; exists {
			shadowed = append(shadowed, ShadowedDep{
				Name:   name,
				Source: sourceAction,
			})
		}
	}

	for _, declaredDep := range r.Metadata.RuntimeDependencies {
		name, _ := parseDependency(declaredDep)
		if sourceAction, exists := aggregatedRuntime[name]; exists {
			shadowed = append(shadowed, ShadowedDep{
				Name:   name,
				Source: sourceAction,
			})
		}
	}

	// Check recipe-level extra dependencies for shadowing
	for _, declaredDep := range r.Metadata.ExtraDependencies {
		name, _ := parseDependency(declaredDep)
		if sourceAction, exists := aggregatedInstall[name]; exists {
			shadowed = append(shadowed, ShadowedDep{
				Name:   name,
				Source: sourceAction,
			})
		}
	}

	for _, declaredDep := range r.Metadata.ExtraRuntimeDependencies {
		name, _ := parseDependency(declaredDep)
		if sourceAction, exists := aggregatedRuntime[name]; exists {
			shadowed = append(shadowed, ShadowedDep{
				Name:   name,
				Source: sourceAction,
			})
		}
	}

	// Check step-level dependencies for shadowing
	for _, step := range r.Steps {
		// Get aggregated deps from THIS step's action
		stepAggregated := aggregatePrimitiveDeps(step.Action, step.Params)
		stepAggregatedInstall := make(map[string]bool)
		stepAggregatedRuntime := make(map[string]bool)

		for _, dep := range stepAggregated.InstallTime {
			stepAggregatedInstall[dep] = true
		}
		for _, dep := range stepAggregated.Runtime {
			stepAggregatedRuntime[dep] = true
		}

		// Check step-level install dependencies
		if stepDeps := getStringSliceParam(step.Params, "dependencies"); stepDeps != nil {
			for _, declaredDep := range stepDeps {
				name, _ := parseDependency(declaredDep)
				if stepAggregatedInstall[name] {
					shadowed = append(shadowed, ShadowedDep{
						Name:   name,
						Source: step.Action,
					})
				}
			}
		}

		// Check step-level extra dependencies
		if extraDeps := getStringSliceParam(step.Params, "extra_dependencies"); extraDeps != nil {
			for _, declaredDep := range extraDeps {
				name, _ := parseDependency(declaredDep)
				if stepAggregatedInstall[name] {
					shadowed = append(shadowed, ShadowedDep{
						Name:   name,
						Source: step.Action,
					})
				}
			}
		}

		// Check step-level runtime dependencies
		if stepRuntimeDeps := getStringSliceParam(step.Params, "runtime_dependencies"); stepRuntimeDeps != nil {
			for _, declaredDep := range stepRuntimeDeps {
				name, _ := parseDependency(declaredDep)
				if stepAggregatedRuntime[name] {
					shadowed = append(shadowed, ShadowedDep{
						Name:   name,
						Source: step.Action,
					})
				}
			}
		}

		// Check step-level extra runtime dependencies
		if extraRuntimeDeps := getStringSliceParam(step.Params, "extra_runtime_dependencies"); extraRuntimeDeps != nil {
			for _, declaredDep := range extraRuntimeDeps {
				name, _ := parseDependency(declaredDep)
				if stepAggregatedRuntime[name] {
					shadowed = append(shadowed, ShadowedDep{
						Name:   name,
						Source: step.Action,
					})
				}
			}
		}
	}

	return shadowed
}
