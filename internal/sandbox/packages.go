package sandbox

import (
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/executor"
)

// ExtractPackages collects all package requirements from a filtered plan.
// The plan is already filtered for the target platform, so steps contain only
// the actions needed for that platform.
//
// Returns a map where keys are package manager names ("apt", "brew", "dnf", etc.)
// and values are lists of package names for that manager.
//
// Returns nil if the plan has no system dependency actions. This allows callers
// to distinguish between "no packages needed" (nil) and "empty package list" (empty map).
func ExtractPackages(plan *executor.InstallationPlan) map[string][]string {
	if plan == nil {
		return nil
	}

	packages := make(map[string][]string)
	hasSystemDeps := false

	for _, step := range plan.Steps {
		// Get the package list from step params
		pkgs, ok := actions.GetStringSlice(step.Params, "packages")
		if !ok {
			pkgs = nil // No packages in this step
		}

		switch step.Action {
		case "apt_install":
			hasSystemDeps = true
			packages["apt"] = append(packages["apt"], pkgs...)
		case "apt_repo", "apt_ppa":
			// apt_repo and apt_ppa don't install packages directly,
			// but signal that apt will be used for subsequent installs
			hasSystemDeps = true
		case "brew_install", "brew_cask":
			hasSystemDeps = true
			packages["brew"] = append(packages["brew"], pkgs...)
		case "dnf_install":
			hasSystemDeps = true
			packages["dnf"] = append(packages["dnf"], pkgs...)
		case "dnf_repo":
			// dnf_repo doesn't install packages directly
			hasSystemDeps = true
		case "pacman_install":
			hasSystemDeps = true
			packages["pacman"] = append(packages["pacman"], pkgs...)
		case "apk_install":
			hasSystemDeps = true
			packages["apk"] = append(packages["apk"], pkgs...)
		case "zypper_install":
			hasSystemDeps = true
			packages["zypper"] = append(packages["zypper"], pkgs...)
		}
	}

	if !hasSystemDeps {
		return nil // No system dependencies - use default container
	}

	return packages
}
