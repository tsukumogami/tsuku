package actions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// DownloadCache provides caching for downloaded files.
// Cache entries are stored in $TSUKU_HOME/cache/downloads/ with URL hash as filename.
type DownloadCache struct {
	cacheDir string
}

// downloadCacheEntry represents metadata for a cached download
type downloadCacheEntry struct {
	URL        string    `json:"url"`
	Checksum   string    `json:"checksum,omitempty"`    // Expected checksum if known
	ActualHash string    `json:"actual_hash,omitempty"` // Computed SHA256 of cached file
	Size       int64     `json:"size"`
	CachedAt   time.Time `json:"cached_at"`
}

// NewDownloadCache creates a new download cache.
// The cacheDir should be $TSUKU_HOME/cache/downloads.
func NewDownloadCache(cacheDir string) *DownloadCache {
	return &DownloadCache{cacheDir: cacheDir}
}

// cacheKey generates a filesystem-safe cache key from a URL
func (c *DownloadCache) cacheKey(url string) string {
	hash := sha256.Sum256([]byte(url))
	return hex.EncodeToString(hash[:])
}

// cachePaths returns the paths to the cached file and its metadata
func (c *DownloadCache) cachePaths(url string) (filePath, metaPath string) {
	key := c.cacheKey(url)
	filePath = filepath.Join(c.cacheDir, key+".data")
	metaPath = filepath.Join(c.cacheDir, key+".meta")
	return
}

// Check looks up a cached download by URL.
// If found and valid, it copies the cached file to destPath and returns true.
// If checksum is provided, it verifies the cached file matches before returning.
// Returns (found, error) where found indicates cache hit.
func (c *DownloadCache) Check(url, destPath, expectedChecksum, checksumAlgo string) (bool, error) {
	filePath, metaPath := c.cachePaths(url)

	// Check if cache entry exists
	meta, err := c.readMeta(metaPath)
	if err != nil {
		// Cache miss - no metadata found
		return false, nil
	}

	// Verify cached file exists
	info, err := os.Stat(filePath)
	if err != nil {
		// Cached file missing, clean up orphaned metadata
		os.Remove(metaPath)
		return false, nil
	}

	// Verify size matches
	if info.Size() != meta.Size {
		// Size mismatch - cache corrupted, clean up
		c.invalidate(url)
		return false, nil
	}

	// If checksum provided, verify it matches
	if expectedChecksum != "" {
		if checksumAlgo == "" {
			checksumAlgo = "sha256"
		}
		if err := VerifyChecksum(filePath, expectedChecksum, checksumAlgo); err != nil {
			// Checksum mismatch - cache corrupted or different version
			c.invalidate(url)
			return false, nil
		}
	}

	// Cache hit - copy to destination
	if err := copyFile(filePath, destPath); err != nil {
		return false, fmt.Errorf("failed to copy cached file: %w", err)
	}

	return true, nil
}

// Save stores a downloaded file in the cache.
// The file at sourcePath is copied to the cache.
// checksum is optional and stored for reference.
func (c *DownloadCache) Save(url, sourcePath, checksum string) error {
	// Ensure cache directory exists
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	filePath, metaPath := c.cachePaths(url)

	// Get source file info
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	// Compute SHA256 of source file for integrity verification
	actualHash, err := computeSHA256(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to compute hash: %w", err)
	}

	// Copy file to cache (atomic: write to temp, then rename)
	tempPath := filePath + ".tmp"
	if err := copyFile(sourcePath, tempPath); err != nil {
		return fmt.Errorf("failed to copy to cache: %w", err)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to finalize cache file: %w", err)
	}

	// Write metadata
	meta := downloadCacheEntry{
		URL:        url,
		Checksum:   checksum,
		ActualHash: actualHash,
		Size:       info.Size(),
		CachedAt:   time.Now(),
	}

	if err := c.writeMeta(metaPath, &meta); err != nil {
		// Best effort - don't fail if metadata write fails
		return nil
	}

	return nil
}

// Clear removes all cached downloads.
func (c *DownloadCache) Clear() error {
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to clear
		}
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	var removedCount int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(c.cacheDir, entry.Name())
		if err := os.Remove(path); err != nil {
			// Continue on error, try to remove as many as possible
			continue
		}
		removedCount++
	}

	return nil
}

// CacheInfo returns information about the cache contents
type CacheInfo struct {
	EntryCount int
	TotalSize  int64
}

// Info returns information about the current cache state
func (c *DownloadCache) Info() (*CacheInfo, error) {
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &CacheInfo{}, nil
		}
		return nil, fmt.Errorf("failed to read cache directory: %w", err)
	}

	info := &CacheInfo{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Only count .data files as entries
		if filepath.Ext(entry.Name()) == ".data" {
			info.EntryCount++
			if fi, err := entry.Info(); err == nil {
				info.TotalSize += fi.Size()
			}
		}
	}

	return info, nil
}

// invalidate removes a cache entry
func (c *DownloadCache) invalidate(url string) {
	filePath, metaPath := c.cachePaths(url)
	os.Remove(filePath)
	os.Remove(metaPath)
}

// readMeta reads cache entry metadata
func (c *DownloadCache) readMeta(metaPath string) (*downloadCacheEntry, error) {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}

	var meta downloadCacheEntry
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// writeMeta writes cache entry metadata atomically
func (c *DownloadCache) writeMeta(metaPath string, meta *downloadCacheEntry) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	tempPath := metaPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tempPath, metaPath); err != nil {
		os.Remove(tempPath)
		return err
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}

// computeSHA256 computes the SHA256 hash of a file
func computeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
