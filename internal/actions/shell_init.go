package actions

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// execCommandFunc is the function used to create exec.Cmd. Override in tests.
var execCommandFunc = exec.Command

// InstallShellInitAction copies a source file or runs a source command
// to produce shell initialization scripts at
// $TSUKU_HOME/share/shell.d/{target}.{shell} for each configured shell.
type InstallShellInitAction struct{ BaseAction }

// Name returns the action name.
func (a *InstallShellInitAction) Name() string {
	return "install_shell_init"
}

// IsDeterministic returns true for source_file (copying is deterministic),
// but source_command output may vary. We return true since the action itself
// is well-defined.
func (InstallShellInitAction) IsDeterministic() bool { return true }

// Preflight validates parameters without side effects.
func (a *InstallShellInitAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	_, hasSourceFile := GetString(params, "source_file")
	_, hasSourceCommand := GetString(params, "source_command")

	// Exactly one of source_file or source_command is required
	if hasSourceFile && hasSourceCommand {
		result.AddError("install_shell_init: 'source_file' and 'source_command' are mutually exclusive")
	} else if !hasSourceFile && !hasSourceCommand {
		result.AddError("install_shell_init requires 'source_file' or 'source_command' parameter")
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

// Execute copies a source file or runs a source command for each shell,
// writing output to shell.d.
//
// Parameters:
//   - source_file (string): path relative to the tool install dir (mutually exclusive with source_command)
//   - source_command (string): command template with {shell} and {install_dir} placeholders (mutually exclusive with source_file)
//   - target (required): name for the shell.d file (e.g., "niwa")
//   - shells (optional): list of shells, defaults to ["bash", "zsh"]
func (a *InstallShellInitAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	target, ok := GetString(params, "target")
	if !ok {
		return fmt.Errorf("install_shell_init requires 'target' parameter")
	}

	shells := defaultShells
	if s, ok := GetStringSlice(params, "shells"); ok && len(s) > 0 {
		for _, sh := range s {
			if !allowedShells[sh] {
				return fmt.Errorf("install_shell_init: invalid shell %q (allowed: bash, zsh, fish)", sh)
			}
		}
		shells = s
	}

	// Determine shell.d directory: $TSUKU_HOME/share/shell.d/
	// Derive $TSUKU_HOME from ToolsDir (which is $TSUKU_HOME/tools)
	tsukuHome := filepath.Dir(ctx.ToolsDir)
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")

	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		return fmt.Errorf("install_shell_init: failed to create shell.d directory: %w", err)
	}

	sourceFile, hasSourceFile := GetString(params, "source_file")
	sourceCommand, hasSourceCommand := GetString(params, "source_command")

	if hasSourceFile && hasSourceCommand {
		return fmt.Errorf("install_shell_init: 'source_file' and 'source_command' are mutually exclusive")
	}
	if !hasSourceFile && !hasSourceCommand {
		return fmt.Errorf("install_shell_init requires 'source_file' or 'source_command' parameter")
	}

	if hasSourceFile {
		return a.executeSourceFile(ctx, sourceFile, target, shells, shellDDir)
	}
	return a.executeSourceCommand(ctx, sourceCommand, target, shells, shellDDir)
}

// executeSourceFile copies a file from the tool install dir to shell.d for each shell.
func (a *InstallShellInitAction) executeSourceFile(ctx *ExecutionContext, sourceFile, target string, shells []string, shellDDir string) error {
	srcPath := filepath.Join(ctx.InstallDir, sourceFile)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("install_shell_init: source file not found: %s", srcPath)
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

// executeSourceCommand runs a command template for each shell, writing stdout to shell.d.
func (a *InstallShellInitAction) executeSourceCommand(ctx *ExecutionContext, sourceCommand, target string, shells []string, shellDDir string) error {
	// Validate the executable is within ToolInstallDir before running anything.
	// Use the raw template (before shell substitution) to find the binary,
	// since {shell} only appears in arguments, not the executable name.
	if err := validateCommandBinary(sourceCommand, ctx.ToolInstallDir); err != nil {
		return fmt.Errorf("install_shell_init: %w", err)
	}

	for _, shell := range shells {
		// Substitute placeholders
		cmd := sourceCommand
		cmd = strings.ReplaceAll(cmd, "{shell}", shell)
		cmd = strings.ReplaceAll(cmd, "{install_dir}", ctx.ToolInstallDir)

		// Split on spaces for exec.Command (no shell interpretation)
		args := strings.Fields(cmd)
		if len(args) == 0 {
			return fmt.Errorf("install_shell_init: source_command is empty after substitution")
		}

		var stdout, stderr bytes.Buffer
		c := execCommandFunc(args[0], args[1:]...)
		c.Stdout = &stdout
		c.Stderr = &stderr

		err := c.Run()
		if stderr.Len() > 0 {
			fmt.Printf("   [shell_init stderr] %s\n", stderr.String())
		}

		if err != nil {
			// Non-zero exit: log warning and skip this shell
			fmt.Printf("   Warning: source_command failed for shell %q: %v (skipping)\n", shell, err)
			continue
		}

		// Empty output: skip file creation
		output := stdout.Bytes()
		if len(output) == 0 {
			fmt.Printf("   Warning: source_command produced empty output for shell %q (skipping)\n", shell)
			continue
		}

		destPath := filepath.Join(shellDDir, fmt.Sprintf("%s.%s", target, shell))
		if err := os.WriteFile(destPath, output, 0644); err != nil {
			return fmt.Errorf("install_shell_init: failed to write %s: %w", destPath, err)
		}
		fmt.Printf("   Installed shell init: %s\n", destPath)
	}

	return nil
}

// validateCommandBinary checks that the executable in the command template
// resolves to a path within toolInstallDir. Symlinks are resolved before
// the containment check.
func validateCommandBinary(commandTemplate, toolInstallDir string) error {
	if toolInstallDir == "" {
		return fmt.Errorf("source_command requires ToolInstallDir to be set in ExecutionContext")
	}

	// Extract the executable (first token). We don't substitute {shell}
	// here because it only appears in arguments -- but we do substitute
	// {install_dir} since it may be part of the binary path.
	cmd := strings.ReplaceAll(commandTemplate, "{install_dir}", toolInstallDir)
	args := strings.Fields(cmd)
	if len(args) == 0 {
		return fmt.Errorf("source_command is empty")
	}

	binaryPath := args[0]

	// If the path is not absolute, resolve it relative to toolInstallDir
	if !filepath.IsAbs(binaryPath) {
		binaryPath = filepath.Join(toolInstallDir, binaryPath)
	}

	// Resolve symlinks
	resolved, err := filepath.EvalSymlinks(binaryPath)
	if err != nil {
		return fmt.Errorf("cannot resolve executable %q: %w", args[0], err)
	}

	// Resolve the toolInstallDir too (it might contain symlinks)
	resolvedDir, err := filepath.EvalSymlinks(toolInstallDir)
	if err != nil {
		return fmt.Errorf("cannot resolve ToolInstallDir %q: %w", toolInstallDir, err)
	}

	// Containment check: resolved binary must be under resolved toolInstallDir
	relPath, err := filepath.Rel(resolvedDir, resolved)
	if err != nil {
		return fmt.Errorf("executable %q is not within ToolInstallDir", args[0])
	}
	if strings.HasPrefix(relPath, "..") {
		return fmt.Errorf("executable %q resolves to %s, which is outside ToolInstallDir %s", args[0], resolved, resolvedDir)
	}

	return nil
}
