package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	// EnvRecipeCacheSizeLimit is the environment variable to configure recipe cache size limit
	EnvRecipeCacheSizeLimit = "TSUKU_RECIPE_CACHE_SIZE_LIMIT"

	// EnvRecipeCacheMaxStale is the environment variable to configure maximum cache staleness
	EnvRecipeCacheMaxStale = "TSUKU_RECIPE_CACHE_MAX_STALE"

	// EnvRecipeCacheStaleFallback is the environment variable to enable/disable stale fallback
	EnvRecipeCacheStaleFallback = "TSUKU_RECIPE_CACHE_STALE_FALLBACK"

	// DefaultAPITimeout is the default timeout for API requests (30 seconds)
	DefaultAPITimeout = 30 * time.Second

	// DefaultVersionCacheTTL is the default TTL for cached version lists (1 hour)
	DefaultVersionCacheTTL = 1 * time.Hour

	// DefaultRecipeCacheTTL is the default TTL for cached recipes (24 hours)
	DefaultRecipeCacheTTL = 24 * time.Hour

	// DefaultRecipeCacheSizeLimit is the default size limit for the recipe cache (50MB)
	DefaultRecipeCacheSizeLimit = 50 * 1024 * 1024

	// DefaultRecipeCacheMaxStale is the default maximum staleness for cache fallback (7 days)
	DefaultRecipeCacheMaxStale = 7 * 24 * time.Hour
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

// ParseByteSize parses a human-readable byte size string into bytes.
// Accepts formats: plain numbers (52428800), KB/K (50K, 50KB), MB/M (50M, 50MB), GB/G (1G, 1GB).
// Case-insensitive. Returns an error for invalid formats.
func ParseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}

	s = strings.ToUpper(s)

	// Try to parse as plain number first
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, nil
	}

	// Try to extract numeric prefix and suffix
	var numStr string
	var suffix string
	for i, c := range s {
		if c >= '0' && c <= '9' || c == '.' {
			numStr += string(c)
		} else {
			suffix = s[i:]
			break
		}
	}

	if numStr == "" {
		return 0, fmt.Errorf("invalid size format: %q", s)
	}

	// Parse the numeric part
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size number: %q", numStr)
	}

	// Apply multiplier based on suffix
	var multiplier float64
	switch suffix {
	case "", "B":
		multiplier = 1
	case "K", "KB":
		multiplier = 1024
	case "M", "MB":
		multiplier = 1024 * 1024
	case "G", "GB":
		multiplier = 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("invalid size suffix: %q", suffix)
	}

	return int64(num * multiplier), nil
}

// GetRecipeCacheSizeLimit returns the configured recipe cache size limit from TSUKU_RECIPE_CACHE_SIZE_LIMIT.
// If not set or invalid, returns DefaultRecipeCacheSizeLimit (50MB).
// Accepts human-readable sizes like "50MB", "50M", "52428800".
func GetRecipeCacheSizeLimit() int64 {
	envValue := os.Getenv(EnvRecipeCacheSizeLimit)
	if envValue == "" {
		return DefaultRecipeCacheSizeLimit
	}

	size, err := ParseByteSize(envValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: invalid %s value %q, using default %dMB\n",
			EnvRecipeCacheSizeLimit, envValue, DefaultRecipeCacheSizeLimit/(1024*1024))
		return DefaultRecipeCacheSizeLimit
	}

	// Validate reasonable range (1MB to 10GB)
	minSize := int64(1 * 1024 * 1024)         // 1MB
	maxSize := int64(10 * 1024 * 1024 * 1024) // 10GB

	if size < minSize {
		fmt.Fprintf(os.Stderr, "Warning: %s too low (%d bytes), using minimum 1MB\n",
			EnvRecipeCacheSizeLimit, size)
		return minSize
	}
	if size > maxSize {
		fmt.Fprintf(os.Stderr, "Warning: %s too high (%d bytes), using maximum 10GB\n",
			EnvRecipeCacheSizeLimit, size)
		return maxSize
	}

	return size
}

// GetRecipeCacheMaxStale returns the configured maximum cache staleness from TSUKU_RECIPE_CACHE_MAX_STALE.
// If not set or invalid, returns DefaultRecipeCacheMaxStale (7 days).
// If set to 0, stale fallback is disabled.
// Accepts duration strings like "24h", "7d", "168h".
func GetRecipeCacheMaxStale() time.Duration {
	envValue := os.Getenv(EnvRecipeCacheMaxStale)
	if envValue == "" {
		return DefaultRecipeCacheMaxStale
	}

	// Handle "Xd" format for days (Go's time.ParseDuration doesn't support days)
	if len(envValue) > 1 && (envValue[len(envValue)-1] == 'd' || envValue[len(envValue)-1] == 'D') {
		daysStr := envValue[:len(envValue)-1]
		days, err := strconv.ParseFloat(daysStr, 64)
		if err == nil {
			duration := time.Duration(days * 24 * float64(time.Hour))
			// Allow 0 to disable, otherwise clamp to range
			if duration == 0 {
				return 0
			}
			if duration > 30*24*time.Hour {
				fmt.Fprintf(os.Stderr, "Warning: %s too high (%v), using maximum 30d\n",
					EnvRecipeCacheMaxStale, duration)
				return 30 * 24 * time.Hour
			}
			return duration
		}
	}

	duration, err := time.ParseDuration(envValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: invalid %s value %q, using default %v\n",
			EnvRecipeCacheMaxStale, envValue, DefaultRecipeCacheMaxStale)
		return DefaultRecipeCacheMaxStale
	}

	// Allow 0 to disable stale fallback
	if duration == 0 {
		return 0
	}

	// Validate reasonable range (minimum 1 hour, maximum 30 days)
	if duration < 1*time.Hour {
		fmt.Fprintf(os.Stderr, "Warning: %s too low (%v), using minimum 1h\n",
			EnvRecipeCacheMaxStale, duration)
		return 1 * time.Hour
	}
	if duration > 30*24*time.Hour {
		fmt.Fprintf(os.Stderr, "Warning: %s too high (%v), using maximum 30d\n",
			EnvRecipeCacheMaxStale, duration)
		return 30 * 24 * time.Hour
	}

	return duration
}

// GetRecipeCacheStaleFallback returns whether stale-if-error fallback is enabled.
// Reads from TSUKU_RECIPE_CACHE_STALE_FALLBACK environment variable.
// Accepts "true", "1", "false", "0" (case-insensitive). Default is true.
func GetRecipeCacheStaleFallback() bool {
	envValue := os.Getenv(EnvRecipeCacheStaleFallback)
	if envValue == "" {
		return true // Default enabled
	}

	switch strings.ToLower(envValue) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		fmt.Fprintf(os.Stderr, "Warning: invalid %s value %q, using default true\n",
			EnvRecipeCacheStaleFallback, envValue)
		return true
	}
}

// DefaultHomeOverride can be set by the binary's main package to change the
// default home directory. Used by dev builds (via ldflags) to default to
// .tsuku-dev instead of ~/.tsuku. TSUKU_HOME env var still takes precedence.
var DefaultHomeOverride string

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
		if DefaultHomeOverride != "" {
			tsukuHome = DefaultHomeOverride
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get user home directory: %w", err)
			}
			tsukuHome = filepath.Join(home, ".tsuku")
		}
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
