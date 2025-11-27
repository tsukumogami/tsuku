package version

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// CacheTTL defines how long cached assets remain valid
	CacheTTL = 5 * time.Minute

	// MaxCacheSize limits the number of cached entries to prevent unbounded growth
	MaxCacheSize = 1000

	// APITimeout is the timeout for individual GitHub API calls
	APITimeout = 30 * time.Second
)

// cachedAssets holds asset list with expiration timestamp
type cachedAssets struct {
	assets    []string
	expiresAt time.Time
}

// assetCache provides thread-safe caching of GitHub release assets with TTL
type assetCache struct {
	mu      sync.Mutex // Use Mutex (not RWMutex) to prevent race in GetOrFetch pattern
	entries map[string]*cachedAssets // key: "owner/repo:tag"
	fetching map[string]*sync.WaitGroup // tracks in-flight fetches to prevent duplicate requests
}

// globalAssetCache is the singleton cache instance
var globalAssetCache = &assetCache{
	entries:  make(map[string]*cachedAssets),
	fetching: make(map[string]*sync.WaitGroup),
}

// FetchReleaseAssets fetches the list of asset names for a specific GitHub release
// Results are cached with TTL to minimize API calls
// Uses the Resolver's authenticated GitHub client
// Prevents duplicate concurrent fetches for the same release
func (r *Resolver) FetchReleaseAssets(ctx context.Context, repo, tag string) ([]string, error) {
	// Parse repo into owner/name
	owner, repoName, err := parseRepo(repo)
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("%s/%s:%s", owner, repoName, tag)

	// GetOrFetch pattern: Check cache and handle concurrent requests
	globalAssetCache.mu.Lock()

	// Check if cached and not expired
	if cached, ok := globalAssetCache.entries[cacheKey]; ok {
		if time.Now().Before(cached.expiresAt) {
			// Cache hit - return immediately
			globalAssetCache.mu.Unlock()
			return cached.assets, nil
		}
		// Cache expired - remove it
		delete(globalAssetCache.entries, cacheKey)
	}

	// Check if another goroutine is already fetching this
	if wg, fetching := globalAssetCache.fetching[cacheKey]; fetching {
		// Wait for the other fetch to complete
		globalAssetCache.mu.Unlock()
		wg.Wait()

		// Try cache again after waiting
		globalAssetCache.mu.Lock()
		if cached, ok := globalAssetCache.entries[cacheKey]; ok && time.Now().Before(cached.expiresAt) {
			globalAssetCache.mu.Unlock()
			return cached.assets, nil
		}
		globalAssetCache.mu.Unlock()
		// Fetch completed but didn't populate cache - likely failed
		// The original fetch error was already logged/returned to the first caller
		return nil, fmt.Errorf("failed to fetch release assets for %s (concurrent fetch failed - check logs for details)", repo)
	}

	// Mark this key as being fetched
	wg := &sync.WaitGroup{}
	wg.Add(1)
	globalAssetCache.fetching[cacheKey] = wg
	globalAssetCache.mu.Unlock()

	// Ensure cleanup on exit
	// IMPORTANT: wg.Done() must run first to unblock waiters even if cleanup panics
	defer func() {
		wg.Done() // Release waiters first (most critical)
		globalAssetCache.mu.Lock()
		delete(globalAssetCache.fetching, cacheKey)
		globalAssetCache.mu.Unlock()
	}()

	// Enforce cache size limit to prevent unbounded growth
	globalAssetCache.mu.Lock()
	if len(globalAssetCache.entries) >= MaxCacheSize {
		// Simple eviction: clear half the cache
		// Collect keys first to make eviction deterministic
		keys := make([]string, 0, len(globalAssetCache.entries))
		for k := range globalAssetCache.entries {
			keys = append(keys, k)
		}
		// Delete first half of collected keys
		deleteCount := len(keys) / 2
		for i := 0; i < deleteCount; i++ {
			delete(globalAssetCache.entries, keys[i])
		}
	}
	globalAssetCache.mu.Unlock()

	// Proactive rate limit check
	if err := r.checkRateLimit(ctx); err != nil {
		return nil, err
	}

	// Fetch from GitHub API with timeout (use passed context, not Background!)
	fetchCtx, cancel := context.WithTimeout(ctx, APITimeout)
	defer cancel()

	release, resp, err := r.client.Repositories.GetReleaseByTag(fetchCtx, owner, repoName, tag)
	if err != nil {
		// Handle specific error cases
		if resp != nil {
			switch resp.StatusCode {
			case 404:
				return nil, fmt.Errorf("release '%s' not found in '%s'. It may be a draft or the tag doesn't exist", tag, repo)
			case 403:
				return nil, fmt.Errorf("GitHub API rate limit exceeded. Set GITHUB_TOKEN environment variable to increase limits")
			}
		}

		// Network errors
		if strings.Contains(err.Error(), "network is unreachable") ||
			strings.Contains(err.Error(), "no such host") ||
			strings.Contains(err.Error(), "dial tcp") ||
			strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, fmt.Errorf("network error while fetching release assets: %w", err)
		}

		return nil, fmt.Errorf("failed to fetch release: %w", err)
	}

	// Check if release has any assets
	if len(release.Assets) == 0 {
		return nil, fmt.Errorf("release '%s' for '%s' has no assets", tag, repo)
	}

	// Extract asset names (GitHub API returns newest first)
	assetNames := make([]string, 0, len(release.Assets))
	for _, asset := range release.Assets {
		if asset.Name != nil {
			assetNames = append(assetNames, *asset.Name)
		}
	}

	// Store in cache with TTL
	globalAssetCache.mu.Lock()
	globalAssetCache.entries[cacheKey] = &cachedAssets{
		assets:    assetNames,
		expiresAt: time.Now().Add(CacheTTL),
	}
	globalAssetCache.mu.Unlock()

	return assetNames, nil
}

// MatchAssetPattern matches a glob pattern against a list of asset names
// Returns the first match (in list order, which is newest first from GitHub API)
// Supports *, ?, and [] wildcards via filepath.Match
func MatchAssetPattern(pattern string, assets []string) (string, error) {
	// Validate pattern before matching
	if pattern == "" {
		return "", fmt.Errorf("pattern cannot be empty")
	}

	// Test if pattern is valid glob syntax
	if _, err := filepath.Match(pattern, "test"); err != nil {
		return "", fmt.Errorf("invalid glob pattern '%s': %w", pattern, err)
	}

	// Match in order and return first match (GitHub API returns newest first)
	// Optimized: return immediately on first match instead of collecting all
	for _, asset := range assets {
		matched, err := filepath.Match(pattern, asset)
		if err != nil {
			return "", fmt.Errorf("error matching pattern '%s' against '%s': %w", pattern, asset, err)
		}
		if matched {
			return asset, nil // Return immediately on first match
		}
	}

	// No matches found
	return "", formatNoMatchError(pattern, assets)
}

// parseRepo parses "owner/repo" format into components
func parseRepo(repo string) (owner, name string, err error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo format '%s': expected 'owner/repo'", repo)
	}

	owner = strings.TrimSpace(parts[0])
	name = strings.TrimSpace(parts[1])

	if owner == "" || name == "" {
		return "", "", fmt.Errorf("invalid repo format '%s': owner and name must not be empty", repo)
	}

	return owner, name, nil
}

// checkRateLimit checks GitHub API rate limits before making requests
// Returns an error if rate limit is exhausted, otherwise returns nil
func (r *Resolver) checkRateLimit(ctx context.Context) error {
	rateLimits, resp, err := r.client.RateLimit.Get(ctx)
	if err != nil {
		// If the rate limit check itself fails, check if it's a critical error
		if resp != nil && resp.StatusCode == 403 {
			// Likely already rate limited
			return fmt.Errorf("GitHub API rate limit check failed with 403 (likely exhausted). Set GITHUB_TOKEN to increase limits: %w", err)
		}
		// For other errors (network, timeout), log but don't fail - proceed with the request
		// The actual request will fail if there's a real issue
		return nil
	}

	// If we're close to exhausting the limit, warn the user
	if rateLimits.Core.Remaining < 10 {
		resetTime := rateLimits.Core.Reset.Time.Format(time.RFC3339)

		// If completely exhausted, return error
		if rateLimits.Core.Remaining == 0 {
			return fmt.Errorf("GitHub API rate limit exhausted (%d remaining). Resets at %s. Set GITHUB_TOKEN to increase limits",
				rateLimits.Core.Remaining, resetTime)
		}

		// Otherwise, just warn (don't block the request)
		fmt.Printf("   ⚠ Low GitHub API rate limit: %d requests remaining (resets at %s)\n",
			rateLimits.Core.Remaining, resetTime)
	}

	return nil
}

// formatNoMatchError creates a user-friendly error message when no assets match
// Lists up to 10 available assets to help debug the pattern
func formatNoMatchError(pattern string, assets []string) error {
	maxDisplay := 10
	displayAssets := assets
	if len(assets) > maxDisplay {
		displayAssets = assets[:maxDisplay]
	}

	msg := fmt.Sprintf("no asset matched pattern '%s'\nAvailable assets:", pattern)
	for _, asset := range displayAssets {
		msg += fmt.Sprintf("\n  - %s", asset)
	}

	if len(assets) > maxDisplay {
		msg += fmt.Sprintf("\n  ... and %d more", len(assets)-maxDisplay)
	}

	return fmt.Errorf("%s", msg)
}

// ContainsWildcards checks if a string contains glob wildcard characters (*, ?, [])
// Exported for use by actions that need to detect wildcard patterns
func ContainsWildcards(s string) bool {
	return strings.ContainsAny(s, "*?[]")
}
