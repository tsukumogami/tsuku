package userconfig

import (
	"os"
	"path/filepath"
	"strings"
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

// --- Secrets section tests (Scenario 7) ---

func TestSetSecretStoresInSecretsMap(t *testing.T) {
	cfg := DefaultConfig()

	if err := cfg.Set("secrets.foo_key", "bar_value"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Secrets == nil {
		t.Fatal("expected Secrets map to be initialized")
	}
	if cfg.Secrets["foo_key"] != "bar_value" {
		t.Errorf("expected Secrets[\"foo_key\"]=\"bar_value\", got %q", cfg.Secrets["foo_key"])
	}
}

func TestGetSecretRetrievesFromSecretsMap(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Secrets = map[string]string{
		"test_api_key": "secret-123",
	}

	val, ok := cfg.Get("secrets.test_api_key")
	if !ok {
		t.Error("expected secrets.test_api_key to be found")
	}
	if val != "secret-123" {
		t.Errorf("expected 'secret-123', got %q", val)
	}
}

func TestGetSecretReturnsFalseWhenMissing(t *testing.T) {
	cfg := DefaultConfig()

	_, ok := cfg.Get("secrets.nonexistent")
	if ok {
		t.Error("expected false for missing secret")
	}
}

func TestGetSecretReturnsFalseWhenEmpty(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Secrets = map[string]string{
		"empty_key": "",
	}

	_, ok := cfg.Get("secrets.empty_key")
	if ok {
		t.Error("expected false for empty secret value")
	}
}

func TestSetSecretIsCaseInsensitive(t *testing.T) {
	cfg := DefaultConfig()

	if err := cfg.Set("SECRETS.My_Key", "value"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The stored key should be lowercase.
	if cfg.Secrets["my_key"] != "value" {
		t.Errorf("expected Secrets[\"my_key\"]=\"value\", got %q", cfg.Secrets["my_key"])
	}
}

func TestSetSecretInitializesNilMap(t *testing.T) {
	cfg := &Config{Telemetry: true}
	if cfg.Secrets != nil {
		t.Fatal("precondition: Secrets should be nil")
	}

	if err := cfg.Set("secrets.key", "val"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Secrets == nil {
		t.Error("expected Secrets map to be initialized after Set")
	}
}

func TestSecretsSaveAndLoadRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	cfg := DefaultConfig()
	cfg.Secrets = map[string]string{
		"anthropic_api_key": "sk-ant-test",
		"github_token":      "ghp_test",
	}

	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	if loaded.Secrets == nil {
		t.Fatal("expected Secrets map to be loaded")
	}
	if loaded.Secrets["anthropic_api_key"] != "sk-ant-test" {
		t.Errorf("expected 'sk-ant-test', got %q", loaded.Secrets["anthropic_api_key"])
	}
	if loaded.Secrets["github_token"] != "ghp_test" {
		t.Errorf("expected 'ghp_test', got %q", loaded.Secrets["github_token"])
	}
}

func TestSecretsSerializeToTOMLSection(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	cfg := DefaultConfig()
	cfg.Secrets = map[string]string{
		"test_key": "test_value",
	}

	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "[secrets]") {
		t.Error("expected [secrets] section in TOML output")
	}
	if !strings.Contains(content, "test_key") {
		t.Error("expected test_key in TOML output")
	}
}

func TestAvailableKeysDoesNotIncludeSecrets(t *testing.T) {
	keys := AvailableKeys()
	for k := range keys {
		if strings.HasPrefix(k, "secrets.") {
			t.Errorf("AvailableKeys() should not include secrets keys, found %q", k)
		}
	}
}

func TestSecretsNotAffectExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	cfg := DefaultConfig()
	cfg.Telemetry = false
	cfg.Secrets = map[string]string{
		"my_key": "my_value",
	}

	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	// Existing config should be preserved.
	if loaded.Telemetry {
		t.Error("expected Telemetry=false to be preserved")
	}
	if loaded.Secrets["my_key"] != "my_value" {
		t.Errorf("expected Secrets[\"my_key\"]=\"my_value\", got %q", loaded.Secrets["my_key"])
	}
}

// --- Atomic write and permission tests (Scenario 8) ---

func TestAtomicWriteProduces0600Permissions(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	cfg := DefaultConfig()
	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected permissions 0600, got %04o", perm)
	}
}

func TestAtomicWritePreserves0600OnOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	// First write
	cfg := DefaultConfig()
	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Manually loosen permissions to simulate an older file
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatalf("failed to chmod: %v", err)
	}

	// Second write should restore 0600 via atomic replace
	cfg.Telemetry = false
	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save (2nd): %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected permissions 0600 after overwrite, got %04o", perm)
	}
}

func TestAtomicWriteDoesNotLeaveTemps(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	cfg := DefaultConfig()
	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to readdir: %v", err)
	}

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".config.toml.tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestAtomicWriteContentIntegrity(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	cfg := DefaultConfig()
	cfg.Telemetry = false
	cfg.Secrets = map[string]string{
		"key1": "val1",
		"key2": "val2",
	}

	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	if loaded.Telemetry != false {
		t.Error("expected Telemetry=false")
	}
	if loaded.Secrets["key1"] != "val1" {
		t.Errorf("expected key1=val1, got %q", loaded.Secrets["key1"])
	}
	if loaded.Secrets["key2"] != "val2" {
		t.Errorf("expected key2=val2, got %q", loaded.Secrets["key2"])
	}
}

func TestPermissionWarningOnPermissiveFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	// Write with 0644 permissions (simulating an older config file).
	err := os.WriteFile(path, []byte("telemetry = true\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// loadFromPath should succeed even with permissive permissions.
	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Telemetry {
		t.Error("expected Telemetry=true")
	}
}

func TestPermissionWarningNotTriggeredFor0600(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	// Write with correct 0600 permissions.
	err := os.WriteFile(path, []byte("telemetry = true\n"), 0600)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// loadFromPath should succeed without any permission issue.
	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Telemetry {
		t.Error("expected Telemetry=true")
	}
}

func TestAtomicWriteCreatesParentDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "dir", "config.toml")

	cfg := DefaultConfig()
	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("config file was not created in nested directory")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %04o", info.Mode().Perm())
	}
}

// --- LLM backend config tests (Scenario 15) ---

func TestGetLLMBackendDefault(t *testing.T) {
	cfg := DefaultConfig()
	val, ok := cfg.Get("llm.backend")
	if !ok {
		t.Error("expected llm.backend key to exist")
	}
	if val != "" {
		t.Errorf("expected empty string for default, got %q", val)
	}
}

func TestLLMBackendDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.LLMBackend() != "" {
		t.Errorf("expected LLMBackend() to default to empty string, got %q", cfg.LLMBackend())
	}
}

func TestSetLLMBackendCPU(t *testing.T) {
	cfg := DefaultConfig()

	if err := cfg.Set("llm.backend", "cpu"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMBackend() != "cpu" {
		t.Errorf("expected LLMBackend()='cpu', got %q", cfg.LLMBackend())
	}

	// Verify via Get
	val, ok := cfg.Get("llm.backend")
	if !ok {
		t.Error("expected llm.backend key to exist")
	}
	if val != "cpu" {
		t.Errorf("expected 'cpu' via Get, got %q", val)
	}
}

func TestSetLLMBackendClearWithEmptyString(t *testing.T) {
	cfg := DefaultConfig()

	// Set to cpu first
	if err := cfg.Set("llm.backend", "cpu"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMBackend() != "cpu" {
		t.Errorf("expected LLMBackend()='cpu', got %q", cfg.LLMBackend())
	}

	// Clear with empty string
	if err := cfg.Set("llm.backend", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMBackend() != "" {
		t.Errorf("expected LLMBackend()='' after clearing, got %q", cfg.LLMBackend())
	}

	// Backend pointer should be nil after clearing
	if cfg.LLM.Backend != nil {
		t.Error("expected Backend to be nil after clearing with empty string")
	}
}

func TestSetLLMBackendInvalid(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.Set("llm.backend", "invalid")
	if err == nil {
		t.Error("expected error for invalid backend value")
	}

	// Error message should list valid values
	if err != nil && !strings.Contains(err.Error(), "cpu") {
		t.Errorf("error should mention valid values, got: %v", err)
	}
}

func TestSetLLMBackendCaseInsensitiveKey(t *testing.T) {
	cfg := DefaultConfig()

	if err := cfg.Set("LLM.BACKEND", "cpu"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMBackend() != "cpu" {
		t.Errorf("expected LLMBackend()='cpu' (case insensitive key), got %q", cfg.LLMBackend())
	}
}

func TestLLMBackendTOMLRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	cfg := DefaultConfig()
	backend := "cpu"
	cfg.LLM.Backend = &backend

	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if loaded.LLMBackend() != "cpu" {
		t.Errorf("expected LLMBackend()='cpu' after save/load, got %q", loaded.LLMBackend())
	}
}

func TestLLMBackendTOMLRoundTripUnset(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	cfg := DefaultConfig()
	// Backend is nil (unset)

	if err := cfg.saveToPath(path); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if loaded.LLMBackend() != "" {
		t.Errorf("expected LLMBackend()='' after save/load of unset value, got %q", loaded.LLMBackend())
	}
}

func TestLoadLLMBackendFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	content := `telemetry = true

[llm]
backend = "cpu"
`
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LLMBackend() != "cpu" {
		t.Errorf("expected LLMBackend()='cpu' from file, got %q", cfg.LLMBackend())
	}
}

func TestAvailableKeysIncludesLLMBackend(t *testing.T) {
	keys := AvailableKeys()
	desc, ok := keys["llm.backend"]
	if !ok {
		t.Error("expected llm.backend in available keys")
	}
	if !strings.Contains(desc, "cpu") {
		t.Errorf("expected llm.backend description to mention cpu, got %q", desc)
	}
}

func TestSetLLMBackendRejectsMultipleInvalidValues(t *testing.T) {
	cfg := DefaultConfig()

	invalidValues := []string{"cuda", "vulkan", "metal", "gpu", "auto", "GPU", "CPU"}
	for _, v := range invalidValues {
		err := cfg.Set("llm.backend", v)
		if err == nil {
			t.Errorf("expected error for invalid backend value %q", v)
		}
	}
}

func TestLoadSecretsFromTOMLFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.toml")

	content := `telemetry = true

[secrets]
anthropic_api_key = "sk-ant-from-file"
github_token = "ghp-from-file"
`
	err := os.WriteFile(path, []byte(content), 0600)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Secrets == nil {
		t.Fatal("expected Secrets map to be populated")
	}
	if cfg.Secrets["anthropic_api_key"] != "sk-ant-from-file" {
		t.Errorf("expected 'sk-ant-from-file', got %q", cfg.Secrets["anthropic_api_key"])
	}
	if cfg.Secrets["github_token"] != "ghp-from-file" {
		t.Errorf("expected 'ghp-from-file', got %q", cfg.Secrets["github_token"])
	}
}
