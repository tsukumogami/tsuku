package recipe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// EvictionStrategy determines how entries are evicted when cache exceeds size limits.
type EvictionStrategy int

const (
	// EvictLRU removes least-recently-accessed entries first.
	// Used by the central registry cache.
	EvictLRU EvictionStrategy = iota

	// EvictOldest removes the oldest entries by cache time first.
	// Used by the distributed cache.
	EvictOldest
)

// CacheMeta stores metadata about a cached file in a JSON sidecar.
// This is the unified format covering both central and distributed cache schemas.
type CacheMeta struct {
	// CachedAt is when the content was first cached.
	CachedAt time.Time `json:"cached_at"`

	// ExpiresAt is the TTL-based expiry time (CachedAt + TTL).
	ExpiresAt time.Time `json:"expires_at"`

	// LastAccess is the most recent read time, used for LRU eviction.
	LastAccess time.Time `json:"last_access"`

	// Size is the content size in bytes.
	Size int64 `json:"size"`

	// ContentHash is the SHA256 hex digest of the content.
	ContentHash string `json:"content_hash,omitempty"`

	// ETag from the HTTP response, used for conditional requests.
	ETag string `json:"etag,omitempty"`

	// LastModified from the HTTP response, used for conditional requests.
	LastModified string `json:"last_modified,omitempty"`
}

// DiskCacheConfig holds parameters for creating a DiskCache.
type DiskCacheConfig struct {
	Dir       string
	TTL       time.Duration
	MaxSize   int64
	HighWater float64 // Fraction of MaxSize that triggers eviction (default 0.80)
	LowWater  float64 // Target fraction after eviction (default 0.60)
	Eviction  EvictionStrategy
	MaxStale  time.Duration // Maximum staleness for stale-if-error (0 = disabled)
}

// DiskCache is a unified on-disk cache with TTL, eviction, and stale-if-error support.
// It stores content files alongside JSON metadata sidecars.
type DiskCache struct {
	dir       string
	ttl       time.Duration
	maxSize   int64
	highWater float64
	lowWater  float64
	eviction  EvictionStrategy
	maxStale  time.Duration
	mu        sync.Mutex
}

// NewDiskCache creates a DiskCache from the given config.
func NewDiskCache(cfg DiskCacheConfig) *DiskCache {
	hw := cfg.HighWater
	if hw <= 0 {
		hw = 0.80
	}
	lw := cfg.LowWater
	if lw <= 0 {
		lw = 0.60
	}
	return &DiskCache{
		dir:       cfg.Dir,
		ttl:       cfg.TTL,
		maxSize:   cfg.MaxSize,
		highWater: hw,
		lowWater:  lw,
		eviction:  cfg.Eviction,
		maxStale:  cfg.MaxStale,
	}
}

// Dir returns the cache root directory.
func (c *DiskCache) Dir() string {
	return c.dir
}

// TTL returns the configured time-to-live.
func (c *DiskCache) TTL() time.Duration {
	return c.ttl
}

// Get reads cached content and its metadata.
// Returns nil, nil, nil if the entry doesn't exist.
func (c *DiskCache) Get(key string) ([]byte, *CacheMeta, error) {
	contentPath := c.contentPath(key)

	data, err := os.ReadFile(contentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("reading cache entry %q: %w", key, err)
	}

	meta, err := c.ReadMeta(key)
	if err != nil {
		// Content exists but metadata is broken: return content with nil meta
		return data, nil, nil
	}

	// Update last access for LRU tracking
	if meta != nil {
		meta.LastAccess = time.Now()
		// Best-effort write; don't fail the read
		_ = c.writeMeta(key, meta)
	}

	return data, meta, nil
}

// Put writes content and metadata to the cache, then enforces size limits.
func (c *DiskCache) Put(key string, data []byte, meta *CacheMeta) error {
	contentPath := c.contentPath(key)
	dir := filepath.Dir(contentPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	if err := os.WriteFile(contentPath, data, 0644); err != nil {
		return fmt.Errorf("writing cache entry %q: %w", key, err)
	}

	if meta == nil {
		meta = c.newMeta(data)
	}

	if err := c.writeMeta(key, meta); err != nil {
		return err
	}

	// Best-effort eviction after write
	c.enforceLimit()
	return nil
}

// Delete removes a cache entry and its metadata sidecar.
func (c *DiskCache) Delete(key string) error {
	var lastErr error
	if err := os.Remove(c.contentPath(key)); err != nil && !os.IsNotExist(err) {
		lastErr = err
	}
	if err := os.Remove(c.metaPath(key)); err != nil && !os.IsNotExist(err) {
		lastErr = err
	}
	return lastErr
}

// IsFresh returns true if the metadata indicates the entry hasn't expired.
// Uses the configured TTL computed from CachedAt, not the stored ExpiresAt,
// so TTL changes take effect without cache invalidation.
func (c *DiskCache) IsFresh(meta *CacheMeta) bool {
	if meta == nil {
		return false
	}
	return time.Now().Before(meta.CachedAt.Add(c.ttl))
}

// IsStaleUsable returns true if the entry is expired but still within MaxStale
// bounds for stale-if-error fallback.
func (c *DiskCache) IsStaleUsable(meta *CacheMeta) bool {
	if meta == nil || c.maxStale <= 0 {
		return false
	}
	age := time.Since(meta.CachedAt)
	return age < c.ttl+c.maxStale
}

// Has returns true if a content file exists for the given key.
func (c *DiskCache) Has(key string) bool {
	_, err := os.Stat(c.contentPath(key))
	return err == nil
}

// ReadMeta reads the metadata sidecar for a key.
// Returns nil, nil if the sidecar doesn't exist.
func (c *DiskCache) ReadMeta(key string) (*CacheMeta, error) {
	data, err := os.ReadFile(c.metaPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading cache metadata %q: %w", key, err)
	}

	var meta CacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing cache metadata %q: %w", key, err)
	}
	return &meta, nil
}

// Keys returns all content keys in the cache.
func (c *DiskCache) Keys() ([]string, error) {
	return c.walkKeys()
}

// CacheStats holds cache statistics for introspection.
type DiskCacheStats struct {
	TotalSize    int64
	EntryCount   int
	StaleCount   int
	OldestAccess time.Time
	NewestAccess time.Time
	SizeLimit    int64
	TTL          time.Duration
}

// Stats returns current cache statistics.
func (c *DiskCache) Stats() (*DiskCacheStats, error) {
	entries, err := c.listEntries()
	if err != nil {
		return nil, err
	}

	stats := &DiskCacheStats{
		EntryCount: len(entries),
		SizeLimit:  c.maxSize,
		TTL:        c.ttl,
	}

	now := time.Now()
	for _, e := range entries {
		stats.TotalSize += e.size

		if stats.OldestAccess.IsZero() || e.sortTime.Before(stats.OldestAccess) {
			stats.OldestAccess = e.sortTime
		}
		if stats.NewestAccess.IsZero() || e.sortTime.After(stats.NewestAccess) {
			stats.NewestAccess = e.sortTime
		}

		if e.meta != nil && now.After(e.meta.CachedAt.Add(c.ttl)) {
			stats.StaleCount++
		}
	}

	return stats, nil
}

// Size returns the total size of all cached files in bytes.
func (c *DiskCache) Size() (int64, error) {
	var total int64
	err := filepath.Walk(c.dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	return total, nil
}

// newMeta creates fresh metadata for just-cached content.
func (c *DiskCache) newMeta(data []byte) *CacheMeta {
	now := time.Now()
	return &CacheMeta{
		CachedAt:    now,
		ExpiresAt:   now.Add(c.ttl),
		LastAccess:  now,
		Size:        int64(len(data)),
		ContentHash: contentHash(data),
	}
}

// NewMeta creates a CacheMeta for freshly cached content with the cache's TTL.
// Callers can set additional fields (ETag, LastModified) on the returned value.
func (c *DiskCache) NewMeta(data []byte) *CacheMeta {
	return c.newMeta(data)
}

// contentPath returns the file path for cached content.
func (c *DiskCache) contentPath(key string) string {
	return filepath.Join(c.dir, key)
}

// metaPath returns the metadata sidecar path for a key.
// For key "f/fzf.toml", returns "f/fzf.meta.json".
func (c *DiskCache) metaPath(key string) string {
	ext := filepath.Ext(key)
	base := strings.TrimSuffix(key, ext)
	return filepath.Join(c.dir, base+".meta.json")
}

func (c *DiskCache) writeMeta(key string, meta *CacheMeta) error {
	path := c.metaPath(key)
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating metadata directory: %w", err)
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling cache metadata: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing cache metadata %q: %w", key, err)
	}
	return nil
}

// diskCacheEntry is an internal type for eviction sorting.
type diskCacheEntry struct {
	key      string
	size     int64
	sortTime time.Time // lastAccess for LRU, cachedAt for oldest
	meta     *CacheMeta
}

// walkKeys returns all content file keys relative to the cache directory.
// Content files are identified as non-.meta.json, non-directory entries.
func (c *DiskCache) walkKeys() ([]string, error) {
	var keys []string
	err := filepath.Walk(c.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".meta.json") {
			return nil
		}
		rel, err := filepath.Rel(c.dir, path)
		if err != nil {
			return nil
		}
		keys = append(keys, rel)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return keys, nil
}

// listEntries returns all cache entries with size and sort-time for eviction.
func (c *DiskCache) listEntries() ([]diskCacheEntry, error) {
	keys, err := c.walkKeys()
	if err != nil {
		return nil, err
	}

	entries := make([]diskCacheEntry, 0, len(keys))
	for _, key := range keys {
		contentPath := c.contentPath(key)

		var totalSize int64
		if info, statErr := os.Stat(contentPath); statErr == nil {
			totalSize += info.Size()
		}
		metaP := c.metaPath(key)
		if info, statErr := os.Stat(metaP); statErr == nil {
			totalSize += info.Size()
		}

		meta, _ := c.ReadMeta(key)

		sortTime := time.Now()
		if meta != nil {
			switch c.eviction {
			case EvictLRU:
				sortTime = meta.LastAccess
			case EvictOldest:
				sortTime = meta.CachedAt
			}
		} else {
			// No metadata: use file mtime
			if info, statErr := os.Stat(contentPath); statErr == nil {
				sortTime = info.ModTime()
			}
		}

		entries = append(entries, diskCacheEntry{
			key:      key,
			size:     totalSize,
			sortTime: sortTime,
			meta:     meta,
		})
	}

	return entries, nil
}

// enforceLimit evicts entries when cache exceeds the high-water mark.
func (c *DiskCache) enforceLimit() {
	if c.maxSize <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	currentSize, err := c.Size()
	if err != nil || currentSize <= int64(float64(c.maxSize)*c.highWater) {
		return
	}

	entries, err := c.listEntries()
	if err != nil {
		return
	}

	// Sort: earliest sortTime first (evict first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].sortTime.Before(entries[j].sortTime)
	})

	lowTarget := int64(float64(c.maxSize) * c.lowWater)
	for _, entry := range entries {
		if currentSize <= lowTarget {
			break
		}
		if err := c.Delete(entry.key); err == nil {
			currentSize -= entry.size
		}
	}
}

// contentHash computes a SHA256 hex digest.
func contentHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// CacheIntrospectable is an optional interface for providers with an inspectable
// disk cache. The update-registry command uses this instead of type-asserting
// to a concrete provider type.
type CacheIntrospectable interface {
	CacheStats() (*DiskCacheStats, error)
}
