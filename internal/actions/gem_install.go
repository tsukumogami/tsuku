package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GemInstallAction installs Ruby gems with GEM_HOME isolation
type GemInstallAction struct{ BaseAction }

// Dependencies returns ruby as both install-time and runtime dependency.
func (GemInstallAction) Dependencies() ActionDeps {
	return ActionDeps{InstallTime: []string{"ruby"}, Runtime: []string{"ruby"}}
}

// Name returns the action name
func (a *GemInstallAction) Name() string {
	return "gem_install"
}

// Execute installs a Ruby gem to the install directory
//
// Parameters:
//   - gem (required): gem name on RubyGems.org
//   - executables (required): List of executable names to verify
//   - gem_path (optional): Path to gem executable (defaults to system gem or tsuku's ruby)
//
// Environment Strategy:
//
//	GEM_HOME=<install_dir>        - Where gems are installed
//	GEM_PATH=<install_dir>        - Where gems are searched (isolation)
//	PATH=<gem_bin_dir>:$PATH      - For finding ruby
//
// Installation:
//
//	gem install <gem> --version <version> --no-document --install-dir <install_dir>
//
// Directory Structure Created:
//
//	<install_dir>/
//	  bin/<executable>           - Wrapper scripts
//	  gems/<gem>-<version>/      - Gem files
//	  specifications/            - Gemspecs
//	  cache/                     - Downloaded .gem files
func (a *GemInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get gem name (required)
	gemName, ok := GetString(params, "gem")
	if !ok {
		return fmt.Errorf("gem_install action requires 'gem' parameter")
	}

	// SECURITY: Validate gem name to prevent command injection
	if !isValidGemName(gemName) {
		return fmt.Errorf("invalid gem name '%s': must match RubyGems naming rules", gemName)
	}

	// SECURITY: Validate version string
	if !isValidGemVersion(ctx.Version) {
		return fmt.Errorf("invalid version format '%s': must match RubyGems version format", ctx.Version)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("gem_install action requires 'executables' parameter with at least one executable")
	}

	// SECURITY: Validate executable names to prevent path traversal and injection
	for _, exe := range executables {
		if len(exe) == 0 || len(exe) > 256 {
			return fmt.Errorf("invalid executable name length: %s", exe)
		}
		if strings.Contains(exe, "/") || strings.Contains(exe, "\\") ||
			strings.Contains(exe, "..") || exe == "." {
			return fmt.Errorf("invalid executable name '%s': must not contain path separators", exe)
		}
		// Check for control characters and null bytes
		for _, c := range exe {
			if c < 32 || c == 127 || c == 0 {
				return fmt.Errorf("invalid executable name '%s': contains control characters", exe)
			}
		}
		// Check for shell metacharacters
		if strings.ContainsAny(exe, "$`|;&<>()[]{}") {
			return fmt.Errorf("invalid executable name '%s': contains shell metacharacters", exe)
		}
	}

	// Get gem path (optional, defaults to system gem or tsuku's ruby)
	gemPath, _ := GetString(params, "gem_path")
	if gemPath == "" {
		// Try to resolve gem from tsuku's tools directory first
		gemPath = ResolveGem()
		if gemPath == "" {
			// Fallback to gem in PATH
			gemPath = "gem"
		}
	}

	fmt.Printf("   Gem: %s@%s\n", gemName, ctx.Version)
	fmt.Printf("   Executables: %v\n", executables)
	fmt.Printf("   Using gem: %s\n", gemPath)

	// Install gem with --install-dir for isolation
	installDir := ctx.InstallDir

	fmt.Printf("   Installing: gem install %s --version %s --install-dir %s\n",
		gemName, ctx.Version, installDir)

	// Build command: gem install <gem> --version <version> --no-document --install-dir <dir>
	// Use CommandContext for cancellation support
	cmd := exec.CommandContext(ctx.Context, gemPath, "install", gemName,
		"--version", ctx.Version,
		"--no-document",             // Skip documentation
		"--install-dir", installDir, // Install to our directory
		"--bindir", filepath.Join(installDir, "bin"), // Put executables in bin/
	)

	// Set environment for isolated installation
	// GEM_HOME and GEM_PATH ensure gems are installed and found only in our directory
	env := os.Environ()
	env = append(env, fmt.Sprintf("GEM_HOME=%s", installDir))
	env = append(env, fmt.Sprintf("GEM_PATH=%s", installDir))

	// Build PATH incrementally - start with gem's directory
	gemDir := filepath.Dir(gemPath)
	pathValue := fmt.Sprintf("%s:%s", gemDir, os.Getenv("PATH"))

	// Set up C compiler for native extensions
	// Prefer system compiler (gcc) when available because it has better compatibility
	// Fall back to zig if no system compiler is found
	if !hasSystemCompiler() {
		if zigPath := ResolveZig(); zigPath != "" {
			wrapperDir := filepath.Join(ctx.ToolsDir, "zig-cc-wrapper")
			if err := setupZigWrappers(zigPath, wrapperDir); err == nil {
				// Prepend wrapper directory to PATH and set CC/CXX
				pathValue = fmt.Sprintf("%s:%s", wrapperDir, pathValue)
				env = append(env, fmt.Sprintf("CC=%s", filepath.Join(wrapperDir, "cc")))
				env = append(env, fmt.Sprintf("CXX=%s", filepath.Join(wrapperDir, "c++")))
				fmt.Printf("   Using zig as C compiler for native extensions\n")
			}
		}
	}

	// Update PATH in env (remove existing PATH entries first)
	newEnv := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "PATH=") {
			newEnv = append(newEnv, e)
		}
	}
	newEnv = append(newEnv, fmt.Sprintf("PATH=%s", pathValue))
	env = newEnv
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gem install failed: %w\nOutput: %s", err, string(output))
	}

	// gem is verbose, only show output if debugging
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		fmt.Printf("   gem output:\n%s\n", outputStr)
	}

	// Verify executables exist and create self-contained wrappers
	// The gem-generated wrappers need GEM_HOME/GEM_PATH at runtime,
	// so we create wrapper scripts that set these before calling the original
	binDir := filepath.Join(installDir, "bin")
	for _, exe := range executables {
		exePath := filepath.Join(binDir, exe)
		if _, err := os.Stat(exePath); err != nil {
			return fmt.Errorf("expected executable %s not found at %s", exe, exePath)
		}

		// Read the original gem wrapper
		originalContent, err := os.ReadFile(exePath)
		if err != nil {
			return fmt.Errorf("failed to read gem wrapper %s: %w", exe, err)
		}

		// Rename original wrapper to .gem suffix
		gemWrapperPath := exePath + ".gem"
		if err := os.Rename(exePath, gemWrapperPath); err != nil {
			return fmt.Errorf("failed to rename gem wrapper: %w", err)
		}

		// Create new wrapper that sets GEM_HOME/GEM_PATH and adds Ruby to PATH
		// Uses SCRIPT_DIR to make wrapper relocatable (works after install dir is moved)
		// gemDir contains the Ruby bin directory (where gem and ruby executables are)
		wrapperContent := fmt.Sprintf(`#!/bin/bash
# tsuku wrapper for %s (sets GEM_HOME/GEM_PATH for isolated gem)
SCRIPT_PATH="${BASH_SOURCE[0]}"
# Resolve symlinks to get the actual script location
while [ -L "$SCRIPT_PATH" ]; do
    SCRIPT_DIR="$(cd -P "$(dirname "$SCRIPT_PATH")" && pwd)"
    SCRIPT_PATH="$(readlink "$SCRIPT_PATH")"
    [[ $SCRIPT_PATH != /* ]] && SCRIPT_PATH="$SCRIPT_DIR/$SCRIPT_PATH"
done
SCRIPT_DIR="$(cd -P "$(dirname "$SCRIPT_PATH")" && pwd)"
INSTALL_DIR="$(dirname "$SCRIPT_DIR")"
export GEM_HOME="$INSTALL_DIR"
export GEM_PATH="$INSTALL_DIR"
# Add Ruby to PATH and explicitly call ruby (don't rely on shebang)
export PATH="%s:$PATH"
exec ruby "$SCRIPT_DIR/%s.gem" "$@"
`, exe, gemDir, exe)

		if err := os.WriteFile(exePath, []byte(wrapperContent), 0755); err != nil {
			// Restore original on failure
			_ = os.Rename(gemWrapperPath, exePath)
			return fmt.Errorf("failed to create wrapper script: %w", err)
		}

		// Log debug info if enabled
		if os.Getenv("TSUKU_DEBUG") != "" {
			fmt.Printf("   Created wrapper for %s (original at %s)\n", exe, gemWrapperPath)
			fmt.Printf("   Original wrapper content:\n%s\n", string(originalContent)[:min(200, len(originalContent))])
		}
	}

	fmt.Printf("   ✓ Gem installed successfully\n")
	fmt.Printf("   ✓ Created %d self-contained wrapper(s)\n", len(executables))

	return nil
}

// isValidGemName validates gem names to prevent command injection
// Gem names on RubyGems.org: start with letter, alphanumeric + hyphens + underscores
// Max length: 100 characters (reasonable limit)
func isValidGemName(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}

	// Must start with a letter
	first := name[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
		return false
	}

	// Check allowed characters: alphanumeric, hyphens, underscores
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}

	return true
}

// isValidGemVersion validates RubyGems version strings
// Valid: 1.0.0, 1.2.3.pre, 1.2.3.rc1, 1.2.3.beta.2, 1.0.0-pre.1
// Invalid: anything with shell metacharacters
func isValidGemVersion(version string) bool {
	if version == "" || len(version) > 50 {
		return false
	}

	// Must start with a digit
	if version[0] < '0' || version[0] > '9' {
		return false
	}

	// Allow RubyGems version characters: digits, dots, letters (for pre/rc/beta), hyphens
	for _, c := range version {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') || c == '.' || c == '-') {
			return false
		}
	}

	return true
}
