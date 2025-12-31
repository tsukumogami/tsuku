package actions

import "fmt"

// debianConstraint is the implicit constraint for all apt-based actions.
var debianConstraint = &Constraint{OS: "linux", LinuxFamily: "debian"}

// AptInstallAction installs packages using apt-get on Debian-family systems.
// This is a system action that requires sudo and modifies the system state.
//
// Parameters:
//   - packages (required): List of package names to install
//   - fallback (optional): Text shown if installation fails
//   - unless_command (optional): Skip if this command exists
type AptInstallAction struct {
	BaseAction
}

// RequiresNetwork returns true because apt_install fetches packages from repositories.
func (AptInstallAction) RequiresNetwork() bool { return true }

// Name returns the action name.
func (a *AptInstallAction) Name() string {
	return "apt_install"
}

// Validate checks that required parameters are present and valid.
func (a *AptInstallAction) Validate(params map[string]interface{}) error {
	_, err := ValidatePackages(params, a.Name())
	return err
}

// ImplicitConstraint returns the debian family constraint.
func (a *AptInstallAction) ImplicitConstraint() *Constraint {
	return debianConstraint
}

// Execute logs what would be installed (stub implementation).
func (a *AptInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("apt_install action requires 'packages' parameter")
	}

	fmt.Printf("   Would install via apt: %v\n", packages)
	fmt.Printf("   (Skipped - requires sudo and system modification)\n")
	return nil
}

// AptRepoAction adds an APT repository with its GPG key.
// This is a system action that requires sudo and modifies the system state.
//
// Parameters:
//   - url (required): Repository URL (e.g., "https://download.docker.com/linux/ubuntu")
//   - key_url (required): GPG key URL
//   - key_sha256 (required): SHA256 hash of the GPG key for verification
type AptRepoAction struct {
	BaseAction
}

// RequiresNetwork returns true because apt_repo fetches repository metadata.
func (AptRepoAction) RequiresNetwork() bool { return true }

// Name returns the action name.
func (a *AptRepoAction) Name() string {
	return "apt_repo"
}

// Validate checks that required parameters are present and valid.
func (a *AptRepoAction) Validate(params map[string]interface{}) error {
	if _, ok := params["url"].(string); !ok {
		return fmt.Errorf("apt_repo requires 'url' parameter")
	}
	if _, ok := params["key_url"].(string); !ok {
		return fmt.Errorf("apt_repo requires 'key_url' parameter")
	}
	if _, ok := params["key_sha256"].(string); !ok {
		return fmt.Errorf("apt_repo requires 'key_sha256' parameter")
	}
	return nil
}

// ImplicitConstraint returns the debian family constraint.
func (a *AptRepoAction) ImplicitConstraint() *Constraint {
	return debianConstraint
}

// Execute logs what would be configured (stub implementation).
func (a *AptRepoAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	url := params["url"].(string)
	fmt.Printf("   Would add APT repository: %s\n", url)
	fmt.Printf("   (Skipped - requires sudo and system modification)\n")
	return nil
}

// AptPPAAction adds an Ubuntu PPA (Personal Package Archive).
// This is a system action that requires sudo and modifies the system state.
// Note: PPAs are Ubuntu-specific but work on Ubuntu derivatives.
//
// Parameters:
//   - ppa (required): PPA identifier (e.g., "deadsnakes/ppa")
type AptPPAAction struct {
	BaseAction
}

// RequiresNetwork returns true because apt_ppa fetches PPA metadata.
func (AptPPAAction) RequiresNetwork() bool { return true }

// Name returns the action name.
func (a *AptPPAAction) Name() string {
	return "apt_ppa"
}

// Validate checks that required parameters are present and valid.
func (a *AptPPAAction) Validate(params map[string]interface{}) error {
	if _, ok := params["ppa"].(string); !ok {
		return fmt.Errorf("apt_ppa requires 'ppa' parameter")
	}
	return nil
}

// ImplicitConstraint returns the debian family constraint.
func (a *AptPPAAction) ImplicitConstraint() *Constraint {
	return debianConstraint
}

// Execute logs what would be configured (stub implementation).
func (a *AptPPAAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	ppa := params["ppa"].(string)
	fmt.Printf("   Would add PPA: %s\n", ppa)
	fmt.Printf("   (Skipped - requires sudo and system modification)\n")
	return nil
}
