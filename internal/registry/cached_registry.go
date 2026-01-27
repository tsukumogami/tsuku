package registry

import (
	"context"
	"fmt"
	"os"
	"time"
)

// CacheInfo provides information about cache state when returning recipes.
type CacheInfo struct {
	// IsStale indicates the returned content is from an expired cache entry
	IsStale bool
	// CachedAt is when the content was originally cached
	CachedAt time.Time
}

// RefreshStats contains statistics from a refresh operation.
type RefreshStats struct {
	Total     int
	Refreshed int
	Fresh     int
	Errors    int
	Details   []RefreshDetail
}

// RefreshDetail contains information about a single recipe refresh.
type RefreshDetail struct {
	Name   string
	Status string // "refreshed", "already fresh", "error"
	Age    time.Duration
	Error  error
}

// CachedRegistry wraps a Registry with TTL-based cache expiration.
// It checks cache freshness before returning recipes and refreshes
// expired entries from the network. Supports stale-if-error fallback
// for resilience during network issues.
type CachedRegistry struct {
	registry      *Registry
	ttl           time.Duration
	maxStale      time.Duration
	staleFallback bool
	cacheManager  *CacheManager
}

// NewCachedRegistry creates a CachedRegistry that wraps the given Registry.
// The ttl parameter controls how long cached recipes are considered fresh.
// Stale fallback is enabled by default with a 7-day maximum staleness.
func NewCachedRegistry(reg *Registry, ttl time.Duration) *CachedRegistry {
	return &CachedRegistry{
		registry:      reg,
		ttl:           ttl,
		maxStale:      7 * 24 * time.Hour, // Default 7 days
		staleFallback: true,               // Default enabled
		// CacheManager is nil by default - call SetCacheManager to enable size management
	}
}

// SetCacheManager configures size-based cache management.
// When set, EnforceLimit() is called after each cache write.
func (c *CachedRegistry) SetCacheManager(cm *CacheManager) {
	c.cacheManager = cm
}

// CacheManager returns the configured CacheManager, or nil if not set.
func (c *CachedRegistry) CacheManager() *CacheManager {
	return c.cacheManager
}

// SetMaxStale configures the maximum staleness allowed for stale-if-error fallback.
// Set to 0 to disable stale fallback regardless of staleFallback setting.
func (c *CachedRegistry) SetMaxStale(d time.Duration) {
	c.maxStale = d
}

// SetStaleFallback enables or disables stale-if-error fallback.
func (c *CachedRegistry) SetStaleFallback(enabled bool) {
	c.staleFallback = enabled
}

// GetRecipe returns a recipe by name, using cache when fresh.
//
// Behavior:
//   - If cached and fresh (within TTL): returns cached content
//   - If cached but expired: attempts refresh from network
//   - If network fails with stale cache within maxStale: returns stale content with warning
//   - If network fails with cache too old: returns ErrTypeCacheTooStale error
//   - If not cached: fetches from network
//
// The CacheInfo return value indicates whether stale data was returned.
func (c *CachedRegistry) GetRecipe(ctx context.Context, name string) ([]byte, *CacheInfo, error) {
	// Check cache first
	cached, err := c.registry.GetCached(name)
	if err != nil {
		return nil, nil, err
	}

	if cached != nil {
		// Cache hit - check freshness
		meta, err := c.registry.ReadMeta(name)
		if err != nil {
			// Metadata read error - treat as cache miss, try network
			content, fetchErr := c.fetchAndCache(ctx, name)
			return content, nil, fetchErr
		}

		if meta != nil && c.isFresh(meta) {
			// Fresh cache hit
			return cached, &CacheInfo{IsStale: false, CachedAt: meta.CachedAt}, nil
		}

		// Expired - try to refresh
		content, fetchErr := c.registry.FetchRecipe(ctx, name)
		if fetchErr != nil {
			// Network failed - try stale fallback
			return c.handleStaleFallback(name, cached, meta, fetchErr)
		}

		// Refresh succeeded - update cache
		if cacheErr := c.cacheWithTTL(name, content); cacheErr != nil {
			// Cache write failed but we have fresh content - return it anyway
			return content, &CacheInfo{IsStale: false, CachedAt: time.Now()}, nil
		}

		return content, &CacheInfo{IsStale: false, CachedAt: time.Now()}, nil
	}

	// Cache miss - fetch from network
	content, fetchErr := c.fetchAndCache(ctx, name)
	if fetchErr != nil {
		return nil, nil, fetchErr
	}
	return content, &CacheInfo{IsStale: false, CachedAt: time.Now()}, nil
}

// handleStaleFallback decides whether to return stale cached content or an error
// when network fetch fails for an expired cache entry.
func (c *CachedRegistry) handleStaleFallback(name string, cached []byte, meta *CacheMetadata, fetchErr error) ([]byte, *CacheInfo, error) {
	// Check if stale fallback is disabled
	if !c.staleFallback || c.maxStale == 0 {
		return nil, nil, fetchErr
	}

	// Check if we have valid metadata
	if meta == nil {
		return nil, nil, fetchErr
	}

	// Calculate cache age
	age := time.Since(meta.CachedAt)

	// Check if cache is within max stale bound
	if age < c.maxStale {
		// Log warning to stderr
		fmt.Fprintf(os.Stderr, "Warning: Using cached recipe '%s' (last updated %s ago). "+
			"Run 'tsuku update-registry' to refresh.\n", name, formatDuration(age))
		return cached, &CacheInfo{IsStale: true, CachedAt: meta.CachedAt}, nil
	}

	// Cache too stale - return error
	return nil, nil, &RegistryError{
		Type:   ErrTypeCacheTooStale,
		Recipe: name,
		Message: fmt.Sprintf("cache expired %s ago (max %s)",
			formatDuration(age), formatDuration(c.maxStale)),
	}
}

// formatDuration formats a duration for human-readable display.
func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// Registry returns the underlying Registry for direct access when needed.
func (c *CachedRegistry) Registry() *Registry {
	return c.registry
}

// isFresh checks if cache metadata indicates the entry is still fresh.
func (c *CachedRegistry) isFresh(meta *CacheMetadata) bool {
	// Calculate expiration based on configured TTL, not stored ExpiresAt
	// This allows TTL changes to take effect without cache invalidation
	expiresAt := meta.CachedAt.Add(c.ttl)
	return time.Now().Before(expiresAt)
}

// fetchAndCache fetches a recipe from network and caches it.
func (c *CachedRegistry) fetchAndCache(ctx context.Context, name string) ([]byte, error) {
	content, err := c.registry.FetchRecipe(ctx, name)
	if err != nil {
		return nil, err
	}

	// Cache the fetched content
	if cacheErr := c.cacheWithTTL(name, content); cacheErr != nil {
		// Cache write failed but we have content - return it anyway
		return content, nil
	}

	return content, nil
}

// cacheWithTTL caches content with metadata using the configured TTL.
func (c *CachedRegistry) cacheWithTTL(name string, content []byte) error {
	// CacheRecipe already writes metadata with DefaultCacheTTL,
	// but we want to use our configured TTL. Write recipe then update metadata.
	if err := c.registry.CacheRecipe(name, content); err != nil {
		return err
	}

	// Update metadata with our configured TTL
	meta := newCacheMetadata(content, c.ttl)
	if err := c.registry.WriteMeta(name, meta); err != nil {
		return err
	}

	// Enforce size limit if CacheManager is configured
	if c.cacheManager != nil {
		// Errors from EnforceLimit are non-fatal - cache write succeeded
		_, _ = c.cacheManager.EnforceLimit()
	}

	return nil
}

// GetCacheStatus returns the cache status for a recipe without fetching.
// Returns nil if the recipe is not cached.
func (c *CachedRegistry) GetCacheStatus(name string) (*RefreshDetail, error) {
	if !c.registry.IsCached(name) {
		return nil, nil
	}

	meta, err := c.registry.ReadMeta(name)
	if err != nil {
		return nil, err
	}

	age := time.Since(meta.CachedAt)
	status := "already fresh"
	if !c.isFresh(meta) {
		status = "expired"
	}

	return &RefreshDetail{
		Name:   name,
		Status: status,
		Age:    age,
	}, nil
}

// Refresh forces a refresh of a single recipe from the registry.
// Returns the refreshed content and details about the operation.
// Returns an error if the recipe is not cached.
func (c *CachedRegistry) Refresh(ctx context.Context, name string) (*RefreshDetail, error) {
	// Check if recipe is cached
	if !c.registry.IsCached(name) {
		return &RefreshDetail{
			Name:   name,
			Status: "error",
			Error:  fmt.Errorf("recipe '%s' is not cached", name),
		}, fmt.Errorf("recipe '%s' is not cached", name)
	}

	// Get current metadata for age reporting
	var age time.Duration
	meta, err := c.registry.ReadMeta(name)
	if err == nil && meta != nil {
		age = time.Since(meta.CachedAt)
	}

	// Fetch fresh content from network
	content, fetchErr := c.registry.FetchRecipe(ctx, name)
	if fetchErr != nil {
		return &RefreshDetail{
			Name:   name,
			Status: "error",
			Age:    age,
			Error:  fetchErr,
		}, fetchErr
	}

	// Update cache
	if cacheErr := c.cacheWithTTL(name, content); cacheErr != nil {
		return &RefreshDetail{
			Name:   name,
			Status: "error",
			Age:    age,
			Error:  cacheErr,
		}, cacheErr
	}

	return &RefreshDetail{
		Name:   name,
		Status: "refreshed",
		Age:    age,
	}, nil
}

// RefreshAll refreshes all cached recipes from the registry.
// Continues on individual recipe errors and returns aggregated statistics.
func (c *CachedRegistry) RefreshAll(ctx context.Context) (*RefreshStats, error) {
	cachedMeta, err := c.registry.ListCachedWithMeta()
	if err != nil {
		return nil, err
	}

	stats := &RefreshStats{
		Total:   len(cachedMeta),
		Details: make([]RefreshDetail, 0, len(cachedMeta)),
	}

	for name, meta := range cachedMeta {
		var age time.Duration
		if meta != nil {
			age = time.Since(meta.CachedAt)
		}

		// Check if fresh - skip refresh for fresh entries
		if meta != nil && c.isFresh(meta) {
			stats.Fresh++
			stats.Details = append(stats.Details, RefreshDetail{
				Name:   name,
				Status: "already fresh",
				Age:    age,
			})
			continue
		}

		// Expired - refresh from network
		content, fetchErr := c.registry.FetchRecipe(ctx, name)
		if fetchErr != nil {
			stats.Errors++
			stats.Details = append(stats.Details, RefreshDetail{
				Name:   name,
				Status: "error",
				Age:    age,
				Error:  fetchErr,
			})
			continue
		}

		// Update cache
		if cacheErr := c.cacheWithTTL(name, content); cacheErr != nil {
			stats.Errors++
			stats.Details = append(stats.Details, RefreshDetail{
				Name:   name,
				Status: "error",
				Age:    age,
				Error:  cacheErr,
			})
			continue
		}

		stats.Refreshed++
		stats.Details = append(stats.Details, RefreshDetail{
			Name:   name,
			Status: "refreshed",
			Age:    age,
		})
	}

	return stats, nil
}
