package actions

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// shellInitShells is the hardcoded allowlist for install_shell_init's shell
// values. Arbitrary strings are rejected to prevent template injection.
//
// fish is absent for the same reason it is absent from set_env's envShells:
// fragments only reach a shell through the init cache, and the file that
// sources that cache -- config.EnvFileContent, written to $TSUKU_HOME/env --
// is POSIX shell that fish cannot parse. A fish fragment would be written,
// hashed, and never loaded. Accepting the value would advertise support the
// delivery path does not have, so it is rejected at validation instead.
var shellInitShells = map[string]bool{
	"bash": true,
	"zsh":  true,
}

// shellInitShellError explains a rejected shell value. fish gets a message
// naming what does work, because a recipe author reaching for it has a real
// need that other parts of tsuku do serve; anything else is a typo and only
// needs the allowed set.
func shellInitShellError(shell string) string {
	if shell == "fish" {
		return fmt.Sprintf("install_shell_init: shell %q is not supported (allowed: bash, zsh) -- "+
			"init fragments load from the cache that $TSUKU_HOME/env sources, which is POSIX shell fish cannot parse. "+
			"fish users get tsuku on PATH with 'tsuku shellenv | source', and install_completions still accepts fish", shell)
	}
	return fmt.Sprintf("install_shell_init: invalid shell %q (allowed: bash, zsh)", shell)
}

// shellDTargetPattern constrains the target segment of a shell.d filename.
//
// Two properties depend on it. Excluding "@" makes (target, version, shell) ->
// filename injective: the first "@" in a filename is always the version
// separator, so no tool can write over another tool's fragment. Requiring a
// leading [A-Za-z_] (>= 0x41) keeps every init filename sorted after every
// exports filename, which begins with "0" (0x30) -- the ordering the
// EnvFilePrefix exists to guarantee, now holding without case analysis.
var shellDTargetPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9._-]*$`)

// validateShellDTarget rejects a target that would break filename injectivity
// or the exports-before-init sort order.
func validateShellDTarget(target string) error {
	if !shellDTargetPattern.MatchString(target) {
		return fmt.Errorf("target %q must match %s", target, shellDTargetPattern)
	}
	return nil
}

// defaultShells is used when the "shells" parameter is omitted.
var defaultShells = []string{"bash", "zsh"}

// execCommandFunc is the function used to create exec.Cmd. Override in tests.
var execCommandFunc = exec.Command

// InstallShellInitAction copies a source file or runs a source command
// to produce shell initialization scripts at
// $TSUKU_HOME/share/shell.d/{target}@{version}.{shell} for each configured
// shell. The version key is what keeps a second version's install from
// overwriting the first version's fragment.
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
	if target, ok := GetString(params, "target"); !ok {
		result.AddError("install_shell_init requires 'target' parameter")
	} else if err := validateShellDTarget(target); err != nil {
		// The charset rule subsumes the older "may not claim the 00-env-
		// prefix" rejection: a leading digit is illegal, so no target can
		// sort ahead of the exports set_env writes.
		result.AddErrorf("install_shell_init: %v", err)
	}

	// Validate shells if provided
	if shells, ok := GetStringSlice(params, "shells"); ok {
		for _, s := range shells {
			if !shellInitShells[s] {
				result.AddError(shellInitShellError(s))
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
	// When --no-shell-init is set, skip shell init entirely.
	// No files are written and no CleanupActions are recorded.
	reporter := ctx.GetReporter()
	if ctx.NoShellInit {
		reporter.Log("   Skipping shell init (--no-shell-init)")
		return nil
	}

	target, ok := GetString(params, "target")
	if !ok {
		return fmt.Errorf("install_shell_init requires 'target' parameter")
	}
	if err := validateShellDTarget(target); err != nil {
		return fmt.Errorf("install_shell_init: %w", err)
	}

	version, err := shellDVersion(ctx)
	if err != nil {
		return fmt.Errorf("install_shell_init: %w", err)
	}

	shells := defaultShells
	if s, ok := GetStringSlice(params, "shells"); ok && len(s) > 0 {
		for _, sh := range s {
			if !shellInitShells[sh] {
				return errors.New(shellInitShellError(sh))
			}
		}
		shells = s
	}

	// Determine shell.d directory: $TSUKU_HOME/share/shell.d/
	// Derive $TSUKU_HOME from ToolsDir (which is $TSUKU_HOME/tools)
	tsukuHome := filepath.Dir(ctx.ToolsDir)
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")

	if err := os.MkdirAll(shellDDir, 0700); err != nil {
		return fmt.Errorf("install_shell_init: failed to create shell.d directory: %w", err)
	}
	// Ensure restrictive permissions even if directory already existed
	if err := os.Chmod(shellDDir, 0700); err != nil {
		return fmt.Errorf("install_shell_init: failed to set permissions on shell.d directory: %w", err)
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
		return a.executeSourceFile(ctx, sourceFile, target, version, shells, shellDDir)
	}
	return a.executeSourceCommand(ctx, sourceCommand, target, version, shells, shellDDir)
}

// contentHash computes the SHA-256 hex digest of the given data.
func contentHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// shellDFileName is the basename of a shell.d fragment. Every version gets its
// own file, so nothing a version writes is ever reachable from another version
// and a recorded ContentHash stays accurate for as long as the file exists.
func shellDFileName(target, version, shell string) string {
	return fmt.Sprintf("%s@%s.%s", target, version, shell)
}

// shellDCleanupPath is the $TSUKU_HOME-relative path recorded for a shell.d
// file. It is the single place that formula lives; anything matching against
// recorded cleanup paths must build them here. Both writers -- set_env and
// install_shell_init -- route their destination through it, so the path that
// is written and the path that is recorded cannot drift apart.
func shellDCleanupPath(target, version, shell string) string {
	return "share/shell.d/" + shellDFileName(target, version, shell)
}

// shellDVersion returns the version to key this install's shell.d files on.
// An empty version would collapse every version onto one filename, which is
// the bug the version key exists to remove, so it fails rather than guessing.
func shellDVersion(ctx *ExecutionContext) (string, error) {
	if ctx.Version == "" {
		return "", fmt.Errorf("the tool version is not known, so the shell.d filename cannot be version-keyed")
	}
	return ctx.Version, nil
}

// recordCleanup appends a CleanupAction for a shell.d file to the execution context.
// The path is stored relative to $TSUKU_HOME. The hash is the SHA-256 of the file content.
func recordCleanup(ctx *ExecutionContext, target, version, shell, hash string) {
	ctx.CleanupActions = append(ctx.CleanupActions, CleanupAction{
		Action:      "delete_file",
		Path:        shellDCleanupPath(target, version, shell),
		ContentHash: hash,
	})
}

// cleanupRecorded reports whether this execution already recorded a cleanup
// action for a shell.d file, which means the file on disk is this install's own
// output rather than a leftover from a previous version.
func cleanupRecorded(ctx *ExecutionContext, target, version, shell string) bool {
	relPath := shellDCleanupPath(target, version, shell)
	for _, ca := range ctx.CleanupActions {
		if ca.Path == relPath {
			return true
		}
	}
	return false
}

// upsertCleanup records a cleanup action for a shell.d file, refreshing the
// content hash when this execution already recorded that path. Only actions
// that may write the same path more than once in a single install need this;
// the append-only recordCleanup is right for everything else.
func upsertCleanup(ctx *ExecutionContext, target, version, shell, hash string) {
	relPath := shellDCleanupPath(target, version, shell)
	for i := range ctx.CleanupActions {
		if ctx.CleanupActions[i].Path == relPath {
			ctx.CleanupActions[i].ContentHash = hash
			return
		}
	}
	ctx.CleanupActions = append(ctx.CleanupActions, CleanupAction{
		Action:      "delete_file",
		Path:        relPath,
		ContentHash: hash,
	})
}

// executeSourceFile copies a file from the tool install dir to shell.d for each shell.
func (a *InstallShellInitAction) executeSourceFile(ctx *ExecutionContext, sourceFile, target, version string, shells []string, shellDDir string) error {
	srcPath := filepath.Join(ctx.InstallDir, sourceFile)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return fmt.Errorf("install_shell_init: source file not found: %s", srcPath)
	}

	for _, shell := range shells {
		destPath := filepath.Join(shellDDir, shellDFileName(target, version, shell))
		if err := copyFile(srcPath, destPath); err != nil {
			return fmt.Errorf("install_shell_init: failed to copy to %s: %w", destPath, err)
		}
		// Set restrictive permissions on the shell.d file
		if err := os.Chmod(destPath, 0600); err != nil {
			return fmt.Errorf("install_shell_init: failed to set permissions on %s: %w", destPath, err)
		}
		// Compute content hash for integrity verification
		written, err := os.ReadFile(destPath)
		if err != nil {
			return fmt.Errorf("install_shell_init: failed to read back %s for hashing: %w", destPath, err)
		}
		hash := contentHash(written)
		ctx.GetReporter().Status(fmt.Sprintf("   Installed shell init: %s", destPath))
		recordCleanup(ctx, target, version, shell, hash)
	}

	return nil
}

// executeSourceCommand runs a command template for each shell, writing stdout to shell.d.
func (a *InstallShellInitAction) executeSourceCommand(ctx *ExecutionContext, sourceCommand, target, version string, shells []string, shellDDir string) error {
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
		shellReporter := ctx.GetReporter()
		if stderr.Len() > 0 {
			shellReporter.Log("   [shell_init stderr] %s", stderr.String())
		}

		if err != nil {
			// Non-zero exit: log warning and skip this shell
			shellReporter.Warn("   Warning: source_command failed for shell %q: %v (skipping)", shell, err)
			continue
		}

		// Empty output: skip file creation
		output := stdout.Bytes()
		if len(output) == 0 {
			shellReporter.Warn("   Warning: source_command produced empty output for shell %q (skipping)", shell)
			continue
		}

		destPath := filepath.Join(shellDDir, shellDFileName(target, version, shell))
		if err := os.WriteFile(destPath, output, 0600); err != nil {
			return fmt.Errorf("install_shell_init: failed to write %s: %w", destPath, err)
		}
		hash := contentHash(output)
		shellReporter.Status(fmt.Sprintf("   Installed shell init: %s", destPath))
		recordCleanup(ctx, target, version, shell, hash)
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
