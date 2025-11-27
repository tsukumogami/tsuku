package actions

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunCommandAction implements command execution
type RunCommandAction struct{}

// Name returns the action name
func (a *RunCommandAction) Name() string {
	return "run_command"
}

// Execute runs a shell command
//
// Parameters:
//   - command (required): Command to execute
//   - description (optional): Human-readable description
//   - working_dir (optional): Working directory (defaults to work_dir)
//   - requires_sudo (optional): Whether command requires sudo (default: false) - for validation, we skip these
func (a *RunCommandAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get command (required)
	cmdPattern, ok := GetString(params, "command")
	if !ok {
		return fmt.Errorf("run_command action requires 'command' parameter")
	}

	// Get description (optional)
	description, _ := GetString(params, "description")

	// Get working_dir (optional)
	workingDir, _ := GetString(params, "working_dir")
	if workingDir == "" {
		workingDir = ctx.WorkDir
	}

	// Check if requires sudo
	requiresSudo, _ := GetBool(params, "requires_sudo")
	if requiresSudo {
		fmt.Printf("   Skipping (requires sudo): %s\n", cmdPattern)
		if description != "" {
			fmt.Printf("   Description: %s\n", description)
		}
		return nil
	}

	// Build vars for substitution
	vars := GetStandardVars(ctx.Version, ctx.InstallDir, ctx.WorkDir)
	vars["binary"] = filepath.Join(ctx.InstallDir, "bin", ctx.Recipe.Metadata.Name)

	// Add {PYTHON} variable if python-standalone is installed (for pipx bootstrap)
	if pythonPath := ResolvePythonStandalone(); pythonPath != "" {
		vars["PYTHON"] = pythonPath
	}

	// Expand variables
	command := ExpandVars(cmdPattern, vars)
	workingDir = ExpandVars(workingDir, vars)

	if description != "" {
		fmt.Printf("   Description: %s\n", description)
	}
	fmt.Printf("   Running: %s\n", command)
	if workingDir != ctx.WorkDir {
		fmt.Printf("   Working dir: %s\n", workingDir)
	}

	// Execute command
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = workingDir

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w\nOutput: %s", err, string(output))
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" {
		fmt.Printf("   Output: %s\n", outputStr)
	}

	fmt.Printf("   ✓ Command executed successfully\n")
	return nil
}
