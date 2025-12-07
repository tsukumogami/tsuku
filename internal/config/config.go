package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// EnvTsukuHome is the environment variable to override the default tsuku home directory
	EnvTsukuHome = "TSUKU_HOME"

	// EnvAPITimeout is the environment variable to configure API request timeout
	EnvAPITimeout = "TSUKU_API_TIMEOUT"

	// DefaultAPITimeout is the default timeout for API requests (30 seconds)
	DefaultAPITimeout = 30 * time.Second
)

// GetAPITimeout returns the configured API timeout from TSUKU_API_TIMEOUT environment variable.
// If not set or invalid, returns DefaultAPITimeout (30 seconds).
// Accepts duration strings like "30s", "1m", "2m30s".
func GetAPITimeout() time.Duration {
	envValue := os.Getenv(EnvAPITimeout)
	if envValue == "" {
		return DefaultAPITimeout
	}

	duration, err := time.ParseDuration(envValue)
	if err != nil {
		// Invalid duration format, use default
		fmt.Fprintf(os.Stderr, "Warning: invalid %s value %q, using default %v\n",
			EnvAPITimeout, envValue, DefaultAPITimeout)
		return DefaultAPITimeout
	}

	// Validate reasonable range (1 second to 10 minutes)
	if duration < 1*time.Second {
		fmt.Fprintf(os.Stderr, "Warning: %s too low (%v), using minimum 1s\n",
			EnvAPITimeout, duration)
		return 1 * time.Second
	}
	if duration > 10*time.Minute {
		fmt.Fprintf(os.Stderr, "Warning: %s too high (%v), using maximum 10m\n",
			EnvAPITimeout, duration)
		return 10 * time.Minute
	}

	return duration
}

// Config holds tsuku configuration
type Config struct {
	HomeDir     string // ~/.tsuku
	ToolsDir    string // ~/.tsuku/tools
	CurrentDir  string // ~/.tsuku/tools/current
	RecipesDir  string // ~/.tsuku/recipes
	RegistryDir string // ~/.tsuku/registry (cached recipes from remote registry)
	LibsDir     string // ~/.tsuku/libs (shared libraries)
	ConfigFile  string // ~/.tsuku/config.toml
}

// DefaultConfig returns the default configuration
func DefaultConfig() (*Config, error) {
	// Check for TSUKU_HOME environment variable first
	tsukuHome := os.Getenv(EnvTsukuHome)
	if tsukuHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		tsukuHome = filepath.Join(home, ".tsuku")
	}

	return &Config{
		HomeDir:     tsukuHome,
		ToolsDir:    filepath.Join(tsukuHome, "tools"),
		CurrentDir:  filepath.Join(tsukuHome, "tools", "current"),
		RecipesDir:  filepath.Join(tsukuHome, "recipes"),
		RegistryDir: filepath.Join(tsukuHome, "registry"),
		LibsDir:     filepath.Join(tsukuHome, "libs"),
		ConfigFile:  filepath.Join(tsukuHome, "config.toml"),
	}, nil
}

// EnsureDirectories creates all necessary directories
func (c *Config) EnsureDirectories() error {
	dirs := []string{
		c.HomeDir,
		c.ToolsDir,
		c.CurrentDir,
		c.RecipesDir,
		c.RegistryDir,
		c.LibsDir,
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

// LibDir returns the installation directory for a specific library version
func (c *Config) LibDir(name, version string) string {
	return filepath.Join(c.LibsDir, fmt.Sprintf("%s-%s", name, version))
}
