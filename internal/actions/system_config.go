package actions

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// GroupAddAction adds the current user to a system group.
// This action does not execute on the host - it provides structured data
// for documentation generation and sandbox container building.
type GroupAddAction struct{ BaseAction }

// Name returns the action name.
func (a *GroupAddAction) Name() string { return "group_add" }

// IsDeterministic returns true because adding a user to a group is idempotent.
func (a *GroupAddAction) IsDeterministic() bool { return true }

// Preflight validates parameters without side effects.
func (a *GroupAddAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	group, hasGroup := GetString(params, "group")
	if !hasGroup {
		result.AddError("group_add action requires 'group' parameter")
		return result
	}

	if group == "" {
		result.AddError("group_add action 'group' parameter cannot be empty")
		return result
	}

	// Validate group name: alphanumeric, underscore, hyphen; starts with letter or underscore
	if !isValidGroupName(group) {
		result.AddErrorf("group_add action 'group' parameter contains invalid characters: %s", group)
	}

	return result
}

// Execute displays what would be done (no side effects).
func (a *GroupAddAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	group, ok := GetString(params, "group")
	if !ok {
		return fmt.Errorf("group_add action requires 'group' parameter")
	}

	fmt.Printf("   Would add user to group: %s\n", group)
	fmt.Printf("   (Skipped - requires sudo and system modification)\n")
	return nil
}

// Validate checks that required parameters are present and valid.
func (a *GroupAddAction) Validate(params map[string]interface{}) error {
	group, ok := GetString(params, "group")
	if !ok || group == "" {
		return fmt.Errorf("group_add requires 'group' parameter")
	}
	return nil
}

// ImplicitConstraint returns nil since group_add works on all platforms.
func (a *GroupAddAction) ImplicitConstraint() *Constraint {
	return nil
}

// Describe returns a copy-pasteable usermod command.
func (a *GroupAddAction) Describe(params map[string]interface{}) string {
	group, ok := GetString(params, "group")
	if !ok || group == "" {
		return ""
	}
	return fmt.Sprintf("sudo usermod -aG %s $USER", group)
}

// IsExternallyManaged returns false because group_add does not delegate to a package manager.
func (a *GroupAddAction) IsExternallyManaged() bool { return false }

// ServiceEnableAction enables a systemd service.
// This action does not execute on the host - it provides structured data
// for documentation generation and sandbox container building.
type ServiceEnableAction struct{ BaseAction }

// Name returns the action name.
func (a *ServiceEnableAction) Name() string { return "service_enable" }

// IsDeterministic returns true because enabling a service is idempotent.
func (a *ServiceEnableAction) IsDeterministic() bool { return true }

// Preflight validates parameters without side effects.
func (a *ServiceEnableAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	service, hasService := GetString(params, "service")
	if !hasService {
		result.AddError("service_enable action requires 'service' parameter")
		return result
	}

	if service == "" {
		result.AddError("service_enable action 'service' parameter cannot be empty")
		return result
	}

	// Validate service name: alphanumeric, underscore, hyphen, @, dot
	if !isValidServiceName(service) {
		result.AddErrorf("service_enable action 'service' parameter contains invalid characters: %s", service)
	}

	return result
}

// Execute displays what would be done (no side effects).
func (a *ServiceEnableAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	service, ok := GetString(params, "service")
	if !ok {
		return fmt.Errorf("service_enable action requires 'service' parameter")
	}

	fmt.Printf("   Would enable service: %s\n", service)
	fmt.Printf("   (Skipped - requires sudo and system modification)\n")
	return nil
}

// Validate checks that required parameters are present and valid.
func (a *ServiceEnableAction) Validate(params map[string]interface{}) error {
	service, ok := GetString(params, "service")
	if !ok || service == "" {
		return fmt.Errorf("service_enable requires 'service' parameter")
	}
	return nil
}

// ImplicitConstraint returns nil since service_enable works on all platforms with systemd.
func (a *ServiceEnableAction) ImplicitConstraint() *Constraint {
	return nil
}

// Describe returns a copy-pasteable systemctl enable command.
func (a *ServiceEnableAction) Describe(params map[string]interface{}) string {
	service, ok := GetString(params, "service")
	if !ok || service == "" {
		return ""
	}
	return fmt.Sprintf("sudo systemctl enable %s", service)
}

// IsExternallyManaged returns false because service_enable does not delegate to a package manager.
func (a *ServiceEnableAction) IsExternallyManaged() bool { return false }

// ServiceStartAction starts a systemd service.
// This action does not execute on the host - it provides structured data
// for documentation generation and sandbox container building.
type ServiceStartAction struct{ BaseAction }

// Name returns the action name.
func (a *ServiceStartAction) Name() string { return "service_start" }

// IsDeterministic returns true because starting a service is idempotent.
func (a *ServiceStartAction) IsDeterministic() bool { return true }

// Preflight validates parameters without side effects.
func (a *ServiceStartAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	service, hasService := GetString(params, "service")
	if !hasService {
		result.AddError("service_start action requires 'service' parameter")
		return result
	}

	if service == "" {
		result.AddError("service_start action 'service' parameter cannot be empty")
		return result
	}

	// Validate service name: alphanumeric, underscore, hyphen, @, dot
	if !isValidServiceName(service) {
		result.AddErrorf("service_start action 'service' parameter contains invalid characters: %s", service)
	}

	return result
}

// Execute displays what would be done (no side effects).
func (a *ServiceStartAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	service, ok := GetString(params, "service")
	if !ok {
		return fmt.Errorf("service_start action requires 'service' parameter")
	}

	fmt.Printf("   Would start service: %s\n", service)
	fmt.Printf("   (Skipped - requires sudo and system modification)\n")
	return nil
}

// Validate checks that required parameters are present and valid.
func (a *ServiceStartAction) Validate(params map[string]interface{}) error {
	service, ok := GetString(params, "service")
	if !ok || service == "" {
		return fmt.Errorf("service_start requires 'service' parameter")
	}
	return nil
}

// ImplicitConstraint returns nil since service_start works on all platforms with systemd.
func (a *ServiceStartAction) ImplicitConstraint() *Constraint {
	return nil
}

// Describe returns a copy-pasteable systemctl start command.
func (a *ServiceStartAction) Describe(params map[string]interface{}) string {
	service, ok := GetString(params, "service")
	if !ok || service == "" {
		return ""
	}
	return fmt.Sprintf("sudo systemctl start %s", service)
}

// IsExternallyManaged returns false because service_start does not delegate to a package manager.
func (a *ServiceStartAction) IsExternallyManaged() bool { return false }

// RequireCommandAction verifies that a command exists in PATH.
// Optionally checks that the command meets a minimum version requirement.
type RequireCommandAction struct{ BaseAction }

// Name returns the action name.
func (a *RequireCommandAction) Name() string { return "require_command" }

// IsDeterministic returns true because command verification produces identical results.
func (a *RequireCommandAction) IsDeterministic() bool { return true }

// Preflight validates parameters without side effects.
func (a *RequireCommandAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	command, hasCommand := GetString(params, "command")
	if !hasCommand {
		result.AddError("require_command action requires 'command' parameter")
		return result
	}

	if command == "" {
		result.AddError("require_command action 'command' parameter cannot be empty")
		return result
	}

	// Validate command name: no path separators or shell metacharacters
	if !isValidCommandName(command) {
		result.AddErrorf("require_command action 'command' parameter contains invalid characters: %s", command)
	}

	// If min_version is specified, version_flag and version_regex are required
	minVersion, hasMinVersion := GetString(params, "min_version")
	if hasMinVersion && minVersion != "" {
		_, hasVersionFlag := GetString(params, "version_flag")
		_, hasVersionRegex := GetString(params, "version_regex")

		if !hasVersionFlag {
			result.AddError("require_command action with 'min_version' requires 'version_flag' parameter")
		}
		if !hasVersionRegex {
			result.AddError("require_command action with 'min_version' requires 'version_regex' parameter")
		}
	}

	// Validate version_regex if provided
	if versionRegex, hasVersionRegex := GetString(params, "version_regex"); hasVersionRegex && versionRegex != "" {
		if _, err := regexp.Compile(versionRegex); err != nil {
			result.AddErrorf("require_command action 'version_regex' is invalid: %v", err)
		}
	}

	return result
}

// Execute checks if the command exists and optionally verifies its version.
func (a *RequireCommandAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	command, ok := GetString(params, "command")
	if !ok {
		return fmt.Errorf("require_command action requires 'command' parameter")
	}

	// Check if command exists in PATH
	path, err := exec.LookPath(command)
	if err != nil {
		return fmt.Errorf("command '%s' not found in PATH", command)
	}

	fmt.Printf("   Found command: %s (%s)\n", command, path)

	// Check version if min_version is specified
	minVersion, hasMinVersion := GetString(params, "min_version")
	if hasMinVersion && minVersion != "" {
		versionFlag, _ := GetString(params, "version_flag")
		versionRegex, _ := GetString(params, "version_regex")

		if versionFlag == "" || versionRegex == "" {
			return fmt.Errorf("require_command with min_version requires version_flag and version_regex")
		}

		// Run command with version flag
		cmd := exec.CommandContext(ctx.Context, command, versionFlag)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to get version from '%s %s': %v", command, versionFlag, err)
		}

		// Extract version using regex
		re, err := regexp.Compile(versionRegex)
		if err != nil {
			return fmt.Errorf("invalid version_regex: %v", err)
		}

		matches := re.FindStringSubmatch(string(output))
		if len(matches) < 2 {
			return fmt.Errorf("could not extract version from output using regex '%s'", versionRegex)
		}

		detectedVersion := matches[1]
		fmt.Printf("   Detected version: %s (minimum: %s)\n", detectedVersion, minVersion)

		// Simple version comparison (could be enhanced for semver)
		if !versionMeetsMinimum(detectedVersion, minVersion) {
			return fmt.Errorf("command '%s' version %s does not meet minimum %s", command, detectedVersion, minVersion)
		}
	}

	fmt.Printf("   ✓ Command '%s' verified\n", command)
	return nil
}

// Validate checks that required parameters are present and valid.
func (a *RequireCommandAction) Validate(params map[string]interface{}) error {
	command, ok := GetString(params, "command")
	if !ok || command == "" {
		return fmt.Errorf("require_command requires 'command' parameter")
	}
	return nil
}

// ImplicitConstraint returns nil since require_command works on all platforms.
func (a *RequireCommandAction) ImplicitConstraint() *Constraint {
	return nil
}

// Describe returns an informational message about the command requirement.
func (a *RequireCommandAction) Describe(params map[string]interface{}) string {
	command, ok := GetString(params, "command")
	if !ok || command == "" {
		return ""
	}
	minVersion, hasMinVersion := GetString(params, "min_version")
	if hasMinVersion && minVersion != "" {
		return fmt.Sprintf("Requires: %s (version >= %s)", command, minVersion)
	}
	return fmt.Sprintf("Requires: %s", command)
}

// IsExternallyManaged returns false because require_command does not delegate to a package manager.
func (a *RequireCommandAction) IsExternallyManaged() bool { return false }

// ManualAction displays instructions for manual installation.
// This action is used when automation is not possible or not desired.
type ManualAction struct{ BaseAction }

// Name returns the action name.
func (a *ManualAction) Name() string { return "manual" }

// IsDeterministic returns true because displaying text is deterministic.
func (a *ManualAction) IsDeterministic() bool { return true }

// Preflight validates parameters without side effects.
func (a *ManualAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	text, hasText := GetString(params, "text")
	if !hasText {
		result.AddError("manual action requires 'text' parameter")
		return result
	}

	if text == "" {
		result.AddError("manual action 'text' parameter cannot be empty")
	}

	return result
}

// Execute displays the manual installation instructions.
func (a *ManualAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	text, ok := GetString(params, "text")
	if !ok {
		return fmt.Errorf("manual action requires 'text' parameter")
	}

	fmt.Printf("\n   ╭─ Manual Installation Required ─────────────────────────────╮\n")
	for _, line := range strings.Split(text, "\n") {
		fmt.Printf("   │ %s\n", line)
	}
	fmt.Printf("   ╰───────────────────────────────────────────────────────────╯\n\n")

	return nil
}

// Validate checks that required parameters are present and valid.
func (a *ManualAction) Validate(params map[string]interface{}) error {
	text, ok := GetString(params, "text")
	if !ok || text == "" {
		return fmt.Errorf("manual requires 'text' parameter")
	}
	return nil
}

// ImplicitConstraint returns nil since manual works on all platforms.
func (a *ManualAction) ImplicitConstraint() *Constraint {
	return nil
}

// Describe returns the manual instruction text directly.
func (a *ManualAction) Describe(params map[string]interface{}) string {
	text, ok := GetString(params, "text")
	if !ok {
		return ""
	}
	return text
}

// IsExternallyManaged returns false because manual does not delegate to a package manager.
func (a *ManualAction) IsExternallyManaged() bool { return false }

// isValidGroupName checks if a group name is valid.
// Valid names: start with letter or underscore, contain only alphanumeric, underscore, hyphen.
func isValidGroupName(name string) bool {
	if len(name) == 0 {
		return false
	}
	// Must start with letter or underscore
	first := name[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return false
	}
	// Rest can be alphanumeric, underscore, or hyphen
	for _, c := range name[1:] {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// isValidServiceName checks if a service name is valid.
// Valid names: alphanumeric, underscore, hyphen, @, dot
func isValidServiceName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '@' || c == '.') {
			return false
		}
	}
	return true
}

// isValidCommandName checks if a command name is valid.
// Invalid characters: path separators, shell metacharacters
func isValidCommandName(name string) bool {
	if len(name) == 0 {
		return false
	}
	// No path separators
	if strings.ContainsAny(name, "/\\") {
		return false
	}
	// No shell metacharacters
	if strings.ContainsAny(name, "|&;$`\"'<>(){}[]!*?~") {
		return false
	}
	// No spaces or control characters
	for _, c := range name {
		if c <= 32 || c == 127 {
			return false
		}
	}
	return true
}

// versionMeetsMinimum performs a simple version comparison.
// Returns true if detected >= minimum.
// This is a basic comparison that works for simple version strings.
func versionMeetsMinimum(detected, minimum string) bool {
	// Strip common prefixes
	detected = strings.TrimPrefix(detected, "v")
	minimum = strings.TrimPrefix(minimum, "v")

	// Simple string comparison for now
	// A more robust implementation would use semver parsing
	detectedParts := strings.Split(detected, ".")
	minimumParts := strings.Split(minimum, ".")

	for i := 0; i < len(minimumParts); i++ {
		if i >= len(detectedParts) {
			return false
		}

		// Try numeric comparison
		var detNum, minNum int
		if _, err := fmt.Sscanf(detectedParts[i], "%d", &detNum); err == nil {
			if _, err := fmt.Sscanf(minimumParts[i], "%d", &minNum); err == nil {
				if detNum < minNum {
					return false
				}
				if detNum > minNum {
					return true
				}
				continue
			}
		}

		// Fall back to string comparison
		if detectedParts[i] < minimumParts[i] {
			return false
		}
		if detectedParts[i] > minimumParts[i] {
			return true
		}
	}

	return true
}
