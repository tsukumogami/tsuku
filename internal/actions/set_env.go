package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// envVarNamePattern is the set of names accepted by set_env. The generated file
// is sourced by the user's shell, so anything outside a plain identifier is
// rejected rather than escaped.
var envVarNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// envShells are the shells set_env writes exports for. These are the two shells
// $TSUKU_HOME/env sources (see config.EnvFileContent) and the two that share
// `export NAME=value` syntax. fish would need `set -gx` and is not supported.
var envShells = []string{"bash", "zsh"}

// EnvFilePrefix is reserved for the shell.d files set_env writes. The shell
// cache concatenates share/shell.d in alphabetical order, so this prefix is
// what keeps a recipe's exports ahead of that tool's own init script. No other
// action may claim it -- see the sort in shellenv.RebuildShellCache.
const EnvFilePrefix = "00-env-"

// SetEnvAction exports environment variables into the user's shell by writing
// $TSUKU_HOME/share/shell.d/00-env-{tool}@{version}.{shell}, which
// RebuildShellCache concatenates into the init cache that $TSUKU_HOME/env
// sources. The version key gives each installed version its own file, so a
// second install never overwrites the first version's exports.
//
// The "00-env-" prefix is load-bearing: the cache builder concatenates shell.d
// in alphabetical order, and a tool's own init script often reads the variables
// its recipe exports (nvm.sh only honors NVM_DIR when it is already set). The
// numeric prefix keeps exports ahead of every plausible init filename.
type SetEnvAction struct{ BaseAction }

// IsDeterministic returns true because set_env produces identical results.
func (SetEnvAction) IsDeterministic() bool { return true }

// Name returns the action name
func (a *SetEnvAction) Name() string {
	return "set_env"
}

// DefaultPhase puts set_env in the post-install phase when the recipe does not
// name one. {install_dir} has to expand to ToolInstallDir, and that is only
// populated once the tool reaches its permanent directory; during the install
// phase it is empty.
func (a *SetEnvAction) DefaultPhase() string {
	return "post-install"
}

// Preflight validates parameters without side effects.
func (a *SetEnvAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	varsRaw, ok := params["vars"]
	if !ok {
		result.AddError("set_env action requires 'vars' parameter")
		return result
	}

	envVars, err := a.parseVars(varsRaw)
	if err != nil {
		result.AddErrorf("set_env: failed to parse vars: %v", err)
		return result
	}

	if len(envVars) == 0 {
		result.AddError("set_env: 'vars' must not be empty")
	}

	for _, envVar := range envVars {
		if err := validateEnvName(envVar.Name); err != nil {
			result.AddErrorf("set_env: %v", err)
		}
		if err := validateEnvValue(envVar.Name, envVar.Value); err != nil {
			result.AddErrorf("set_env: %v", err)
		}
	}

	return result
}

// Execute writes the recipe's environment variables into shell.d.
//
// Parameters:
//   - vars (required): List of environment variables [{name: "JAVA_HOME", value: "{install_dir}"}]
//
// This action runs in the post-install phase (see DefaultPhase) so that
// {install_dir} expands to the tool's final install directory rather than the
// staging directory that is deleted once the install finishes.
func (a *SetEnvAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	reporter := ctx.GetReporter()

	// When --no-shell-init is set, skip entirely: no files, no CleanupActions.
	if ctx.NoShellInit {
		reporter.Log("   Skipping environment exports (--no-shell-init)")
		return nil
	}

	// Get vars list (required)
	varsRaw, ok := params["vars"]
	if !ok {
		return fmt.Errorf("set_env action requires 'vars' parameter")
	}

	// Parse vars list
	envVars, err := a.parseVars(varsRaw)
	if err != nil {
		return fmt.Errorf("failed to parse vars: %w", err)
	}

	for _, envVar := range envVars {
		if err := validateEnvName(envVar.Name); err != nil {
			return fmt.Errorf("set_env: %w", err)
		}
	}

	if ctx.ToolInstallDir == "" {
		// Reached when the step runs outside the post-install phase, which in
		// practice means a recipe overrode the phase. Both install paths --
		// the top-level one and Executor.installSingleDependency -- populate
		// ToolInstallDir before they run post-install steps. Failing loudly
		// beats writing an export that points nowhere.
		return fmt.Errorf("set_env ran outside the post-install phase, so the tool's final " +
			"install directory is not known yet. Remove any 'phase' override from the step")
	}

	target, err := envTargetName(ctx)
	if err != nil {
		return fmt.Errorf("set_env: %w", err)
	}

	version, err := shellDVersion(ctx)
	if err != nil {
		return fmt.Errorf("set_env: %w", err)
	}

	// {install_dir} must resolve to the tool's permanent directory, not the
	// staging directory that ctx.InstallDir points at.
	vars := GetStandardVars(ctx.Version, ctx.ToolInstallDir, ctx.WorkDir, ctx.LibsDir)

	// Derive $TSUKU_HOME from ToolsDir (which is $TSUKU_HOME/tools)
	tsukuHome := filepath.Dir(ctx.ToolsDir)
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")

	if err := os.MkdirAll(shellDDir, 0700); err != nil {
		return fmt.Errorf("set_env: failed to create shell.d directory: %w", err)
	}
	// Ensure restrictive permissions even if the directory already existed
	if err := os.Chmod(shellDDir, 0700); err != nil {
		return fmt.Errorf("set_env: failed to set permissions on shell.d directory: %w", err)
	}

	reporter.Log("   Setting %d environment variable(s)", len(envVars))

	var buf strings.Builder
	for _, envVar := range envVars {
		value := ExpandVars(envVar.Value, vars)
		if err := validateEnvValue(envVar.Name, value); err != nil {
			return fmt.Errorf("set_env: %w", err)
		}
		fmt.Fprintf(&buf, "export %s=%s\n", envVar.Name, shellQuote(value))
		reporter.Log("   %s=%s", envVar.Name, value)
	}
	content := []byte(buf.String())

	for _, shell := range envShells {
		destPath := filepath.Join(shellDDir, shellDFileName(target, version, shell))

		// A recipe may carry more than one set_env step, and they all target the
		// same file. Later steps append to what this install already wrote --
		// otherwise the earlier steps' exports would vanish without a word. A
		// file left by a previous version is not ours to keep, so the first step
		// of each install truncates.
		written := content
		if cleanupRecorded(ctx, target, version, shell) {
			existing, err := os.ReadFile(destPath)
			if err != nil {
				return fmt.Errorf("set_env: failed to read %s to append to: %w", destPath, err)
			}
			written = append(existing, content...)
		}

		if err := os.WriteFile(destPath, written, 0600); err != nil {
			return fmt.Errorf("set_env: failed to write %s: %w", destPath, err)
		}
		reporter.Status(fmt.Sprintf("   Installed environment exports: %s", destPath))

		upsertCleanup(ctx, target, version, shell, contentHash(written))
	}

	return nil
}

// envTargetName builds the shell.d basename for this recipe's exports.
// The "00-env-" prefix keeps exports sorted ahead of tool init scripts.
func envTargetName(ctx *ExecutionContext) (string, error) {
	if ctx.Recipe == nil || ctx.Recipe.Metadata.Name == "" {
		return "", fmt.Errorf("recipe name is not available in ExecutionContext")
	}
	name := ctx.Recipe.Metadata.Name
	if name != filepath.Base(name) || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("recipe name %q is not a valid path segment", name)
	}
	// "@" separates the target from the version in a shell.d filename. A name
	// carrying one would make the decomposition ambiguous, so two tools could
	// end up writing the same file.
	if strings.Contains(name, "@") {
		return "", fmt.Errorf("recipe name %q may not contain %q", name, "@")
	}
	return EnvFilePrefix + name, nil
}

// validateEnvName rejects names that would not survive being written to an
// `export` line verbatim.
func validateEnvName(name string) error {
	if !envVarNamePattern.MatchString(name) {
		return fmt.Errorf("invalid variable name %q (must match [A-Za-z_][A-Za-z0-9_]*)", name)
	}
	return nil
}

// validateEnvValue rejects values that cannot be represented on a single
// `export` line. Everything else is made safe by shellQuote.
//
// Execute checks values after {version} and the other placeholders have been
// substituted, since that is what actually reaches the file; Preflight checks
// the recipe's literal, which is all it has.
func validateEnvValue(name, value string) error {
	if strings.ContainsAny(value, "\n\r\x00") {
		return fmt.Errorf("value for %q must not contain newlines or NUL bytes", name)
	}
	return nil
}

// shellQuote wraps a value in single quotes, escaping embedded single quotes
// the POSIX way, so no metacharacter in the value is interpreted by the shell.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// parseVars parses the vars parameter
func (a *SetEnvAction) parseVars(raw interface{}) ([]recipe.EnvVar, error) {
	// Handle []interface{} from TOML
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("vars must be an array")
	}

	var result []recipe.EnvVar

	for i, item := range arr {
		varMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("var %d: must be a map", i)
		}

		name, ok := varMap["name"].(string)
		if !ok {
			return nil, fmt.Errorf("var %d: 'name' must be a string", i)
		}

		value, ok := varMap["value"].(string)
		if !ok {
			return nil, fmt.Errorf("var %d: 'value' must be a string", i)
		}

		result = append(result, recipe.EnvVar{Name: name, Value: value})
	}

	return result, nil
}
