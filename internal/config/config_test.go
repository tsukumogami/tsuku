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
	dirs := []string{cfg.HomeDir, cfg.ToolsDir, cfg.CurrentDir, cfg.RecipesDir, cfg.RegistryDir, cfg.LibsDir, cfg.CacheDir, cfg.VersionCacheDir, cfg.DownloadCacheDir, cfg.KeyCacheDir, cfg.TapCacheDir}
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
