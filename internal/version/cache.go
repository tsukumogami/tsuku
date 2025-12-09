package version

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CachedVersionLister wraps a VersionLister with file-based caching.
// It stores version lists in $TSUKU_HOME/cache/versions/ with configurable TTL.
type CachedVersionLister struct {
	underlying VersionLister
	cacheDir   string
	ttl        time.Duration
}

// cacheEntry represents a cached version list with metadata
type cacheEntry struct {
	Versions  []string  `json:"versions"`
	CachedAt  time.Time `json:"cached_at"`
	Source    string    `json:"source"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewCachedVersionLister creates a caching wrapper around a VersionLister.
// The cacheDir should be $TSUKU_HOME/cache/versions.
// The ttl controls how long cached entries are valid.
func NewCachedVersionLister(underlying VersionLister, cacheDir string, ttl time.Duration) *CachedVersionLister {
	return &CachedVersionLister{
		underlying: underlying,
		cacheDir:   cacheDir,
		ttl:        ttl,
	}
}

// ListVersions returns cached versions if valid, otherwise fetches fresh data.
// Returns (versions, fromCache, error) where fromCache indicates cache hit.
func (c *CachedVersionLister) ListVersions(ctx context.Context) ([]string, error) {
	versions, _, err := c.ListVersionsWithCacheInfo(ctx)
	return versions, err
}

// ListVersionsWithCacheInfo returns versions with cache status information.
// Returns (versions, fromCache, error).
func (c *CachedVersionLister) ListVersionsWithCacheInfo(ctx context.Context) ([]string, bool, error) {
	cacheFile := c.cacheFilePath()

	// Try to read from cache
	if entry, err := c.readCache(cacheFile); err == nil {
		if time.Now().Before(entry.ExpiresAt) {
			return entry.Versions, true, nil
		}
	}

	// Cache miss or expired, fetch fresh data
	versions, err := c.underlying.ListVersions(ctx)
	if err != nil {
		return nil, false, err
	}

	// Write to cache (best effort, don't fail if cache write fails)
	_ = c.writeCache(cacheFile, versions)

	return versions, false, nil
}

// Refresh bypasses the cache and fetches fresh data, updating the cache.
func (c *CachedVersionLister) Refresh(ctx context.Context) ([]string, error) {
	versions, err := c.underlying.ListVersions(ctx)
	if err != nil {
		return nil, err
	}

	// Update cache
	cacheFile := c.cacheFilePath()
	_ = c.writeCache(cacheFile, versions)

	return versions, nil
}

// ResolveLatest delegates to the underlying provider (no caching for resolution)
func (c *CachedVersionLister) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	return c.underlying.ResolveLatest(ctx)
}

// ResolveVersion delegates to the underlying provider (no caching for resolution)
func (c *CachedVersionLister) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	return c.underlying.ResolveVersion(ctx, version)
}

// SourceDescription returns the underlying provider's source description
func (c *CachedVersionLister) SourceDescription() string {
	return c.underlying.SourceDescription()
}

// cacheFilePath returns the path to the cache file for this provider
func (c *CachedVersionLister) cacheFilePath() string {
	// Use SHA256 of source description for unique, filesystem-safe filename
	source := c.underlying.SourceDescription()
	hash := sha256.Sum256([]byte(source))
	filename := hex.EncodeToString(hash[:8]) + ".json" // Use first 8 bytes (16 hex chars)
	return filepath.Join(c.cacheDir, filename)
}

// readCache reads and parses a cache file
func (c *CachedVersionLister) readCache(path string) (*cacheEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}

	return &entry, nil
}

// writeCache atomically writes a cache entry to disk
func (c *CachedVersionLister) writeCache(path string, versions []string) error {
	// Ensure cache directory exists
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	entry := cacheEntry{
		Versions:  versions,
		CachedAt:  time.Now(),
		Source:    c.underlying.SourceDescription(),
		ExpiresAt: time.Now().Add(c.ttl),
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

// CacheInfo returns information about the cache entry for this provider
type CacheInfo struct {
	Exists    bool
	CachedAt  time.Time
	ExpiresAt time.Time
	IsExpired bool
}

// GetCacheInfo returns information about the current cache state without fetching
func (c *CachedVersionLister) GetCacheInfo() CacheInfo {
	cacheFile := c.cacheFilePath()
	entry, err := c.readCache(cacheFile)
	if err != nil {
		return CacheInfo{Exists: false}
	}

	return CacheInfo{
		Exists:    true,
		CachedAt:  entry.CachedAt,
		ExpiresAt: entry.ExpiresAt,
		IsExpired: time.Now().After(entry.ExpiresAt),
	}
}

// Cache provides direct access to the version cache directory for management operations.
// Use this for global cache operations like clearing all entries.
type Cache struct {
	cacheDir string
}

// NewCache creates a new Cache for managing the version cache directory.
func NewCache(cacheDir string) *Cache {
	return &Cache{cacheDir: cacheDir}
}

// VersionCacheInfo contains information about the version cache
type VersionCacheInfo struct {
	EntryCount int
	TotalSize  int64
}

// Info returns information about the version cache
func (c *Cache) Info() (*VersionCacheInfo, error) {
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &VersionCacheInfo{}, nil
		}
		return nil, fmt.Errorf("failed to read cache directory: %w", err)
	}

	info := &VersionCacheInfo{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Count .json files as cache entries
		if filepath.Ext(entry.Name()) == ".json" {
			info.EntryCount++
			if fi, err := entry.Info(); err == nil {
				info.TotalSize += fi.Size()
			}
		}
	}

	return info, nil
}

// Clear removes all version cache entries
func (c *Cache) Clear() error {
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to clear
		}
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(c.cacheDir, entry.Name())
		if err := os.Remove(path); err != nil {
			// Continue on error, try to remove as many as possible
			continue
		}
	}

	return nil
}
