package shim

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// ShimContent is the exact content written to every shim script.
// The shim discovers its own name via basename "$0" at runtime,
// so a single template works for all tools.
const ShimContent = "#!/bin/sh\nexec tsuku run \"$(basename \"$0\")\" -- \"$@\"\n"

// shimMetadataFile is the name of the JSON file that tracks which recipe
// owns which shim. Stored in $TSUKU_HOME/bin/.
const shimMetadataFile = ".tsuku-shims.json"

// ShimEntry describes a single installed shim.
type ShimEntry struct {
	Name   string `json:"name"`   // Binary name (e.g., "rg")
	Recipe string `json:"recipe"` // Recipe that owns it (e.g., "ripgrep")
}

// shimMetadata maps binary name -> recipe name. Persisted as JSON in
// $TSUKU_HOME/bin/.tsuku-shims.json.
type shimMetadata map[string]string

// Manager creates, removes, and lists shim scripts in $TSUKU_HOME/bin/.
type Manager struct {
	binDir string
	loader *recipe.Loader
}

// NewManager creates a Manager that writes shims to cfg.HomeDir/bin.
// The loader is used to resolve recipe names to binary lists.
func NewManager(cfg *config.Config, loader *recipe.Loader) *Manager {
	return &Manager{
		binDir: filepath.Join(cfg.HomeDir, "bin"),
		loader: loader,
	}
}

// Install creates shim scripts for every binary provided by recipeName.
// Returns the list of created shim paths. If a non-shim file already exists
// at one of the target paths, Install returns an error without creating any
// shims (atomic: all or nothing).
func (m *Manager) Install(recipeName string) ([]string, error) {
	r, err := m.loader.Get(recipeName, recipe.LoaderOptions{})
	if err != nil {
		return nil, fmt.Errorf("loading recipe %q: %w", recipeName, err)
	}

	bins := extractBinaryNames(r)
	if len(bins) == 0 {
		return nil, fmt.Errorf("recipe %q does not declare any binaries", recipeName)
	}

	if err := os.MkdirAll(m.binDir, 0755); err != nil {
		return nil, fmt.Errorf("creating bin directory: %w", err)
	}

	// Pre-flight: check for non-shim conflicts.
	for _, name := range bins {
		p := filepath.Join(m.binDir, name)
		if fileExists(p) && !IsShim(p) {
			return nil, fmt.Errorf("refusing to overwrite non-shim file %s", p)
		}
	}

	// Write shims.
	var created []string
	for _, name := range bins {
		p := filepath.Join(m.binDir, name)
		if err := os.WriteFile(p, []byte(ShimContent), 0755); err != nil {
			return created, fmt.Errorf("writing shim %s: %w", p, err)
		}
		created = append(created, p)
	}

	// Update metadata.
	meta, _ := m.loadMetadata()
	if meta == nil {
		meta = make(shimMetadata)
	}
	for _, name := range bins {
		meta[name] = recipeName
	}
	if err := m.saveMetadata(meta); err != nil {
		return created, fmt.Errorf("saving shim metadata: %w", err)
	}

	return created, nil
}

// Uninstall removes shim scripts owned by recipeName. Only files whose
// content matches ShimContent are removed.
func (m *Manager) Uninstall(recipeName string) error {
	meta, err := m.loadMetadata()
	if err != nil {
		return fmt.Errorf("loading shim metadata: %w", err)
	}

	var toRemove []string
	for name, owner := range meta {
		if owner == recipeName {
			toRemove = append(toRemove, name)
		}
	}

	if len(toRemove) == 0 {
		return fmt.Errorf("no shims found for recipe %q", recipeName)
	}

	for _, name := range toRemove {
		p := filepath.Join(m.binDir, name)
		if IsShim(p) {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing shim %s: %w", p, err)
			}
		}
		delete(meta, name)
	}

	return m.saveMetadata(meta)
}

// List returns all installed shims with their owning recipe.
func (m *Manager) List() ([]ShimEntry, error) {
	meta, err := m.loadMetadata()
	if err != nil {
		// No metadata file means no shims.
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("loading shim metadata: %w", err)
	}

	var entries []ShimEntry
	for name, rec := range meta {
		// Only include entries whose file still exists and is actually a shim.
		p := filepath.Join(m.binDir, name)
		if IsShim(p) {
			entries = append(entries, ShimEntry{Name: name, Recipe: rec})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

// IsShim reports whether the file at path is a tsuku shim by checking
// its content against ShimContent.
func IsShim(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return string(data) == ShimContent
}

// metadataPath returns the full path to the shim metadata file.
func (m *Manager) metadataPath() string {
	return filepath.Join(m.binDir, shimMetadataFile)
}

func (m *Manager) loadMetadata() (shimMetadata, error) {
	data, err := os.ReadFile(m.metadataPath())
	if err != nil {
		return nil, err
	}
	var meta shimMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing shim metadata: %w", err)
	}
	return meta, nil
}

func (m *Manager) saveMetadata(meta shimMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding shim metadata: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(m.metadataPath(), data, 0644)
}

// extractBinaryNames returns the list of binary base names from a recipe.
// ExtractBinaries returns paths like "bin/rg"; we strip the directory prefix.
func extractBinaryNames(r *recipe.Recipe) []string {
	raw := r.ExtractBinaries()
	seen := make(map[string]bool)
	var names []string
	for _, entry := range raw {
		name := filepath.Base(entry)
		if name == "." || name == "/" || seen[name] {
			continue
		}
		// Skip entries that are just directory paths without a real binary name.
		if strings.TrimSpace(name) == "" {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// fileExists reports whether a regular file exists at path.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
