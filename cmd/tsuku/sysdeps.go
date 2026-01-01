package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// systemActionNames lists all system dependency action types.
// These actions require system-level privileges and are displayed to users.
var systemActionNames = map[string]bool{
	"apt_install":     true,
	"apt_repo":        true,
	"apt_ppa":         true,
	"brew_install":    true,
	"brew_cask":       true,
	"dnf_install":     true,
	"dnf_repo":        true,
	"pacman_install":  true,
	"apk_install":     true,
	"zypper_install":  true,
	"group_add":       true,
	"service_enable":  true,
	"service_start":   true,
	"require_command": true,
	"manual":          true,
}

// familyDisplayNames maps linux_family values to human-readable headers.
var familyDisplayNames = map[string]string{
	"debian": "Ubuntu/Debian",
	"rhel":   "Fedora/RHEL/CentOS",
	"arch":   "Arch Linux",
	"alpine": "Alpine Linux",
	"suse":   "openSUSE/SLES",
}

// getTargetDisplayName returns a human-readable name for the target.
func getTargetDisplayName(target platform.Target) string {
	if target.OS() == "darwin" {
		return "macOS"
	}
	if target.LinuxFamily != "" {
		if name, ok := familyDisplayNames[target.LinuxFamily]; ok {
			return name
		}
		return target.LinuxFamily
	}
	return target.OS()
}

// hasSystemDeps checks if a recipe has any system dependency steps.
func hasSystemDeps(r *recipe.Recipe) bool {
	for _, step := range r.Steps {
		if systemActionNames[step.Action] {
			return true
		}
	}
	return false
}

// getSystemDepsForTarget filters recipe steps to only system dependency steps
// that match the given target.
func getSystemDepsForTarget(r *recipe.Recipe, target platform.Target) []recipe.Step {
	// First filter all steps by target
	filtered := executor.FilterStepsByTarget(r.Steps, target)

	// Then keep only system dependency steps
	var sysDeps []recipe.Step
	for _, step := range filtered {
		if systemActionNames[step.Action] {
			sysDeps = append(sysDeps, step)
		}
	}
	return sysDeps
}

// displaySystemDeps displays system dependency instructions for a recipe.
// Returns true if there were system deps to display.
func displaySystemDeps(r *recipe.Recipe, target platform.Target) bool {
	sysDeps := getSystemDepsForTarget(r, target)
	if len(sysDeps) == 0 {
		return false
	}

	// Group steps by category for better organization
	var packageSteps []recipe.Step // apt_install, brew_cask, etc.
	var configSteps []recipe.Step  // group_add, service_enable, etc.
	var verifySteps []recipe.Step  // require_command
	var manualSteps []recipe.Step  // manual

	for _, step := range sysDeps {
		switch step.Action {
		case "require_command":
			verifySteps = append(verifySteps, step)
		case "group_add", "service_enable", "service_start":
			configSteps = append(configSteps, step)
		case "manual":
			manualSteps = append(manualSteps, step)
		default:
			packageSteps = append(packageSteps, step)
		}
	}

	// Display header
	targetName := getTargetDisplayName(target)
	fmt.Println()
	fmt.Printf("This recipe requires system dependencies for %s:\n", targetName)
	fmt.Println()

	stepNum := 1

	// Display package installation steps
	if len(packageSteps) > 0 {
		for _, step := range packageSteps {
			desc := describeSystemStep(step)
			if desc != "" {
				fmt.Printf("  %d. %s\n", stepNum, desc)
				stepNum++
			}
		}
	}

	// Display configuration steps
	if len(configSteps) > 0 {
		for _, step := range configSteps {
			desc := describeSystemStep(step)
			if desc != "" {
				fmt.Printf("  %d. %s\n", stepNum, desc)
				stepNum++
			}
		}
	}

	// Display manual steps
	if len(manualSteps) > 0 {
		for _, step := range manualSteps {
			desc := describeSystemStep(step)
			if desc != "" {
				fmt.Printf("  %d. %s\n", stepNum, desc)
				stepNum++
			}
		}
	}

	// Display verification steps at the end
	if len(verifySteps) > 0 {
		fmt.Println()
		fmt.Println("After installation, verify with:")
		for _, step := range verifySteps {
			desc := describeSystemStep(step)
			if desc != "" {
				fmt.Printf("  %s\n", desc)
			}
		}
	}

	fmt.Println()
	fmt.Println("After completing these steps, run the install command again.")
	fmt.Println()

	return true
}

// describeSystemStep returns a human-readable description for a system dependency step.
func describeSystemStep(step recipe.Step) string {
	action := actions.Get(step.Action)
	if action == nil {
		return ""
	}

	sysAction, ok := action.(actions.SystemAction)
	if !ok {
		return ""
	}

	return sysAction.Describe(step.Params)
}

// resolveTarget determines the target for system dependency display.
// Uses the override if provided, otherwise detects from the current system.
func resolveTarget(familyOverride string) (platform.Target, error) {
	// Build target from current platform
	platformStr := runtime.GOOS + "/" + runtime.GOARCH

	if familyOverride != "" {
		// Validate the override
		validFamilies := []string{"debian", "rhel", "arch", "alpine", "suse"}
		valid := false
		for _, f := range validFamilies {
			if f == familyOverride {
				valid = true
				break
			}
		}
		if !valid {
			return platform.Target{}, fmt.Errorf("invalid target-family %q, must be one of: %s",
				familyOverride, strings.Join(validFamilies, ", "))
		}

		// For family override, assume linux platform
		if runtime.GOOS != "linux" {
			return platform.Target{Platform: "linux/amd64", LinuxFamily: familyOverride}, nil
		}
		return platform.Target{Platform: platformStr, LinuxFamily: familyOverride}, nil
	}

	// Use platform detection
	return platform.DetectTarget()
}
