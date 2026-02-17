// Package secrets provides centralized resolution of API keys and tokens.
//
// Secrets are resolved by checking environment variables first, then
// the [secrets] section in $TSUKU_HOME/config.toml. If neither source
// has a value, an error with guidance is returned.
//
// Each known secret is defined in the knownKeys table (specs.go), which maps
// a canonical name to one or more environment variable aliases. Requesting
// an unknown key returns an error.
package secrets

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// KeyInfo describes a registered secret for external consumers.
type KeyInfo struct {
	// Name is the canonical key name (e.g., "anthropic_api_key").
	Name string

	// EnvVars lists environment variables checked, in priority order.
	EnvVars []string

	// Desc is a human-readable description.
	Desc string
}

// cachedConfig holds the lazily loaded userconfig.
var (
	configOnce  sync.Once
	cachedCfg   *userconfig.Config
	configError error
)

// loadConfig loads the userconfig lazily on the first call.
func loadConfig() {
	configOnce.Do(func() {
		cachedCfg, configError = userconfig.Load()
	})
}

// getConfig returns the cached userconfig, loading it lazily if needed.
func getConfig() (*userconfig.Config, error) {
	loadConfig()
	return cachedCfg, configError
}

// ResetConfig resets the cached config so the next call to Get()/IsSet()
// reloads from disk. This is intended for testing only.
func ResetConfig() {
	configOnce = sync.Once{}
	cachedCfg = nil
	configError = nil
}

// Get resolves a secret by name, checking environment variables first,
// then the [secrets] section in config.toml.
// Returns the first non-empty value found, or an error if the key is
// unknown or no source has a value set.
func Get(name string) (string, error) {
	spec, ok := knownKeys[name]
	if !ok {
		return "", fmt.Errorf("unknown secret key: %q", name)
	}

	// Check environment variables in priority order.
	for _, env := range spec.EnvVars {
		if val := os.Getenv(env); val != "" {
			return val, nil
		}
	}

	// Fall through to config file.
	cfg, err := getConfig()
	if err == nil && cfg != nil && cfg.Secrets != nil {
		if val, ok := cfg.Secrets[name]; ok && val != "" {
			return val, nil
		}
	}

	// Build a guidance message listing all env var options.
	envList := strings.Join(spec.EnvVars, " or ")
	return "", fmt.Errorf(
		"%s not configured. Set the %s environment variable, or add %s to [secrets] in $TSUKU_HOME/config.toml",
		name, envList, name,
	)
}

// IsSet checks whether a secret is available without returning its value.
// Returns false for unknown keys.
func IsSet(name string) bool {
	spec, ok := knownKeys[name]
	if !ok {
		return false
	}

	for _, env := range spec.EnvVars {
		if os.Getenv(env) != "" {
			return true
		}
	}

	// Fall through to config file.
	cfg, err := getConfig()
	if err == nil && cfg != nil && cfg.Secrets != nil {
		if val, ok := cfg.Secrets[name]; ok && val != "" {
			return true
		}
	}

	return false
}

// KnownKeys returns metadata for all registered secrets, sorted by name.
func KnownKeys() []KeyInfo {
	keys := make([]KeyInfo, 0, len(knownKeys))
	for name, spec := range knownKeys {
		keys = append(keys, KeyInfo{
			Name:    name,
			EnvVars: spec.EnvVars,
			Desc:    spec.Desc,
		})
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Name < keys[j].Name
	})
	return keys
}
