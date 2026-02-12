// Package userconfig provides user configuration management for tsuku.
// Configuration is stored in ~/.tsuku/config.toml and can be modified
// via the `tsuku config` command.
package userconfig

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/tsukumogami/tsuku/internal/config"
)

// Config represents user-configurable settings.
type Config struct {
	// Telemetry enables or disables anonymous usage statistics.
	// Default is true (enabled).
	Telemetry bool `toml:"telemetry"`

	// LLM contains LLM-related configuration.
	LLM LLMConfig `toml:"llm"`
}

// LLMConfig holds LLM-specific settings.
type LLMConfig struct {
	// Enabled enables or disables LLM features.
	// Default is true (enabled).
	Enabled *bool `toml:"enabled,omitempty"`

	// LocalEnabled enables or disables local LLM inference via tsuku-llm addon.
	// When false, LocalProvider is not registered in the factory.
	// Default is true (enabled).
	LocalEnabled *bool `toml:"local_enabled,omitempty"`

	// LocalPreemptive starts the addon server at the beginning of tsuku create
	// to hide model loading latency. When false, server starts on first inference.
	// Default is true (enabled).
	LocalPreemptive *bool `toml:"local_preemptive,omitempty"`

	// IdleTimeout specifies how long the addon server stays alive after the last request.
	// Accepts Go duration format (e.g., "5m", "30s").
	// Default is "5m". Can be overridden by TSUKU_LLM_IDLE_TIMEOUT env var.
	IdleTimeout string `toml:"idle_timeout,omitempty"`

	// Providers specifies the preferred provider order.
	// The first provider in the list becomes the primary.
	// Empty means auto-detect from environment variables.
	Providers []string `toml:"providers,omitempty"`

	// DailyBudget is the maximum daily LLM cost in USD.
	// Default is $5. Set to 0 to disable the limit.
	DailyBudget *float64 `toml:"daily_budget,omitempty"`

	// HourlyRateLimit is the maximum LLM generations per hour.
	// Default is 10. Set to 0 to disable the limit.
	HourlyRateLimit *int `toml:"hourly_rate_limit,omitempty"`
}

const (
	// DefaultDailyBudget is the default daily LLM cost limit in USD.
	DefaultDailyBudget = 5.0

	// DefaultHourlyRateLimit is the default maximum LLM generations per hour.
	DefaultHourlyRateLimit = 10

	// DefaultIdleTimeout is the default addon idle timeout.
	DefaultIdleTimeout = 5 * time.Minute

	// IdleTimeoutEnvVar is the env var that overrides the idle timeout config.
	IdleTimeoutEnvVar = "TSUKU_LLM_IDLE_TIMEOUT"
)

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		Telemetry: true, // Enabled by default
	}
}

// Load reads the config file and returns the configuration.
// Returns default values if the file doesn't exist.
// Returns an error only for file parsing issues, not missing files.
func Load() (*Config, error) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		return DefaultConfig(), nil // Silently use defaults
	}

	return loadFromPath(cfg.ConfigFile)
}

// loadFromPath reads config from a specific file path (for testing).
func loadFromPath(path string) (*Config, error) {
	userCfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return userCfg, nil // File doesn't exist, use defaults
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if _, err := toml.Decode(string(data), userCfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return userCfg, nil
}

// Save writes the configuration to the config file.
func (c *Config) Save() error {
	cfg, err := config.DefaultConfig()
	if err != nil {
		return fmt.Errorf("failed to get config path: %w", err)
	}

	return c.saveToPath(cfg.ConfigFile)
}

// saveToPath writes config to a specific file path (for testing).
func (c *Config) saveToPath(path string) error {
	// Ensure parent directory exists
	dir := path[:strings.LastIndex(path, "/")]
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(c); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// LLMEnabled returns whether LLM features are enabled.
// Returns true if not explicitly set (default behavior).
func (c *Config) LLMEnabled() bool {
	if c.LLM.Enabled == nil {
		return true // Default to enabled
	}
	return *c.LLM.Enabled
}

// LLMLocalEnabled returns whether local LLM inference is enabled.
// Returns true if not explicitly set (default behavior).
func (c *Config) LLMLocalEnabled() bool {
	if c.LLM.LocalEnabled == nil {
		return true // Default to enabled
	}
	return *c.LLM.LocalEnabled
}

// LLMLocalPreemptive returns whether to start the addon server preemptively.
// Returns true if not explicitly set (default behavior).
func (c *Config) LLMLocalPreemptive() bool {
	if c.LLM.LocalPreemptive == nil {
		return true // Default to enabled
	}
	return *c.LLM.LocalPreemptive
}

// LLMIdleTimeout returns the idle timeout for the addon server.
// The TSUKU_LLM_IDLE_TIMEOUT env var takes precedence over the config file.
// Returns DefaultIdleTimeout (5m) if not configured.
func (c *Config) LLMIdleTimeout() time.Duration {
	// Check env var first (takes precedence)
	if envVal := os.Getenv(IdleTimeoutEnvVar); envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil {
			return d
		}
	}

	// Check config file value
	if c.LLM.IdleTimeout != "" {
		if d, err := time.ParseDuration(c.LLM.IdleTimeout); err == nil {
			return d
		}
	}

	return DefaultIdleTimeout
}

// LLMProviders returns the configured provider order.
// Returns nil if not set (use auto-detection).
func (c *Config) LLMProviders() []string {
	return c.LLM.Providers
}

// LLMDailyBudget returns the daily LLM cost limit in USD.
// Returns DefaultDailyBudget if not explicitly set.
func (c *Config) LLMDailyBudget() float64 {
	if c.LLM.DailyBudget == nil {
		return DefaultDailyBudget
	}
	return *c.LLM.DailyBudget
}

// LLMHourlyRateLimit returns the maximum LLM generations per hour.
// Returns DefaultHourlyRateLimit if not explicitly set.
func (c *Config) LLMHourlyRateLimit() int {
	if c.LLM.HourlyRateLimit == nil {
		return DefaultHourlyRateLimit
	}
	return *c.LLM.HourlyRateLimit
}

// Get returns the value of a config key as a string.
// Returns empty string and false if the key doesn't exist.
func (c *Config) Get(key string) (string, bool) {
	switch strings.ToLower(key) {
	case "telemetry":
		return strconv.FormatBool(c.Telemetry), true
	case "llm.enabled":
		return strconv.FormatBool(c.LLMEnabled()), true
	case "llm.local_enabled":
		return strconv.FormatBool(c.LLMLocalEnabled()), true
	case "llm.local_preemptive":
		return strconv.FormatBool(c.LLMLocalPreemptive()), true
	case "llm.idle_timeout":
		return c.LLMIdleTimeout().String(), true
	case "llm.providers":
		if len(c.LLM.Providers) == 0 {
			return "", true
		}
		return strings.Join(c.LLM.Providers, ","), true
	case "llm.daily_budget":
		return strconv.FormatFloat(c.LLMDailyBudget(), 'g', -1, 64), true
	case "llm.hourly_rate_limit":
		return strconv.Itoa(c.LLMHourlyRateLimit()), true
	default:
		return "", false
	}
}

// Set updates a config value from a string.
// Returns an error if the key doesn't exist or the value is invalid.
func (c *Config) Set(key, value string) error {
	switch strings.ToLower(key) {
	case "telemetry":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for telemetry: must be true or false")
		}
		c.Telemetry = b
		return nil
	case "llm.enabled":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for llm.enabled: must be true or false")
		}
		c.LLM.Enabled = &b
		return nil
	case "llm.local_enabled":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for llm.local_enabled: must be true or false")
		}
		c.LLM.LocalEnabled = &b
		return nil
	case "llm.local_preemptive":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value for llm.local_preemptive: must be true or false")
		}
		c.LLM.LocalPreemptive = &b
		return nil
	case "llm.idle_timeout":
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("invalid value for llm.idle_timeout: must be a duration (e.g., 5m, 30s)")
		}
		c.LLM.IdleTimeout = value
		return nil
	case "llm.providers":
		if value == "" {
			c.LLM.Providers = nil
			return nil
		}
		providers := strings.Split(value, ",")
		for i, p := range providers {
			providers[i] = strings.TrimSpace(p)
		}
		c.LLM.Providers = providers
		return nil
	case "llm.daily_budget":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid value for llm.daily_budget: must be a number")
		}
		if f < 0 {
			return fmt.Errorf("invalid value for llm.daily_budget: must be non-negative")
		}
		c.LLM.DailyBudget = &f
		return nil
	case "llm.hourly_rate_limit":
		i, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid value for llm.hourly_rate_limit: must be an integer")
		}
		if i < 0 {
			return fmt.Errorf("invalid value for llm.hourly_rate_limit: must be non-negative")
		}
		c.LLM.HourlyRateLimit = &i
		return nil
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
}

// AvailableKeys returns a list of all configurable keys with descriptions.
func AvailableKeys() map[string]string {
	return map[string]string{
		"telemetry":             "Enable anonymous usage statistics (true/false)",
		"llm.enabled":           "Enable LLM features for recipe generation (true/false)",
		"llm.local_enabled":     "Enable local LLM inference via tsuku-llm addon (true/false)",
		"llm.local_preemptive":  "Start local LLM addon early to hide loading latency (true/false)",
		"llm.idle_timeout":      "How long addon stays alive after last request (e.g., 5m, 30s)",
		"llm.providers":         "Preferred LLM provider order (comma-separated, e.g., claude,gemini)",
		"llm.daily_budget":      "Daily LLM cost limit in USD (default: 5.0, 0 to disable)",
		"llm.hourly_rate_limit": "Max LLM generations per hour (default: 10, 0 to disable)",
	}
}
