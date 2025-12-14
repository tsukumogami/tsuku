package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GoBuildAction is an ecosystem primitive that builds Go modules with locked dependencies.
// Unlike go_install (composite), go_build receives pre-captured go.sum and builds with isolation.
type GoBuildAction struct{ BaseAction }

// Dependencies returns go as an install-time dependency.
func (GoBuildAction) Dependencies() ActionDeps {
	return ActionDeps{InstallTime: []string{"go"}}
}

// Name returns the action name
func (a *GoBuildAction) Name() string {
	return "go_build"
}

// Execute builds a Go module with locked dependencies using the captured go.sum.
//
// Parameters:
//   - module (required): Go module path (e.g., "github.com/jesseduffield/lazygit")
//   - version (required): Module version (e.g., "v0.40.2")
//   - executables (required): List of executable names to verify
//   - go_sum (required): Complete go.sum content captured at eval time
//   - go_version (required for reproducibility): Go toolchain version (e.g., "1.21.5")
//     If specified, requires that exact version to be installed. If not found, returns
//     an error instructing the user to install it (tsuku install go@<version>).
//   - cgo_enabled (optional): Enable CGO (default: false)
//   - build_flags (optional): Build flags (default: ["-trimpath", "-buildvcs=false"])
//
// Environment Isolation:
//   - GOBIN: Set to $INSTALL_DIR/bin
//   - GOMODCACHE: Isolated module cache
//   - CGO_ENABLED: Set to 0 by default
//   - GOPROXY: Set to "off" (use cached modules only)
//   - GOSUMDB: Set to "off" (checksums already verified)
//
// Execution Flow:
//  1. Resolve Go toolchain binary
//  2. Create isolated module cache and temp directory
//  3. Write go.sum to temp directory
//  4. Create minimal go.mod with module requirement
//  5. Run `go mod download` to populate cache
//  6. Run `go install` with isolation flags
//  7. Verify executables exist
func (a *GoBuildAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get module path (required)
	module, ok := GetString(params, "module")
	if !ok {
		return fmt.Errorf("go_build action requires 'module' parameter")
	}

	// SECURITY: Validate module path to prevent command injection
	if !isValidGoModule(module) {
		return fmt.Errorf("invalid module path '%s': must match Go module naming rules", module)
	}

	// Get version (required for go_build - already resolved at eval time)
	version, ok := GetString(params, "version")
	if !ok {
		return fmt.Errorf("go_build action requires 'version' parameter")
	}

	// SECURITY: Validate version string
	if !isValidGoVersion(version) {
		return fmt.Errorf("invalid version format '%s': must match semver format", version)
	}

	// Get executables list (required)
	executables, ok := GetStringSlice(params, "executables")
	if !ok || len(executables) == 0 {
		return fmt.Errorf("go_build action requires 'executables' parameter with at least one executable")
	}

	// SECURITY: Validate executable names to prevent path traversal
	for _, exe := range executables {
		if strings.Contains(exe, "/") || strings.Contains(exe, "\\") ||
			strings.Contains(exe, "..") || exe == "." || exe == "" {
			return fmt.Errorf("invalid executable name '%s': must not contain path separators", exe)
		}
	}

	// Get go.sum content (required for locked builds)
	goSum, ok := GetString(params, "go_sum")
	if !ok {
		return fmt.Errorf("go_build action requires 'go_sum' parameter")
	}

	// Get optional parameters with defaults
	cgoEnabled := false
	if cgo, ok := GetBool(params, "cgo_enabled"); ok {
		cgoEnabled = cgo
	}

	buildFlags := []string{"-trimpath", "-buildvcs=false"}
	if flags, ok := GetStringSlice(params, "build_flags"); ok {
		buildFlags = flags
	}

	// Get required Go version (captured at eval time for reproducibility)
	requiredGoVersion, hasGoVersion := GetString(params, "go_version")

	// Find Go binary - prefer specific version if specified
	var goPath string
	if hasGoVersion && requiredGoVersion != "" {
		goPath = ResolveGoVersion(requiredGoVersion)
		if goPath == "" {
			return fmt.Errorf("go %s not found: install it first (tsuku install go@%s)", requiredGoVersion, requiredGoVersion)
		}
	} else {
		// Fallback to any available Go for backwards compatibility
		goPath = ResolveGo()
		if goPath == "" {
			return fmt.Errorf("go not found: install go first (tsuku install go)")
		}
	}

	fmt.Printf("   Module: %s@%s\n", module, version)
	fmt.Printf("   Executables: %v\n", executables)
	if hasGoVersion && requiredGoVersion != "" {
		fmt.Printf("   Go version: %s (locked)\n", requiredGoVersion)
	}
	fmt.Printf("   Using go: %s\n", goPath)
	fmt.Printf("   CGO enabled: %v\n", cgoEnabled)
	fmt.Printf("   Build flags: %v\n", buildFlags)

	// Get home directory for paths
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	installDir := ctx.InstallDir
	binDir := filepath.Join(installDir, "bin")

	// Create bin directory
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Create temp directory for the locked build
	tempDir := filepath.Join(ctx.WorkDir, "go_build_temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Write go.sum to temp directory
	goSumPath := filepath.Join(tempDir, "go.sum")
	if err := os.WriteFile(goSumPath, []byte(goSum), 0644); err != nil {
		return fmt.Errorf("failed to write go.sum: %w", err)
	}

	// Create minimal go.mod that requires the target module
	goModContent := fmt.Sprintf("module temp\n\ngo 1.21\n\nrequire %s %s\n", module, version)
	goModPath := filepath.Join(tempDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		return fmt.Errorf("failed to write go.mod: %w", err)
	}

	// Set up isolated environment
	goDir := filepath.Dir(goPath)
	modCache := filepath.Join(homeDir, ".tsuku", ".gomodcache")

	env := buildGoEnv(goDir, binDir, modCache, cgoEnabled, true)

	// First, download modules to populate the cache
	// We use GOPROXY to allow downloading, then switch to offline mode for install
	downloadEnv := buildGoEnv(goDir, binDir, modCache, cgoEnabled, false)

	downloadCmd := exec.CommandContext(ctx.Context, goPath, "mod", "download", "-x")
	downloadCmd.Dir = tempDir
	downloadCmd.Env = downloadEnv

	fmt.Printf("   Downloading modules...\n")
	if output, err := downloadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod download failed: %w\nOutput: %s", err, string(output))
	}

	// Verify checksums match the captured go.sum
	verifyCmd := exec.CommandContext(ctx.Context, goPath, "mod", "verify")
	verifyCmd.Dir = tempDir
	verifyCmd.Env = downloadEnv

	if output, err := verifyCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod verify failed (checksums mismatch): %w\nOutput: %s", err, string(output))
	}

	// Build install target
	target := module + "@" + version

	// Build install command with flags
	installArgs := []string{"install"}
	installArgs = append(installArgs, buildFlags...)
	installArgs = append(installArgs, target)

	fmt.Printf("   Installing: go %s\n", strings.Join(installArgs, " "))

	// Use offline mode for the actual install (GOPROXY=off)
	installCmd := exec.CommandContext(ctx.Context, goPath, installArgs...)
	installCmd.Dir = tempDir
	installCmd.Env = env

	output, err := installCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go install failed: %w\nOutput: %s", err, string(output))
	}

	// Show output if debugging
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

	fmt.Printf("   Module built successfully with locked dependencies\n")
	fmt.Printf("   Verified %d executable(s)\n", len(executables))

	return nil
}

// buildGoEnv constructs an isolated Go environment for building.
// If offline is true, GOPROXY is set to "off" for fully offline builds.
func buildGoEnv(goDir, binDir, modCache string, cgoEnabled, offline bool) []string {
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

	// Set CGO_ENABLED
	cgoValue := "0"
	if cgoEnabled {
		cgoValue = "1"
	}

	// Set proxy configuration
	goproxy := "https://proxy.golang.org,direct"
	gosumdb := "sum.golang.org"
	if offline {
		goproxy = "off"
		gosumdb = "off"
	}

	// Set isolation environment variables
	filteredEnv = append(filteredEnv,
		"GOBIN="+binDir,
		"GOMODCACHE="+modCache,
		"CGO_ENABLED="+cgoValue,
		"GOPROXY="+goproxy,
		"GOSUMDB="+gosumdb,
	)

	return filteredEnv
}
