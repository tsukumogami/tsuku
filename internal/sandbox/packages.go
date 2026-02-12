package sandbox

import (
	"github.com/tsukumogami/tsuku/internal/executor"
)

// SystemRequirements is an alias to executor.SystemRequirements for backward compatibility.
// Use executor.SystemRequirements directly for new code.
type SystemRequirements = executor.SystemRequirements

// RepositoryConfig is an alias to executor.RepositoryConfig for backward compatibility.
// Use executor.RepositoryConfig directly for new code.
type RepositoryConfig = executor.RepositoryConfig

// ExtractSystemRequirements collects all system-level dependencies from a filtered plan.
// The plan is already filtered for the target platform, so steps contain only
// the actions needed for that platform.
//
// Returns a SystemRequirements struct containing both packages and repository configurations.
// Returns nil if the plan has no system dependency actions.
func ExtractSystemRequirements(plan *executor.InstallationPlan) *SystemRequirements {
	return executor.ExtractSystemRequirementsFromPlan(plan)
}

// ExtractPackages collects all package requirements from a filtered plan.
// Deprecated: Use ExtractSystemRequirements instead, which also extracts repository configurations.
//
// This function is kept for backward compatibility and wraps ExtractSystemRequirements.
func ExtractPackages(plan *executor.InstallationPlan) map[string][]string {
	reqs := ExtractSystemRequirements(plan)
	if reqs == nil {
		return nil
	}
	return reqs.Packages
}
