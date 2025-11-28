package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds tsuku configuration
type Config struct {
	HomeDir    string // ~/.tsuku
	ToolsDir   string // ~/.tsuku/tools
	CurrentDir string // ~/.tsuku/tools/current
	RecipesDir string // ~/.tsuku/recipes
	ConfigFile string // ~/.tsuku/config.toml
}

// DefaultConfig returns the default configuration
func DefaultConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	tsukuHome := filepath.Join(home, ".tsuku")

	return &Config{
		HomeDir:    tsukuHome,
		ToolsDir:   filepath.Join(tsukuHome, "tools"),
		CurrentDir: filepath.Join(tsukuHome, "tools", "current"),
		RecipesDir: filepath.Join(tsukuHome, "recipes"),
		ConfigFile: filepath.Join(tsukuHome, "config.toml"),
	}, nil
}

// EnsureDirectories creates all necessary directories
func (c *Config) EnsureDirectories() error {
	dirs := []string{
		c.HomeDir,
		c.ToolsDir,
		c.CurrentDir,
		c.RecipesDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// ToolDir returns the installation directory for a specific tool version
func (c *Config) ToolDir(name, version string) string {
	return filepath.Join(c.ToolsDir, fmt.Sprintf("%s-%s", name, version))
}

// ToolBinDir returns the bin directory for a specific tool version
func (c *Config) ToolBinDir(name, version string) string {
	return filepath.Join(c.ToolDir(name, version), "bin")
}

// CurrentSymlink returns the path to the current symlink for a tool
func (c *Config) CurrentSymlink(name string) string {
	return filepath.Join(c.CurrentDir, name)
}
