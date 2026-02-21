package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// manifestCacheFile is the filename for the cached manifest within CacheDir.
	manifestCacheFile = "manifest.json"
)

// Manifest represents the registry manifest (recipes.json).
type Manifest struct {
	SchemaVersion string           `json:"schema_version"`
	GeneratedAt   string           `json:"generated_at"`
	Recipes       []ManifestRecipe `json:"recipes"`
}

// ManifestRecipe represents a single recipe entry in the manifest.
type ManifestRecipe struct {
	Name                string              `json:"name"`
	Description         string              `json:"description"`
	Homepage            string              `json:"homepage"`
	Dependencies        []string            `json:"dependencies"`
	RuntimeDependencies []string            `json:"runtime_dependencies"`
	Satisfies           map[string][]string `json:"satisfies,omitempty"`
}

// GetCachedManifest reads the locally cached manifest without network access.
// Returns nil, nil if no cached manifest exists.
func (r *Registry) GetCachedManifest() (*Manifest, error) {
	if r.CacheDir == "" {
		return nil, nil
	}

	cachePath := filepath.Join(r.CacheDir, manifestCacheFile)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read cached manifest: %w", err)
	}

	return parseManifest(data)
}

// parseManifest parses raw JSON bytes into a Manifest struct.
func parseManifest(data []byte) (*Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest JSON: %w", err)
	}
	return &manifest, nil
}
