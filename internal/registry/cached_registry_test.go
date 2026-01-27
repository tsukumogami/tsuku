package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCachedRegistry_FreshCacheHit(t *testing.T) {
	cacheDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Network should not be called for fresh cache hit")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("network content"))
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	// Pre-populate cache with fresh content
	content := []byte("[metadata]\nname = \"test-tool\"\n")
	if err := reg.CacheRecipe("test-tool", content); err != nil {
		t.Fatalf("CacheRecipe failed: %v", err)
	}

	// Create cached registry with 1 hour TTL
	cached := NewCachedRegistry(reg, 1*time.Hour)

	// GetRecipe should return cached content without network call
	result, err := cached.GetRecipe(context.Background(), "test-tool")
	if err != nil {
		t.Fatalf("GetRecipe failed: %v", err)
	}

	if string(result) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", result, content)
	}
}

func TestCachedRegistry_ExpiredCacheRefresh(t *testing.T) {
	cacheDir := t.TempDir()
	networkContent := []byte("[metadata]\nname = \"test-tool\"\nversion = \"2.0\"\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(networkContent)
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	// Pre-populate cache with old content
	oldContent := []byte("[metadata]\nname = \"test-tool\"\nversion = \"1.0\"\n")
	recipePath := filepath.Join(cacheDir, "t", "test-tool.toml")
	if err := os.MkdirAll(filepath.Dir(recipePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recipePath, oldContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Create metadata that's already expired (cached 2 hours ago, TTL was 1 hour)
	meta := &CacheMetadata{
		CachedAt:    time.Now().Add(-2 * time.Hour),
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
		LastAccess:  time.Now().Add(-2 * time.Hour),
		Size:        int64(len(oldContent)),
		ContentHash: computeContentHash(oldContent),
	}
	if err := reg.WriteMeta("test-tool", meta); err != nil {
		t.Fatalf("WriteMeta failed: %v", err)
	}

	// Create cached registry with 1 hour TTL
	cached := NewCachedRegistry(reg, 1*time.Hour)

	// GetRecipe should refresh from network since cache is expired
	result, err := cached.GetRecipe(context.Background(), "test-tool")
	if err != nil {
		t.Fatalf("GetRecipe failed: %v", err)
	}

	if string(result) != string(networkContent) {
		t.Errorf("content mismatch: got %q, want %q", result, networkContent)
	}

	// Verify cache was updated
	newCached, _ := reg.GetCached("test-tool")
	if string(newCached) != string(networkContent) {
		t.Errorf("cache should be updated with network content")
	}
}

func TestCachedRegistry_ExpiredCacheNetworkFailure(t *testing.T) {
	cacheDir := t.TempDir()

	// Server returns 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	// Pre-populate cache with old content
	oldContent := []byte("[metadata]\nname = \"test-tool\"\n")
	recipePath := filepath.Join(cacheDir, "t", "test-tool.toml")
	if err := os.MkdirAll(filepath.Dir(recipePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recipePath, oldContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Create expired metadata
	meta := &CacheMetadata{
		CachedAt:    time.Now().Add(-2 * time.Hour),
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
		LastAccess:  time.Now().Add(-2 * time.Hour),
		Size:        int64(len(oldContent)),
		ContentHash: computeContentHash(oldContent),
	}
	if err := reg.WriteMeta("test-tool", meta); err != nil {
		t.Fatalf("WriteMeta failed: %v", err)
	}

	// Create cached registry with 1 hour TTL
	cached := NewCachedRegistry(reg, 1*time.Hour)

	// GetRecipe should fail - no stale fallback in this issue
	_, err := cached.GetRecipe(context.Background(), "test-tool")
	if err == nil {
		t.Error("expected error for expired cache with network failure")
	}
}

func TestCachedRegistry_CacheMissNetworkSuccess(t *testing.T) {
	cacheDir := t.TempDir()
	networkContent := []byte("[metadata]\nname = \"new-tool\"\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(networkContent)
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	cached := NewCachedRegistry(reg, 1*time.Hour)

	// GetRecipe should fetch from network
	result, err := cached.GetRecipe(context.Background(), "new-tool")
	if err != nil {
		t.Fatalf("GetRecipe failed: %v", err)
	}

	if string(result) != string(networkContent) {
		t.Errorf("content mismatch: got %q, want %q", result, networkContent)
	}

	// Verify content was cached
	cachedContent, _ := reg.GetCached("new-tool")
	if string(cachedContent) != string(networkContent) {
		t.Error("content should be cached after fetch")
	}

	// Verify metadata was written with correct TTL
	meta, _ := reg.ReadMeta("new-tool")
	if meta == nil {
		t.Fatal("metadata should exist")
		return
	}
	expectedExpiry := meta.CachedAt.Add(1 * time.Hour)
	if !meta.ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("ExpiresAt mismatch: got %v, want %v", meta.ExpiresAt, expectedExpiry)
	}
}

func TestCachedRegistry_CacheMissNetworkFailure(t *testing.T) {
	cacheDir := t.TempDir()

	// Server returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	cached := NewCachedRegistry(reg, 1*time.Hour)

	// GetRecipe should fail
	_, err := cached.GetRecipe(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for cache miss with network failure")
	}

	// Verify it's the right error type
	regErr, ok := err.(*RegistryError)
	if !ok {
		t.Errorf("expected *RegistryError, got %T", err)
	} else if regErr.Type != ErrTypeNotFound {
		t.Errorf("expected ErrTypeNotFound, got %v", regErr.Type)
	}
}

func TestCachedRegistry_TTLRespected(t *testing.T) {
	cacheDir := t.TempDir()
	fetchCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[metadata]\nname = \"test\"\n"))
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	// Use very short TTL for testing
	cached := NewCachedRegistry(reg, 100*time.Millisecond)

	// First call - should fetch
	_, err := cached.GetRecipe(context.Background(), "test")
	if err != nil {
		t.Fatalf("first GetRecipe failed: %v", err)
	}
	if fetchCount != 1 {
		t.Errorf("expected 1 fetch, got %d", fetchCount)
	}

	// Immediate second call - should use cache
	_, err = cached.GetRecipe(context.Background(), "test")
	if err != nil {
		t.Fatalf("second GetRecipe failed: %v", err)
	}
	if fetchCount != 1 {
		t.Errorf("expected 1 fetch (cached), got %d", fetchCount)
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Third call - should refresh
	_, err = cached.GetRecipe(context.Background(), "test")
	if err != nil {
		t.Fatalf("third GetRecipe failed: %v", err)
	}
	if fetchCount != 2 {
		t.Errorf("expected 2 fetches after TTL expiry, got %d", fetchCount)
	}
}

func TestCachedRegistry_Registry(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)
	cached := NewCachedRegistry(reg, 1*time.Hour)

	// Registry() should return the underlying registry
	if cached.Registry() != reg {
		t.Error("Registry() should return underlying registry")
	}
}

func TestCachedRegistry_WithCacheManager(t *testing.T) {
	cacheDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 1KB of content
		content := make([]byte, 1024)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	// Set up CacheManager with small limit to trigger eviction
	// 2KB limit, 80% high water = 1638 bytes
	cm := NewCacheManager(cacheDir, 2048)

	cached := NewCachedRegistry(reg, 1*time.Hour)
	cached.SetCacheManager(cm)

	// Verify CacheManager is set
	if cached.CacheManager() != cm {
		t.Error("CacheManager() should return configured manager")
	}

	// First fetch - should cache successfully
	_, err := cached.GetRecipe(context.Background(), "tool1")
	if err != nil {
		t.Fatalf("First GetRecipe failed: %v", err)
	}

	// Second fetch - will push cache above high water mark, triggering eviction
	_, err = cached.GetRecipe(context.Background(), "tool2")
	if err != nil {
		t.Fatalf("Second GetRecipe failed: %v", err)
	}

	// Cache should now be at or below low water mark (60% = 1228 bytes)
	size, _ := cm.Size()
	lowWater := int64(2048 * 60 / 100)
	if size > lowWater {
		t.Errorf("Cache size %d should be <= low water mark %d after eviction", size, lowWater)
	}
}

func TestCachedRegistry_NoCacheManager(t *testing.T) {
	cacheDir := t.TempDir()
	content := []byte("[metadata]\nname = \"test\"\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	// CachedRegistry without CacheManager should work normally
	cached := NewCachedRegistry(reg, 1*time.Hour)

	// CacheManager should be nil
	if cached.CacheManager() != nil {
		t.Error("CacheManager() should be nil by default")
	}

	// GetRecipe should still work
	result, err := cached.GetRecipe(context.Background(), "test")
	if err != nil {
		t.Fatalf("GetRecipe failed: %v", err)
	}

	if string(result) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", result, content)
	}
}
