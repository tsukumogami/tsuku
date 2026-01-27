package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultCacheTTL is the default time-to-live for cached recipes.
// The actual configurable TTL will be added in a subsequent issue.
const DefaultCacheTTL = 24 * time.Hour

// CacheMetadata stores metadata about a cached recipe file.
// This is stored in a sidecar file alongside the recipe (e.g., fzf.meta.json).
type CacheMetadata struct {
	// CachedAt is when the recipe was originally cached.
	CachedAt time.Time `json:"cached_at"`

	// ExpiresAt is when the cache entry expires (CachedAt + TTL).
	ExpiresAt time.Time `json:"expires_at"`

	// LastAccess is the last time this cache entry was read.
	LastAccess time.Time `json:"last_access"`

	// Size is the size of the recipe file in bytes.
	Size int64 `json:"size"`

	// ContentHash is the SHA256 hash of the recipe content for integrity verification.
	ContentHash string `json:"content_hash"`
}

// metaPath returns the path to a cached recipe's metadata sidecar file.
// For recipe path "registry/f/fzf.toml", returns "registry/f/fzf.meta.json".
func (r *Registry) metaPath(name string) string {
	return filepath.Join(r.CacheDir, firstLetter(name), name+".meta.json")
}

// WriteMeta writes cache metadata to the sidecar file.
func (r *Registry) WriteMeta(name string, meta *CacheMetadata) error {
	path := r.metaPath(name)
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return &Error{Type: ErrTypeCacheWrite, Recipe: name, Cause: err, Message: "failed to create metadata directory"}
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return &Error{Type: ErrTypeCacheWrite, Recipe: name, Cause: err, Message: "failed to marshal metadata"}
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return &Error{Type: ErrTypeCacheWrite, Recipe: name, Cause: err, Message: "failed to write metadata"}
	}

	return nil
}

// ReadMeta reads cache metadata from the sidecar file.
// Returns nil, nil if the metadata file doesn't exist (cache miss).
// Returns nil, error if the file exists but can't be read or parsed.
func (r *Registry) ReadMeta(name string) (*CacheMetadata, error) {
	path := r.metaPath(name)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Metadata doesn't exist yet
		}
		return nil, &Error{Type: ErrTypeCacheRead, Recipe: name, Cause: err, Message: "failed to read metadata"}
	}

	var meta CacheMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		// Invalid metadata file, treat as cache miss but log the error
		return nil, &Error{Type: ErrTypeCacheRead, Recipe: name, Cause: err, Message: "failed to parse metadata"}
	}

	return &meta, nil
}

// UpdateLastAccess updates the LastAccess timestamp in the metadata file.
// This is called when a cached recipe is read to support LRU eviction.
func (r *Registry) UpdateLastAccess(name string) error {
	meta, err := r.ReadMeta(name)
	if err != nil {
		return err
	}
	if meta == nil {
		return nil // No metadata to update
	}

	meta.LastAccess = time.Now()
	return r.WriteMeta(name, meta)
}

// computeContentHash computes the SHA256 hash of content and returns it as a hex string.
func computeContentHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// newCacheMetadata creates a new CacheMetadata for freshly cached content.
func newCacheMetadata(content []byte, ttl time.Duration) *CacheMetadata {
	now := time.Now()
	return &CacheMetadata{
		CachedAt:    now,
		ExpiresAt:   now.Add(ttl),
		LastAccess:  now,
		Size:        int64(len(content)),
		ContentHash: computeContentHash(content),
	}
}

// newCacheMetadataFromFile creates CacheMetadata for an existing cached file
// that doesn't have metadata yet (migration case).
// Uses the file's modification time as CachedAt.
func newCacheMetadataFromFile(path string, content []byte, ttl time.Duration) (*CacheMetadata, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	modTime := info.ModTime()
	return &CacheMetadata{
		CachedAt:    modTime,
		ExpiresAt:   modTime.Add(ttl),
		LastAccess:  time.Now(),
		Size:        int64(len(content)),
		ContentHash: computeContentHash(content),
	}, nil
}

// DeleteMeta removes the metadata sidecar file for a recipe.
// This is used when removing a cached recipe.
func (r *Registry) DeleteMeta(name string) error {
	path := r.metaPath(name)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return &Error{Type: ErrTypeCacheWrite, Recipe: name, Cause: err, Message: "failed to delete metadata"}
	}
	return nil
}

// ListCachedWithMeta returns recipe names and their metadata for all cached recipes.
// This is useful for cache statistics and LRU eviction.
func (r *Registry) ListCachedWithMeta() (map[string]*CacheMetadata, error) {
	result := make(map[string]*CacheMetadata)

	entries, err := os.ReadDir(r.CacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		letterDir := filepath.Join(r.CacheDir, entry.Name())
		subEntries, err := os.ReadDir(letterDir)
		if err != nil {
			continue
		}

		for _, subEntry := range subEntries {
			if subEntry.IsDir() {
				continue
			}
			// Only process .toml files (skip .meta.json)
			if !strings.HasSuffix(subEntry.Name(), ".toml") {
				continue
			}

			name := strings.TrimSuffix(subEntry.Name(), ".toml")
			meta, _ := r.ReadMeta(name) // Ignore errors, just skip
			result[name] = meta         // May be nil if no metadata
		}
	}

	return result, nil
}
