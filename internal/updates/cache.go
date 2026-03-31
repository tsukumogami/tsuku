package updates

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SentinelFile is the name of the sentinel file used for global staleness detection.
const SentinelFile = ".last-check"

// LockFile is the name of the advisory lock file for spawn dedup.
const LockFile = ".lock"

// UpdateCheckEntry represents the cached result of a background update check
// for a single installed tool. Stored at $TSUKU_HOME/cache/updates/<toolname>.json.
type UpdateCheckEntry struct {
	// Tool is the recipe name (e.g., "node", "ripgrep").
	Tool string `json:"tool"`

	// ActiveVersion is the installed version at check time.
	ActiveVersion string `json:"active_version"`

	// Requested is the user's pin constraint at check time (e.g., "20", "1.29", "").
	// Used for pin-change detection: if state.json's Requested differs from this
	// value, the cache entry is logically stale regardless of mtime.
	Requested string `json:"requested"`

	// LatestWithinPin is the newest version that satisfies the pin constraint,
	// or empty if the active version is already the latest within the pin.
	LatestWithinPin string `json:"latest_within_pin,omitempty"`

	// LatestOverall is the newest version available regardless of pin constraints.
	LatestOverall string `json:"latest_overall"`

	// Source is the version provider description (e.g., "GitHub:nodejs/node").
	Source string `json:"source"`

	// CheckedAt is when the background process performed this check.
	CheckedAt time.Time `json:"checked_at"`

	// ExpiresAt is when this entry should be considered stale for per-tool reads.
	ExpiresAt time.Time `json:"expires_at"`

	// Error records a non-empty string if the check failed for this tool.
	Error string `json:"error,omitempty"`
}

// ReadEntry reads a single tool's update check cache entry.
// Returns (nil, nil) when the file does not exist.
// Returns (nil, err) on parse failure.
func ReadEntry(cacheDir, toolName string) (*UpdateCheckEntry, error) {
	path := filepath.Join(cacheDir, toolName+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read update cache for %s: %w", toolName, err)
	}

	var entry UpdateCheckEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("parse update cache for %s: %w", toolName, err)
	}
	return &entry, nil
}

// ReadAllEntries scans the cache directory and returns all valid entries.
// Skips files that fail to parse, non-.json files, and dotfiles.
func ReadAllEntries(cacheDir string) ([]UpdateCheckEntry, error) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read update cache directory: %w", err)
	}

	var results []UpdateCheckEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip dotfiles (sentinel, lock) and non-JSON files
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		toolName := strings.TrimSuffix(name, ".json")
		entry, err := ReadEntry(cacheDir, toolName)
		if err != nil || entry == nil {
			continue
		}
		results = append(results, *entry)
	}
	return results, nil
}

// WriteEntry atomically writes an update check entry to the cache directory.
// Creates the directory with os.MkdirAll if it does not exist.
func WriteEntry(cacheDir string, entry *UpdateCheckEntry) error {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("create update cache directory: %w", err)
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal update cache entry for %s: %w", entry.Tool, err)
	}

	path := filepath.Join(cacheDir, entry.Tool+".json")
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp cache file for %s: %w", entry.Tool, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename cache file for %s: %w", entry.Tool, err)
	}
	return nil
}

// RemoveEntry deletes a tool's update check cache file.
// Returns nil if the file does not exist.
func RemoveEntry(cacheDir, toolName string) error {
	path := filepath.Join(cacheDir, toolName+".json")
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove update cache for %s: %w", toolName, err)
	}
	return nil
}

// TouchSentinel creates or updates the mtime of the sentinel file.
func TouchSentinel(cacheDir string) error {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("create update cache directory: %w", err)
	}

	path := filepath.Join(cacheDir, SentinelFile)
	now := time.Now()

	// Try to update mtime of existing file first
	if err := os.Chtimes(path, now, now); err != nil {
		// File doesn't exist, create it
		if err := os.WriteFile(path, nil, 0644); err != nil {
			return fmt.Errorf("create sentinel file: %w", err)
		}
	}
	return nil
}

// IsCheckStale returns true if the sentinel indicates a check is needed.
// Returns true if the sentinel is missing, unreadable, or older than the interval.
func IsCheckStale(cacheDir string, interval time.Duration) bool {
	path := filepath.Join(cacheDir, SentinelFile)
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) > interval
}

// CacheDir returns the update check cache directory path for a given config home.
func CacheDir(homeDir string) string {
	return filepath.Join(homeDir, "cache", "updates")
}
