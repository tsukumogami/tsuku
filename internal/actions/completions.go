package actions

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstallCompletionsAction copies a source file or runs a source command
// to produce shell completion scripts at
// $TSUKU_HOME/share/completions/{shell}/{target} for each configured shell.
// For zsh, the target is prefixed with _ per convention (e.g., _gh).
type InstallCompletionsAction struct{ BaseAction }

// Name returns the action name.
func (a *InstallCompletionsAction) Name() string {
	return "install_completions"
}

// IsDeterministic returns true -- copying and command execution are
// well-defined given the same inputs.
func (InstallCompletionsAction) IsDeterministic() bool { return true }

// Preflight validates parameters without side effects.
func (a *InstallCompletionsAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	_, hasSourceFile := GetString(params, "source_file")
	_, hasSourceCommand := GetString(params, "source_command")

	if hasSourceFile && hasSourceCommand {
		result.AddError("install_completions: 'source_file' and 'source_command' are mutually exclusive")
	} else if !hasSourceFile && !hasSourceCommand {
		result.AddError("install_completions requires 'source_file' or 'source_command' parameter")
	}

	if _, ok := GetString(params, "target"); !ok {
		result.AddError("install_completions requires 'target' parameter")
	}

	if shells, ok := GetStringSlice(params, "shells"); ok {
		for _, s := range shells {
			if !allowedShells[s] {
				result.AddErrorf("install_completions: invalid shell %q (allowed: bash, zsh, fish)", s)
			}
		}
	}

	return result
}

// Execute copies a source file or runs a source command for each shell,
// writing output to $TSUKU_HOME/share/completions/{shell}/.
//
// Parameters:
//   - source_file (string): path relative to the tool install dir (mutually exclusive with source_command)
//   - source_command (string): command template with {shell} and {install_dir} placeholders (mutually exclusive with source_file)
//   - target (required): name for the completion file (e.g., "gh")
//   - shells (optional): list of shells, defaults to ["bash", "zsh"]
func (a *InstallCompletionsAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	target, ok := GetString(params, "target")
	if !ok {
		return fmt.Errorf("install_completions requires 'target' parameter")
	}

	shells := defaultShells
	if s, ok := GetStringSlice(params, "shells"); ok && len(s) > 0 {
		for _, sh := range s {
			if !allowedShells[sh] {
				return fmt.Errorf("install_completions: invalid shell %q (allowed: bash, zsh, fish)", sh)
			}
		}
		shells = s
	}

	tsukuHome := filepath.Dir(ctx.ToolsDir)
	completionsBase := filepath.Join(tsukuHome, "share", "completions")

	sourceFile, hasSourceFile := GetString(params, "source_file")
	sourceCommand, hasSourceCommand := GetString(params, "source_command")

	if hasSourceFile && hasSourceCommand {
		return fmt.Errorf("install_completions: 'source_file' and 'source_command' are mutually exclusive")
	}
	if !hasSourceFile && !hasSourceCommand {
		return fmt.Errorf("install_completions requires 'source_file' or 'source_command' parameter")
	}

	if hasSourceFile {
		return a.executeSourceFile(ctx, sourceFile, target, shells, completionsBase)
	}
	return a.executeSourceCommand(ctx, sourceCommand, target, shells, completionsBase)
}

// completionFileName returns the filename for a completion target in a given shell.
// For zsh, names are prefixed with _ per convention.
func completionFileName(target, shell string) string {
	if shell == "zsh" {
		return "_" + target
	}
	return target
}

// recordCompletionCleanup appends a CleanupAction for a completion file.
func recordCompletionCleanup(ctx *ExecutionContext, target, shell, hash string) {
	fileName := completionFileName(target, shell)
	relPath := fmt.Sprintf("share/completions/%s/%s", shell, fileName)
	ctx.CleanupActions = append(ctx.CleanupActions, CleanupAction{
		Action:      "delete_file",
		Path:        relPath,
		ContentHash: hash,
	})
}

// executeSourceFile copies a file from the tool install dir to completions dir for each shell.
func (a *InstallCompletionsAction) executeSourceFile(ctx *ExecutionContext, sourceFile, target string, shells []string, completionsBase string) error {
	srcPath := filepath.Join(ctx.InstallDir, sourceFile)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("install_completions: source file not found: %s", srcPath)
	}

	for _, shell := range shells {
		shellDir := filepath.Join(completionsBase, shell)
		if err := os.MkdirAll(shellDir, 0700); err != nil {
			return fmt.Errorf("install_completions: failed to create completions directory %s: %w", shellDir, err)
		}
		if err := os.Chmod(shellDir, 0700); err != nil {
			return fmt.Errorf("install_completions: failed to set permissions on %s: %w", shellDir, err)
		}

		fileName := completionFileName(target, shell)
		destPath := filepath.Join(shellDir, fileName)
		if err := copyFile(srcPath, destPath); err != nil {
			return fmt.Errorf("install_completions: failed to copy to %s: %w", destPath, err)
		}
		if err := os.Chmod(destPath, 0600); err != nil {
			return fmt.Errorf("install_completions: failed to set permissions on %s: %w", destPath, err)
		}

		written, err := os.ReadFile(destPath)
		if err != nil {
			return fmt.Errorf("install_completions: failed to read back %s for hashing: %w", destPath, err)
		}
		hash := contentHash(written)
		ctx.GetReporter().Log("   Installed completion: %s", destPath)
		recordCompletionCleanup(ctx, target, shell, hash)
	}

	return nil
}

// executeSourceCommand runs a command template for each shell, writing stdout to completions dir.
func (a *InstallCompletionsAction) executeSourceCommand(ctx *ExecutionContext, sourceCommand, target string, shells []string, completionsBase string) error {
	if err := validateCommandBinary(sourceCommand, ctx.ToolInstallDir); err != nil {
		return fmt.Errorf("install_completions: %w", err)
	}

	for _, shell := range shells {
		shellDir := filepath.Join(completionsBase, shell)
		if err := os.MkdirAll(shellDir, 0700); err != nil {
			return fmt.Errorf("install_completions: failed to create completions directory %s: %w", shellDir, err)
		}
		if err := os.Chmod(shellDir, 0700); err != nil {
			return fmt.Errorf("install_completions: failed to set permissions on %s: %w", shellDir, err)
		}

		cmd := sourceCommand
		cmd = strings.ReplaceAll(cmd, "{shell}", shell)
		cmd = strings.ReplaceAll(cmd, "{install_dir}", ctx.ToolInstallDir)

		args := strings.Fields(cmd)
		if len(args) == 0 {
			return fmt.Errorf("install_completions: source_command is empty after substitution")
		}

		var stdout, stderr bytes.Buffer
		c := execCommandFunc(args[0], args[1:]...)
		c.Stdout = &stdout
		c.Stderr = &stderr

		err := c.Run()
		completionReporter := ctx.GetReporter()
		if stderr.Len() > 0 {
			completionReporter.Log("   [completions stderr] %s", stderr.String())
		}

		if err != nil {
			completionReporter.Warn("   Warning: source_command failed for shell %q: %v (skipping)", shell, err)
			continue
		}

		output := stdout.Bytes()
		if len(output) == 0 {
			completionReporter.Warn("   Warning: source_command produced empty output for shell %q (skipping)", shell)
			continue
		}

		fileName := completionFileName(target, shell)
		destPath := filepath.Join(shellDir, fileName)
		if err := os.WriteFile(destPath, output, 0600); err != nil {
			return fmt.Errorf("install_completions: failed to write %s: %w", destPath, err)
		}
		hash := contentHash(output)
		completionReporter.Log("   Installed completion: %s", destPath)
		recordCompletionCleanup(ctx, target, shell, hash)
	}

	return nil
}
