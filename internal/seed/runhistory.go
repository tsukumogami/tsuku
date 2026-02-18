package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AppendSeedingRun appends one JSON line to the seeding-runs.jsonl file.
// It creates the directory and file if they don't exist.
// The write is performed atomically: write to a temp file, then rename.
func AppendSeedingRun(path string, entry SeedingRunEntry) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory for seeding runs: %w", err)
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal seeding run entry: %w", err)
	}
	line = append(line, '\n')

	// Read existing content (if file exists).
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read existing seeding runs: %w", err)
	}

	// Write to temp file, then rename for atomicity.
	tmpPath := path + ".tmp"
	content := append(existing, line...)
	if err := os.WriteFile(tmpPath, content, 0644); err != nil {
		return fmt.Errorf("write temp seeding runs file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		// Clean up temp file on rename failure.
		os.Remove(tmpPath)
		return fmt.Errorf("rename seeding runs file: %w", err)
	}

	return nil
}
