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
	if _, ok := keys["llm.enabled"]; !ok {
		t.Error("expected llm.enabled in available keys")
	}
	if _, ok := keys["llm.providers"]; !ok {
		t.Error("expected llm.providers in available keys")
	}
}

func TestGetLLMEnabled(t *testing.T) {
	// Default (nil) should return true
	cfg := DefaultConfig()
	val, ok := cfg.Get("llm.enabled")
	if !ok {
		t.Error("expected llm.enabled key to exist")
	}
	if val != "true" {
		t.Errorf("expected 'true' for default, got %q", val)
	}

	// Explicitly set to false
	enabled := false
	cfg.LLM.Enabled = &enabled
	val, ok = cfg.Get("llm.enabled")
	if !ok {
		t.Error("expected llm.enabled key to exist")
	}
	if val != "false" {
		t.Errorf("expected 'false', got %q", val)
	}

	// Explicitly set to true
	enabled = true
	cfg.LLM.Enabled = &enabled
	val, ok = cfg.Get("llm.enabled")
	if !ok {
		t.Error("expected llm.enabled key to exist")
	}
	if val != "true" {
		t.Errorf("expected 'true', got %q", val)
	}
}

func TestGetLLMProviders(t *testing.T) {
	// Default (empty) should return empty string
	cfg := DefaultConfig()
	val, ok := cfg.Get("llm.providers")
	if !ok {
		t.Error("expected llm.providers key to exist")
	}
	if val != "" {
		t.Errorf("expected empty string for default, got %q", val)
	}

	// Set providers
	cfg.LLM.Providers = []string{"gemini", "claude"}
	val, ok = cfg.Get("llm.providers")
	if !ok {
		t.Error("expected llm.providers key to exist")
	}
	if val != "gemini,claude" {
		t.Errorf("expected 'gemini,claude', got %q", val)
	}
}

func TestSetLLMEnabled(t *testing.T) {
	cfg := DefaultConfig()

	if err := cfg.Set("llm.enabled", "false"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMEnabled() {
		t.Error("expected LLMEnabled()=false")
	}

	if err := cfg.Set("llm.enabled", "true"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.LLMEnabled() {
		t.Error("expected LLMEnabled()=true")
	}

	// Test case insensitivity
	if err := cfg.Set("LLM.ENABLED", "false"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMEnabled() {
		t.Error("expected LLMEnabled()=false (case insensitive)")
	}
}

func TestSetLLMEnabledInvalid(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.Set("llm.enabled", "invalid")
	if err == nil {
		t.Error("expected error for invalid boolean value")
	}
}

func TestSetLLMProviders(t *testing.T) {
	cfg := DefaultConfig()

	if err := cfg.Set("llm.providers", "gemini,claude"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	providers := cfg.LLMProviders()
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}
	if providers[0] != "gemini" || providers[1] != "claude" {
		t.Errorf("expected [gemini, claude], got %v", providers)
	}

	// Test with spaces
	if err := cfg.Set("llm.providers", "claude , gemini"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	providers = cfg.LLMProviders()
	if providers[0] != "claude" || providers[1] != "gemini" {
		t.Errorf("expected [claude, gemini], got %v", providers)
	}

	// Test clearing
	if err := cfg.Set("llm.providers", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMProviders() != nil {
		t.Error("expected nil providers after clearing")
	}
}

func TestLLMEnabledDefault(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.LLMEnabled() {
		t.Error("expected LLMEnabled() to default to true")
	}
}

func TestLLMProvidersDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.LLMProviders() != nil {
		t.Error("expected LLMProviders() to default to nil")
	}
}

func TestLoadLLMConfigFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	// Write config with LLM settings
	content := `telemetry = true

[llm]
enabled = false
providers = ["gemini", "claude"]
`
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMEnabled() {
		t.Error("expected LLMEnabled()=false from file")
	}
	providers := cfg.LLMProviders()
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}
	if providers[0] != "gemini" || providers[1] != "claude" {
		t.Errorf("expected [gemini, claude], got %v", providers)
	}
}

func TestSaveLLMConfig(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	cfg := DefaultConfig()
	enabled := false
	cfg.LLM.Enabled = &enabled
	cfg.LLM.Providers = []string{"claude", "gemini"}

	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if loaded.LLMEnabled() {
		t.Error("expected LLMEnabled()=false after save/load")
	}
	providers := loaded.LLMProviders()
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}
	if providers[0] != "claude" || providers[1] != "gemini" {
		t.Errorf("expected [claude, gemini], got %v", providers)
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
