package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RegistryEntry maps a tool name to its builder and source.
// Optional metadata fields support enrichment and disambiguation.
type RegistryEntry struct {
	Builder        string `json:"builder"`
	Source         string `json:"source"`
	Binary         string `json:"binary,omitempty"`
	Description    string `json:"description,omitempty"`
	Homepage       string `json:"homepage,omitempty"`
	Repo           string `json:"repo,omitempty"`
	Disambiguation bool   `json:"disambiguation,omitempty"`
	Downloads      int    `json:"downloads,omitempty"`
	VersionCount   int    `json:"version_count,omitempty"`
	HasRepository  bool   `json:"has_repository,omitempty"`
}

// DiscoveryRegistry holds the tool-to-source mapping used by the registry
// lookup stage. Each entry is stored as a separate JSON file under
// $TSUKU_HOME/registry/discovery/{first-letter}/{first-two-letters}/{name}.json.
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

// RegistryEntryPath returns the relative path for a tool's discovery entry.
// The path uses two levels of prefix: {first-letter}/{first-two-letters}/{name}.json.
// Single-character names use {letter}/_/{name}.json.
func RegistryEntryPath(name string) string {
	lower := strings.ToLower(name)
	first := string(lower[0])
	var prefix string
	if len(lower) >= 2 {
		prefix = lower[:2]
	} else {
		prefix = "_"
	}
	return filepath.Join(first, prefix, lower+".json")
}

// LoadRegistryEntry reads a single discovery entry from a directory tree.
// The name is case-insensitive.
func LoadRegistryEntry(dir, name string) (*RegistryEntry, error) {
	path := filepath.Join(dir, RegistryEntryPath(name))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load discovery entry %q: %w", name, err)
	}
	var entry RegistryEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("parse discovery entry %q: %w", name, err)
	}
	if entry.Builder == "" {
		return nil, fmt.Errorf("invalid discovery entry %q: builder field is empty", name)
	}
	if entry.Source == "" {
		return nil, fmt.Errorf("invalid discovery entry %q: source field is empty", name)
	}
	return &entry, nil
}

// LoadRegistryDir reads all discovery entries from a directory tree and
// assembles them into a DiscoveryRegistry. Used for validation and bulk operations.
func LoadRegistryDir(dir string) (*DiscoveryRegistry, error) {
	reg := &DiscoveryRegistry{
		SchemaVersion: 1,
		Tools:         make(map[string]RegistryEntry),
	}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		var entry RegistryEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		reg.Tools[name] = entry
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load discovery directory: %w", err)
	}
	reg.buildIndex()
	return reg, nil
}
