package registry

import (
	"context"
	"time"
)

// CachedRegistry wraps a Registry with TTL-based cache expiration.
// It checks cache freshness before returning recipes and refreshes
// expired entries from the network.
type CachedRegistry struct {
	registry     *Registry
	ttl          time.Duration
	cacheManager *CacheManager
}

// NewCachedRegistry creates a CachedRegistry that wraps the given Registry.
// The ttl parameter controls how long cached recipes are considered fresh.
func NewCachedRegistry(reg *Registry, ttl time.Duration) *CachedRegistry {
	return &CachedRegistry{
		registry: reg,
		ttl:      ttl,
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

// GetRecipe returns a recipe by name, using cache when fresh.
//
// Behavior:
//   - If cached and fresh (within TTL): returns cached content
//   - If cached but expired: attempts refresh from network
//   - If not cached: fetches from network
//
// On network failure with expired cache, returns error (no stale fallback).
// Stale-if-error fallback is implemented in a subsequent issue.
func (c *CachedRegistry) GetRecipe(ctx context.Context, name string) ([]byte, error) {
	// Check cache first
	cached, err := c.registry.GetCached(name)
	if err != nil {
		return nil, err
	}

	if cached != nil {
		// Cache hit - check freshness
		meta, err := c.registry.ReadMeta(name)
		if err != nil {
			// Metadata read error - treat as cache miss, try network
			return c.fetchAndCache(ctx, name)
		}

		if meta != nil && c.isFresh(meta) {
			// Fresh cache hit
			return cached, nil
		}

		// Expired - try to refresh
		content, fetchErr := c.registry.FetchRecipe(ctx, name)
		if fetchErr != nil {
			// Network failed, cache is expired - return error
			// (no stale fallback in this issue - that's #1159)
			return nil, fetchErr
		}

		// Refresh succeeded - update cache
		if cacheErr := c.cacheWithTTL(name, content); cacheErr != nil {
			// Cache write failed but we have fresh content - return it anyway
			return content, nil
		}

		return content, nil
	}

	// Cache miss - fetch from network
	return c.fetchAndCache(ctx, name)
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
