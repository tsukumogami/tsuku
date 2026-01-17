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

	// Always record library in state (even if not used by any tool yet)
	// This ensures verify command can find standalone library installations
	if err := m.state.UpdateLibrary(name, version, func(ls *LibraryVersionState) {
		// Add to used_by if a tool depends on this library
		if opts.ToolNameVersion != "" {
			for _, u := range ls.UsedBy {
				if u == opts.ToolNameVersion {
					return // Already in list
				}
			}
			ls.UsedBy = append(ls.UsedBy, opts.ToolNameVersion)
		}
	}); err != nil {
		return fmt.Errorf("failed to update library state: %w", err)
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

// InstalledLibrary represents an installed library
type InstalledLibrary struct {
	Name    string
	Version string
	Path    string
}

// ListLibraries returns a list of all installed libraries from $TSUKU_HOME/libs/
func (m *Manager) ListLibraries() ([]InstalledLibrary, error) {
	libsDir := m.config.LibsDir

	// Return empty list if libs directory doesn't exist
	if _, err := os.Stat(libsDir); os.IsNotExist(err) {
		return []InstalledLibrary{}, nil
	}

	entries, err := os.ReadDir(libsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read libs directory: %w", err)
	}

	var libs []InstalledLibrary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Expected format: name-version (e.g., libyaml-0.2.5)
		name := entry.Name()
		lastHyphen := -1
		// Find the last hyphen that's followed by a digit (version start)
		for i := len(name) - 1; i >= 0; i-- {
			if name[i] == '-' && i < len(name)-1 && name[i+1] >= '0' && name[i+1] <= '9' {
				lastHyphen = i
				break
			}
		}

		if lastHyphen == -1 || lastHyphen == 0 {
			// Invalid format, skip
			continue
		}

		libName := name[:lastHyphen]
		libVersion := name[lastHyphen+1:]

		libs = append(libs, InstalledLibrary{
			Name:    libName,
			Version: libVersion,
			Path:    filepath.Join(libsDir, name),
		})
	}

	return libs, nil
}
