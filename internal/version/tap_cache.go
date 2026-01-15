package version

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TapCache provides file-based caching for tap formula metadata.
// Cache entries are stored under $TSUKU_HOME/cache/taps/{owner}/{repo}/{formula}.json
type TapCache struct {
	dir string        // Base directory for tap cache
	ttl time.Duration // Time-to-live for cache entries
}

// TapCacheEntry represents a cached tap formula with metadata
type TapCacheEntry struct {
	Version   string            `json:"version"`
	Formula   string            `json:"formula"`
	BottleURL string            `json:"bottle_url"`
	Checksum  string            `json:"checksum"`
	Tap       string            `json:"tap"`
	Extra     map[string]string `json:"extra,omitempty"`
	CachedAt  time.Time         `json:"cached_at"`
}

// NewTapCache creates a new TapCache with the given base directory and TTL.
// The dir should typically be $TSUKU_HOME/cache/taps.
func NewTapCache(dir string, ttl time.Duration) *TapCache {
	return &TapCache{
		dir: dir,
		ttl: ttl,
	}
}

// Get retrieves a cached entry for the given tap and formula.
// Returns nil if the entry is missing, stale, or corrupted.
func (c *TapCache) Get(tap, formula string) *TapCacheEntry {
	path := c.cachePath(tap, formula)

	data, err := os.ReadFile(path)
	if err != nil {
		// Cache miss - file doesn't exist or isn't readable
		return nil
	}

	var entry TapCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		// Corrupted cache file - treat as miss
		return nil
	}

	// Check if entry is stale
	if time.Since(entry.CachedAt) > c.ttl {
		return nil
	}

	return &entry
}

// GetWithVersionCheck retrieves a cached entry only if it matches the requested version.
// This enables cache invalidation when the requested version differs from cached.
// Returns nil if the entry is missing, stale, corrupted, or version mismatch.
func (c *TapCache) GetWithVersionCheck(tap, formula, requestedVersion string) *TapCacheEntry {
	entry := c.Get(tap, formula)
	if entry == nil {
		return nil
	}

	// If a specific version is requested, ensure cache matches
	if requestedVersion != "" && entry.Version != requestedVersion {
		return nil
	}

	return entry
}

// Set writes a cache entry for the given tap and formula.
// Creates necessary directories if they don't exist.
// Returns an error if the write fails, but callers should treat errors as non-fatal.
func (c *TapCache) Set(tap, formula string, info *VersionInfo) error {
	if info == nil {
		return fmt.Errorf("cannot cache nil VersionInfo")
	}

	path := c.cachePath(tap, formula)

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	entry := TapCacheEntry{
		Version:   info.Version,
		Formula:   info.Metadata["formula"],
		BottleURL: info.Metadata["bottle_url"],
		Checksum:  info.Metadata["checksum"],
		Tap:       info.Metadata["tap"],
		CachedAt:  time.Now(),
	}

	// Copy any extra metadata
	entry.Extra = make(map[string]string)
	for k, v := range info.Metadata {
		if k != "formula" && k != "bottle_url" && k != "checksum" && k != "tap" {
			entry.Extra[k] = v
		}
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	// Atomic write: write to temp file, then rename
	tempFile := path + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp cache file: %w", err)
	}

	if err := os.Rename(tempFile, path); err != nil {
		os.Remove(tempFile) // Clean up temp file on rename failure
		return fmt.Errorf("failed to rename cache file: %w", err)
	}

	return nil
}

// cachePath returns the cache file path for a tap formula.
// Format: {dir}/{owner}/{repo}/{formula}.json
func (c *TapCache) cachePath(tap, formula string) string {
	// Parse tap into owner/repo (e.g., "hashicorp/tap" -> "hashicorp", "tap")
	parts := strings.SplitN(tap, "/", 2)
	if len(parts) != 2 {
		// Invalid tap format, use sanitized version
		return filepath.Join(c.dir, sanitizePathComponent(tap), sanitizePathComponent(formula)+".json")
	}

	owner := sanitizePathComponent(parts[0])
	repo := sanitizePathComponent(parts[1])
	formulaName := sanitizePathComponent(formula)

	return filepath.Join(c.dir, owner, repo, formulaName+".json")
}

// sanitizePathComponent removes or replaces characters that are unsafe for filesystem paths
func sanitizePathComponent(s string) string {
	// Replace path separators and null bytes
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, "\x00", "_")
	return s
}

// ToVersionInfo converts a cache entry back to VersionInfo for use by the provider
func (e *TapCacheEntry) ToVersionInfo() *VersionInfo {
	metadata := map[string]string{
		"formula":    e.Formula,
		"bottle_url": e.BottleURL,
		"checksum":   e.Checksum,
		"tap":        e.Tap,
	}

	// Merge any extra metadata
	for k, v := range e.Extra {
		metadata[k] = v
	}

	return &VersionInfo{
		Tag:      e.Version,
		Version:  e.Version,
		Metadata: metadata,
	}
}

// Clear removes all entries from the tap cache
func (c *TapCache) Clear() error {
	// Remove the entire cache directory and its contents
	if err := os.RemoveAll(c.dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear tap cache: %w", err)
	}
	return nil
}

// TapCacheInfo contains statistics about the tap cache
type TapCacheInfo struct {
	EntryCount int   // Number of cached formulas
	TotalSize  int64 // Total size in bytes
}

// Info returns statistics about the tap cache
func (c *TapCache) Info() (*TapCacheInfo, error) {
	info := &TapCacheInfo{}

	err := filepath.Walk(c.dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // Directory doesn't exist yet
			}
			return err
		}

		if !fi.IsDir() && filepath.Ext(path) == ".json" {
			info.EntryCount++
			info.TotalSize += fi.Size()
		}

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to scan tap cache: %w", err)
	}

	return info, nil
}
