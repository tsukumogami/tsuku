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

func TestGetLLMDailyBudget(t *testing.T) {
	// Default (nil) should return DefaultDailyBudget
	cfg := DefaultConfig()
	val, ok := cfg.Get("llm.daily_budget")
	if !ok {
		t.Error("expected llm.daily_budget key to exist")
	}
	if val != "5" {
		t.Errorf("expected '5' for default, got %q", val)
	}

	// Explicitly set to 10
	budget := 10.0
	cfg.LLM.DailyBudget = &budget
	val, ok = cfg.Get("llm.daily_budget")
	if !ok {
		t.Error("expected llm.daily_budget key to exist")
	}
	if val != "10" {
		t.Errorf("expected '10', got %q", val)
	}

	// Explicitly set to 0 (disabled)
	budget = 0.0
	cfg.LLM.DailyBudget = &budget
	val, ok = cfg.Get("llm.daily_budget")
	if !ok {
		t.Error("expected llm.daily_budget key to exist")
	}
	if val != "0" {
		t.Errorf("expected '0', got %q", val)
	}
}

func TestGetLLMHourlyRateLimit(t *testing.T) {
	// Default (nil) should return DefaultHourlyRateLimit
	cfg := DefaultConfig()
	val, ok := cfg.Get("llm.hourly_rate_limit")
	if !ok {
		t.Error("expected llm.hourly_rate_limit key to exist")
	}
	if val != "10" {
		t.Errorf("expected '10' for default, got %q", val)
	}

	// Explicitly set to 20
	limit := 20
	cfg.LLM.HourlyRateLimit = &limit
	val, ok = cfg.Get("llm.hourly_rate_limit")
	if !ok {
		t.Error("expected llm.hourly_rate_limit key to exist")
	}
	if val != "20" {
		t.Errorf("expected '20', got %q", val)
	}

	// Explicitly set to 0 (disabled)
	limit = 0
	cfg.LLM.HourlyRateLimit = &limit
	val, ok = cfg.Get("llm.hourly_rate_limit")
	if !ok {
		t.Error("expected llm.hourly_rate_limit key to exist")
	}
	if val != "0" {
		t.Errorf("expected '0', got %q", val)
	}
}

func TestSetLLMDailyBudget(t *testing.T) {
	cfg := DefaultConfig()

	if err := cfg.Set("llm.daily_budget", "10"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMDailyBudget() != 10.0 {
		t.Errorf("expected LLMDailyBudget()=10.0, got %v", cfg.LLMDailyBudget())
	}

	// Set to 0 (disabled)
	if err := cfg.Set("llm.daily_budget", "0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMDailyBudget() != 0.0 {
		t.Errorf("expected LLMDailyBudget()=0.0, got %v", cfg.LLMDailyBudget())
	}

	// Set with decimal
	if err := cfg.Set("llm.daily_budget", "2.5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMDailyBudget() != 2.5 {
		t.Errorf("expected LLMDailyBudget()=2.5, got %v", cfg.LLMDailyBudget())
	}

	// Test case insensitivity
	if err := cfg.Set("LLM.DAILY_BUDGET", "7"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMDailyBudget() != 7.0 {
		t.Errorf("expected LLMDailyBudget()=7.0 (case insensitive), got %v", cfg.LLMDailyBudget())
	}
}

func TestSetLLMDailyBudgetInvalid(t *testing.T) {
	cfg := DefaultConfig()

	// Non-numeric value
	err := cfg.Set("llm.daily_budget", "invalid")
	if err == nil {
		t.Error("expected error for non-numeric value")
	}

	// Negative value
	err = cfg.Set("llm.daily_budget", "-5")
	if err == nil {
		t.Error("expected error for negative value")
	}
}

func TestSetLLMHourlyRateLimit(t *testing.T) {
	cfg := DefaultConfig()

	if err := cfg.Set("llm.hourly_rate_limit", "20"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMHourlyRateLimit() != 20 {
		t.Errorf("expected LLMHourlyRateLimit()=20, got %v", cfg.LLMHourlyRateLimit())
	}

	// Set to 0 (disabled)
	if err := cfg.Set("llm.hourly_rate_limit", "0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMHourlyRateLimit() != 0 {
		t.Errorf("expected LLMHourlyRateLimit()=0, got %v", cfg.LLMHourlyRateLimit())
	}

	// Test case insensitivity
	if err := cfg.Set("LLM.HOURLY_RATE_LIMIT", "15"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMHourlyRateLimit() != 15 {
		t.Errorf("expected LLMHourlyRateLimit()=15 (case insensitive), got %v", cfg.LLMHourlyRateLimit())
	}
}

func TestSetLLMHourlyRateLimitInvalid(t *testing.T) {
	cfg := DefaultConfig()

	// Non-integer value
	err := cfg.Set("llm.hourly_rate_limit", "invalid")
	if err == nil {
		t.Error("expected error for non-integer value")
	}

	// Float value (should fail for int)
	err = cfg.Set("llm.hourly_rate_limit", "5.5")
	if err == nil {
		t.Error("expected error for float value")
	}

	// Negative value
	err = cfg.Set("llm.hourly_rate_limit", "-5")
	if err == nil {
		t.Error("expected error for negative value")
	}
}

func TestLLMDailyBudgetDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.LLMDailyBudget() != DefaultDailyBudget {
		t.Errorf("expected LLMDailyBudget() to default to %v, got %v", DefaultDailyBudget, cfg.LLMDailyBudget())
	}
}

func TestLLMHourlyRateLimitDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.LLMHourlyRateLimit() != DefaultHourlyRateLimit {
		t.Errorf("expected LLMHourlyRateLimit() to default to %v, got %v", DefaultHourlyRateLimit, cfg.LLMHourlyRateLimit())
	}
}

func TestLoadLLMBudgetConfigFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	// Write config with LLM budget settings
	content := `telemetry = true

[llm]
enabled = true
daily_budget = 10.0
hourly_rate_limit = 20
`
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMDailyBudget() != 10.0 {
		t.Errorf("expected LLMDailyBudget()=10.0 from file, got %v", cfg.LLMDailyBudget())
	}
	if cfg.LLMHourlyRateLimit() != 20 {
		t.Errorf("expected LLMHourlyRateLimit()=20 from file, got %v", cfg.LLMHourlyRateLimit())
	}
}

func TestSaveLLMBudgetConfig(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	cfg := DefaultConfig()
	budget := 15.0
	limit := 25
	cfg.LLM.DailyBudget = &budget
	cfg.LLM.HourlyRateLimit = &limit

	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if loaded.LLMDailyBudget() != 15.0 {
		t.Errorf("expected LLMDailyBudget()=15.0 after save/load, got %v", loaded.LLMDailyBudget())
	}
	if loaded.LLMHourlyRateLimit() != 25 {
		t.Errorf("expected LLMHourlyRateLimit()=25 after save/load, got %v", loaded.LLMHourlyRateLimit())
	}
}

func TestAvailableKeysIncludesBudgetSettings(t *testing.T) {
	keys := AvailableKeys()
	if _, ok := keys["llm.daily_budget"]; !ok {
		t.Error("expected llm.daily_budget in available keys")
	}
	if _, ok := keys["llm.hourly_rate_limit"]; !ok {
		t.Error("expected llm.hourly_rate_limit in available keys")
	}
}
