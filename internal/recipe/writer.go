package recipe

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// recipeForEncoding is an intermediate structure for TOML encoding.
// It converts []Step to []map[string]interface{} since the Step struct
// has custom UnmarshalTOML that doesn't work well for encoding.
type recipeForEncoding struct {
	Metadata  MetadataSection          `toml:"metadata"`
	Version   VersionSection           `toml:"version"`
	Resources []Resource               `toml:"resources,omitempty"`
	Patches   []Patch                  `toml:"patches,omitempty"`
	Steps     []map[string]interface{} `toml:"steps"`
	Verify    VerifySection            `toml:"verify"`
}

// toEncodable converts a Recipe to the encoding-friendly structure.
func toEncodable(r *Recipe) *recipeForEncoding {
	steps := make([]map[string]interface{}, len(r.Steps))
	for i, step := range r.Steps {
		steps[i] = step.ToMap()
	}

	return &recipeForEncoding{
		Metadata:  r.Metadata,
		Version:   r.Version,
		Resources: r.Resources,
		Patches:   r.Patches,
		Steps:     steps,
		Verify:    r.Verify,
	}
}

// WriteRecipe writes a recipe to the specified path using atomic file operations.
// It uses a write-temp-rename pattern to prevent partial writes:
// 1. Write to a temporary file in the same directory
// 2. Sync the file to ensure data is on disk
// 3. Atomically rename to the final destination
func WriteRecipe(r *Recipe, path string) error {
	// Ensure the directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Create a temporary file in the same directory
	// Using the same directory ensures the rename is atomic (same filesystem)
	tmpFile, err := os.CreateTemp(dir, ".recipe-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up the temp file on any error
	success := false
	defer func() {
		if !success {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	// Convert to encodable structure and encode to TOML
	encodable := toEncodable(r)
	encoder := toml.NewEncoder(tmpFile)
	if err := encoder.Encode(encodable); err != nil {
		return fmt.Errorf("failed to encode recipe: %w", err)
	}

	// Sync to ensure data is on disk
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	// Close the file before renaming
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Atomically rename to the final destination
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	success = true
	return nil
}
