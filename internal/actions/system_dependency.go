package actions

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/tsukumogami/tsuku/internal/platform"
)

// SystemDependencyAction checks for system package dependencies and guides
// users to install them. This action is designed for musl systems (Alpine)
// where embedded libraries use system packages instead of Homebrew bottles.
//
// The action is read-only: it checks if packages are installed but never
// runs privileged commands. If a package is missing, it returns a structured
// error with the install command for the user to run.
type SystemDependencyAction struct{ BaseAction }

// IsDeterministic returns true because package presence checks are deterministic.
func (SystemDependencyAction) IsDeterministic() bool { return true }

// Name returns the action name.
func (a *SystemDependencyAction) Name() string {
	return "system_dependency"
}

// Preflight validates parameters without side effects.
func (a *SystemDependencyAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	if _, ok := GetString(params, "name"); !ok {
		result.AddError("system_dependency action requires 'name' parameter")
	}

	if _, ok := params["packages"]; !ok {
		result.AddError("system_dependency action requires 'packages' parameter")
	}

	return result
}

// Execute checks if a system package is installed and returns an error with
// installation guidance if it's missing.
//
// Parameters:
//   - name (required): Library/dependency name for display (e.g., "zlib")
//   - packages (required): Map of family -> package name (e.g., {"alpine": "zlib-dev"})
//
// On success (package installed), returns nil.
// On missing package, returns *DependencyMissingError with install command.
func (a *SystemDependencyAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	name, ok := GetString(params, "name")
	if !ok || name == "" {
		return fmt.Errorf("system_dependency action requires 'name' parameter")
	}

	packagesRaw, ok := params["packages"]
	if !ok {
		return fmt.Errorf("system_dependency action requires 'packages' parameter")
	}

	packages, err := parsePackagesMap(packagesRaw)
	if err != nil {
		return fmt.Errorf("invalid 'packages' parameter: %w", err)
	}

	// Detect Linux family
	family, err := platform.DetectFamily()
	if err != nil {
		return fmt.Errorf("failed to detect Linux family: %w", err)
	}
	if family == "" {
		return fmt.Errorf("system_dependency action requires Linux")
	}

	// Get package name for this family
	pkg, ok := packages[family]
	if !ok {
		return fmt.Errorf("no package mapping for family %q (available: %v)", family, mapKeys(packages))
	}

	fmt.Printf("   Checking system dependency: %s (%s)\n", name, pkg)

	// Check if installed
	if isPackageInstalled(pkg, family) {
		fmt.Printf("   System dependency satisfied: %s\n", name)
		return nil
	}

	// Package is missing - return structured error
	cmd := getInstallCommand(pkg, family)
	return &DependencyMissingError{
		Library: name,
		Package: pkg,
		Command: cmd,
		Family:  family,
	}
}

// parsePackagesMap converts the raw packages parameter to a string map.
// Handles both map[string]interface{} (from TOML) and map[string]string.
func parsePackagesMap(raw interface{}) (map[string]string, error) {
	result := make(map[string]string)

	switch v := raw.(type) {
	case map[string]interface{}:
		for k, val := range v {
			if s, ok := val.(string); ok {
				result[k] = s
			} else {
				return nil, fmt.Errorf("package value for %q must be a string", k)
			}
		}
	case map[string]string:
		result = v
	default:
		return nil, fmt.Errorf("packages must be a map, got %T", raw)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("packages map is empty")
	}

	return result, nil
}

// mapKeys returns the keys of a string map for error messages.
func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// isPackageInstalled checks if a package is installed on the system.
// Currently supports Alpine (apk). Extensible to other families.
func isPackageInstalled(pkg string, family string) bool {
	switch family {
	case "alpine":
		// apk info -e returns 0 if package is installed, non-zero otherwise
		cmd := exec.Command("apk", "info", "-e", pkg)
		return cmd.Run() == nil
	// Future: add debian, rhel, arch, suse support
	default:
		// Unknown family - assume not installed to trigger error
		return false
	}
}

// getInstallCommand returns the command to install a package.
// Includes sudo/doas prefix when not running as root.
func getInstallCommand(pkg string, family string) string {
	prefix := getRootPrefix()

	switch family {
	case "alpine":
		return prefix + "apk add " + pkg
	// Future: add debian, rhel, arch, suse support
	default:
		return ""
	}
}

// getRootPrefix returns "sudo " or "doas " if not running as root,
// or empty string if already root.
func getRootPrefix() string {
	if os.Getuid() == 0 {
		return ""
	}

	// Prefer doas if available (common on Alpine/BSD)
	if _, err := exec.LookPath("doas"); err == nil {
		return "doas "
	}

	return "sudo "
}

// DependencyMissingError indicates a required system package is not installed.
// This is a structured error that can be collected and formatted by the CLI.
type DependencyMissingError struct {
	Library string // Display name (e.g., "zlib")
	Package string // Package name (e.g., "zlib-dev")
	Command string // Install command (e.g., "sudo apk add zlib-dev")
	Family  string // Linux family (e.g., "alpine")
}

// Error implements the error interface.
func (e *DependencyMissingError) Error() string {
	return fmt.Sprintf("missing system dependency: %s (install with: %s)", e.Library, e.Command)
}

// IsDependencyMissing checks if an error is a DependencyMissingError.
// This helper allows callers to easily detect and collect missing dependencies.
func IsDependencyMissing(err error) bool {
	var depErr *DependencyMissingError
	return errors.As(err, &depErr)
}

// AsDependencyMissing extracts the DependencyMissingError from an error.
// Returns nil if the error is not a DependencyMissingError.
func AsDependencyMissing(err error) *DependencyMissingError {
	var depErr *DependencyMissingError
	if errors.As(err, &depErr) {
		return depErr
	}
	return nil
}
