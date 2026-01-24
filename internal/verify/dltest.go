package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

// DlopenResult represents the outcome of a dlopen test for a single library.
type DlopenResult struct {
	// Path is the absolute path to the library that was tested.
	Path string `json:"path"`

	// OK is true if dlopen succeeded for this library.
	OK bool `json:"ok"`

	// Error contains the dlerror() message if OK is false.
	Error string `json:"error,omitempty"`
}

// EnsureDltest checks if the tsuku-dltest helper is installed with the correct
// version, installing it if necessary, and returns the path to the binary.
//
// The helper is installed via tsuku's standard recipe system, which provides:
// - Checksum verification for supply chain security
// - Version tracking in state.json
// - Standard installation patterns
//
// Version behavior:
// - When pinnedDltestVersion is "dev": accept any installed version, or install latest
// - When pinnedDltestVersion is a specific version: require that exact version
func EnsureDltest(cfg *config.Config) (string, error) {
	stateManager := install.NewStateManager(cfg)

	// Check current installation state
	toolState, err := stateManager.GetToolState("tsuku-dltest")
	if err != nil {
		return "", fmt.Errorf("failed to check tsuku-dltest state: %w", err)
	}

	// Determine installed version (handle both old and new state format)
	var installedVersion string
	if toolState != nil {
		if toolState.ActiveVersion != "" {
			installedVersion = toolState.ActiveVersion
		} else {
			installedVersion = toolState.Version
		}
	}

	// Dev mode: accept any installed version
	if pinnedDltestVersion == "dev" {
		if installedVersion != "" {
			// Use whatever version is installed
			dltestPath := filepath.Join(cfg.ToolBinDir("tsuku-dltest", installedVersion), "tsuku-dltest")
			if _, err := os.Stat(dltestPath); err == nil {
				return dltestPath, nil
			}
			// State says installed but binary missing - fall through to install latest
		}
		// Nothing installed, install latest
		if err := installDltest(""); err != nil {
			return "", err
		}
		// Re-check state to get the installed version
		toolState, err = stateManager.GetToolState("tsuku-dltest")
		if err != nil {
			return "", fmt.Errorf("failed to check tsuku-dltest state after install: %w", err)
		}
		if toolState == nil {
			return "", fmt.Errorf("tsuku-dltest install succeeded but no state found")
		}
		if toolState.ActiveVersion != "" {
			installedVersion = toolState.ActiveVersion
		} else {
			installedVersion = toolState.Version
		}
		dltestPath := filepath.Join(cfg.ToolBinDir("tsuku-dltest", installedVersion), "tsuku-dltest")
		if _, err := os.Stat(dltestPath); err != nil {
			return "", fmt.Errorf("tsuku-dltest installed but binary not found at %s", dltestPath)
		}
		return dltestPath, nil
	}

	// Release mode: require exact pinned version
	if installedVersion == pinnedDltestVersion {
		dltestPath := filepath.Join(cfg.ToolBinDir("tsuku-dltest", pinnedDltestVersion), "tsuku-dltest")
		if _, err := os.Stat(dltestPath); err == nil {
			return dltestPath, nil
		}
		// State says installed but binary missing - fall through to reinstall
	}

	// Need to install the pinned version
	if err := installDltest(pinnedDltestVersion); err != nil {
		return "", err
	}

	dltestPath := filepath.Join(cfg.ToolBinDir("tsuku-dltest", pinnedDltestVersion), "tsuku-dltest")
	if _, err := os.Stat(dltestPath); err != nil {
		return "", fmt.Errorf("tsuku-dltest installed but binary not found at %s", dltestPath)
	}

	return dltestPath, nil
}

// installDltest installs tsuku-dltest using the standard recipe flow.
// This invokes tsuku as a subprocess to reuse all installation infrastructure.
// If version is empty, installs the latest available version.
func installDltest(version string) error {
	// Find tsuku binary - should be in PATH or we can use os.Executable
	tsukuPath, err := os.Executable()
	if err != nil {
		// Fall back to looking in PATH
		tsukuPath, err = exec.LookPath("tsuku")
		if err != nil {
			return fmt.Errorf("cannot find tsuku binary to install helper: %w", err)
		}
	}

	// Build install command - use version spec if provided, otherwise install latest
	var toolSpec string
	if version != "" {
		toolSpec = fmt.Sprintf("tsuku-dltest@%s", version)
	} else {
		toolSpec = "tsuku-dltest"
	}
	cmd := exec.Command(tsukuPath, "install", toolSpec)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run installation
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install %s: %w\nstderr: %s",
			toolSpec, err, strings.TrimSpace(stderr.String()))
	}

	return nil
}

// InvokeDltest calls the tsuku-dltest helper to test dlopen on the given library paths.
// It returns a DlopenResult for each path, preserving order.
func InvokeDltest(ctx context.Context, helperPath string, paths []string) ([]DlopenResult, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	cmd := exec.CommandContext(ctx, helperPath, paths...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Check for context cancellation/timeout
	if ctx.Err() != nil {
		return nil, fmt.Errorf("dltest helper: %w", ctx.Err())
	}

	// Parse JSON even on non-zero exit (exit 1 means some libraries failed)
	var results []DlopenResult
	if parseErr := json.Unmarshal(stdout.Bytes(), &results); parseErr != nil {
		// If we can't parse JSON, report the error
		if err != nil {
			return nil, fmt.Errorf("dltest helper failed: %w (stderr: %s)", err, stderr.String())
		}
		return nil, fmt.Errorf("failed to parse dltest output: %w", parseErr)
	}

	return results, nil
}
