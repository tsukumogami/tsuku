// Package userconfig provides user configuration management for tsuku.
// Configuration is stored in ~/.tsuku/config.toml and can be modified
// via the `tsuku config` command.
package userconfig

import (
	"fmt"
	"os"
	"strconv"
	"strings"

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

	// Providers specifies the preferred provider order.
	// The first provider in the list becomes the primary.
	// Empty means auto-detect from environment variables.
	Providers []string `toml:"providers,omitempty"`
}

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

// LLMProviders returns the configured provider order.
// Returns nil if not set (use auto-detection).
func (c *Config) LLMProviders() []string {
	return c.LLM.Providers
}

// Get returns the value of a config key as a string.
// Returns empty string and false if the key doesn't exist.
func (c *Config) Get(key string) (string, bool) {
	switch strings.ToLower(key) {
	case "telemetry":
		return strconv.FormatBool(c.Telemetry), true
	case "llm.enabled":
		return strconv.FormatBool(c.LLMEnabled()), true
	case "llm.providers":
		if len(c.LLM.Providers) == 0 {
			return "", true
		}
		return strings.Join(c.LLM.Providers, ","), true
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
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
}

// AvailableKeys returns a list of all configurable keys with descriptions.
func AvailableKeys() map[string]string {
	return map[string]string{
		"telemetry":     "Enable anonymous usage statistics (true/false)",
		"llm.enabled":   "Enable LLM features for recipe generation (true/false)",
		"llm.providers": "Preferred LLM provider order (comma-separated, e.g., claude,gemini)",
	}
}
