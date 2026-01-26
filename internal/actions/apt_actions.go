package actions

import (
	"fmt"
	"strings"
)

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

// Preflight validates parameters without side effects.
func (a *AptInstallAction) Preflight(params map[string]interface{}) *PreflightResult {
	return ValidatePackagesPreflight(params, a.Name())
}

// ImplicitConstraint returns the debian family constraint.
func (a *AptInstallAction) ImplicitConstraint() *Constraint {
	return debianConstraint
}

// Execute checks if packages are installed and returns an error if any are missing.
func (a *AptInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("apt_install action requires 'packages' parameter")
	}

	// Check which packages are missing
	missing := checkMissingPackages(packages, "debian")
	if len(missing) > 0 {
		return &DependencyMissingError{
			Packages: missing,
			Command:  buildInstallCommand("apt-get install -y", missing),
			Family:   "debian",
		}
	}

	fmt.Printf("   System packages verified: %v\n", packages)
	return nil
}

// Describe returns a copy-pasteable apt-get install command.
func (a *AptInstallAction) Describe(params map[string]interface{}) string {
	packages, ok := GetStringSlice(params, "packages")
	if !ok || len(packages) == 0 {
		return ""
	}
	return fmt.Sprintf("sudo apt-get install -y %s", strings.Join(packages, " "))
}

// IsExternallyManaged returns true because apt delegates to the system package manager.
func (a *AptInstallAction) IsExternallyManaged() bool { return true }

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

// Preflight validates parameters without side effects.
func (a *AptRepoAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	url, hasURL := GetString(params, "url")
	if !hasURL || url == "" {
		result.AddError("apt_repo requires 'url' parameter")
	}

	keyURL, hasKeyURL := GetString(params, "key_url")
	if !hasKeyURL || keyURL == "" {
		result.AddError("apt_repo requires 'key_url' parameter")
	}

	keySha256, hasKeySha256 := GetString(params, "key_sha256")
	if !hasKeySha256 || keySha256 == "" {
		result.AddError("apt_repo requires 'key_sha256' parameter")
	}

	// Security: Validate HTTPS for URLs
	validateHTTPSURL(result, url, "apt_repo", "url")
	validateHTTPSURL(result, keyURL, "apt_repo", "key_url")

	return result
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

// Describe returns instructions for adding an APT repository.
// The output includes fetching the GPG key and adding the repository.
func (a *AptRepoAction) Describe(params map[string]interface{}) string {
	url, hasURL := GetString(params, "url")
	keyURL, hasKeyURL := GetString(params, "key_url")
	if !hasURL || !hasKeyURL || url == "" || keyURL == "" {
		return ""
	}
	// Generate a simple repo name from the URL for the keyring filename
	return fmt.Sprintf("curl -fsSL %s | sudo gpg --dearmor -o /etc/apt/keyrings/repo.gpg && "+
		"echo \"deb [signed-by=/etc/apt/keyrings/repo.gpg] %s stable main\" | "+
		"sudo tee /etc/apt/sources.list.d/repo.list", keyURL, url)
}

// IsExternallyManaged returns true because apt delegates to the system package manager.
func (a *AptRepoAction) IsExternallyManaged() bool { return true }

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

// Preflight validates parameters without side effects.
func (a *AptPPAAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	ppa, hasPPA := GetString(params, "ppa")
	if !hasPPA || ppa == "" {
		result.AddError("apt_ppa requires 'ppa' parameter")
	}

	return result
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

// Describe returns a copy-pasteable add-apt-repository command.
func (a *AptPPAAction) Describe(params map[string]interface{}) string {
	ppa, ok := GetString(params, "ppa")
	if !ok || ppa == "" {
		return ""
	}
	return fmt.Sprintf("sudo add-apt-repository ppa:%s", ppa)
}

// IsExternallyManaged returns true because apt delegates to the system package manager.
func (a *AptPPAAction) IsExternallyManaged() bool { return true }
