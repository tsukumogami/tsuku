package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// RegistryEntry maps a tool name to its builder and source.
// Schema v2 adds optional metadata fields for enrichment and disambiguation.
type RegistryEntry struct {
	Builder        string `json:"builder"`
	Source         string `json:"source"`
	Binary         string `json:"binary,omitempty"`
	Description    string `json:"description,omitempty"`
	Homepage       string `json:"homepage,omitempty"`
	Repo           string `json:"repo,omitempty"`
	Disambiguation bool   `json:"disambiguation,omitempty"`
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
	if reg.SchemaVersion != 1 && reg.SchemaVersion != 2 {
		return nil, fmt.Errorf("unsupported discovery registry schema version %d (expected 1 or 2)", reg.SchemaVersion)
	}
	if err := reg.validateEntries(); err != nil {
		return nil, err
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

// validateEntries checks that all registry entries have non-empty builder and source fields.
func (r *DiscoveryRegistry) validateEntries() error {
	for name, entry := range r.Tools {
		if entry.Builder == "" {
			return fmt.Errorf("invalid discovery registry entry %q: builder field is empty", name)
		}
		if entry.Source == "" {
			return fmt.Errorf("invalid discovery registry entry %q: source field is empty", name)
		}
	}
	return nil
}

// buildIndex creates a lowercase-to-canonical mapping for case-insensitive lookup.
func (r *DiscoveryRegistry) buildIndex() {
	r.normalized = make(map[string]string, len(r.Tools))
	for name := range r.Tools {
		r.normalized[strings.ToLower(name)] = name
	}
}
