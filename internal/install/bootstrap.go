package install

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tsuku-dev/tsuku/internal/executor"
	"github.com/tsuku-dev/tsuku/internal/recipe"
	"github.com/tsuku-dev/tsuku/internal/registry"
)

// EnsureNpm ensures npm is available, installing nodejs explicitly (not hidden) if needed
// nodejs is installed as explicit because ALL npm-based tools require nodejs at runtime
// Returns the path to npm executable
func EnsureNpm(mgr *Manager) (string, error) {
	return ensurePackageManager(mgr, "nodejs", "node", "npm", false)
}

// EnsurePython ensures Python and pip are available, installing as hidden if needed
// Returns the path to python3 executable
func EnsurePython(mgr *Manager) (string, error) {
	return ensurePackageManager(mgr, "python-standalone", "python3", "python3", true)
}

// EnsureCargo ensures Rust cargo is available, installing as hidden if needed
// Returns the path to cargo executable
func EnsureCargo(mgr *Manager) (string, error) {
	return ensurePackageManager(mgr, "rust", "cargo", "cargo", true)
}

// EnsurePipx ensures pipx is available, installing as hidden if needed
// Returns the path to pipx executable
func EnsurePipx(mgr *Manager) (string, error) {
	return ensurePackageManager(mgr, "pipx", "pipx", "pipx", true)
}

// ensurePackageManager is a helper that checks for a package manager and installs if needed
// toolName: recipe name (e.g., "nodejs")
// checkCmd: command to check if available in PATH (e.g., "node")
// execName: executable name to return path for (e.g., "npm")
// hidden: whether to install as hidden (true) or explicit (false)
func ensurePackageManager(mgr *Manager, toolName, checkCmd, execName string, hidden bool) (string, error) {
	cfg := mgr.config

	// 1. Check if already in PATH (system-installed)
	if _, err := exec.LookPath(checkCmd); err == nil {
		// checkCmd is in PATH, so execName should be too (e.g., if node is there, npm should be)
		// Get the actual executable we need
		execPath, err := exec.LookPath(execName)
		if err != nil {
			return "", fmt.Errorf("%s found in PATH but %s is not available: %w", checkCmd, execName, err)
		}
		fmt.Printf("Using system %s: %s\n", execName, execPath)
		return execPath, nil
	}

	// 2. Check if already installed as hidden
	state, err := mgr.state.Load()
	if err != nil {
		return "", fmt.Errorf("failed to load state: %w", err)
	}

	if toolState, exists := state.Installed[toolName]; exists {
		// Already installed - get the path using stored binaries metadata
		toolDir := cfg.ToolDir(toolName, toolState.Version)
		execPath := findExecutableInBinaries(toolDir, execName, toolState.Binaries)

		if execPath != "" {
			if info, err := os.Stat(execPath); err == nil && info.Mode()&0111 != 0 {
				if toolState.IsHidden {
					fmt.Printf("Using tsuku-managed %s (hidden)\n", toolName)
				}
				return execPath, nil
			}
		}
	}

	// 3. Need to install as execution dependency
	if hidden {
		fmt.Printf("Installing %s as execution dependency...\n", toolName)
	} else {
		fmt.Printf("Installing %s as runtime dependency...\n", toolName)
	}

	// Load recipe from registry
	reg := registry.New(cfg.RegistryDir)
	loader := recipe.New(reg)

	r, err := loader.Get(toolName)
	if err != nil {
		return "", fmt.Errorf("failed to load recipe for %s: %w", toolName, err)
	}

	// Install dependencies first (e.g., pipx needs python-standalone)
	if len(r.Metadata.Dependencies) > 0 {
		for _, dep := range r.Metadata.Dependencies {
			fmt.Printf("  Ensuring dependency: %s...\n", dep)
			// Map dependency names to their executable names
			// Some tools have different names for the recipe vs the executable
			checkCmd, execName := getDepExecutableNames(dep)
			// Bootstrap dependency as hidden
			if _, err := ensurePackageManager(mgr, dep, checkCmd, execName, true); err != nil {
				return "", fmt.Errorf("failed to ensure dependency %s: %w", dep, err)
			}
		}
	}

	// Create executor (use latest version)
	exec, err := executor.New(r)
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}
	defer exec.Cleanup()

	// Execute recipe
	ctx := context.Background()
	if err := exec.Execute(ctx); err != nil {
		return "", fmt.Errorf("failed to install %s: %w", toolName, err)
	}

	// Get version
	version := exec.Version()
	if version == "" {
		version = "latest"
	}

	// Install with appropriate options (hidden or explicit)
	var opts InstallOptions
	if hidden {
		opts = HiddenInstallOptions()
	} else {
		opts = DefaultInstallOptions()
	}
	// Extract binaries from recipe to store in state (important for multi-binary tools like nodejs)
	opts.Binaries = r.ExtractBinaries()
	if err := mgr.InstallWithOptions(toolName, version, exec.WorkDir(), opts); err != nil {
		return "", fmt.Errorf("failed to install %s to permanent location: %w", toolName, err)
	}

	// Get the installed executable path using the binaries from the recipe
	toolDir := cfg.ToolDir(toolName, version)
	execPath := findExecutableInBinaries(toolDir, execName, opts.Binaries)

	// Verify the executable exists and is executable
	if info, err := os.Stat(execPath); err != nil || info.Mode()&0111 == 0 {
		return "", fmt.Errorf("executable %s not found or not executable at %s", execName, execPath)
	}

	fmt.Printf("✓ %s is now available\n", toolName)
	return execPath, nil
}

// getDepExecutableNames maps dependency tool names to their executable names
// Some tools have different names for the recipe vs the actual executable
func getDepExecutableNames(dep string) (checkCmd, execName string) {
	switch dep {
	case "python-standalone":
		// python-standalone recipe produces python3 executable
		return "python3", "python3"
	case "rust":
		// rust recipe produces cargo executable
		return "cargo", "cargo"
	case "nodejs":
		// nodejs recipe produces node executable
		return "node", "npm"
	default:
		// Default: use dependency name for both
		return dep, dep
	}
}

// findExecutableInBinaries searches for an executable in the stored binaries list
// Binaries are stored as paths like "bin/cargo" or "cargo/bin/cargo"
// Returns the full path if found, empty string if not found
func findExecutableInBinaries(toolDir, execName string, binaries []string) string {
	// First, search in stored binaries metadata
	for _, binary := range binaries {
		// Check if this binary matches the executable name we're looking for
		if filepath.Base(binary) == execName {
			return filepath.Join(toolDir, binary)
		}
	}

	// Fallback to standard bin/<execName> location
	// This handles cases where binaries list is empty or doesn't include the exec
	return filepath.Join(toolDir, "bin", execName)
}

// CheckAndExposeHidden checks if a tool is installed as hidden and exposes it if requested
// This is used when user explicitly runs: tsuku install npm
func CheckAndExposeHidden(mgr *Manager, toolName string) (bool, error) {
	hidden, err := IsHidden(mgr.config, toolName)
	if err != nil {
		return false, err
	}

	if !hidden {
		return false, nil
	}

	// Tool is hidden, expose it
	if err := ExposeHidden(mgr, toolName); err != nil {
		return false, fmt.Errorf("failed to expose hidden tool: %w", err)
	}

	fmt.Printf("✓ %s is now available\n", toolName)
	return true, nil
}
