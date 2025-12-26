package actions

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/tsukumogami/tsuku/internal/version"
)

// RequireSystemAction validates that a required system dependency is installed.
// This action is used for dependencies tsuku cannot provision (Docker, CUDA, etc.).
// It detects command presence, validates versions, and provides installation guidance.
type RequireSystemAction struct{ BaseAction }

// IsDeterministic returns true because system dependency checks are deterministic.
func (RequireSystemAction) IsDeterministic() bool { return true }

// Name returns the action name
func (a *RequireSystemAction) Name() string {
	return "require_system"
}

// Preflight validates parameters without side effects.
func (a *RequireSystemAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}
	if _, ok := GetString(params, "command"); !ok {
		result.AddError("require_system action requires 'command' parameter")
	}

	// WARNING: Missing install_guide
	if _, hasGuide := params["install_guide"]; !hasGuide {
		result.AddWarning("consider adding 'install_guide' with platform-specific installation instructions")
	}

	// ERROR: min_version without version detection
	if _, hasMinVersion := GetString(params, "min_version"); hasMinVersion {
		_, hasVersionFlag := GetString(params, "version_flag")
		_, hasVersionRegex := GetString(params, "version_regex")
		if !hasVersionFlag || !hasVersionRegex {
			result.AddError("min_version specified but version detection incomplete; add version_flag and version_regex")
		}
	}
	return result
}

// Execute validates a system dependency is installed and meets version requirements.
//
// Parameters:
//   - command (required): Command name to check (e.g., "docker")
//   - version_flag (optional): Flag to get version (e.g., "--version")
//   - version_regex (optional): Regex to extract version from output (e.g., "version ([0-9.]+)")
//   - min_version (optional): Minimum required version (e.g., "20.10.0")
//   - install_guide (optional): Map of platform-specific installation instructions
//
// The action performs hierarchical validation:
//  1. Command exists check (via exec.LookPath)
//  2. Version check (if version_flag and version_regex provided)
//  3. Min version validation (if min_version provided)
//
// If validation fails, returns an error with platform-specific installation guidance.
func (a *RequireSystemAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get command name (required)
	command, ok := GetString(params, "command")
	if !ok || command == "" {
		return fmt.Errorf("require_system action requires 'command' parameter")
	}

	// Validate command name for security (no path traversal, shell metacharacters)
	if err := validateCommandName(command); err != nil {
		return err
	}

	// Get optional parameters
	versionFlag, _ := GetString(params, "version_flag")
	versionRegex, _ := GetString(params, "version_regex")
	minVersion, _ := GetString(params, "min_version")
	installGuide, _ := GetMapStringString(params, "install_guide")

	fmt.Printf("   Checking system dependency: %s\n", command)

	// Step 1: Check if command exists
	cmdPath, err := exec.LookPath(command)
	if err != nil {
		// Command not found - return error with installation guidance
		guide := getPlatformGuide(installGuide, runtime.GOOS)
		return &SystemDepMissingError{
			Command:      command,
			InstallGuide: guide,
		}
	}

	fmt.Printf("   Found %s at: %s\n", command, cmdPath)

	// Step 2: Check version if version_flag and version_regex provided
	if versionFlag != "" && versionRegex != "" {
		versionStr, err := detectVersion(command, versionFlag, versionRegex)
		if err != nil {
			return fmt.Errorf("failed to detect version for %s: %w", command, err)
		}

		fmt.Printf("   Detected version: %s\n", versionStr)

		// Step 3: Validate minimum version if specified
		if minVersion != "" {
			if !versionSatisfied(versionStr, minVersion) {
				guide := getPlatformGuide(installGuide, runtime.GOOS)
				return &SystemDepVersionError{
					Command:      command,
					Found:        versionStr,
					Required:     minVersion,
					InstallGuide: guide,
				}
			}
			fmt.Printf("   Version %s satisfies minimum %s\n", versionStr, minVersion)
		}
	}

	fmt.Printf("   System dependency satisfied: %s\n", command)
	return nil
}

// validateCommandName ensures the command name contains no dangerous characters.
// Only allows alphanumeric, hyphen, underscore, and dot.
func validateCommandName(name string) error {
	if name == "" {
		return fmt.Errorf("command name cannot be empty")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("command name cannot contain path separators: %s", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("command name cannot contain '..': %s", name)
	}

	// Only allow alphanumeric, hyphen, underscore, and dot
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return fmt.Errorf("command name contains invalid character '%c': %s", c, name)
		}
	}

	return nil
}

// detectVersion runs a command with a version flag and extracts the version using a regex.
// Returns the extracted version string or an error if detection fails.
func detectVersion(command, versionFlag, versionRegex string) (string, error) {
	// Run command with version flag (no shell execution for security)
	cmd := exec.Command(command, versionFlag)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run '%s %s': %w", command, versionFlag, err)
	}

	// Parse version from output using regex
	re, err := regexp.Compile(versionRegex)
	if err != nil {
		return "", fmt.Errorf("invalid version regex '%s': %w", versionRegex, err)
	}

	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("version regex did not match output (regex: %s)", versionRegex)
	}

	// Return first capture group (version string)
	return strings.TrimSpace(matches[1]), nil
}

// versionSatisfied checks if the found version satisfies the minimum required version.
// Uses the version comparison utility from internal/version.
func versionSatisfied(found, required string) bool {
	// CompareVersions returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
	cmp := version.CompareVersions(found, required)
	return cmp >= 0 // found >= required
}

// getPlatformGuide returns the installation guide for the current platform.
// Tries platform-specific key first, then falls back to "fallback" key.
func getPlatformGuide(installGuide map[string]string, platform string) string {
	if installGuide == nil {
		return ""
	}

	// Try platform-specific guide (e.g., "darwin", "linux")
	if guide, ok := installGuide[platform]; ok {
		return guide
	}

	// Try fallback
	if guide, ok := installGuide["fallback"]; ok {
		return guide
	}

	return ""
}

// SystemDepMissingError indicates a required system dependency is not installed.
type SystemDepMissingError struct {
	Command      string
	InstallGuide string
}

func (e *SystemDepMissingError) Error() string {
	msg := fmt.Sprintf("required system dependency not found: %s", e.Command)
	if e.InstallGuide != "" {
		msg += fmt.Sprintf("\n\nInstallation guide:\n%s", e.InstallGuide)
	}
	return msg
}

// SystemDepVersionError indicates an installed dependency does not meet version requirements.
type SystemDepVersionError struct {
	Command      string
	Found        string
	Required     string
	InstallGuide string
}

func (e *SystemDepVersionError) Error() string {
	msg := fmt.Sprintf("system dependency %s version %s does not meet minimum requirement %s",
		e.Command, e.Found, e.Required)
	if e.InstallGuide != "" {
		msg += fmt.Sprintf("\n\nInstallation guide:\n%s", e.InstallGuide)
	}
	return msg
}
