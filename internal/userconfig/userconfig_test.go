package userconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Telemetry {
		t.Error("expected Telemetry to default to true")
	}
}

func TestLoadMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Telemetry {
		t.Error("expected default Telemetry=true when file missing")
	}
}

func TestLoadExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	// Write config with telemetry disabled
	err := os.WriteFile(path, []byte("telemetry = false\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Telemetry {
		t.Error("expected Telemetry=false from file")
	}
}

func TestLoadInvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	// Write invalid TOML
	err := os.WriteFile(path, []byte("this is not valid toml [[["), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err = loadFromPath(path)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "config.toml")

	cfg := &Config{Telemetry: false}
	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if loaded.Telemetry != false {
		t.Error("expected Telemetry=false after save/load")
	}
}

func TestGetTelemetry(t *testing.T) {
	cfg := &Config{Telemetry: true}
	val, ok := cfg.Get("telemetry")
	if !ok {
		t.Error("expected telemetry key to exist")
	}
	if val != "true" {
		t.Errorf("expected 'true', got %q", val)
	}

	cfg.Telemetry = false
	val, ok = cfg.Get("telemetry")
	if !ok {
		t.Error("expected telemetry key to exist")
	}
	if val != "false" {
		t.Errorf("expected 'false', got %q", val)
	}
}

func TestGetUnknownKey(t *testing.T) {
	cfg := DefaultConfig()
	_, ok := cfg.Get("unknown")
	if ok {
		t.Error("expected unknown key to return false")
	}
}

func TestSetTelemetry(t *testing.T) {
	cfg := DefaultConfig()

	if err := cfg.Set("telemetry", "false"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Telemetry {
		t.Error("expected Telemetry=false")
	}

	if err := cfg.Set("telemetry", "true"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Telemetry {
		t.Error("expected Telemetry=true")
	}

	// Test case insensitivity
	if err := cfg.Set("TELEMETRY", "false"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Telemetry {
		t.Error("expected Telemetry=false (case insensitive)")
	}
}

func TestSetInvalidValue(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.Set("telemetry", "invalid")
	if err == nil {
		t.Error("expected error for invalid boolean value")
	}
}

func TestSetUnknownKey(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.Set("unknown", "value")
	if err == nil {
		t.Error("expected error for unknown key")
	}
}

func TestAvailableKeys(t *testing.T) {
	keys := AvailableKeys()
	if _, ok := keys["telemetry"]; !ok {
		t.Error("expected telemetry in available keys")
	}
}

func TestLoadWithTsukuHome(t *testing.T) {
	tmpDir := t.TempDir()

	// Write config file
	configPath := filepath.Join(tmpDir, "config.toml")
	err := os.WriteFile(configPath, []byte("telemetry = false\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Set TSUKU_HOME
	oldHome := os.Getenv("TSUKU_HOME")
	os.Setenv("TSUKU_HOME", tmpDir)
	defer os.Setenv("TSUKU_HOME", oldHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Telemetry {
		t.Error("expected Telemetry=false from TSUKU_HOME config")
	}
}

func TestLoadMissingHomeDir(t *testing.T) {
	// Set TSUKU_HOME to non-existent directory
	oldHome := os.Getenv("TSUKU_HOME")
	os.Setenv("TSUKU_HOME", "/nonexistent/path/tsuku")
	defer os.Setenv("TSUKU_HOME", oldHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return defaults when config doesn't exist
	if !cfg.Telemetry {
		t.Error("expected default Telemetry=true")
	}
}

func TestSaveWithTsukuHome(t *testing.T) {
	tmpDir := t.TempDir()

	// Set TSUKU_HOME
	oldHome := os.Getenv("TSUKU_HOME")
	os.Setenv("TSUKU_HOME", tmpDir)
	defer os.Setenv("TSUKU_HOME", oldHome)

	cfg := &Config{Telemetry: false}
	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(tmpDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Verify contents
	loaded, err := Load()
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if loaded.Telemetry {
		t.Error("expected Telemetry=false after save")
	}
}

func TestLoadReadError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a directory where the config file should be (causes read error)
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.MkdirAll(configPath, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	_, err := loadFromPath(configPath)
	if err == nil {
		t.Error("expected error when config path is a directory")
	}
}

func TestSaveToPathCreateError(t *testing.T) {
	// Try to save to an invalid path (no parent directory creation possible)
	cfg := &Config{Telemetry: false}

	// Use /dev/null/subdir which can't have a subdirectory
	err := cfg.saveToPath("/dev/null/subdir/config.toml")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}
