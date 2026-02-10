package executor

import (
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// SystemPackageActions lists action names that install system packages.
// These actions require system-level privileges and should be extracted
// when computing system dependencies for a recipe.
var SystemPackageActions = map[string]bool{
	"apt_install":    true,
	"apk_install":    true,
	"dnf_install":    true,
	"pacman_install": true,
	"zypper_install": true,
}

// ExtractSystemPackages extracts system package names from a recipe for a target.
// It filters steps by target platform, then extracts packages from system
// dependency actions (apk_install, apt_install, etc.).
//
// Returns deduplicated package names.
func ExtractSystemPackages(r *recipe.Recipe, target platform.Target) []string {
	// Filter steps for target platform
	filtered := FilterStepsByTarget(r.Steps, target)

	// Extract packages from system dependency steps
	seen := make(map[string]bool)
	var packages []string

	for _, step := range filtered {
		if !SystemPackageActions[step.Action] {
			continue
		}

		// Extract packages from step params
		pkgs, ok := actions.GetStringSlice(step.Params, "packages")
		if !ok {
			continue
		}

		for _, pkg := range pkgs {
			if !seen[pkg] {
				seen[pkg] = true
				packages = append(packages, pkg)
			}
		}
	}

	return packages
}

// ExtractSystemPackagesFromSteps extracts system package names from a slice of steps.
// This is a lower-level function that doesn't do platform filtering.
// Returns deduplicated package names.
func ExtractSystemPackagesFromSteps(steps []recipe.Step) []string {
	seen := make(map[string]bool)
	var packages []string

	for _, step := range steps {
		if !SystemPackageActions[step.Action] {
			continue
		}

		pkgs, ok := actions.GetStringSlice(step.Params, "packages")
		if !ok {
			continue
		}

		for _, pkg := range pkgs {
			if !seen[pkg] {
				seen[pkg] = true
				packages = append(packages, pkg)
			}
		}
	}

	return packages
}
