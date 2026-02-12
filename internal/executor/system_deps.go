package executor

import (
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// SystemRequirements contains all system-level dependencies extracted from a recipe or plan.
// This includes both packages and repository configurations.
type SystemRequirements struct {
	Packages     map[string][]string // Package manager -> package names (e.g., {"apt": ["curl"]})
	Repositories []RepositoryConfig  // Repository configurations
}

// RepositoryConfig describes a package repository to be added before installing packages.
type RepositoryConfig struct {
	Manager   string // Package manager: "apt", "dnf", "zypper", etc.
	Type      string // Repository type: "repo", "ppa", "tap"
	URL       string // Repository URL (for "repo" type)
	KeyURL    string // GPG key URL (optional, for repositories requiring key verification)
	KeySHA256 string // Expected SHA256 hash of GPG key (required if KeyURL is set)
	PPA       string // PPA identifier (for "ppa" type, e.g., "user/repo")
	Tap       string // Homebrew tap name (for "tap" type, e.g., "user/repo")
}

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

// SystemDependencyActions lists all actions that configure system-level dependencies.
// This includes both package installation and repository configuration actions.
var SystemDependencyActions = map[string]bool{
	// Package installation
	"apt_install":    true,
	"apk_install":    true,
	"dnf_install":    true,
	"pacman_install": true,
	"zypper_install": true,
	"brew_install":   true,
	"brew_cask":      true,
	// Repository configuration
	"apt_repo": true,
	"apt_ppa":  true,
	"dnf_repo": true,
	"brew_tap": true,
}

// systemRequirementsBuilder accumulates system requirements during extraction.
type systemRequirementsBuilder struct {
	packages      map[string][]string
	repositories  []RepositoryConfig
	hasSystemDeps bool
}

func newSystemRequirementsBuilder() *systemRequirementsBuilder {
	return &systemRequirementsBuilder{
		packages: make(map[string][]string),
	}
}

// processStep extracts system requirements from a single step's action and params.
func (b *systemRequirementsBuilder) processStep(action string, params map[string]interface{}) {
	// Get the package list from step params (if present)
	pkgs, ok := actions.GetStringSlice(params, "packages")
	if !ok {
		pkgs = nil
	}

	switch action {
	case "apt_install":
		b.hasSystemDeps = true
		b.packages["apt"] = append(b.packages["apt"], pkgs...)

	case "apt_repo":
		b.hasSystemDeps = true
		url, _ := actions.GetString(params, "url")
		keyURL, _ := actions.GetString(params, "key_url")
		keySHA256, _ := actions.GetString(params, "key_sha256")
		b.repositories = append(b.repositories, RepositoryConfig{
			Manager:   "apt",
			Type:      "repo",
			URL:       url,
			KeyURL:    keyURL,
			KeySHA256: keySHA256,
		})

	case "apt_ppa":
		b.hasSystemDeps = true
		ppa, _ := actions.GetString(params, "ppa")
		b.repositories = append(b.repositories, RepositoryConfig{
			Manager: "apt",
			Type:    "ppa",
			PPA:     ppa,
		})

	case "brew_install", "brew_cask":
		b.hasSystemDeps = true
		b.packages["brew"] = append(b.packages["brew"], pkgs...)

	case "brew_tap":
		b.hasSystemDeps = true
		tap, _ := actions.GetString(params, "tap")
		b.repositories = append(b.repositories, RepositoryConfig{
			Manager: "brew",
			Type:    "tap",
			Tap:     tap,
		})

	case "dnf_install":
		b.hasSystemDeps = true
		b.packages["dnf"] = append(b.packages["dnf"], pkgs...)

	case "dnf_repo":
		b.hasSystemDeps = true
		url, _ := actions.GetString(params, "url")
		gpgkey, _ := actions.GetString(params, "gpgkey")
		b.repositories = append(b.repositories, RepositoryConfig{
			Manager: "dnf",
			Type:    "repo",
			URL:     url,
			KeyURL:  gpgkey,
		})

	case "pacman_install":
		b.hasSystemDeps = true
		b.packages["pacman"] = append(b.packages["pacman"], pkgs...)

	case "apk_install":
		b.hasSystemDeps = true
		b.packages["apk"] = append(b.packages["apk"], pkgs...)

	case "zypper_install":
		b.hasSystemDeps = true
		b.packages["zypper"] = append(b.packages["zypper"], pkgs...)
	}
}

// build returns the final SystemRequirements, or nil if no system deps were found.
func (b *systemRequirementsBuilder) build() *SystemRequirements {
	if !b.hasSystemDeps {
		return nil
	}
	return &SystemRequirements{
		Packages:     b.packages,
		Repositories: b.repositories,
	}
}

// ExtractSystemRequirementsFromSteps extracts all system dependencies from a slice of steps.
// This is the core extraction function used by both recipe and plan entry points.
// Returns nil if no system dependency actions are found.
func ExtractSystemRequirementsFromSteps(steps []recipe.Step) *SystemRequirements {
	builder := newSystemRequirementsBuilder()
	for _, step := range steps {
		builder.processStep(step.Action, step.Params)
	}
	return builder.build()
}

// ExtractSystemRequirementsFromRecipe extracts system dependencies from a recipe for a target.
// It filters steps by target platform, then extracts packages and repository configurations.
// Returns nil if no system dependency actions are found.
func ExtractSystemRequirementsFromRecipe(r *recipe.Recipe, target platform.Target) *SystemRequirements {
	filtered := FilterStepsByTarget(r.Steps, target)
	return ExtractSystemRequirementsFromSteps(filtered)
}

// ExtractSystemRequirementsFromPlan extracts system dependencies from an installation plan.
// The plan is already filtered for the target platform.
// Returns nil if no system dependency actions are found.
func ExtractSystemRequirementsFromPlan(plan *InstallationPlan) *SystemRequirements {
	if plan == nil {
		return nil
	}
	builder := newSystemRequirementsBuilder()
	for _, step := range plan.Steps {
		builder.processStep(step.Action, step.Params)
	}
	return builder.build()
}

// ExtractSystemPackages extracts system package names from a recipe for a target.
// It filters steps by target platform, then extracts packages from system
// dependency actions (apk_install, apt_install, etc.).
//
// Returns deduplicated package names.
//
// Deprecated: Use ExtractSystemRequirementsFromRecipe for full repository support.
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
//
// Deprecated: Use ExtractSystemRequirementsFromSteps for full repository support.
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
