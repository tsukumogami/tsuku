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
const DefaultCacheTTL = 1 * time.Hour

// SourceMeta holds cached metadata about a repository's .tsuku-recipes directory.
// Stored as _source.json in the per-repo cache directory.
type SourceMeta struct {
	Branch    string            `json:"branch"`
	Files     map[string]string `json:"files"` // recipe name -> download URL
	FetchedAt time.Time         `json:"fetched_at"`
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
	baseDir string
	ttl     time.Duration
}

// NewCacheManager creates a CacheManager rooted at the given base directory.
// The ttl controls how long a directory listing is considered fresh.
func NewCacheManager(baseDir string, ttl time.Duration) *CacheManager {
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	return &CacheManager{
		baseDir: baseDir,
		ttl:     ttl,
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
func (cm *CacheManager) IsSourceFresh(meta *SourceMeta) bool {
	if meta == nil {
		return false
	}
	return time.Since(meta.FetchedAt) < cm.ttl
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

	return nil
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
