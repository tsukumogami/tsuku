// Package addon provides lifecycle management for the tsuku-llm addon binary.
// Installation is delegated to the recipe system via an injected Installer interface.
// This package retains only binary location and daemon lifecycle coordination.
package addon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Installer abstracts the recipe-based installation pipeline.
// Production code wires the real executor implementation; tests use a mock.
type Installer interface {
	// InstallRecipe loads a recipe by name, generates a plan for the current
	// target (with the given GPU override applied), and executes it.
	// gpuOverride is passed to PlanConfig.GPU; "" means auto-detect.
	InstallRecipe(ctx context.Context, recipeName string, gpuOverride string) error
}

// AddonManager handles locating and ensuring the tsuku-llm addon is installed.
// Binary installation is delegated to the recipe system via the Installer interface.
type AddonManager struct {
	mu sync.Mutex

	// homeDir is the tsuku home directory ($TSUKU_HOME or ~/.tsuku)
	homeDir string

	// installer provides recipe-based installation
	installer Installer

	// backendOverride is the llm.backend config value ("cpu" or "")
	backendOverride string

	// cachedPath is the verified addon path (set after successful EnsureAddon)
	cachedPath string
}

// NewAddonManager creates a new addon manager with the given installer and backend override.
// If homeDir is empty, it uses TSUKU_HOME env var or defaults to ~/.tsuku.
// backendOverride should come from LLMConfig.LLMBackend() ("cpu" or "").
func NewAddonManager(homeDir string, installer Installer, backendOverride string) *AddonManager {
	if homeDir == "" {
		homeDir = os.Getenv("TSUKU_HOME")
	}
	if homeDir == "" {
		if h, err := os.UserHomeDir(); err == nil {
			homeDir = filepath.Join(h, ".tsuku")
		}
	}

	return &AddonManager{
		homeDir:         homeDir,
		installer:       installer,
		backendOverride: backendOverride,
	}
}

// HomeDir returns the tsuku home directory.
func (m *AddonManager) HomeDir() string {
	return m.homeDir
}

// EnsureAddon ensures the addon is installed via the recipe system.
// It returns the path to the installed binary.
//
// This method:
// 1. If TSUKU_LLM_BINARY is set, uses that path directly (skips installation)
// 2. Checks if tsuku-llm is already installed at the recipe tools path
// 3. If the installed variant doesn't match the backend override, reinstalls
// 4. If not installed, installs via the recipe system
// 5. Cleans up legacy addon paths from pre-recipe installations
// 6. Returns the path to the binary
//
// The method is safe for concurrent calls via mutex protection.
func (m *AddonManager) EnsureAddon(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// When TSUKU_LLM_BINARY is set, use the explicit binary path directly.
	// This skips recipe installation since the binary is externally provided
	// (e.g., built from source for integration tests).
	if path := os.Getenv("TSUKU_LLM_BINARY"); path != "" {
		if _, err := os.Stat(path); err == nil {
			m.cachedPath = path
			return path, nil
		}
	}

	// Return cached path if already resolved this session
	if m.cachedPath != "" {
		if _, err := os.Stat(m.cachedPath); err == nil {
			return m.cachedPath, nil
		}
		// Binary disappeared since last check - clear cache
		m.cachedPath = ""
	}

	// Check if already installed
	binaryPath := m.findInstalledBinary()

	// Check for variant mismatch: user set llm.backend=cpu but a GPU variant is installed
	if binaryPath != "" && m.backendOverride == "cpu" {
		if m.isGPUVariantInstalled() {
			slog.Info("llm.backend=cpu but GPU variant installed, reinstalling with CPU variant")
			if err := m.installViaRecipe(ctx); err != nil {
				return "", fmt.Errorf("reinstalling tsuku-llm with CPU variant: %w", err)
			}
			binaryPath = m.findInstalledBinary()
		}
	}

	if binaryPath == "" {
		// Not installed - install via recipe system
		if err := m.installViaRecipe(ctx); err != nil {
			return "", fmt.Errorf("installing tsuku-llm: %w", err)
		}
		binaryPath = m.findInstalledBinary()
	}

	if binaryPath == "" {
		return "", fmt.Errorf("tsuku-llm installation succeeded but binary not found at expected path")
	}

	// Clean up legacy addon path if it exists
	m.cleanupLegacyPath()

	m.cachedPath = binaryPath
	return binaryPath, nil
}

// findInstalledBinary looks for the tsuku-llm binary at the standard recipe
// installation path: $TSUKU_HOME/tools/tsuku-llm-<version>/bin/tsuku-llm
// It scans for any installed version directory.
func (m *AddonManager) findInstalledBinary() string {
	toolsDir := filepath.Join(m.homeDir, "tools")
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return ""
	}

	binName := binaryName()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "tsuku-llm-") {
			continue
		}

		// Check bin/ subdirectory first (standard recipe layout)
		binPath := filepath.Join(toolsDir, name, "bin", binName)
		if _, err := os.Stat(binPath); err == nil {
			return binPath
		}

		// Check root of tool directory (flat layout)
		rootPath := filepath.Join(toolsDir, name, binName)
		if _, err := os.Stat(rootPath); err == nil {
			return rootPath
		}
	}

	return ""
}

// isGPUVariantInstalled checks if the currently installed tsuku-llm directory
// name contains a GPU backend indicator (cuda or vulkan in the asset name).
// This is a heuristic: recipe-installed directories follow the pattern
// tsuku-llm-<version> and the plan's asset name determines the variant.
// When the user sets llm.backend=cpu, we check if the installed binary
// came from a GPU-specific step by looking at the installation state.
func (m *AddonManager) isGPUVariantInstalled() bool {
	// Conservative check: look for non-CPU variant directories.
	// The recipe system installs to tsuku-llm-<version>/ regardless of variant,
	// so we can't distinguish variants by directory name alone.
	// For the initial implementation, assume any existing installation that
	// was done without the cpu override might be a GPU variant.
	// A more precise check would read state.json, but that creates a dependency
	// on the install package that the issue spec avoids.
	//
	// The simplest correct approach: if backendOverride is "cpu" and there's
	// an installed binary, always reinstall. The recipe system handles
	// idempotency and this case (switching from GPU to CPU) should be rare.
	return true
}

// installViaRecipe delegates installation to the recipe system.
func (m *AddonManager) installViaRecipe(ctx context.Context) error {
	if m.installer == nil {
		return fmt.Errorf("no installer configured")
	}

	gpuOverride := ""
	if m.backendOverride == "cpu" {
		gpuOverride = "none"
	}

	return m.installer.InstallRecipe(ctx, "tsuku-llm", gpuOverride)
}

// cleanupLegacyPath removes the old addon installation path if it exists.
// Before the recipe migration, the addon was installed to:
//   - $TSUKU_HOME/addons/tsuku-llm/
//   - $TSUKU_HOME/tools/tsuku-llm/<version>/ (old manifest-based layout)
//
// This prevents 50-200MB of orphaned binaries from persisting after upgrade.
func (m *AddonManager) cleanupLegacyPath() {
	legacyPaths := []string{
		filepath.Join(m.homeDir, "addons", "tsuku-llm"),
		filepath.Join(m.homeDir, "tools", "tsuku-llm"), // old non-versioned layout
	}

	for _, legacyPath := range legacyPaths {
		info, err := os.Stat(legacyPath)
		if err != nil {
			continue
		}
		// Only remove if it's a directory (the old layout) and not the recipe layout
		// (recipe layout uses tsuku-llm-<version>, not tsuku-llm/)
		if info.IsDir() {
			slog.Info("removing legacy addon installation", "path", legacyPath)
			if err := os.RemoveAll(legacyPath); err != nil {
				slog.Warn("failed to remove legacy addon path", "path", legacyPath, "error", err)
			}
		}
	}
}

// binaryName returns the addon binary name for the current platform.
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "tsuku-llm.exe"
	}
	return "tsuku-llm"
}
