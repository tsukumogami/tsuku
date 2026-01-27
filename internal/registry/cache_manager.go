package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CacheStats holds statistics about the recipe cache.
type CacheStats struct {
	// TotalSize is the total size of all cached files in bytes.
	TotalSize int64

	// EntryCount is the number of cached recipes.
	EntryCount int

	// OldestAccess is the oldest last_access timestamp among all entries.
	// Zero if cache is empty.
	OldestAccess time.Time

	// NewestAccess is the newest last_access timestamp among all entries.
	// Zero if cache is empty.
	NewestAccess time.Time
}

// CacheManager handles cache size management with LRU eviction.
type CacheManager struct {
	cacheDir  string
	sizeLimit int64
	highWater float64 // Threshold to trigger eviction (default 0.80)
	lowWater  float64 // Target after eviction (default 0.60)
}

// NewCacheManager creates a new CacheManager.
// The sizeLimit is the maximum cache size in bytes.
func NewCacheManager(cacheDir string, sizeLimit int64) *CacheManager {
	return &CacheManager{
		cacheDir:  cacheDir,
		sizeLimit: sizeLimit,
		highWater: 0.80,
		lowWater:  0.60,
	}
}

// Size returns the total size of the cache in bytes.
// It sums the sizes of all .toml and .meta.json files in the cache directory.
func (m *CacheManager) Size() (int64, error) {
	var totalSize int64

	entries, err := os.ReadDir(m.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		letterDir := filepath.Join(m.cacheDir, entry.Name())
		subEntries, err := os.ReadDir(letterDir)
		if err != nil {
			continue
		}

		for _, subEntry := range subEntries {
			if subEntry.IsDir() {
				continue
			}

			// Count .toml and .meta.json files
			name := subEntry.Name()
			if !strings.HasSuffix(name, ".toml") && !strings.HasSuffix(name, ".meta.json") {
				continue
			}

			info, err := subEntry.Info()
			if err != nil {
				continue
			}
			totalSize += info.Size()
		}
	}

	return totalSize, nil
}

// cacheEntry represents a cached recipe with its metadata for LRU sorting.
type cacheEntry struct {
	name       string
	lastAccess time.Time
	size       int64 // Size of both .toml and .meta.json
}

// listEntries returns all cache entries with their metadata.
func (m *CacheManager) listEntries() ([]cacheEntry, error) {
	var entries []cacheEntry

	dirEntries, err := os.ReadDir(m.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range dirEntries {
		if !entry.IsDir() {
			continue
		}

		letterDir := filepath.Join(m.cacheDir, entry.Name())
		subEntries, err := os.ReadDir(letterDir)
		if err != nil {
			continue
		}

		for _, subEntry := range subEntries {
			if subEntry.IsDir() {
				continue
			}

			// Only process .toml files (not .meta.json)
			if !strings.HasSuffix(subEntry.Name(), ".toml") {
				continue
			}

			name := strings.TrimSuffix(subEntry.Name(), ".toml")
			tomlPath := filepath.Join(letterDir, subEntry.Name())
			metaPath := filepath.Join(letterDir, name+".meta.json")

			// Get file sizes
			var totalSize int64
			if info, err := os.Stat(tomlPath); err == nil {
				totalSize += info.Size()
			}
			if info, err := os.Stat(metaPath); err == nil {
				totalSize += info.Size()
			}

			// Get last access from metadata
			lastAccess := time.Now() // Default to now if no metadata
			if metaData, err := os.ReadFile(metaPath); err == nil {
				var meta CacheMetadata
				if err := jsonUnmarshal(metaData, &meta); err == nil && !meta.LastAccess.IsZero() {
					lastAccess = meta.LastAccess
				}
			} else {
				// No metadata - use file modification time
				if info, err := os.Stat(tomlPath); err == nil {
					lastAccess = info.ModTime()
				}
			}

			entries = append(entries, cacheEntry{
				name:       name,
				lastAccess: lastAccess,
				size:       totalSize,
			})
		}
	}

	return entries, nil
}

// deleteEntry removes a cached recipe and its metadata file.
func (m *CacheManager) deleteEntry(name string) error {
	letter := firstLetter(name)
	letterDir := filepath.Join(m.cacheDir, letter)

	tomlPath := filepath.Join(letterDir, name+".toml")
	metaPath := filepath.Join(letterDir, name+".meta.json")

	// Delete both files, ignoring not-found errors
	var lastErr error
	if err := os.Remove(tomlPath); err != nil && !os.IsNotExist(err) {
		lastErr = err
	}
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		lastErr = err
	}

	return lastErr
}

// EnforceLimit checks if the cache exceeds the high water mark (80%) and
// evicts least-recently-used entries until the cache is below the low water
// mark (60%). Returns the number of entries evicted.
func (m *CacheManager) EnforceLimit() (int, error) {
	currentSize, err := m.Size()
	if err != nil {
		return 0, err
	}

	highWaterSize := int64(float64(m.sizeLimit) * m.highWater)
	if currentSize <= highWaterSize {
		return 0, nil
	}

	// Cache is above high water mark - need to evict
	// Log warning
	percentUsed := float64(currentSize) / float64(m.sizeLimit) * 100
	fmt.Fprintf(os.Stderr, "Warning: Recipe cache is %.0f%% full (%dMB of %dMB). Run 'tsuku cache cleanup' to free space.\n",
		percentUsed, currentSize/(1024*1024), m.sizeLimit/(1024*1024))

	entries, err := m.listEntries()
	if err != nil {
		return 0, err
	}

	// Sort by last_access ascending (oldest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lastAccess.Before(entries[j].lastAccess)
	})

	lowWaterSize := int64(float64(m.sizeLimit) * m.lowWater)
	evicted := 0

	for _, entry := range entries {
		if currentSize <= lowWaterSize {
			break
		}

		if err := m.deleteEntry(entry.name); err != nil {
			// Log but continue trying to evict other entries
			continue
		}

		currentSize -= entry.size
		evicted++
	}

	return evicted, nil
}

// Cleanup removes cache entries that haven't been accessed within maxAge.
// Returns the number of entries removed.
func (m *CacheManager) Cleanup(maxAge time.Duration) (int, error) {
	entries, err := m.listEntries()
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for _, entry := range entries {
		if entry.lastAccess.Before(cutoff) {
			if err := m.deleteEntry(entry.name); err != nil {
				continue
			}
			removed++
		}
	}

	return removed, nil
}

// Info returns statistics about the cache.
func (m *CacheManager) Info() (*CacheStats, error) {
	entries, err := m.listEntries()
	if err != nil {
		return nil, err
	}

	stats := &CacheStats{
		EntryCount: len(entries),
	}

	for _, entry := range entries {
		stats.TotalSize += entry.size

		if stats.OldestAccess.IsZero() || entry.lastAccess.Before(stats.OldestAccess) {
			stats.OldestAccess = entry.lastAccess
		}
		if stats.NewestAccess.IsZero() || entry.lastAccess.After(stats.NewestAccess) {
			stats.NewestAccess = entry.lastAccess
		}
	}

	return stats, nil
}

// jsonUnmarshal is a helper that wraps encoding/json.Unmarshal.
// This avoids importing encoding/json twice (already imported in cache.go).
func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
