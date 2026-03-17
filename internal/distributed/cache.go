package distributed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultCacheTTL is the default time before a directory listing is considered stale.
// Shorter than the central registry's 24-hour TTL because distributed sources
// are typically smaller repos that change more frequently.
const DefaultCacheTTL = 1 * time.Hour

// DefaultMaxCacheSize is the default maximum size for the distributed cache (20MB).
// Independent from the central registry cache limit (50MB).
const DefaultMaxCacheSize int64 = 20 * 1024 * 1024

// SourceMeta holds cached metadata about a repository's .tsuku-recipes directory.
// Stored as _source.json in the per-repo cache directory.
type SourceMeta struct {
	Branch    string            `json:"branch"`
	Files     map[string]string `json:"files"` // recipe name -> download URL
	FetchedAt time.Time         `json:"fetched_at"`
	// Incomplete is true when the listing was obtained via branch probing
	// (rate-limit fallback) rather than the Contents API. Incomplete entries
	// use a shorter TTL so the full listing is fetched once rate limits reset.
	Incomplete bool `json:"incomplete,omitempty"`
}

// RecipeMeta holds cached metadata about an individual recipe file.
// Stored as {recipe}.meta.json alongside the TOML file.
type RecipeMeta struct {
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	FetchedAt    time.Time `json:"fetched_at"`
}

// CacheManager manages the on-disk cache for distributed recipe sources.
// Each repository gets its own directory under $TSUKU_HOME/cache/distributed/{owner}/{repo}/.
type CacheManager struct {
	baseDir  string
	ttl      time.Duration
	maxBytes int64 // maximum total cache size in bytes
}

// NewCacheManager creates a CacheManager rooted at the given base directory.
// The ttl controls how long a directory listing is considered fresh.
func NewCacheManager(baseDir string, ttl time.Duration) *CacheManager {
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &CacheManager{
		baseDir:  baseDir,
		ttl:      ttl,
		maxBytes: DefaultMaxCacheSize,
	}
}

// SetMaxSize overrides the default maximum cache size.
func (cm *CacheManager) SetMaxSize(maxBytes int64) {
	if maxBytes > 0 {
		cm.maxBytes = maxBytes
	}
}

// repoDir returns the cache directory for a given owner/repo, validating
// that neither component contains path traversal sequences.
func (cm *CacheManager) repoDir(owner, repo string) (string, error) {
	if strings.Contains(owner, "..") || strings.Contains(repo, "..") ||
		strings.Contains(owner, "/") || strings.Contains(repo, "/") ||
		strings.Contains(owner, string(os.PathSeparator)) || strings.Contains(repo, string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid owner/repo: %s/%s", owner, repo)
	}
	return filepath.Join(cm.baseDir, owner, repo), nil
}

// GetSourceMeta reads the cached _source.json for a repository.
// Returns nil, nil if the file does not exist.
func (cm *CacheManager) GetSourceMeta(owner, repo string) (*SourceMeta, error) {
	dir, err := cm.repoDir(owner, repo)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Join(dir, "_source.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading source meta: %w", err)
	}

	var meta SourceMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing source meta: %w", err)
	}
	return &meta, nil
}

// PutSourceMeta writes the _source.json for a repository.
func (cm *CacheManager) PutSourceMeta(owner, repo string, meta *SourceMeta) error {
	dir, err := cm.repoDir(owner, repo)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling source meta: %w", err)
	}

	return os.WriteFile(filepath.Join(dir, "_source.json"), data, 0644)
}

// IsSourceFresh returns true if the source meta was fetched within the TTL window.
// Incomplete entries (from branch probing) use a 5-minute TTL so the full
// listing is re-fetched once API rate limits reset.
func (cm *CacheManager) IsSourceFresh(meta *SourceMeta) bool {
	if meta == nil {
		return false
	}
	ttl := cm.ttl
	if meta.Incomplete {
		ttl = 5 * time.Minute
	}
	return time.Since(meta.FetchedAt) < ttl
}

// GetRecipe reads a cached recipe TOML file.
// Returns nil, nil if the file does not exist.
func (cm *CacheManager) GetRecipe(owner, repo, name string) ([]byte, error) {
	dir, err := cm.repoDir(owner, repo)
	if err != nil {
		return nil, err
	}

	// Sanitize recipe name against path traversal
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, string(os.PathSeparator)) {
		return nil, fmt.Errorf("invalid recipe name: %s", name)
	}

	data, err := os.ReadFile(filepath.Join(dir, name+".toml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading cached recipe: %w", err)
	}
	return data, nil
}

// PutRecipe writes a recipe TOML file and its metadata sidecar to the cache.
func (cm *CacheManager) PutRecipe(owner, repo, name string, data []byte, meta *RecipeMeta) error {
	dir, err := cm.repoDir(owner, repo)
	if err != nil {
		return err
	}

	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, string(os.PathSeparator)) {
		return fmt.Errorf("invalid recipe name: %s", name)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	// Write the TOML file
	if err := os.WriteFile(filepath.Join(dir, name+".toml"), data, 0644); err != nil {
		return fmt.Errorf("writing cached recipe: %w", err)
	}

	// Write the metadata sidecar
	if meta != nil {
		metaData, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling recipe meta: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".meta.json"), metaData, 0644); err != nil {
			return fmt.Errorf("writing recipe meta: %w", err)
		}
	}

	// Best-effort eviction when cache exceeds size limit
	if cm.Size() > cm.maxBytes {
		cm.evictOldest()
	}

	return nil
}

// IsRecipeFresh returns true if the recipe was fetched within the TTL window.
func (cm *CacheManager) IsRecipeFresh(meta *RecipeMeta) bool {
	if meta == nil {
		return false
	}
	return time.Since(meta.FetchedAt) < cm.ttl
}

// GetRecipeMeta reads the metadata sidecar for a cached recipe.
// Returns nil, nil if the file does not exist.
func (cm *CacheManager) GetRecipeMeta(owner, repo, name string) (*RecipeMeta, error) {
	dir, err := cm.repoDir(owner, repo)
	if err != nil {
		return nil, err
	}

	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, string(os.PathSeparator)) {
		return nil, fmt.Errorf("invalid recipe name: %s", name)
	}

	data, err := os.ReadFile(filepath.Join(dir, name+".meta.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading recipe meta: %w", err)
	}

	var meta RecipeMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing recipe meta: %w", err)
	}
	return &meta, nil
}

// Size returns the total size of the cache directory in bytes.
// Returns 0 if the directory doesn't exist or on errors.
func (cm *CacheManager) Size() int64 {
	var total int64
	_ = filepath.Walk(cm.baseDir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// evictOldest removes the oldest repo cache directory to free space.
// This is a best-effort operation used after writes when the cache exceeds maxBytes.
func (cm *CacheManager) evictOldest() {
	entries, err := os.ReadDir(cm.baseDir)
	if err != nil {
		return
	}

	var oldestPath string
	var oldestTime time.Time

	for _, ownerEntry := range entries {
		if !ownerEntry.IsDir() {
			continue
		}
		ownerDir := filepath.Join(cm.baseDir, ownerEntry.Name())
		repoEntries, err := os.ReadDir(ownerDir)
		if err != nil {
			continue
		}
		for _, repoEntry := range repoEntries {
			if !repoEntry.IsDir() {
				continue
			}
			sourcePath := filepath.Join(ownerDir, repoEntry.Name(), "_source.json")
			info, err := os.Stat(sourcePath)
			if err != nil {
				continue
			}
			if oldestPath == "" || info.ModTime().Before(oldestTime) {
				oldestPath = filepath.Join(ownerDir, repoEntry.Name())
				oldestTime = info.ModTime()
			}
		}
	}

	if oldestPath != "" {
		_ = os.RemoveAll(oldestPath)
	}
}
