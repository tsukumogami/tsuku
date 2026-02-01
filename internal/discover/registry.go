package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// RegistryEntry maps a tool name to its builder and source.
type RegistryEntry struct {
	Builder string `json:"builder"`
	Source  string `json:"source"`
	Binary  string `json:"binary,omitempty"`
}

// DiscoveryRegistry holds the tool-to-source mapping used by the registry
// lookup stage. Loaded from a JSON file cached at $TSUKU_HOME/registry/discovery.json.
type DiscoveryRegistry struct {
	SchemaVersion int                      `json:"schema_version"`
	Tools         map[string]RegistryEntry `json:"tools"`

	// normalized is the lowercase lookup index built at load time.
	normalized map[string]string
}

// LoadRegistry reads a discovery registry from a JSON file.
func LoadRegistry(path string) (*DiscoveryRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load discovery registry: %w", err)
	}
	return ParseRegistry(data)
}

// ParseRegistry parses discovery registry JSON bytes.
func ParseRegistry(data []byte) (*DiscoveryRegistry, error) {
	var reg DiscoveryRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse discovery registry: %w", err)
	}
	if reg.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported discovery registry schema version %d (expected 1)", reg.SchemaVersion)
	}
	reg.buildIndex()
	return &reg, nil
}

// Lookup returns the registry entry for a tool name (case-insensitive).
func (r *DiscoveryRegistry) Lookup(name string) (*RegistryEntry, bool) {
	canonical, ok := r.normalized[strings.ToLower(name)]
	if !ok {
		return nil, false
	}
	entry := r.Tools[canonical]
	return &entry, true
}

// buildIndex creates a lowercase-to-canonical mapping for case-insensitive lookup.
func (r *DiscoveryRegistry) buildIndex() {
	r.normalized = make(map[string]string, len(r.Tools))
	for name := range r.Tools {
		r.normalized[strings.ToLower(name)] = name
	}
}
