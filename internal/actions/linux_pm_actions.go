package actions

import "fmt"

// Linux family constraints for each package manager.
var (
	archConstraint   = &Constraint{OS: "linux", LinuxFamily: "arch"}
	alpineConstraint = &Constraint{OS: "linux", LinuxFamily: "alpine"}
	suseConstraint   = &Constraint{OS: "linux", LinuxFamily: "suse"}
)

// PacmanInstallAction installs packages using pacman on Arch-family systems.
// This includes Arch Linux, Manjaro, and EndeavourOS.
//
// Parameters:
//   - packages (required): List of package names to install
//   - fallback (optional): Text shown if installation fails
//   - unless_command (optional): Skip if this command exists
type PacmanInstallAction struct {
	BaseAction
}

// RequiresNetwork returns true because pacman_install fetches packages from repositories.
func (PacmanInstallAction) RequiresNetwork() bool { return true }

// Name returns the action name.
func (a *PacmanInstallAction) Name() string {
	return "pacman_install"
}

// Validate checks that required parameters are present and valid.
func (a *PacmanInstallAction) Validate(params map[string]interface{}) error {
	_, err := ValidatePackages(params, a.Name())
	return err
}

// Preflight validates parameters without side effects.
func (a *PacmanInstallAction) Preflight(params map[string]interface{}) *PreflightResult {
	return ValidatePackagesPreflight(params, a.Name())
}

// ImplicitConstraint returns the Arch family constraint.
func (a *PacmanInstallAction) ImplicitConstraint() *Constraint {
	return archConstraint
}

// Execute logs what would be installed (stub implementation).
func (a *PacmanInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("pacman_install action requires 'packages' parameter")
	}

	fmt.Printf("   Would install via pacman: %v\n", packages)
	fmt.Printf("   (Skipped - requires sudo and system modification)\n")
	return nil
}

// ApkInstallAction installs packages using apk on Alpine Linux.
//
// Parameters:
//   - packages (required): List of package names to install
//   - fallback (optional): Text shown if installation fails
//   - unless_command (optional): Skip if this command exists
type ApkInstallAction struct {
	BaseAction
}

// RequiresNetwork returns true because apk_install fetches packages from repositories.
func (ApkInstallAction) RequiresNetwork() bool { return true }

// Name returns the action name.
func (a *ApkInstallAction) Name() string {
	return "apk_install"
}

// Validate checks that required parameters are present and valid.
func (a *ApkInstallAction) Validate(params map[string]interface{}) error {
	_, err := ValidatePackages(params, a.Name())
	return err
}

// Preflight validates parameters without side effects.
func (a *ApkInstallAction) Preflight(params map[string]interface{}) *PreflightResult {
	return ValidatePackagesPreflight(params, a.Name())
}

// ImplicitConstraint returns the Alpine constraint.
func (a *ApkInstallAction) ImplicitConstraint() *Constraint {
	return alpineConstraint
}

// Execute logs what would be installed (stub implementation).
func (a *ApkInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("apk_install action requires 'packages' parameter")
	}

	fmt.Printf("   Would install via apk: %v\n", packages)
	fmt.Printf("   (Skipped - requires sudo and system modification)\n")
	return nil
}

// ZypperInstallAction installs packages using zypper on SUSE-family systems.
// This includes openSUSE and SUSE Linux Enterprise Server (SLES).
//
// Parameters:
//   - packages (required): List of package names to install
//   - fallback (optional): Text shown if installation fails
//   - unless_command (optional): Skip if this command exists
type ZypperInstallAction struct {
	BaseAction
}

// RequiresNetwork returns true because zypper_install fetches packages from repositories.
func (ZypperInstallAction) RequiresNetwork() bool { return true }

// Name returns the action name.
func (a *ZypperInstallAction) Name() string {
	return "zypper_install"
}

// Validate checks that required parameters are present and valid.
func (a *ZypperInstallAction) Validate(params map[string]interface{}) error {
	_, err := ValidatePackages(params, a.Name())
	return err
}

// Preflight validates parameters without side effects.
func (a *ZypperInstallAction) Preflight(params map[string]interface{}) *PreflightResult {
	return ValidatePackagesPreflight(params, a.Name())
}

// ImplicitConstraint returns the SUSE family constraint.
func (a *ZypperInstallAction) ImplicitConstraint() *Constraint {
	return suseConstraint
}

// Execute logs what would be installed (stub implementation).
func (a *ZypperInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("zypper_install action requires 'packages' parameter")
	}

	fmt.Printf("   Would install via zypper: %v\n", packages)
	fmt.Printf("   (Skipped - requires sudo and system modification)\n")
	return nil
}
