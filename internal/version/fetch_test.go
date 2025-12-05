package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/tsukumogami/tsuku/internal/config"
)

// mockGitHubServer creates a test HTTP server that mimics GitHub API responses
// The handler is called for release endpoints. Rate limit endpoint is handled automatically.
func mockGitHubServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	// Wrap the handler to also handle rate limit checks
	wrappedHandler := func(w http.ResponseWriter, r *http.Request) {
		// Handle rate limit endpoint
		if strings.Contains(r.URL.Path, "/rate_limit") {
			// Return a mock rate limit response with plenty of remaining calls
			// Use raw JSON to match GitHub API format exactly
			w.Header().Set("Content-Type", "application/json")
			resetTime := time.Now().Add(1 * time.Hour).Unix()
			response := fmt.Sprintf(`{
				"resources": {
					"core": {
						"limit": 5000,
						"remaining": 4999,
						"reset": %d,
						"used": 1
					}
				}
			}`, resetTime)
			_, _ = w.Write([]byte(response))
			return
		}

		// Call the custom handler for other endpoints
		handler(w, r)
	}

	return httptest.NewServer(http.HandlerFunc(wrappedHandler))
}

// mockRelease creates a GitHub release response with specified assets
func mockRelease(assetNames []string) *github.RepositoryRelease {
	assets := make([]*github.ReleaseAsset, len(assetNames))
	for i, name := range assetNames {
		nameCopy := name
		assets[i] = &github.ReleaseAsset{
			Name: &nameCopy,
		}
	}
	return &github.RepositoryRelease{
		Assets: assets,
	}
}

// resetGlobalCache clears the global cache between tests
func resetGlobalCache() {
	globalAssetCache.mu.Lock()
	globalAssetCache.entries = make(map[string]*cachedAssets)
	globalAssetCache.fetching = make(map[string]*sync.WaitGroup)
	globalAssetCache.mu.Unlock()
}

func TestFetchReleaseAssets_CacheHit(t *testing.T) {
	resetGlobalCache()

	callCount := 0
	server := mockGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Only count release endpoint calls (rate limit handled by wrapper)
		if strings.Contains(r.URL.Path, "/repos/owner/repo/releases/tags/v1.0.0") {
			callCount++
			release := mockRelease([]string{"asset1.tar.gz", "asset2.tar.gz"})
			_ = json.NewEncoder(w).Encode(release)
		}
	})
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	// Override base URL to point to mock server
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()

	// First call - should hit API
	assets1, err := resolver.FetchReleaseAssets(ctx, "owner/repo", "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assets1) != 2 {
		t.Errorf("expected 2 assets, got %d", len(assets1))
	}

	if callCount != 1 {
		t.Errorf("expected 1 API call, got %d", callCount)
	}

	// Second call - should hit cache, not API
	assets2, err := resolver.FetchReleaseAssets(ctx, "owner/repo", "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error on cache hit: %v", err)
	}

	if len(assets2) != 2 {
		t.Errorf("expected 2 assets from cache, got %d", len(assets2))
	}

	if callCount != 1 {
		t.Errorf("expected still only 1 API call (cache hit), got %d", callCount)
	}

	// Verify assets are the same
	for i := range assets1 {
		if assets1[i] != assets2[i] {
			t.Errorf("cached assets differ at index %d: %s vs %s", i, assets1[i], assets2[i])
		}
	}
}

func TestFetchReleaseAssets_CacheMiss(t *testing.T) {
	resetGlobalCache()

	callCount := 0
	server := mockGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		release := mockRelease([]string{"new-asset.tar.gz"})
		_ = json.NewEncoder(w).Encode(release)
	})
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()

	// Cache is empty - should fetch from API
	assets, err := resolver.FetchReleaseAssets(ctx, "owner/repo", "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assets) != 1 {
		t.Errorf("expected 1 asset, got %d", len(assets))
	}

	if assets[0] != "new-asset.tar.gz" {
		t.Errorf("expected 'new-asset.tar.gz', got %s", assets[0])
	}

	if callCount != 1 {
		t.Errorf("expected 1 API call, got %d", callCount)
	}
}

func TestFetchReleaseAssets_TTLExpiration(t *testing.T) {
	resetGlobalCache()

	// Temporarily reduce TTL for faster testing
	originalTTL := CacheTTL
	defer func() {
		// Note: Can't actually change const, but this shows intent
		// In real implementation, would make TTL configurable
	}()

	callCount := 0
	server := mockGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		release := mockRelease([]string{fmt.Sprintf("asset-%d.tar.gz", callCount)})
		_ = json.NewEncoder(w).Encode(release)
	})
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()

	// First fetch
	assets1, err := resolver.FetchReleaseAssets(ctx, "owner/repo", "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 API call, got %d", callCount)
	}

	// Manually expire the cache entry
	cacheKey := "owner/repo:v1.0.0"
	globalAssetCache.mu.Lock()
	if entry, ok := globalAssetCache.entries[cacheKey]; ok {
		entry.expiresAt = time.Now().Add(-1 * time.Second) // Expire it
	}
	globalAssetCache.mu.Unlock()

	// Second fetch - should hit API because cache expired
	assets2, err := resolver.FetchReleaseAssets(ctx, "owner/repo", "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error after expiration: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 API calls (cache expired), got %d", callCount)
	}

	// Assets should be different (server increments count)
	if assets1[0] == assets2[0] {
		t.Errorf("expected different assets after expiration, both are %s", assets1[0])
	}

	_ = originalTTL // Use variable to avoid unused warning
}

func TestFetchReleaseAssets_404Error(t *testing.T) {
	resetGlobalCache()

	server := mockGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "Not Found",
		})
	})
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()

	_, err := resolver.FetchReleaseAssets(ctx, "owner/repo", "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestFetchReleaseAssets_403RateLimit(t *testing.T) {
	resetGlobalCache()

	server := mockGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "API rate limit exceeded",
		})
	})
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()

	_, err := resolver.FetchReleaseAssets(ctx, "owner/repo", "v1.0.0")
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}

	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("expected 'rate limit' in error, got: %v", err)
	}

	if !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Errorf("expected 'GITHUB_TOKEN' suggestion in error, got: %v", err)
	}
}

func TestFetchReleaseAssets_Timeout(t *testing.T) {
	resetGlobalCache()

	// Use a very short timeout via environment variable to speed up the test
	// The minimum allowed by config.GetAPITimeout() is 1 second
	original := os.Getenv(config.EnvAPITimeout)
	os.Setenv(config.EnvAPITimeout, "1s")
	defer os.Setenv(config.EnvAPITimeout, original)

	server := mockGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the configured timeout (1s) to trigger timeout error
		time.Sleep(2 * time.Second)
		release := mockRelease([]string{"asset.tar.gz"})
		_ = json.NewEncoder(w).Encode(release)
	})
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()

	_, err := resolver.FetchReleaseAssets(ctx, "owner/repo", "v1.0.0")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// Should contain context deadline exceeded or similar
	errStr := err.Error()
	if !strings.Contains(errStr, "deadline") && !strings.Contains(errStr, "timeout") && !strings.Contains(errStr, "context") {
		t.Errorf("expected timeout-related error, got: %v", err)
	}
}

func TestFetchReleaseAssets_EmptyAssets(t *testing.T) {
	resetGlobalCache()

	server := mockGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Release with no assets
		release := mockRelease([]string{})
		_ = json.NewEncoder(w).Encode(release)
	})
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()

	_, err := resolver.FetchReleaseAssets(ctx, "owner/repo", "v1.0.0")
	if err == nil {
		t.Fatal("expected error for empty assets, got nil")
	}

	if !strings.Contains(err.Error(), "no assets") {
		t.Errorf("expected 'no assets' in error, got: %v", err)
	}
}

func TestFetchReleaseAssets_NilAssetNames(t *testing.T) {
	resetGlobalCache()

	server := mockGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Create release with one nil Name pointer
		validName := "valid-asset.tar.gz"
		release := &github.RepositoryRelease{
			Assets: []*github.ReleaseAsset{
				{Name: &validName},
				{Name: nil}, // Nil name - should be skipped
			},
		}
		_ = json.NewEncoder(w).Encode(release)
	})
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()

	assets, err := resolver.FetchReleaseAssets(ctx, "owner/repo", "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only return the valid asset, skipping nil
	if len(assets) != 1 {
		t.Errorf("expected 1 asset (nil skipped), got %d", len(assets))
	}

	if assets[0] != "valid-asset.tar.gz" {
		t.Errorf("expected 'valid-asset.tar.gz', got %s", assets[0])
	}
}

func TestFetchReleaseAssets_Concurrent(t *testing.T) {
	resetGlobalCache()

	callCount := 0
	var mu sync.Mutex

	server := mockGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()

		// Simulate slow API response to allow concurrent requests to pile up
		time.Sleep(100 * time.Millisecond)

		release := mockRelease([]string{"asset1.tar.gz", "asset2.tar.gz", "asset3.tar.gz"})
		_ = json.NewEncoder(w).Encode(release)
	})
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()

	// Launch 100 concurrent requests for the same release
	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	results := make([][]string, numGoroutines)
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			assets, err := resolver.FetchReleaseAssets(ctx, "owner/repo", "v1.0.0")
			results[index] = assets
			errors[index] = err
		}(i)
	}

	wg.Wait()

	// Verify only ONE API call was made (GetOrFetch pattern working)
	mu.Lock()
	finalCallCount := callCount
	mu.Unlock()

	if finalCallCount != 1 {
		t.Errorf("expected exactly 1 API call with 100 concurrent requests, got %d", finalCallCount)
	}

	// Verify all goroutines got the same result
	for i := 0; i < numGoroutines; i++ {
		if errors[i] != nil {
			t.Errorf("goroutine %d got error: %v", i, errors[i])
		}

		if len(results[i]) != 3 {
			t.Errorf("goroutine %d got %d assets, expected 3", i, len(results[i]))
		}

		// Verify all got same assets
		for j, asset := range results[i] {
			if asset != results[0][j] {
				t.Errorf("goroutine %d got different asset at index %d: %s vs %s", i, j, asset, results[0][j])
			}
		}
	}
}

func TestFetchReleaseAssets_CacheEviction(t *testing.T) {
	resetGlobalCache()

	callCount := 0
	server := mockGitHubServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		release := mockRelease([]string{"asset.tar.gz"})
		_ = json.NewEncoder(w).Encode(release)
	})
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()

	// Fill cache to MaxCacheSize + 1
	// This should trigger eviction
	for i := 0; i < MaxCacheSize+1; i++ {
		repo := fmt.Sprintf("owner/repo-%d", i)
		_, err := resolver.FetchReleaseAssets(ctx, repo, "v1.0.0")
		if err != nil {
			t.Fatalf("unexpected error filling cache: %v", err)
		}
	}

	// Verify cache size is at or below MaxCacheSize
	globalAssetCache.mu.Lock()
	cacheSize := len(globalAssetCache.entries)
	globalAssetCache.mu.Unlock()

	if cacheSize > MaxCacheSize {
		t.Errorf("cache size %d exceeds MaxCacheSize %d (eviction didn't work)", cacheSize, MaxCacheSize)
	}

	// Cache size should be roughly MaxCacheSize/2 after eviction
	expectedSize := MaxCacheSize / 2
	if cacheSize < expectedSize-10 || cacheSize > expectedSize+10 {
		t.Logf("cache size %d not near expected %d after eviction (acceptable if within margin)", cacheSize, expectedSize)
	}
}

func TestFetchReleaseAssets_InvalidRepo(t *testing.T) {
	resetGlobalCache()

	resolver := &Resolver{
		client: github.NewClient(nil),
	}

	ctx := context.Background()

	testCases := []struct {
		name string
		repo string
	}{
		{"no slash", "invalid"},
		{"multiple slashes", "owner/repo/extra"},
		{"empty", ""},
		{"whitespace owner", "   /repo"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolver.FetchReleaseAssets(ctx, tc.repo, "v1.0.0")
			if err == nil {
				t.Errorf("expected error for invalid repo '%s', got nil", tc.repo)
			}

			if !strings.Contains(err.Error(), "invalid repo") {
				t.Errorf("expected 'invalid repo' in error, got: %v", err)
			}
		})
	}
}

func TestCheckRateLimit_Healthy(t *testing.T) {
	// Mock server with plenty of rate limit remaining
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/rate_limit") {
			w.Header().Set("Content-Type", "application/json")
			resetTime := time.Now().Add(1 * time.Hour).Unix()
			response := fmt.Sprintf(`{
				"resources": {
					"core": {
						"limit": 5000,
						"remaining": 4999,
						"reset": %d,
						"used": 1
					}
				}
			}`, resetTime)
			_, _ = w.Write([]byte(response))
		}
	}))
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()
	err := resolver.checkRateLimit(ctx)
	if err != nil {
		t.Errorf("expected no error with healthy rate limit, got: %v", err)
	}
}

func TestCheckRateLimit_Low(t *testing.T) {
	// Mock server with low rate limit (< 10 remaining)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/rate_limit") {
			w.Header().Set("Content-Type", "application/json")
			resetTime := time.Now().Add(1 * time.Hour).Unix()
			response := fmt.Sprintf(`{
				"resources": {
					"core": {
						"limit": 5000,
						"remaining": 5,
						"reset": %d,
						"used": 4995
					}
				}
			}`, resetTime)
			_, _ = w.Write([]byte(response))
		}
	}))
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()
	// Should return nil (just warns, doesn't error)
	err := resolver.checkRateLimit(ctx)
	if err != nil {
		t.Errorf("expected no error with low rate limit (just warning), got: %v", err)
	}
}

func TestCheckRateLimit_Exhausted(t *testing.T) {
	// Mock server with exhausted rate limit (0 remaining)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/rate_limit") {
			w.Header().Set("Content-Type", "application/json")
			resetTime := time.Now().Add(1 * time.Hour).Unix()
			response := fmt.Sprintf(`{
				"resources": {
					"core": {
						"limit": 5000,
						"remaining": 0,
						"reset": %d,
						"used": 5000
					}
				}
			}`, resetTime)
			_, _ = w.Write([]byte(response))
		}
	}))
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()
	err := resolver.checkRateLimit(ctx)
	if err == nil {
		t.Error("expected error with exhausted rate limit, got nil")
	}

	if !strings.Contains(err.Error(), "exhausted") {
		t.Errorf("expected 'exhausted' in error message, got: %v", err)
	}
}

func TestCheckRateLimit_403Error(t *testing.T) {
	// Mock server that returns 403 for rate limit check
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/rate_limit") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message": "API rate limit exceeded"}`))
		}
	}))
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()
	err := resolver.checkRateLimit(ctx)
	if err == nil {
		t.Error("expected error when rate limit check returns 403, got nil")
	}

	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected '403' in error message, got: %v", err)
	}
}

func TestCheckRateLimit_NetworkError(t *testing.T) {
	// Mock server that returns 500 for rate limit check (network/server error)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/rate_limit") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message": "Internal server error"}`))
		}
	}))
	defer server.Close()

	resolver := &Resolver{
		client: github.NewClient(nil).WithAuthToken("fake-token"),
	}
	resolver.client, _ = github.NewClient(nil).WithEnterpriseURLs(server.URL, server.URL)

	ctx := context.Background()
	// Should return nil (proceed with request despite rate limit check failure)
	err := resolver.checkRateLimit(ctx)
	if err != nil {
		t.Errorf("expected no error on non-403 rate limit check failure (should proceed), got: %v", err)
	}
}
