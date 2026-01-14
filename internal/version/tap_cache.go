package version

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultTapCacheTTL is the default time-to-live for tap cache entries.
const DefaultTapCacheTTL = 1 * time.Hour

// TapCache provides file-based caching for tap formula metadata.
// Cache entries are stored under $TSUKU_HOME/cache/taps/{tap}/{formula}.json
// and include TTL-based expiration.
type TapCache struct {
	dir string        // Base directory for tap cache
	ttl time.Duration // Time-to-live for cache entries
}

// tapCacheEntry represents a cached tap formula with metadata
type tapCacheEntry struct {
	CachedAt  time.Time       `json:"cached_at"`
	ExpiresAt time.Time       `json:"expires_at"`
	Info      *tapFormulaInfo `json:"info"`
}

// NewTapCache creates a new TapCache with the specified directory and TTL.
// The directory should be $TSUKU_HOME/cache/taps.
func NewTapCache(dir string, ttl time.Duration) *TapCache {
	return &TapCache{
		dir: dir,
		ttl: ttl,
	}
}

// Get retrieves a cached tap formula entry if it exists and is not expired.
// Returns nil if the entry is missing, expired, or corrupted.
func (c *TapCache) Get(tap, formula string) *tapFormulaInfo {
	path := c.cachePath(tap, formula)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil // Cache miss
	}

	var entry tapCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		// Corrupted cache file - treat as cache miss
		// Best effort cleanup
		_ = os.Remove(path)
		return nil
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		return nil // Cache expired
	}

	return entry.Info
}

// Set writes a tap formula entry to the cache with the current timestamp.
// Creates the cache directory structure if it doesn't exist.
func (c *TapCache) Set(tap, formula string, info *tapFormulaInfo) error {
	path := c.cachePath(tap, formula)

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	entry := tapCacheEntry{
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(c.ttl),
		Info:      info,
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
		_ = os.Remove(tempFile) // Clean up temp file on rename failure
		return fmt.Errorf("failed to rename cache file: %w", err)
	}

	return nil
}

// cachePath returns the file path for a cache entry.
// The tap name is converted to be filesystem-safe (/ -> -).
func (c *TapCache) cachePath(tap, formula string) string {
	// Convert tap name to filesystem-safe format: "hashicorp/tap" -> "hashicorp-tap"
	safeTap := strings.ReplaceAll(tap, "/", "-")
	return filepath.Join(c.dir, safeTap, formula+".json")
}

// Invalidate removes a specific cache entry.
func (c *TapCache) Invalidate(tap, formula string) error {
	path := c.cachePath(tap, formula)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil // Nothing to invalidate
	}
	return err
}

// Clear removes all tap cache entries.
func (c *TapCache) Clear() error {
	return os.RemoveAll(c.dir)
}
