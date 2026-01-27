package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig() failed: %v", err)
	}

	home, _ := os.UserHomeDir()
	expectedHome := filepath.Join(home, ".tsuku")

	if cfg.HomeDir != expectedHome {
		t.Errorf("HomeDir = %q, want %q", cfg.HomeDir, expectedHome)
	}
	if cfg.ToolsDir != filepath.Join(expectedHome, "tools") {
		t.Errorf("ToolsDir = %q, want %q", cfg.ToolsDir, filepath.Join(expectedHome, "tools"))
	}
	if cfg.CurrentDir != filepath.Join(expectedHome, "tools", "current") {
		t.Errorf("CurrentDir = %q, want %q", cfg.CurrentDir, filepath.Join(expectedHome, "tools", "current"))
	}
	if cfg.RecipesDir != filepath.Join(expectedHome, "recipes") {
		t.Errorf("RecipesDir = %q, want %q", cfg.RecipesDir, filepath.Join(expectedHome, "recipes"))
	}
	if cfg.RegistryDir != filepath.Join(expectedHome, "registry") {
		t.Errorf("RegistryDir = %q, want %q", cfg.RegistryDir, filepath.Join(expectedHome, "registry"))
	}
	if cfg.LibsDir != filepath.Join(expectedHome, "libs") {
		t.Errorf("LibsDir = %q, want %q", cfg.LibsDir, filepath.Join(expectedHome, "libs"))
	}
	if cfg.ConfigFile != filepath.Join(expectedHome, "config.toml") {
		t.Errorf("ConfigFile = %q, want %q", cfg.ConfigFile, filepath.Join(expectedHome, "config.toml"))
	}
}

func TestEnsureDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		HomeDir:          filepath.Join(tmpDir, "tsuku"),
		ToolsDir:         filepath.Join(tmpDir, "tsuku", "tools"),
		CurrentDir:       filepath.Join(tmpDir, "tsuku", "tools", "current"),
		RecipesDir:       filepath.Join(tmpDir, "tsuku", "recipes"),
		RegistryDir:      filepath.Join(tmpDir, "tsuku", "registry"),
		LibsDir:          filepath.Join(tmpDir, "tsuku", "libs"),
		AppsDir:          filepath.Join(tmpDir, "tsuku", "apps"),
		CacheDir:         filepath.Join(tmpDir, "tsuku", "cache"),
		VersionCacheDir:  filepath.Join(tmpDir, "tsuku", "cache", "versions"),
		DownloadCacheDir: filepath.Join(tmpDir, "tsuku", "cache", "downloads"),
		KeyCacheDir:      filepath.Join(tmpDir, "tsuku", "cache", "keys"),
		TapCacheDir:      filepath.Join(tmpDir, "tsuku", "cache", "taps"),
	}

	err := cfg.EnsureDirectories()
	if err != nil {
		t.Fatalf("EnsureDirectories() failed: %v", err)
	}

	// Verify all directories exist
	dirs := []string{cfg.HomeDir, cfg.ToolsDir, cfg.CurrentDir, cfg.RecipesDir, cfg.RegistryDir, cfg.LibsDir, cfg.AppsDir, cfg.CacheDir, cfg.VersionCacheDir, cfg.DownloadCacheDir, cfg.KeyCacheDir, cfg.TapCacheDir}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("Directory %q does not exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q is not a directory", dir)
		}
	}
}

func TestToolDir(t *testing.T) {
	cfg := &Config{ToolsDir: "/home/user/.tsuku/tools"}

	got := cfg.ToolDir("golang", "1.21.0")
	want := "/home/user/.tsuku/tools/golang-1.21.0"
	if got != want {
		t.Errorf("ToolDir() = %q, want %q", got, want)
	}
}

func TestToolBinDir(t *testing.T) {
	cfg := &Config{ToolsDir: "/home/user/.tsuku/tools"}

	got := cfg.ToolBinDir("golang", "1.21.0")
	want := "/home/user/.tsuku/tools/golang-1.21.0/bin"
	if got != want {
		t.Errorf("ToolBinDir() = %q, want %q", got, want)
	}
}

func TestCurrentSymlink(t *testing.T) {
	cfg := &Config{CurrentDir: "/home/user/.tsuku/tools/current"}

	got := cfg.CurrentSymlink("golang")
	want := "/home/user/.tsuku/tools/current/golang"
	if got != want {
		t.Errorf("CurrentSymlink() = %q, want %q", got, want)
	}
}

func TestLibDir(t *testing.T) {
	cfg := &Config{LibsDir: "/home/user/.tsuku/libs"}

	got := cfg.LibDir("libyaml", "0.2.5")
	want := "/home/user/.tsuku/libs/libyaml-0.2.5"
	if got != want {
		t.Errorf("LibDir() = %q, want %q", got, want)
	}
}

func TestAppDir(t *testing.T) {
	cfg := &Config{AppsDir: "/home/user/.tsuku/apps"}

	got := cfg.AppDir("visual-studio-code", "1.85.0")
	want := "/home/user/.tsuku/apps/visual-studio-code-1.85.0.app"
	if got != want {
		t.Errorf("AppDir() = %q, want %q", got, want)
	}
}

func TestDefaultConfig_WithTsukuHome(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvTsukuHome)
	defer os.Setenv(EnvTsukuHome, original)

	// Set custom TSUKU_HOME
	customHome := "/custom/tsuku/path"
	os.Setenv(EnvTsukuHome, customHome)

	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig() failed: %v", err)
	}

	// Verify all paths are based on custom home
	if cfg.HomeDir != customHome {
		t.Errorf("HomeDir = %q, want %q", cfg.HomeDir, customHome)
	}
	if cfg.ToolsDir != filepath.Join(customHome, "tools") {
		t.Errorf("ToolsDir = %q, want %q", cfg.ToolsDir, filepath.Join(customHome, "tools"))
	}
	if cfg.CurrentDir != filepath.Join(customHome, "tools", "current") {
		t.Errorf("CurrentDir = %q, want %q", cfg.CurrentDir, filepath.Join(customHome, "tools", "current"))
	}
	if cfg.RecipesDir != filepath.Join(customHome, "recipes") {
		t.Errorf("RecipesDir = %q, want %q", cfg.RecipesDir, filepath.Join(customHome, "recipes"))
	}
	if cfg.RegistryDir != filepath.Join(customHome, "registry") {
		t.Errorf("RegistryDir = %q, want %q", cfg.RegistryDir, filepath.Join(customHome, "registry"))
	}
	if cfg.LibsDir != filepath.Join(customHome, "libs") {
		t.Errorf("LibsDir = %q, want %q", cfg.LibsDir, filepath.Join(customHome, "libs"))
	}
	if cfg.ConfigFile != filepath.Join(customHome, "config.toml") {
		t.Errorf("ConfigFile = %q, want %q", cfg.ConfigFile, filepath.Join(customHome, "config.toml"))
	}
}

func TestDefaultConfig_EmptyTsukuHome(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvTsukuHome)
	defer os.Setenv(EnvTsukuHome, original)

	// Ensure TSUKU_HOME is not set
	_ = os.Unsetenv(EnvTsukuHome)

	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig() failed: %v", err)
	}

	// Verify it falls back to ~/.tsuku
	home, _ := os.UserHomeDir()
	expectedHome := filepath.Join(home, ".tsuku")

	if cfg.HomeDir != expectedHome {
		t.Errorf("HomeDir = %q, want %q", cfg.HomeDir, expectedHome)
	}
}

func TestGetAPITimeout_Default(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvAPITimeout)
	defer os.Setenv(EnvAPITimeout, original)

	// Ensure env var is not set
	_ = os.Unsetenv(EnvAPITimeout)

	timeout := GetAPITimeout()
	if timeout != DefaultAPITimeout {
		t.Errorf("GetAPITimeout() = %v, want %v", timeout, DefaultAPITimeout)
	}
}

func TestGetAPITimeout_CustomValue(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvAPITimeout)
	defer os.Setenv(EnvAPITimeout, original)

	// Set custom timeout
	os.Setenv(EnvAPITimeout, "45s")

	timeout := GetAPITimeout()
	expected := 45 * time.Second
	if timeout != expected {
		t.Errorf("GetAPITimeout() = %v, want %v", timeout, expected)
	}
}

func TestGetAPITimeout_InvalidValue(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvAPITimeout)
	defer os.Setenv(EnvAPITimeout, original)

	// Set invalid value
	os.Setenv(EnvAPITimeout, "invalid")

	timeout := GetAPITimeout()
	// Should return default on invalid input
	if timeout != DefaultAPITimeout {
		t.Errorf("GetAPITimeout() = %v, want %v (default)", timeout, DefaultAPITimeout)
	}
}

func TestGetAPITimeout_TooLow(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvAPITimeout)
	defer os.Setenv(EnvAPITimeout, original)

	// Set too low value
	os.Setenv(EnvAPITimeout, "100ms")

	timeout := GetAPITimeout()
	// Should return minimum 1s
	if timeout != 1*time.Second {
		t.Errorf("GetAPITimeout() = %v, want 1s (minimum)", timeout)
	}
}

func TestGetAPITimeout_TooHigh(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvAPITimeout)
	defer os.Setenv(EnvAPITimeout, original)

	// Set too high value
	os.Setenv(EnvAPITimeout, "1h")

	timeout := GetAPITimeout()
	// Should return maximum 10m
	if timeout != 10*time.Minute {
		t.Errorf("GetAPITimeout() = %v, want 10m (maximum)", timeout)
	}
}

func TestGetVersionCacheTTL_Default(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvVersionCacheTTL)
	defer os.Setenv(EnvVersionCacheTTL, original)

	// Ensure env var is not set
	_ = os.Unsetenv(EnvVersionCacheTTL)

	ttl := GetVersionCacheTTL()
	if ttl != DefaultVersionCacheTTL {
		t.Errorf("GetVersionCacheTTL() = %v, want %v", ttl, DefaultVersionCacheTTL)
	}
}

func TestGetVersionCacheTTL_CustomValue(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvVersionCacheTTL)
	defer os.Setenv(EnvVersionCacheTTL, original)

	// Set custom TTL
	os.Setenv(EnvVersionCacheTTL, "30m")

	ttl := GetVersionCacheTTL()
	expected := 30 * time.Minute
	if ttl != expected {
		t.Errorf("GetVersionCacheTTL() = %v, want %v", ttl, expected)
	}
}

func TestGetVersionCacheTTL_InvalidValue(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvVersionCacheTTL)
	defer os.Setenv(EnvVersionCacheTTL, original)

	// Set invalid value
	os.Setenv(EnvVersionCacheTTL, "invalid")

	ttl := GetVersionCacheTTL()
	// Should return default on invalid input
	if ttl != DefaultVersionCacheTTL {
		t.Errorf("GetVersionCacheTTL() = %v, want %v (default)", ttl, DefaultVersionCacheTTL)
	}
}

func TestGetVersionCacheTTL_TooLow(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvVersionCacheTTL)
	defer os.Setenv(EnvVersionCacheTTL, original)

	// Set too low value (minimum is 5m)
	os.Setenv(EnvVersionCacheTTL, "1m")

	ttl := GetVersionCacheTTL()
	// Should return minimum 5m
	if ttl != 5*time.Minute {
		t.Errorf("GetVersionCacheTTL() = %v, want 5m (minimum)", ttl)
	}
}

func TestGetVersionCacheTTL_TooHigh(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvVersionCacheTTL)
	defer os.Setenv(EnvVersionCacheTTL, original)

	// Set too high value (maximum is 7 days)
	os.Setenv(EnvVersionCacheTTL, "200h")

	ttl := GetVersionCacheTTL()
	// Should return maximum 7 days
	if ttl != 7*24*time.Hour {
		t.Errorf("GetVersionCacheTTL() = %v, want 168h (maximum)", ttl)
	}
}

func TestGetRecipeCacheTTL_Default(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvRecipeCacheTTL)
	defer os.Setenv(EnvRecipeCacheTTL, original)

	// Ensure env var is not set
	_ = os.Unsetenv(EnvRecipeCacheTTL)

	ttl := GetRecipeCacheTTL()
	if ttl != DefaultRecipeCacheTTL {
		t.Errorf("GetRecipeCacheTTL() = %v, want %v", ttl, DefaultRecipeCacheTTL)
	}
}

func TestGetRecipeCacheTTL_CustomValue(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvRecipeCacheTTL)
	defer os.Setenv(EnvRecipeCacheTTL, original)

	// Set custom TTL
	os.Setenv(EnvRecipeCacheTTL, "12h")

	ttl := GetRecipeCacheTTL()
	expected := 12 * time.Hour
	if ttl != expected {
		t.Errorf("GetRecipeCacheTTL() = %v, want %v", ttl, expected)
	}
}

func TestGetRecipeCacheTTL_InvalidValue(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvRecipeCacheTTL)
	defer os.Setenv(EnvRecipeCacheTTL, original)

	// Set invalid value
	os.Setenv(EnvRecipeCacheTTL, "invalid")

	ttl := GetRecipeCacheTTL()
	// Should return default on invalid input
	if ttl != DefaultRecipeCacheTTL {
		t.Errorf("GetRecipeCacheTTL() = %v, want %v (default)", ttl, DefaultRecipeCacheTTL)
	}
}

func TestGetRecipeCacheTTL_TooLow(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvRecipeCacheTTL)
	defer os.Setenv(EnvRecipeCacheTTL, original)

	// Set too low value (minimum is 5m)
	os.Setenv(EnvRecipeCacheTTL, "1m")

	ttl := GetRecipeCacheTTL()
	// Should return minimum 5m
	if ttl != 5*time.Minute {
		t.Errorf("GetRecipeCacheTTL() = %v, want 5m (minimum)", ttl)
	}
}

func TestGetRecipeCacheTTL_TooHigh(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvRecipeCacheTTL)
	defer os.Setenv(EnvRecipeCacheTTL, original)

	// Set too high value (maximum is 7 days)
	os.Setenv(EnvRecipeCacheTTL, "200h")

	ttl := GetRecipeCacheTTL()
	// Should return maximum 7 days
	if ttl != 7*24*time.Hour {
		t.Errorf("GetRecipeCacheTTL() = %v, want 168h (maximum)", ttl)
	}
}

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		// Plain numbers
		{"0", 0, false},
		{"1024", 1024, false},
		{"52428800", 52428800, false},

		// Bytes
		{"100B", 100, false},
		{"100b", 100, false},

		// Kilobytes
		{"1K", 1024, false},
		{"1KB", 1024, false},
		{"1k", 1024, false},
		{"1kb", 1024, false},
		{"50K", 51200, false},

		// Megabytes
		{"1M", 1024 * 1024, false},
		{"1MB", 1024 * 1024, false},
		{"1m", 1024 * 1024, false},
		{"1mb", 1024 * 1024, false},
		{"50M", 50 * 1024 * 1024, false},
		{"50MB", 50 * 1024 * 1024, false},

		// Gigabytes
		{"1G", 1024 * 1024 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"1g", 1024 * 1024 * 1024, false},
		{"2GB", 2 * 1024 * 1024 * 1024, false},

		// Decimal values
		{"1.5M", int64(1.5 * 1024 * 1024), false},
		{"0.5G", int64(0.5 * 1024 * 1024 * 1024), false},

		// Invalid inputs
		{"", 0, true},
		{"abc", 0, true},
		{"50TB", 0, true},
		{"MB", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseByteSize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseByteSize(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseByteSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetRecipeCacheSizeLimit_Default(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvRecipeCacheSizeLimit)
	defer os.Setenv(EnvRecipeCacheSizeLimit, original)

	// Ensure env var is not set
	_ = os.Unsetenv(EnvRecipeCacheSizeLimit)

	limit := GetRecipeCacheSizeLimit()
	if limit != DefaultRecipeCacheSizeLimit {
		t.Errorf("GetRecipeCacheSizeLimit() = %d, want %d", limit, DefaultRecipeCacheSizeLimit)
	}
}

func TestGetRecipeCacheSizeLimit_CustomValue(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvRecipeCacheSizeLimit)
	defer os.Setenv(EnvRecipeCacheSizeLimit, original)

	// Set custom value (100MB as bytes)
	os.Setenv(EnvRecipeCacheSizeLimit, "104857600")

	limit := GetRecipeCacheSizeLimit()
	expected := int64(100 * 1024 * 1024)
	if limit != expected {
		t.Errorf("GetRecipeCacheSizeLimit() = %d, want %d", limit, expected)
	}
}

func TestGetRecipeCacheSizeLimit_HumanReadable(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvRecipeCacheSizeLimit)
	defer os.Setenv(EnvRecipeCacheSizeLimit, original)

	tests := []struct {
		envValue string
		expected int64
	}{
		{"100MB", 100 * 1024 * 1024},
		{"100M", 100 * 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"1G", 1024 * 1024 * 1024},
		{"5M", 5 * 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv(EnvRecipeCacheSizeLimit, tt.envValue)
			limit := GetRecipeCacheSizeLimit()
			if limit != tt.expected {
				t.Errorf("GetRecipeCacheSizeLimit() with %q = %d, want %d", tt.envValue, limit, tt.expected)
			}
		})
	}
}

func TestGetRecipeCacheSizeLimit_InvalidValue(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvRecipeCacheSizeLimit)
	defer os.Setenv(EnvRecipeCacheSizeLimit, original)

	// Set invalid value
	os.Setenv(EnvRecipeCacheSizeLimit, "invalid")

	limit := GetRecipeCacheSizeLimit()
	// Should return default on invalid input
	if limit != DefaultRecipeCacheSizeLimit {
		t.Errorf("GetRecipeCacheSizeLimit() = %d, want %d (default)", limit, DefaultRecipeCacheSizeLimit)
	}
}

func TestGetRecipeCacheSizeLimit_TooLow(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvRecipeCacheSizeLimit)
	defer os.Setenv(EnvRecipeCacheSizeLimit, original)

	// Set too low value (minimum is 1MB)
	os.Setenv(EnvRecipeCacheSizeLimit, "100K")

	limit := GetRecipeCacheSizeLimit()
	// Should return minimum 1MB
	expected := int64(1 * 1024 * 1024)
	if limit != expected {
		t.Errorf("GetRecipeCacheSizeLimit() = %d, want %d (minimum)", limit, expected)
	}
}

func TestGetRecipeCacheSizeLimit_TooHigh(t *testing.T) {
	// Save original env value
	original := os.Getenv(EnvRecipeCacheSizeLimit)
	defer os.Setenv(EnvRecipeCacheSizeLimit, original)

	// Set too high value (maximum is 10GB)
	os.Setenv(EnvRecipeCacheSizeLimit, "20GB")

	limit := GetRecipeCacheSizeLimit()
	// Should return maximum 10GB
	expected := int64(10 * 1024 * 1024 * 1024)
	if limit != expected {
		t.Errorf("GetRecipeCacheSizeLimit() = %d, want %d (maximum)", limit, expected)
	}
}

func TestGetRecipeCacheMaxStale_Default(t *testing.T) {
	original := os.Getenv(EnvRecipeCacheMaxStale)
	defer os.Setenv(EnvRecipeCacheMaxStale, original)

	_ = os.Unsetenv(EnvRecipeCacheMaxStale)

	maxStale := GetRecipeCacheMaxStale()
	if maxStale != DefaultRecipeCacheMaxStale {
		t.Errorf("GetRecipeCacheMaxStale() = %v, want %v", maxStale, DefaultRecipeCacheMaxStale)
	}
}

func TestGetRecipeCacheMaxStale_CustomValue(t *testing.T) {
	original := os.Getenv(EnvRecipeCacheMaxStale)
	defer os.Setenv(EnvRecipeCacheMaxStale, original)

	tests := []struct {
		envValue string
		expected time.Duration
	}{
		{"24h", 24 * time.Hour},
		{"48h", 48 * time.Hour},
		{"168h", 168 * time.Hour},
		{"3d", 3 * 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"14D", 14 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv(EnvRecipeCacheMaxStale, tt.envValue)
			maxStale := GetRecipeCacheMaxStale()
			if maxStale != tt.expected {
				t.Errorf("GetRecipeCacheMaxStale() with %q = %v, want %v", tt.envValue, maxStale, tt.expected)
			}
		})
	}
}

func TestGetRecipeCacheMaxStale_Zero(t *testing.T) {
	original := os.Getenv(EnvRecipeCacheMaxStale)
	defer os.Setenv(EnvRecipeCacheMaxStale, original)

	// Setting to 0 should disable stale fallback
	os.Setenv(EnvRecipeCacheMaxStale, "0")

	maxStale := GetRecipeCacheMaxStale()
	if maxStale != 0 {
		t.Errorf("GetRecipeCacheMaxStale() = %v, want 0", maxStale)
	}
}

func TestGetRecipeCacheMaxStale_InvalidValue(t *testing.T) {
	original := os.Getenv(EnvRecipeCacheMaxStale)
	defer os.Setenv(EnvRecipeCacheMaxStale, original)

	os.Setenv(EnvRecipeCacheMaxStale, "invalid")

	maxStale := GetRecipeCacheMaxStale()
	if maxStale != DefaultRecipeCacheMaxStale {
		t.Errorf("GetRecipeCacheMaxStale() = %v, want %v (default)", maxStale, DefaultRecipeCacheMaxStale)
	}
}

func TestGetRecipeCacheMaxStale_TooLow(t *testing.T) {
	original := os.Getenv(EnvRecipeCacheMaxStale)
	defer os.Setenv(EnvRecipeCacheMaxStale, original)

	// Set too low value (minimum is 1h, unless 0 for disabled)
	os.Setenv(EnvRecipeCacheMaxStale, "5m")

	maxStale := GetRecipeCacheMaxStale()
	if maxStale != 1*time.Hour {
		t.Errorf("GetRecipeCacheMaxStale() = %v, want 1h (minimum)", maxStale)
	}
}

func TestGetRecipeCacheMaxStale_TooHigh(t *testing.T) {
	original := os.Getenv(EnvRecipeCacheMaxStale)
	defer os.Setenv(EnvRecipeCacheMaxStale, original)

	// Set too high value (maximum is 30 days)
	os.Setenv(EnvRecipeCacheMaxStale, "60d")

	maxStale := GetRecipeCacheMaxStale()
	expected := 30 * 24 * time.Hour
	if maxStale != expected {
		t.Errorf("GetRecipeCacheMaxStale() = %v, want %v (maximum)", maxStale, expected)
	}
}

func TestGetRecipeCacheStaleFallback_Default(t *testing.T) {
	original := os.Getenv(EnvRecipeCacheStaleFallback)
	defer os.Setenv(EnvRecipeCacheStaleFallback, original)

	_ = os.Unsetenv(EnvRecipeCacheStaleFallback)

	fallback := GetRecipeCacheStaleFallback()
	if !fallback {
		t.Errorf("GetRecipeCacheStaleFallback() = false, want true (default)")
	}
}

func TestGetRecipeCacheStaleFallback_Enabled(t *testing.T) {
	original := os.Getenv(EnvRecipeCacheStaleFallback)
	defer os.Setenv(EnvRecipeCacheStaleFallback, original)

	for _, value := range []string{"true", "TRUE", "True", "1", "yes", "YES", "on", "ON"} {
		t.Run(value, func(t *testing.T) {
			os.Setenv(EnvRecipeCacheStaleFallback, value)
			fallback := GetRecipeCacheStaleFallback()
			if !fallback {
				t.Errorf("GetRecipeCacheStaleFallback() with %q = false, want true", value)
			}
		})
	}
}

func TestGetRecipeCacheStaleFallback_Disabled(t *testing.T) {
	original := os.Getenv(EnvRecipeCacheStaleFallback)
	defer os.Setenv(EnvRecipeCacheStaleFallback, original)

	for _, value := range []string{"false", "FALSE", "False", "0", "no", "NO", "off", "OFF"} {
		t.Run(value, func(t *testing.T) {
			os.Setenv(EnvRecipeCacheStaleFallback, value)
			fallback := GetRecipeCacheStaleFallback()
			if fallback {
				t.Errorf("GetRecipeCacheStaleFallback() with %q = true, want false", value)
			}
		})
	}
}

func TestGetRecipeCacheStaleFallback_InvalidValue(t *testing.T) {
	original := os.Getenv(EnvRecipeCacheStaleFallback)
	defer os.Setenv(EnvRecipeCacheStaleFallback, original)

	os.Setenv(EnvRecipeCacheStaleFallback, "invalid")

	fallback := GetRecipeCacheStaleFallback()
	if !fallback {
		t.Errorf("GetRecipeCacheStaleFallback() with invalid value = false, want true (default)")
	}
}
