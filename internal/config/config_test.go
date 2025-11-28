package config

import (
	"os"
	"path/filepath"
	"testing"
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
	if cfg.ConfigFile != filepath.Join(expectedHome, "config.toml") {
		t.Errorf("ConfigFile = %q, want %q", cfg.ConfigFile, filepath.Join(expectedHome, "config.toml"))
	}
}

func TestEnsureDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Config{
		HomeDir:     filepath.Join(tmpDir, "tsuku"),
		ToolsDir:    filepath.Join(tmpDir, "tsuku", "tools"),
		CurrentDir:  filepath.Join(tmpDir, "tsuku", "tools", "current"),
		RecipesDir:  filepath.Join(tmpDir, "tsuku", "recipes"),
		RegistryDir: filepath.Join(tmpDir, "tsuku", "registry"),
	}

	err := cfg.EnsureDirectories()
	if err != nil {
		t.Fatalf("EnsureDirectories() failed: %v", err)
	}

	// Verify all directories exist
	dirs := []string{cfg.HomeDir, cfg.ToolsDir, cfg.CurrentDir, cfg.RecipesDir, cfg.RegistryDir}
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
