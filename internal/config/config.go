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

	// EnvVersionCacheTTL is the environment variable to configure version cache TTL
	EnvVersionCacheTTL = "TSUKU_VERSION_CACHE_TTL"

	// EnvRecipeCacheTTL is the environment variable to configure recipe cache TTL
	EnvRecipeCacheTTL = "TSUKU_RECIPE_CACHE_TTL"

	// DefaultAPITimeout is the default timeout for API requests (30 seconds)
	DefaultAPITimeout = 30 * time.Second

	// DefaultVersionCacheTTL is the default TTL for cached version lists (1 hour)
	DefaultVersionCacheTTL = 1 * time.Hour

	// DefaultRecipeCacheTTL is the default TTL for cached recipes (24 hours)
	DefaultRecipeCacheTTL = 24 * time.Hour
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

// GetVersionCacheTTL returns the configured version cache TTL from TSUKU_VERSION_CACHE_TTL.
// If not set or invalid, returns DefaultVersionCacheTTL (1 hour).
// Accepts duration strings like "30m", "1h", "24h".
func GetVersionCacheTTL() time.Duration {
	envValue := os.Getenv(EnvVersionCacheTTL)
	if envValue == "" {
		return DefaultVersionCacheTTL
	}

	duration, err := time.ParseDuration(envValue)
	if err != nil {
		// Invalid duration format, use default
		fmt.Fprintf(os.Stderr, "Warning: invalid %s value %q, using default %v\n",
			EnvVersionCacheTTL, envValue, DefaultVersionCacheTTL)
		return DefaultVersionCacheTTL
	}

	// Validate reasonable range (5 minutes to 7 days)
	if duration < 5*time.Minute {
		fmt.Fprintf(os.Stderr, "Warning: %s too low (%v), using minimum 5m\n",
			EnvVersionCacheTTL, duration)
		return 5 * time.Minute
	}
	if duration > 7*24*time.Hour {
		fmt.Fprintf(os.Stderr, "Warning: %s too high (%v), using maximum 7d\n",
			EnvVersionCacheTTL, duration)
		return 7 * 24 * time.Hour
	}

	return duration
}

// GetRecipeCacheTTL returns the configured recipe cache TTL from TSUKU_RECIPE_CACHE_TTL.
// If not set or invalid, returns DefaultRecipeCacheTTL (24 hours).
// Accepts duration strings like "30m", "1h", "24h".
func GetRecipeCacheTTL() time.Duration {
	envValue := os.Getenv(EnvRecipeCacheTTL)
	if envValue == "" {
		return DefaultRecipeCacheTTL
	}

	duration, err := time.ParseDuration(envValue)
	if err != nil {
		// Invalid duration format, use default
		fmt.Fprintf(os.Stderr, "Warning: invalid %s value %q, using default %v\n",
			EnvRecipeCacheTTL, envValue, DefaultRecipeCacheTTL)
		return DefaultRecipeCacheTTL
	}

	// Validate reasonable range (5 minutes to 7 days)
	if duration < 5*time.Minute {
		fmt.Fprintf(os.Stderr, "Warning: %s too low (%v), using minimum 5m\n",
			EnvRecipeCacheTTL, duration)
		return 5 * time.Minute
	}
	if duration > 7*24*time.Hour {
		fmt.Fprintf(os.Stderr, "Warning: %s too high (%v), using maximum 7d\n",
			EnvRecipeCacheTTL, duration)
		return 7 * 24 * time.Hour
	}

	return duration
}

// Config holds tsuku configuration
type Config struct {
	HomeDir          string // $TSUKU_HOME
	ToolsDir         string // $TSUKU_HOME/tools
	CurrentDir       string // $TSUKU_HOME/tools/current
	RecipesDir       string // $TSUKU_HOME/recipes
	RegistryDir      string // $TSUKU_HOME/registry (cached recipes from remote registry)
	LibsDir          string // $TSUKU_HOME/libs (shared libraries)
	AppsDir          string // $TSUKU_HOME/apps (macOS application bundles)
	CacheDir         string // $TSUKU_HOME/cache
	VersionCacheDir  string // $TSUKU_HOME/cache/versions
	DownloadCacheDir string // $TSUKU_HOME/cache/downloads
	KeyCacheDir      string // $TSUKU_HOME/cache/keys (PGP public keys)
	TapCacheDir      string // $TSUKU_HOME/cache/taps (Homebrew tap metadata)
	ConfigFile       string // $TSUKU_HOME/config.toml
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
		HomeDir:          tsukuHome,
		ToolsDir:         filepath.Join(tsukuHome, "tools"),
		CurrentDir:       filepath.Join(tsukuHome, "tools", "current"),
		RecipesDir:       filepath.Join(tsukuHome, "recipes"),
		RegistryDir:      filepath.Join(tsukuHome, "registry"),
		LibsDir:          filepath.Join(tsukuHome, "libs"),
		AppsDir:          filepath.Join(tsukuHome, "apps"),
		CacheDir:         filepath.Join(tsukuHome, "cache"),
		VersionCacheDir:  filepath.Join(tsukuHome, "cache", "versions"),
		DownloadCacheDir: filepath.Join(tsukuHome, "cache", "downloads"),
		KeyCacheDir:      filepath.Join(tsukuHome, "cache", "keys"),
		TapCacheDir:      filepath.Join(tsukuHome, "cache", "taps"),
		ConfigFile:       filepath.Join(tsukuHome, "config.toml"),
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
		c.AppsDir,
		c.CacheDir,
		c.VersionCacheDir,
		c.DownloadCacheDir,
		c.KeyCacheDir,
		c.TapCacheDir,
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

// AppDir returns the installation directory for a specific app bundle version
func (c *Config) AppDir(name, version string) string {
	return filepath.Join(c.AppsDir, fmt.Sprintf("%s-%s.app", name, version))
}
