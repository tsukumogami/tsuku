package actions

import (
	"fmt"
	"os"
	"path/filepath"
)

// allowedShells is the hardcoded allowlist for shell values.
// Arbitrary strings are rejected to prevent template injection.
var allowedShells = map[string]bool{
	"bash": true,
	"zsh":  true,
	"fish": true,
}

// defaultShells is used when the "shells" parameter is omitted.
var defaultShells = []string{"bash", "zsh"}

// InstallShellInitAction copies a source file from the tool install directory
// to $TSUKU_HOME/share/shell.d/{target}.{shell} for each configured shell.
type InstallShellInitAction struct{ BaseAction }

// Name returns the action name.
func (a *InstallShellInitAction) Name() string {
	return "install_shell_init"
}

// IsDeterministic returns true because copying a file is deterministic.
func (InstallShellInitAction) IsDeterministic() bool { return true }

// Preflight validates parameters without side effects.
func (a *InstallShellInitAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	// source_file is required (source_command is issue #2203)
	if _, ok := GetString(params, "source_file"); !ok {
		result.AddError("install_shell_init requires 'source_file' parameter")
	}

	// target is required
	if _, ok := GetString(params, "target"); !ok {
		result.AddError("install_shell_init requires 'target' parameter")
	}

	// Validate shells if provided
	if shells, ok := GetStringSlice(params, "shells"); ok {
		for _, s := range shells {
			if !allowedShells[s] {
				result.AddErrorf("install_shell_init: invalid shell %q (allowed: bash, zsh, fish)", s)
			}
		}
	}

	return result
}

// Execute copies the source file to shell.d for each shell.
//
// Parameters:
//   - source_file (required): path relative to the tool install dir
//   - target (required): name for the shell.d file (e.g., "niwa")
//   - shells (optional): list of shells, defaults to ["bash", "zsh"]
func (a *InstallShellInitAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	sourceFile, ok := GetString(params, "source_file")
	if !ok {
		return fmt.Errorf("install_shell_init requires 'source_file' parameter")
	}

	target, ok := GetString(params, "target")
	if !ok {
		return fmt.Errorf("install_shell_init requires 'target' parameter")
	}

	shells := defaultShells
	if s, ok := GetStringSlice(params, "shells"); ok && len(s) > 0 {
		// Validate against allowlist
		for _, sh := range s {
			if !allowedShells[sh] {
				return fmt.Errorf("install_shell_init: invalid shell %q (allowed: bash, zsh, fish)", sh)
			}
		}
		shells = s
	}

	// Resolve source file path relative to tool install dir
	srcPath := filepath.Join(ctx.InstallDir, sourceFile)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("install_shell_init: source file not found: %s", srcPath)
	}

	// Determine shell.d directory: $TSUKU_HOME/share/shell.d/
	// Derive $TSUKU_HOME from ToolsDir (which is $TSUKU_HOME/tools)
	tsukuHome := filepath.Dir(ctx.ToolsDir)
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")

	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		return fmt.Errorf("install_shell_init: failed to create shell.d directory: %w", err)
	}

	for _, shell := range shells {
		destPath := filepath.Join(shellDDir, fmt.Sprintf("%s.%s", target, shell))
		if err := copyFile(srcPath, destPath); err != nil {
			return fmt.Errorf("install_shell_init: failed to copy to %s: %w", destPath, err)
		}
		fmt.Printf("   Installed shell init: %s\n", destPath)
	}

	return nil
}
