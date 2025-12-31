package actions

import "fmt"

// Constraint represents a platform/family requirement for system actions.
// Actions like apt_install have implicit constraints (linux_family = "debian")
// that determine which targets they apply to during plan generation.
type Constraint struct {
	// OS is the required operating system (e.g., "linux", "darwin")
	OS string
	// LinuxFamily is the required Linux family (e.g., "debian", "rhel").
	// Only relevant when OS == "linux".
	LinuxFamily string
}

// String returns a human-readable representation of the constraint.
func (c *Constraint) String() string {
	if c.LinuxFamily != "" {
		return fmt.Sprintf("linux/%s", c.LinuxFamily)
	}
	return c.OS
}

// SystemAction extends Action with system dependency capabilities.
// System actions represent operations like package installation, repository
// configuration, and service management that require system-level privileges.
type SystemAction interface {
	Action

	// Validate checks that parameters are valid for this action.
	// Returns nil if parameters are valid, or an error describing the problem.
	Validate(params map[string]interface{}) error

	// ImplicitConstraint returns the built-in platform constraint for this action.
	// Package manager actions have immutable constraints (e.g., apt_install -> debian).
	// Returns nil if the action has no implicit constraint and works on all platforms.
	ImplicitConstraint() *Constraint
}

// BaseSystemFields provides shared fields for system install actions.
// These fields are common across all package installation actions.
type BaseSystemFields struct {
	// Fallback is optional text shown to users if the installation fails.
	// Provides manual instructions as a backup when automation doesn't work.
	Fallback string

	// UnlessCommand skips the installation if this command already exists.
	// Useful when a package might already be installed through other means.
	UnlessCommand string
}

// ExtractBaseSystemFields extracts the common system action fields from params.
// Returns the extracted fields for use by specific action implementations.
func ExtractBaseSystemFields(params map[string]interface{}) BaseSystemFields {
	var fields BaseSystemFields

	if fallback, ok := params["fallback"].(string); ok {
		fields.Fallback = fallback
	}

	if unlessCommand, ok := params["unless_command"].(string); ok {
		fields.UnlessCommand = unlessCommand
	}

	return fields
}

// ValidatePackages checks that the packages parameter is a non-empty string slice.
// Returns an error if packages is missing, empty, or not a string slice.
func ValidatePackages(params map[string]interface{}, actionName string) ([]string, error) {
	packages, ok := GetStringSlice(params, "packages")
	if !ok || len(packages) == 0 {
		return nil, fmt.Errorf("%s requires non-empty 'packages' parameter", actionName)
	}
	return packages, nil
}
