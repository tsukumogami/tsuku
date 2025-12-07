package install

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tsukumogami/tsuku/internal/config"
)

// LibraryInstallOptions controls how a library is installed
type LibraryInstallOptions struct {
	// ToolNameVersion is the tool that depends on this library (e.g., "ruby-3.4.0")
	// Used for used_by tracking in state.json
	ToolNameVersion string
}

// InstallLibrary copies a library from the work directory to $TSUKU_HOME/libs/{name}-{version}/
// Unlike tool installation, libraries:
// - Are installed to libs/ instead of tools/
// - Do not create symlinks in current/
// - Track used_by instead of required_by
func (m *Manager) InstallLibrary(name, version, workDir string, opts LibraryInstallOptions) error {
	// Ensure directories exist
	if err := m.config.EnsureDirectories(); err != nil {
		return err
	}

	// Create library-specific directory
	libDir := m.config.LibDir(name, version)
	if err := os.MkdirAll(libDir, 0755); err != nil {
		return fmt.Errorf("failed to create library directory: %w", err)
	}

	// Copy from work directory to library directory
	srcInstallDir := filepath.Join(workDir, ".install")

	if err := copyDir(srcInstallDir, libDir); err != nil {
		return fmt.Errorf("failed to copy library installation: %w", err)
	}

	fmt.Printf("   Installed library to: %s\n", libDir)

	// Update state with used_by tracking
	if opts.ToolNameVersion != "" {
		if err := m.state.AddLibraryUsedBy(name, version, opts.ToolNameVersion); err != nil {
			return fmt.Errorf("failed to update library state: %w", err)
		}
	}

	return nil
}

// IsLibraryInstalled checks if a specific library version is already installed
func (m *Manager) IsLibraryInstalled(name, version string) bool {
	libDir := m.config.LibDir(name, version)
	info, err := os.Stat(libDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// GetInstalledLibraryVersion returns the installed version of a library if present
// For Phase 1, we only support exact version matching
// Returns empty string if not installed
func (m *Manager) GetInstalledLibraryVersion(name string) string {
	// Check libs directory for any version of this library
	libsDir := m.config.LibsDir
	entries, err := os.ReadDir(libsDir)
	if err != nil {
		return ""
	}

	prefix := name + "-"
	for _, entry := range entries {
		if entry.IsDir() && len(entry.Name()) > len(prefix) {
			if entry.Name()[:len(prefix)] == prefix {
				// Extract version from directory name
				return entry.Name()[len(prefix):]
			}
		}
	}

	return ""
}

// AddLibraryUsedBy adds a tool to the used_by list for an installed library
func (m *Manager) AddLibraryUsedBy(libName, libVersion, toolNameVersion string) error {
	return m.state.AddLibraryUsedBy(libName, libVersion, toolNameVersion)
}

// LibDir returns the installation directory for a library version
// This is a convenience method that delegates to config
func (m *Manager) LibDir(name, version string) string {
	return m.config.LibDir(name, version)
}

// CheckLibraryInstalled checks if a library version exists and returns its path
// Returns empty string if not installed
func CheckLibraryInstalled(cfg *config.Config, name, version string) string {
	libDir := cfg.LibDir(name, version)
	if info, err := os.Stat(libDir); err == nil && info.IsDir() {
		return libDir
	}
	return ""
}
