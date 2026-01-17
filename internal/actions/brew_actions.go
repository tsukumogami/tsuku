package actions

import (
	"fmt"
	"strings"
)

// darwinConstraint is the implicit constraint for all brew-based actions.
var darwinConstraint = &Constraint{OS: "darwin"}

// BrewInstallAction installs packages using Homebrew on macOS.
// This action extends the existing stub to implement SystemAction.
//
// Parameters:
//   - packages (required): List of formula names to install
//   - tap (optional): Custom tap to install from (e.g., "owner/repo")
//   - fallback (optional): Text shown if installation fails
//   - unless_command (optional): Skip if this command exists
type BrewInstallAction struct {
	BaseAction
}

// RequiresNetwork returns true because brew_install fetches packages from Homebrew.
func (BrewInstallAction) RequiresNetwork() bool { return true }

// Name returns the action name.
func (a *BrewInstallAction) Name() string {
	return "brew_install"
}

// Validate checks that required parameters are present and valid.
func (a *BrewInstallAction) Validate(params map[string]interface{}) error {
	_, err := ValidatePackages(params, a.Name())
	return err
}

// Preflight validates parameters without side effects.
func (a *BrewInstallAction) Preflight(params map[string]interface{}) *PreflightResult {
	return ValidatePackagesPreflight(params, a.Name())
}

// ImplicitConstraint returns the darwin constraint.
func (a *BrewInstallAction) ImplicitConstraint() *Constraint {
	return darwinConstraint
}

// Execute logs what would be installed (stub implementation).
func (a *BrewInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("brew_install action requires 'packages' parameter")
	}

	fmt.Printf("   Would install via brew: %v\n", packages)
	fmt.Printf("   (Skipped - requires system Homebrew)\n")
	return nil
}

// Describe returns a copy-pasteable brew install command.
func (a *BrewInstallAction) Describe(params map[string]interface{}) string {
	packages, ok := GetStringSlice(params, "packages")
	if !ok || len(packages) == 0 {
		return ""
	}
	return fmt.Sprintf("brew install %s", strings.Join(packages, " "))
}

// IsExternallyManaged returns true because brew delegates to Homebrew.
func (a *BrewInstallAction) IsExternallyManaged() bool { return true }

// BrewCaskAction installs GUI applications using Homebrew Casks on macOS.
// Casks are used for applications that don't fit the formula model.
//
// Parameters:
//   - packages (required): List of cask names to install
//   - tap (optional): Custom tap to install from (e.g., "owner/repo")
//   - fallback (optional): Text shown if installation fails
//   - unless_command (optional): Skip if this command exists
type BrewCaskAction struct {
	BaseAction
}

// RequiresNetwork returns true because brew_cask fetches casks from Homebrew.
func (BrewCaskAction) RequiresNetwork() bool { return true }

// Name returns the action name.
func (a *BrewCaskAction) Name() string {
	return "brew_cask"
}

// Validate checks that required parameters are present and valid.
func (a *BrewCaskAction) Validate(params map[string]interface{}) error {
	_, err := ValidatePackages(params, a.Name())
	return err
}

// Preflight validates parameters without side effects.
func (a *BrewCaskAction) Preflight(params map[string]interface{}) *PreflightResult {
	return ValidatePackagesPreflight(params, a.Name())
}

// ImplicitConstraint returns the darwin constraint.
func (a *BrewCaskAction) ImplicitConstraint() *Constraint {
	return darwinConstraint
}

// Execute logs what would be installed (stub implementation).
func (a *BrewCaskAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("brew_cask action requires 'packages' parameter")
	}

	fmt.Printf("   Would install via brew cask: %v\n", packages)
	fmt.Printf("   (Skipped - requires system Homebrew)\n")
	return nil
}

// Describe returns a copy-pasteable brew install --cask command.
func (a *BrewCaskAction) Describe(params map[string]interface{}) string {
	packages, ok := GetStringSlice(params, "packages")
	if !ok || len(packages) == 0 {
		return ""
	}
	return fmt.Sprintf("brew install --cask %s", strings.Join(packages, " "))
}

// IsExternallyManaged returns true because brew delegates to Homebrew.
func (a *BrewCaskAction) IsExternallyManaged() bool { return true }
