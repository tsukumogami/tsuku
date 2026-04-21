package actions

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunCommandAction implements command execution
type RunCommandAction struct{ BaseAction }

// RequiresNetwork returns true as a conservative default since arbitrary commands
// may need network access. Sandbox testing uses network=host for recipes with run_command.
func (RunCommandAction) RequiresNetwork() bool { return true }

// Name returns the action name
func (a *RunCommandAction) Name() string {
	return "run_command"
}

// Preflight validates parameters without side effects.
func (a *RunCommandAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}
	command, ok := GetString(params, "command")
	if !ok {
		result.AddError("run_command action requires 'command' parameter")
		return result
	}

	// WARNING: Hardcoded paths that should use variables
	tsukuPathPatterns := []string{
		"~/.tsuku",
		"$HOME/.tsuku",
		"${HOME}/.tsuku",
		".tsuku/tools/",
		".tsuku/bin/",
	}
	for _, pattern := range tsukuPathPatterns {
		if strings.Contains(command, pattern) {
			result.AddWarning("command contains hardcoded tsuku paths; consider using {install_dir}, {work_dir} variables")
			break
		}
	}

	return result
}

// Execute runs a shell command
//
// Parameters:
//   - command (required): Command to execute
//   - description (optional): Human-readable description
//   - working_dir (optional): Working directory (defaults to work_dir)
//   - requires_sudo (optional): Whether command requires sudo (default: false) - for validation, we skip these
func (a *RunCommandAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	reporter := ctx.GetReporter()

	// Get command (required)
	cmdPattern, ok := GetString(params, "command")
	if !ok {
		return fmt.Errorf("run_command action requires 'command' parameter")
	}

	// Get working_dir (optional)
	workingDir, _ := GetString(params, "working_dir")
	if workingDir == "" {
		workingDir = ctx.WorkDir
	}

	// Check if requires sudo
	requiresSudo, _ := GetBool(params, "requires_sudo")
	if requiresSudo {
		reporter.Log("   Skipping (requires sudo): %s", cmdPattern)
		return nil
	}

	// Build vars for substitution
	vars := GetStandardVars(ctx.Version, ctx.InstallDir, ctx.WorkDir, ctx.LibsDir)
	vars["binary"] = filepath.Join(ctx.InstallDir, "bin", ctx.Recipe.Metadata.Name)

	// Add {PYTHON} variable if python-standalone is installed (for pipx bootstrap)
	if pythonPath := ResolvePythonStandalone(); pythonPath != "" {
		vars["PYTHON"] = pythonPath
	}

	// Expand variables
	command := ExpandVars(cmdPattern, vars)
	workingDir = ExpandVars(workingDir, vars)

	reporter.Status(fmt.Sprintf("   Running: %s", command))

	// Execute command with context for cancellation support
	cmd := exec.CommandContext(ctx.Context, "sh", "-c", command)
	cmd.Dir = workingDir

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w\nOutput: %s", err, string(output))
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" {
		reporter.Log("   Output: %s", outputStr)
	}

	return nil
}
