package actions

import (
	"fmt"
)

// AptInstallAction implements apt package installation (stub for validation)
type AptInstallAction struct{}

// Name returns the action name
func (a *AptInstallAction) Name() string {
	return "apt_install"
}

// Execute is a stub that logs what would be installed
//
// Parameters:
//   - packages (required): List of packages to install
func (a *AptInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("apt_install action requires 'packages' parameter")
	}

	fmt.Printf("   Would install via apt: %v\n", packages)
	fmt.Printf("   (Skipped - requires sudo and system modification)\n")
	return nil
}

// YumInstallAction implements yum package installation (stub for validation)
type YumInstallAction struct{}

// Name returns the action name
func (a *YumInstallAction) Name() string {
	return "yum_install"
}

// Execute is a stub that logs what would be installed
//
// Parameters:
//   - packages (required): List of packages to install
func (a *YumInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("yum_install action requires 'packages' parameter")
	}

	fmt.Printf("   Would install via yum: %v\n", packages)
	fmt.Printf("   (Skipped - requires sudo and system modification)\n")
	return nil
}

// BrewInstallAction implements Homebrew package installation (stub for validation)
type BrewInstallAction struct{}

// Name returns the action name
func (a *BrewInstallAction) Name() string {
	return "brew_install"
}

// Execute is a stub that logs what would be installed
//
// Parameters:
//   - packages (required): List of packages to install
func (a *BrewInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("brew_install action requires 'packages' parameter")
	}

	fmt.Printf("   Would install via brew: %v\n", packages)
	fmt.Printf("   (Skipped - requires system Homebrew)\n")
	return nil
}
