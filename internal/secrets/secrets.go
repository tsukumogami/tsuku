// Package secrets provides centralized resolution of API keys and tokens.
//
// Secrets are resolved by checking environment variables in priority order.
// Config file fallback is wired separately (see #1734).
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

// configOnce ensures config file loading happens at most once.
// Currently a no-op; wired to userconfig in #1734.
var configOnce sync.Once

// loadConfig is the lazy config loader. It runs once on the first
// Get() or IsSet() call. Currently a no-op placeholder.
func loadConfig() {
	configOnce.Do(func() {
		// No-op: config file integration deferred to #1734.
	})
}

// Get resolves a secret by name, checking environment variables in order.
// Returns the first non-empty value found, or an error if the key is
// unknown or no source has a value set.
func Get(name string) (string, error) {
	spec, ok := knownKeys[name]
	if !ok {
		return "", fmt.Errorf("unknown secret key: %q", name)
	}

	loadConfig()

	// Check environment variables in priority order.
	for _, env := range spec.EnvVars {
		if val := os.Getenv(env); val != "" {
			return val, nil
		}
	}

	// TODO(#1734): check config file here.

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

	loadConfig()

	for _, env := range spec.EnvVars {
		if os.Getenv(env) != "" {
			return true
		}
	}

	// TODO(#1734): check config file here.

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
