package actions

import (
	"fmt"
	"strings"
)

// rhelConstraint is the implicit constraint for all dnf-based actions.
var rhelConstraint = &Constraint{OS: "linux", LinuxFamily: "rhel"}

// DnfInstallAction installs packages using dnf on RHEL-family systems.
// This includes Fedora, RHEL, CentOS, Rocky Linux, and AlmaLinux.
//
// Parameters:
//   - packages (required): List of package names to install
//   - fallback (optional): Text shown if installation fails
//   - unless_command (optional): Skip if this command exists
type DnfInstallAction struct {
	BaseAction
}

// RequiresNetwork returns true because dnf_install fetches packages from repositories.
func (DnfInstallAction) RequiresNetwork() bool { return true }

// Name returns the action name.
func (a *DnfInstallAction) Name() string {
	return "dnf_install"
}

// Validate checks that required parameters are present and valid.
func (a *DnfInstallAction) Validate(params map[string]interface{}) error {
	_, err := ValidatePackages(params, a.Name())
	return err
}

// Preflight validates parameters without side effects.
func (a *DnfInstallAction) Preflight(params map[string]interface{}) *PreflightResult {
	return ValidatePackagesPreflight(params, a.Name())
}

// ImplicitConstraint returns the RHEL family constraint.
func (a *DnfInstallAction) ImplicitConstraint() *Constraint {
	return rhelConstraint
}

// Execute logs what would be installed (stub implementation).
func (a *DnfInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	packages, ok := GetStringSlice(params, "packages")
	if !ok {
		return fmt.Errorf("dnf_install action requires 'packages' parameter")
	}

	fmt.Printf("   Would install via dnf: %v\n", packages)
	fmt.Printf("   (Skipped - requires sudo and system modification)\n")
	return nil
}

// Describe returns a copy-pasteable dnf install command.
func (a *DnfInstallAction) Describe(params map[string]interface{}) string {
	packages, ok := GetStringSlice(params, "packages")
	if !ok || len(packages) == 0 {
		return ""
	}
	return fmt.Sprintf("sudo dnf install -y %s", strings.Join(packages, " "))
}

// IsExternallyManaged returns true because dnf delegates to the system package manager.
func (a *DnfInstallAction) IsExternallyManaged() bool { return true }

// DnfRepoAction adds a DNF repository with its GPG key.
// This is a system action that requires sudo and modifies the system state.
//
// Parameters:
//   - url (required): Repository URL
//   - key_url (required): GPG key URL
//   - key_sha256 (required): SHA256 hash of the GPG key for verification
type DnfRepoAction struct {
	BaseAction
}

// RequiresNetwork returns true because dnf_repo fetches repository metadata.
func (DnfRepoAction) RequiresNetwork() bool { return true }

// Name returns the action name.
func (a *DnfRepoAction) Name() string {
	return "dnf_repo"
}

// Validate checks that required parameters are present and valid.
func (a *DnfRepoAction) Validate(params map[string]interface{}) error {
	if _, ok := params["url"].(string); !ok {
		return fmt.Errorf("dnf_repo requires 'url' parameter")
	}
	if _, ok := params["key_url"].(string); !ok {
		return fmt.Errorf("dnf_repo requires 'key_url' parameter")
	}
	if _, ok := params["key_sha256"].(string); !ok {
		return fmt.Errorf("dnf_repo requires 'key_sha256' parameter")
	}
	return nil
}

// Preflight validates parameters without side effects.
func (a *DnfRepoAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	url, hasURL := GetString(params, "url")
	if !hasURL || url == "" {
		result.AddError("dnf_repo requires 'url' parameter")
	}

	keyURL, hasKeyURL := GetString(params, "key_url")
	if !hasKeyURL || keyURL == "" {
		result.AddError("dnf_repo requires 'key_url' parameter")
	}

	keySha256, hasKeySha256 := GetString(params, "key_sha256")
	if !hasKeySha256 || keySha256 == "" {
		result.AddError("dnf_repo requires 'key_sha256' parameter")
	}

	// Security: Validate HTTPS for URLs
	validateHTTPSURL(result, url, "dnf_repo", "url")
	validateHTTPSURL(result, keyURL, "dnf_repo", "key_url")

	return result
}

// ImplicitConstraint returns the RHEL family constraint.
func (a *DnfRepoAction) ImplicitConstraint() *Constraint {
	return rhelConstraint
}

// Execute logs what would be configured (stub implementation).
func (a *DnfRepoAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	url := params["url"].(string)
	fmt.Printf("   Would add DNF repository: %s\n", url)
	fmt.Printf("   (Skipped - requires sudo and system modification)\n")
	return nil
}

// Describe returns a copy-pasteable dnf config-manager command.
func (a *DnfRepoAction) Describe(params map[string]interface{}) string {
	url, ok := GetString(params, "url")
	if !ok || url == "" {
		return ""
	}
	return fmt.Sprintf("sudo dnf config-manager --add-repo %s", url)
}

// IsExternallyManaged returns true because dnf delegates to the system package manager.
func (a *DnfRepoAction) IsExternallyManaged() bool { return true }
