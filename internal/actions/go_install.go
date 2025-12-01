package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GoInstallAction installs Go modules using go install with GOBIN/GOMODCACHE isolation
type GoInstallAction struct{}

// Name returns the action name
func (a *GoInstallAction) Name() string {
	return "go_install"
}

// Execute installs a Go module to the install directory
//
// Parameters:
//   - module (required): Go module path (e.g., "github.com/jesseduffield/lazygit")
//   - executables (required): List of executable names to verify
//
// Environment Isolation:
//   - GOBIN: Set to $INSTALL_DIR/bin
//   - GOMODCACHE: Set to $TSUKU_HOME/.gomodcache
//   - CGO_ENABLED: Set to 0 (pure Go binaries only)
//   - GOPROXY: Set to https://proxy.golang.org,direct
//   - GOSUMDB: Set to sum.golang.org
//
// Installation:
//
//	go install <module>@<version>
//
// Directory Structure Created:
//
//	<install_dir>/
//	  bin/<executable>     - Compiled binary
func (a *GoInstallAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get module path (required)
	module, ok := GetString(params, "module")
	if !ok {
		return fmt.Errorf("go_install action requires 'module' parameter")
	}

	// SECURITY: Validate module path to prevent command injection
	if !isValidGoModule(module) {
		return fmt.Errorf("invalid module path '%s': must match Go module naming rules", module)
	}

	// SECURITY: Validate version string
	if !isValidGoVersion(ctx.Version) {
		return fmt.Errorf("invalid version format '%s': must match semver format", ctx.Version)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("go_install action requires 'executables' parameter with at least one executable")
	}

	// SECURITY: Validate executable names to prevent path traversal
	for _, exe := range executables {
		if strings.Contains(exe, "/") || strings.Contains(exe, "\\") ||
			strings.Contains(exe, "..") || exe == "." || exe == "" {
			return fmt.Errorf("invalid executable name '%s': must not contain path separators", exe)
		}
	}

	// Find Go binary from tsuku's tools directory
	goPath := ResolveGo()
	if goPath == "" {
		return fmt.Errorf("go not found: install go first (tsuku install go)")
	}

	fmt.Printf("   Module: %s@%s\n", module, ctx.Version)
	fmt.Printf("   Executables: %v\n", executables)
	fmt.Printf("   Using go: %s\n", goPath)

	// Get home directory for GOMODCACHE
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	installDir := ctx.InstallDir
	binDir := filepath.Join(installDir, "bin")

	// Build install target
	var target string
	if ctx.Version != "" {
		target = module + "@" + ctx.Version
	} else {
		target = module + "@latest"
	}

	fmt.Printf("   Installing: go install %s\n", target)

	// Create bin directory
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Use CommandContext for cancellation support
	cmd := exec.CommandContext(ctx.Context, goPath, "install", target)

	// SECURITY: Set up isolated environment with explicit secure defaults
	// These settings prevent contamination from user environment and ensure
	// all module downloads go through the official Go proxy with checksums
	goDir := filepath.Dir(goPath)
	env := os.Environ()

	// Filter out any existing GO* environment variables to ensure isolation
	filteredEnv := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "GO") {
			filteredEnv = append(filteredEnv, e)
		}
	}

	// Add Go's bin directory to PATH
	pathUpdated := false
	for i, e := range filteredEnv {
		if strings.HasPrefix(e, "PATH=") {
			filteredEnv[i] = fmt.Sprintf("PATH=%s:%s", goDir, e[5:])
			pathUpdated = true
			break
		}
	}
	if !pathUpdated {
		filteredEnv = append(filteredEnv, fmt.Sprintf("PATH=%s:%s", goDir, os.Getenv("PATH")))
	}

	// Set isolation environment variables
	filteredEnv = append(filteredEnv,
		"GOBIN="+binDir,
		"GOMODCACHE="+filepath.Join(homeDir, ".tsuku", ".gomodcache"),
		"CGO_ENABLED=0",
		"GOPROXY=https://proxy.golang.org,direct",
		"GOSUMDB=sum.golang.org",
	)

	cmd.Env = filteredEnv

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go install failed: %w\nOutput: %s", err, string(output))
	}

	// go install is typically quiet, but show output if debugging
	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" && os.Getenv("TSUKU_DEBUG") != "" {
		fmt.Printf("   go output:\n%s\n", outputStr)
	}

	// Verify executables exist
	for _, exe := range executables {
		exePath := filepath.Join(binDir, exe)
		if _, err := os.Stat(exePath); err != nil {
			return fmt.Errorf("expected executable %s not found at %s", exe, exePath)
		}
	}

	fmt.Printf("   Module installed successfully\n")
	fmt.Printf("   Verified %d executable(s)\n", len(executables))

	return nil
}

// isValidGoModule validates Go module paths to prevent command injection
// Valid module paths: alphanumeric, slashes, hyphens, dots, underscores
// Must start with a domain-like prefix (e.g., github.com, golang.org)
// Max length: 256 characters (reasonable limit)
func isValidGoModule(path string) bool {
	if path == "" || len(path) > 256 {
		return false
	}

	// Must contain at least one slash (domain/path format)
	if !strings.Contains(path, "/") {
		return false
	}

	// Must start with a letter (domain names start with letters)
	first := path[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
		return false
	}

	// Check allowed characters: alphanumeric, slashes, hyphens, dots, underscores
	for _, c := range path {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '/' || c == '-' || c == '.' || c == '_') {
			return false
		}
	}

	// Reject double slashes (invalid path)
	if strings.Contains(path, "//") {
		return false
	}

	// Reject path traversal attempts
	if strings.Contains(path, "..") {
		return false
	}

	return true
}

// isValidGoVersion validates Go module version strings
// Valid: v1.0.0, 1.0.0, v1.0.0-alpha, v1.0.0+build, latest
// Invalid: anything with shell metacharacters
func isValidGoVersion(version string) bool {
	if version == "" {
		// Empty version is valid (will use @latest)
		return true
	}

	if len(version) > 50 {
		return false
	}

	// Special case: "latest" is valid
	if version == "latest" {
		return true
	}

	// Must start with 'v' or a digit
	first := version[0]
	if first != 'v' && (first < '0' || first > '9') {
		return false
	}

	// Allow semver characters: digits, dots, letters (for prerelease), hyphens, plus
	for _, c := range version {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') || c == '.' || c == '-' || c == '+') {
			return false
		}
	}

	return true
}
