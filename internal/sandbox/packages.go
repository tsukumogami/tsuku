package sandbox

import (
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/executor"
)

// SystemRequirements contains all system-level dependencies extracted from a plan.
// This includes both packages and repository configurations.
type SystemRequirements struct {
	Packages     map[string][]string // Package manager -> package names (e.g., {"apt": ["curl"]})
	Repositories []RepositoryConfig  // Repository configurations
}

// RepositoryConfig describes a package repository to be added to the container.
type RepositoryConfig struct {
	Manager   string // Package manager: "apt", "dnf", "zypper", etc.
	Type      string // Repository type: "repo", "ppa", "tap"
	URL       string // Repository URL (for "repo" type)
	KeyURL    string // GPG key URL (optional, for repositories requiring key verification)
	KeySHA256 string // Expected SHA256 hash of GPG key (required if KeyURL is set)
	PPA       string // PPA identifier (for "ppa" type, e.g., "user/repo")
	Tap       string // Homebrew tap name (for "tap" type, e.g., "user/repo")
}

// ExtractSystemRequirements collects all system-level dependencies from a filtered plan.
// The plan is already filtered for the target platform, so steps contain only
// the actions needed for that platform.
//
// Returns a SystemRequirements struct containing both packages and repository configurations.
// Returns nil if the plan has no system dependency actions.
func ExtractSystemRequirements(plan *executor.InstallationPlan) *SystemRequirements {
	if plan == nil {
		return nil
	}

	packages := make(map[string][]string)
	var repositories []RepositoryConfig
	hasSystemDeps := false

	for _, step := range plan.Steps {
		// Get the package list from step params (if present)
		pkgs, ok := actions.GetStringSlice(step.Params, "packages")
		if !ok {
			pkgs = nil // No packages in this step
		}

		switch step.Action {
		case "apt_install":
			hasSystemDeps = true
			packages["apt"] = append(packages["apt"], pkgs...)

		case "apt_repo":
			hasSystemDeps = true
			url, _ := actions.GetString(step.Params, "url")
			keyURL, _ := actions.GetString(step.Params, "key_url")
			keySHA256, _ := actions.GetString(step.Params, "key_sha256")
			repositories = append(repositories, RepositoryConfig{
				Manager:   "apt",
				Type:      "repo",
				URL:       url,
				KeyURL:    keyURL,
				KeySHA256: keySHA256,
			})

		case "apt_ppa":
			hasSystemDeps = true
			ppa, _ := actions.GetString(step.Params, "ppa")
			repositories = append(repositories, RepositoryConfig{
				Manager: "apt",
				Type:    "ppa",
				PPA:     ppa,
			})

		case "brew_install", "brew_cask":
			hasSystemDeps = true
			packages["brew"] = append(packages["brew"], pkgs...)

		case "brew_tap":
			hasSystemDeps = true
			tap, _ := actions.GetString(step.Params, "tap")
			repositories = append(repositories, RepositoryConfig{
				Manager: "brew",
				Type:    "tap",
				Tap:     tap,
			})

		case "dnf_install":
			hasSystemDeps = true
			packages["dnf"] = append(packages["dnf"], pkgs...)

		case "dnf_repo":
			hasSystemDeps = true
			url, _ := actions.GetString(step.Params, "url")
			gpgkey, _ := actions.GetString(step.Params, "gpgkey")
			// For dnf_repo, gpgkey parameter may contain the key URL
			repositories = append(repositories, RepositoryConfig{
				Manager: "dnf",
				Type:    "repo",
				URL:     url,
				KeyURL:  gpgkey,
			})

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

	return &SystemRequirements{
		Packages:     packages,
		Repositories: repositories,
	}
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
