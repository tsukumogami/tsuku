package actions

import (
	"fmt"
	"strings"
)

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

// Execute checks if packages are installed and returns an error if any are missing.
func (a *PacmanInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("pacman_install action requires 'packages' parameter")
	}

	// Check which packages are missing
	missing := checkMissingPackages(packages, "arch")
	if len(missing) > 0 {
		return &DependencyMissingError{
			Packages: missing,
			Command:  buildInstallCommand("pacman -S --noconfirm", missing),
			Family:   "arch",
		}
	}

	fmt.Printf("   System packages verified: %v\n", packages)
	return nil
}

// Describe returns a copy-pasteable pacman install command.
func (a *PacmanInstallAction) Describe(params map[string]interface{}) string {
	packages, ok := GetStringSlice(params, "packages")
	if !ok || len(packages) == 0 {
		return ""
	}
	return fmt.Sprintf("sudo pacman -S --noconfirm %s", strings.Join(packages, " "))
}

// IsExternallyManaged returns true because pacman delegates to the system package manager.
func (a *PacmanInstallAction) IsExternallyManaged() bool { return true }

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

// Execute checks if packages are installed and returns an error if any are missing.
func (a *ApkInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("apk_install action requires 'packages' parameter")
	}

	// Check which packages are missing
	missing := checkMissingPackages(packages, "alpine")
	if len(missing) > 0 {
		return &DependencyMissingError{
			Packages: missing,
			Command:  buildInstallCommand("apk add", missing),
			Family:   "alpine",
		}
	}

	fmt.Printf("   System packages verified: %v\n", packages)
	return nil
}

// Describe returns a copy-pasteable apk add command.
func (a *ApkInstallAction) Describe(params map[string]interface{}) string {
	packages, ok := GetStringSlice(params, "packages")
	if !ok || len(packages) == 0 {
		return ""
	}
	return fmt.Sprintf("sudo apk add %s", strings.Join(packages, " "))
}

// IsExternallyManaged returns true because apk delegates to the system package manager.
func (a *ApkInstallAction) IsExternallyManaged() bool { return true }

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

// Execute checks if packages are installed and returns an error if any are missing.
func (a *ZypperInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("zypper_install action requires 'packages' parameter")
	}

	// Check which packages are missing
	missing := checkMissingPackages(packages, "suse")
	if len(missing) > 0 {
		return &DependencyMissingError{
			Packages: missing,
			Command:  buildInstallCommand("zypper install -y", missing),
			Family:   "suse",
		}
	}

	fmt.Printf("   System packages verified: %v\n", packages)
	return nil
}

// Describe returns a copy-pasteable zypper install command.
func (a *ZypperInstallAction) Describe(params map[string]interface{}) string {
	packages, ok := GetStringSlice(params, "packages")
	if !ok || len(packages) == 0 {
		return ""
	}
	return fmt.Sprintf("sudo zypper install -y %s", strings.Join(packages, " "))
}

// IsExternallyManaged returns true because zypper delegates to the system package manager.
func (a *ZypperInstallAction) IsExternallyManaged() bool { return true }
