package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Ensure GoInstallAction implements Decomposable
var _ Decomposable = (*GoInstallAction)(nil)

// GoInstallAction installs Go modules using go install with GOBIN/GOMODCACHE isolation
type GoInstallAction struct{ BaseAction }

// Dependencies returns go as install-time and eval-time dependency.
// EvalTime is needed because Decompose() runs `go get` to generate go.sum.
func (GoInstallAction) Dependencies() ActionDeps {
	return ActionDeps{
		InstallTime: []string{"go"},
		EvalTime:    []string{"go"},
	}
}

// RequiresNetwork returns true because go_install fetches modules from the Go proxy.
func (GoInstallAction) RequiresNetwork() bool { return true }

// Name returns the action name
func (a *GoInstallAction) Name() string {
	return "go_install"
}

// Preflight validates parameters without side effects.
func (a *GoInstallAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}
	if _, ok := GetString(params, "module"); !ok {
		result.AddError("go_install action requires 'module' parameter")
	}
	if _, hasExecutables := params["executables"]; !hasExecutables {
		result.AddError("go_install action requires 'executables' parameter")
	}
	return result
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
	// Use VersionTag for Go modules as they require the "v" prefix (e.g., v1.0.0)
	if !isValidGoVersion(ctx.VersionTag) {
		return fmt.Errorf("invalid version format '%s': must match semver format", ctx.VersionTag)
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

	fmt.Printf("   Module: %s@%s\n", module, ctx.VersionTag)
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
	// Use VersionTag which preserves the "v" prefix required by Go modules
	var target string
	if ctx.VersionTag != "" {
		target = module + "@" + ctx.VersionTag
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

// Decompose converts a go_install composite action into a go_build primitive step.
// This is called during plan generation to capture go.sum at eval time.
//
// The decomposition:
//  1. Creates a temporary module context
//  2. Runs `go get <module>@<version>` to resolve dependencies
//  3. Captures the complete go.sum content
//  4. Returns a go_build step with the captured checksums
func (a *GoInstallAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
	// Get module path (required)
	module, ok := GetString(params, "module")
	if !ok {
		return nil, fmt.Errorf("go_install action requires 'module' parameter")
	}

	// Validate module path
	if !isValidGoModule(module) {
		return nil, fmt.Errorf("invalid module path '%s': must match Go module naming rules", module)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return nil, fmt.Errorf("go_install action requires 'executables' parameter with at least one executable")
	}

	// Use VersionTag for Go modules as they require the "v" prefix
	version := ctx.VersionTag
	if version == "" {
		version = ctx.Version
	}

	// Validate version
	if !isValidGoVersion(version) {
		return nil, fmt.Errorf("invalid version format '%s': must match semver format", version)
	}

	// Find Go binary
	goPath := ResolveGo()
	if goPath == "" {
		return nil, fmt.Errorf("go not found: install go first (tsuku install go)")
	}

	// Capture Go version for reproducibility
	goVersion := GetGoVersion(goPath)
	if goVersion == "" {
		return nil, fmt.Errorf("failed to determine Go version from %s", goPath)
	}

	// Create temp directory for capturing go.sum
	tempDir, err := os.MkdirTemp("", "tsuku-go-decompose-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Get home directory for GOMODCACHE
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Create minimal go.mod
	goModContent := "module temp\n\ngo 1.21\n"
	goModPath := filepath.Join(tempDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write go.mod: %w", err)
	}

	// Set up environment for go get
	goDir := filepath.Dir(goPath)
	modCache := filepath.Join(homeDir, ".tsuku", ".gomodcache")

	env := os.Environ()
	filteredEnv := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "GO") {
			filteredEnv = append(filteredEnv, e)
		}
	}

	// Add Go's bin directory to PATH
	for i, e := range filteredEnv {
		if strings.HasPrefix(e, "PATH=") {
			filteredEnv[i] = fmt.Sprintf("PATH=%s:%s", goDir, e[5:])
			break
		}
	}

	filteredEnv = append(filteredEnv,
		"GOMODCACHE="+modCache,
		"CGO_ENABLED=0",
		"GOPROXY=https://proxy.golang.org,direct",
		"GOSUMDB=sum.golang.org",
	)

	// Run go get to populate go.sum
	// Use version module if recipe specifies one (for subpackages like golang.org/x/tools/cmd/goimports)
	moduleForVersioning := module
	if ctx.Recipe != nil && ctx.Recipe.Version.Module != "" {
		moduleForVersioning = ctx.Recipe.Version.Module
	}
	target := moduleForVersioning + "@" + version
	getCmd := exec.CommandContext(ctx.Context, goPath, "get", target)
	getCmd.Dir = tempDir
	getCmd.Env = filteredEnv

	if output, err := getCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("go get failed: %w\nOutput: %s", err, string(output))
	}

	// Read the generated go.sum
	goSumPath := filepath.Join(tempDir, "go.sum")
	goSumBytes, err := os.ReadFile(goSumPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read go.sum: %w", err)
	}

	goSum := string(goSumBytes)

	// Build go_build params
	// Use moduleForVersioning for go.mod require (parent module)
	// Pass original module as install_module for go install (subpackage path)
	goBuildParams := map[string]interface{}{
		"module":         moduleForVersioning,
		"install_module": module, // Subpackage path for go install
		"version":        version,
		"executables":    executables,
		"go_sum":         goSum,
		"go_version":     goVersion, // Captured for reproducibility
	}

	// Pass through optional params if set
	if cgo, ok := GetBool(params, "cgo_enabled"); ok {
		goBuildParams["cgo_enabled"] = cgo
	}
	if flags, ok := GetStringSlice(params, "build_flags"); ok {
		goBuildParams["build_flags"] = flags
	}

	return []Step{
		{
			Action: "go_build",
			Params: goBuildParams,
		},
	}, nil
}
