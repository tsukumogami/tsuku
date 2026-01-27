package registry

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	result, info, err := cached.GetRecipe(context.Background(), "test-tool")
	if err != nil {
		t.Fatalf("GetRecipe failed: %v", err)
	}

	if string(result) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", result, content)
	}

	// CacheInfo should indicate fresh cache
	if info == nil {
		t.Fatal("expected CacheInfo, got nil")
	}
	if info.IsStale {
		t.Error("expected IsStale=false for fresh cache hit")
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
	result, info, err := cached.GetRecipe(context.Background(), "test-tool")
	if err != nil {
		t.Fatalf("GetRecipe failed: %v", err)
	}

	if string(result) != string(networkContent) {
		t.Errorf("content mismatch: got %q, want %q", result, networkContent)
	}

	// CacheInfo should indicate fresh content
	if info == nil {
		t.Fatal("expected CacheInfo, got nil")
	}
	if info.IsStale {
		t.Error("expected IsStale=false after successful refresh")
	}

	// Verify cache was updated
	newCached, _ := reg.GetCached("test-tool")
	if string(newCached) != string(networkContent) {
		t.Errorf("cache should be updated with network content")
	}
}

func TestCachedRegistry_StaleFallbackWithinMaxStale(t *testing.T) {
	cacheDir := t.TempDir()

	// Server returns 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	// Pre-populate cache with content cached 2 hours ago (within 7-day max stale)
	oldContent := []byte("[metadata]\nname = \"test-tool\"\n")
	recipePath := filepath.Join(cacheDir, "t", "test-tool.toml")
	if err := os.MkdirAll(filepath.Dir(recipePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recipePath, oldContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Create expired metadata (cached 2 hours ago, TTL 1 hour, but within 7-day max stale)
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

	// Create cached registry with 1 hour TTL, default 7-day max stale
	cached := NewCachedRegistry(reg, 1*time.Hour)

	// Capture stderr to verify warning
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// GetRecipe should return stale content with warning
	result, info, err := cached.GetRecipe(context.Background(), "test-tool")

	// Restore stderr
	w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)
	stderrOutput := stderrBuf.String()

	if err != nil {
		t.Fatalf("GetRecipe should succeed with stale fallback, got error: %v", err)
	}

	if string(result) != string(oldContent) {
		t.Errorf("content mismatch: got %q, want %q", result, oldContent)
	}

	// CacheInfo should indicate stale content
	if info == nil {
		t.Fatal("expected CacheInfo, got nil")
	}
	if !info.IsStale {
		t.Error("expected IsStale=true for stale fallback")
	}

	// Verify warning was printed
	if !strings.Contains(stderrOutput, "Warning: Using cached recipe 'test-tool'") {
		t.Errorf("expected warning message, got: %q", stderrOutput)
	}
}

func TestCachedRegistry_StaleFallbackExceedsMaxStale(t *testing.T) {
	cacheDir := t.TempDir()

	// Server returns 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	// Pre-populate cache with very old content (10 days ago, exceeds 7-day max stale)
	oldContent := []byte("[metadata]\nname = \"test-tool\"\n")
	recipePath := filepath.Join(cacheDir, "t", "test-tool.toml")
	if err := os.MkdirAll(filepath.Dir(recipePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recipePath, oldContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Create expired metadata (cached 10 days ago, exceeds 7-day max stale)
	meta := &CacheMetadata{
		CachedAt:    time.Now().Add(-10 * 24 * time.Hour),
		ExpiresAt:   time.Now().Add(-10*24*time.Hour + 1*time.Hour),
		LastAccess:  time.Now().Add(-10 * 24 * time.Hour),
		Size:        int64(len(oldContent)),
		ContentHash: computeContentHash(oldContent),
	}
	if err := reg.WriteMeta("test-tool", meta); err != nil {
		t.Fatalf("WriteMeta failed: %v", err)
	}

	// Create cached registry with default max stale (7 days)
	cached := NewCachedRegistry(reg, 1*time.Hour)

	// GetRecipe should fail with ErrTypeCacheTooStale
	_, _, err := cached.GetRecipe(context.Background(), "test-tool")
	if err == nil {
		t.Fatal("expected error for cache exceeding max stale")
	}

	var regErr *RegistryError
	if !errors.As(err, &regErr) {
		t.Fatalf("expected *RegistryError, got %T", err)
	}
	if regErr.Type != ErrTypeCacheTooStale {
		t.Errorf("expected ErrTypeCacheTooStale, got %v", regErr.Type)
	}
}

func TestCachedRegistry_StaleFallbackDisabled(t *testing.T) {
	cacheDir := t.TempDir()

	// Server returns 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	// Pre-populate cache with content (within max stale normally)
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

	// Create cached registry with stale fallback disabled
	cached := NewCachedRegistry(reg, 1*time.Hour)
	cached.SetStaleFallback(false)

	// GetRecipe should fail - stale fallback is disabled
	_, _, err := cached.GetRecipe(context.Background(), "test-tool")
	if err == nil {
		t.Error("expected error when stale fallback is disabled")
	}
}

func TestCachedRegistry_StaleFallbackDisabledViaMaxStaleZero(t *testing.T) {
	cacheDir := t.TempDir()

	// Server returns 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	// Pre-populate cache with content
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

	// Create cached registry with maxStale=0 (disables stale fallback)
	cached := NewCachedRegistry(reg, 1*time.Hour)
	cached.SetMaxStale(0)

	// GetRecipe should fail - max stale is 0 means disabled
	_, _, err := cached.GetRecipe(context.Background(), "test-tool")
	if err == nil {
		t.Error("expected error when max stale is 0")
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
	result, info, err := cached.GetRecipe(context.Background(), "new-tool")
	if err != nil {
		t.Fatalf("GetRecipe failed: %v", err)
	}

	if string(result) != string(networkContent) {
		t.Errorf("content mismatch: got %q, want %q", result, networkContent)
	}

	// CacheInfo should indicate fresh content
	if info == nil {
		t.Fatal("expected CacheInfo, got nil")
	}
	if info.IsStale {
		t.Error("expected IsStale=false for fresh fetch")
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
	_, _, err := cached.GetRecipe(context.Background(), "nonexistent")
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
	_, _, err := cached.GetRecipe(context.Background(), "test")
	if err != nil {
		t.Fatalf("first GetRecipe failed: %v", err)
	}
	if fetchCount != 1 {
		t.Errorf("expected 1 fetch, got %d", fetchCount)
	}

	// Immediate second call - should use cache
	_, _, err = cached.GetRecipe(context.Background(), "test")
	if err != nil {
		t.Fatalf("second GetRecipe failed: %v", err)
	}
	if fetchCount != 1 {
		t.Errorf("expected 1 fetch (cached), got %d", fetchCount)
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Third call - should refresh
	_, _, err = cached.GetRecipe(context.Background(), "test")
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
	_, _, err := cached.GetRecipe(context.Background(), "tool1")
	if err != nil {
		t.Fatalf("First GetRecipe failed: %v", err)
	}

	// Second fetch - will push cache above high water mark, triggering eviction
	_, _, err = cached.GetRecipe(context.Background(), "tool2")
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
	result, _, err := cached.GetRecipe(context.Background(), "test")
	if err != nil {
		t.Fatalf("GetRecipe failed: %v", err)
	}

	if string(result) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", result, content)
	}
}

func TestCachedRegistry_SetMaxStale(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	cached := NewCachedRegistry(reg, 1*time.Hour)

	// Test SetMaxStale
	cached.SetMaxStale(24 * time.Hour)

	// Verify it was set by testing behavior - we'd need to expose the field or test indirectly
	// For now, just verify the method doesn't panic
}

func TestCachedRegistry_SetStaleFallback(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	cached := NewCachedRegistry(reg, 1*time.Hour)

	// Test SetStaleFallback
	cached.SetStaleFallback(false)

	// Verify it was set by testing behavior - we'd need to expose the field or test indirectly
	// For now, just verify the method doesn't panic
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Minute, "30 minutes"},
		{1 * time.Hour, "1 hour"},
		{2 * time.Hour, "2 hours"},
		{23 * time.Hour, "23 hours"},
		{24 * time.Hour, "1 day"},
		{48 * time.Hour, "2 days"},
		{7 * 24 * time.Hour, "7 days"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestCachedRegistry_Refresh(t *testing.T) {
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
	if err := reg.CacheRecipe("test-tool", oldContent); err != nil {
		t.Fatalf("CacheRecipe failed: %v", err)
	}

	// Update metadata to show it was cached 2 hours ago
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

	cached := NewCachedRegistry(reg, 1*time.Hour)

	// Refresh should force fetch from network
	detail, err := cached.Refresh(context.Background(), "test-tool")
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if detail.Status != "refreshed" {
		t.Errorf("expected status 'refreshed', got %q", detail.Status)
	}
	if detail.Name != "test-tool" {
		t.Errorf("expected name 'test-tool', got %q", detail.Name)
	}
	// Age should be approximately 2 hours
	if detail.Age < 1*time.Hour || detail.Age > 3*time.Hour {
		t.Errorf("expected age around 2 hours, got %v", detail.Age)
	}

	// Verify cache was updated
	newCached, _ := reg.GetCached("test-tool")
	if string(newCached) != string(networkContent) {
		t.Errorf("cache should be updated with network content")
	}
}

func TestCachedRegistry_RefreshNotCached(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	cached := NewCachedRegistry(reg, 1*time.Hour)

	// Refresh should fail for non-cached recipe
	detail, err := cached.Refresh(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for non-cached recipe")
	}

	if detail == nil {
		t.Fatal("expected detail even on error")
	}
	if detail.Status != "error" {
		t.Errorf("expected status 'error', got %q", detail.Status)
	}
}

func TestCachedRegistry_RefreshAll(t *testing.T) {
	cacheDir := t.TempDir()
	networkContent := []byte("[metadata]\nname = \"tool\"\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(networkContent)
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	// Create two cached recipes - one fresh, one expired
	freshContent := []byte("[metadata]\nname = \"fresh-tool\"\n")
	expiredContent := []byte("[metadata]\nname = \"expired-tool\"\n")

	if err := reg.CacheRecipe("fresh-tool", freshContent); err != nil {
		t.Fatalf("CacheRecipe failed: %v", err)
	}
	if err := reg.CacheRecipe("expired-tool", expiredContent); err != nil {
		t.Fatalf("CacheRecipe failed: %v", err)
	}

	// Mark expired-tool as expired
	expiredMeta := &CacheMetadata{
		CachedAt:    time.Now().Add(-2 * time.Hour),
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
		LastAccess:  time.Now().Add(-2 * time.Hour),
		Size:        int64(len(expiredContent)),
		ContentHash: computeContentHash(expiredContent),
	}
	if err := reg.WriteMeta("expired-tool", expiredMeta); err != nil {
		t.Fatalf("WriteMeta failed: %v", err)
	}

	cached := NewCachedRegistry(reg, 1*time.Hour)

	// RefreshAll should refresh only expired recipes
	stats, err := cached.RefreshAll(context.Background())
	if err != nil {
		t.Fatalf("RefreshAll failed: %v", err)
	}

	if stats.Total != 2 {
		t.Errorf("expected total=2, got %d", stats.Total)
	}
	if stats.Refreshed != 1 {
		t.Errorf("expected refreshed=1, got %d", stats.Refreshed)
	}
	if stats.Fresh != 1 {
		t.Errorf("expected fresh=1, got %d", stats.Fresh)
	}
	if stats.Errors != 0 {
		t.Errorf("expected errors=0, got %d", stats.Errors)
	}
	if len(stats.Details) != 2 {
		t.Errorf("expected 2 details, got %d", len(stats.Details))
	}
}

func TestCachedRegistry_RefreshAll_PartialFailure(t *testing.T) {
	cacheDir := t.TempDir()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// First request succeeds, second fails
		if requestCount == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("[metadata]\nname = \"tool\"\n"))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	reg := New(cacheDir)
	reg.BaseURL = server.URL

	// Create two expired recipes
	for _, name := range []string{"tool1", "tool2"} {
		content := []byte("[metadata]\nname = \"" + name + "\"\n")
		if err := reg.CacheRecipe(name, content); err != nil {
			t.Fatalf("CacheRecipe failed: %v", err)
		}
		meta := &CacheMetadata{
			CachedAt:    time.Now().Add(-2 * time.Hour),
			ExpiresAt:   time.Now().Add(-1 * time.Hour),
			LastAccess:  time.Now().Add(-2 * time.Hour),
			Size:        int64(len(content)),
			ContentHash: computeContentHash(content),
		}
		if err := reg.WriteMeta(name, meta); err != nil {
			t.Fatalf("WriteMeta failed: %v", err)
		}
	}

	cached := NewCachedRegistry(reg, 1*time.Hour)

	// RefreshAll should continue on individual errors
	stats, err := cached.RefreshAll(context.Background())
	if err != nil {
		t.Fatalf("RefreshAll should not fail on partial errors: %v", err)
	}

	if stats.Total != 2 {
		t.Errorf("expected total=2, got %d", stats.Total)
	}
	if stats.Refreshed != 1 {
		t.Errorf("expected refreshed=1, got %d", stats.Refreshed)
	}
	if stats.Errors != 1 {
		t.Errorf("expected errors=1, got %d", stats.Errors)
	}
}

func TestCachedRegistry_GetCacheStatus(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	// Cache a fresh recipe
	freshContent := []byte("[metadata]\nname = \"fresh-tool\"\n")
	if err := reg.CacheRecipe("fresh-tool", freshContent); err != nil {
		t.Fatalf("CacheRecipe failed: %v", err)
	}

	// Cache an expired recipe
	expiredContent := []byte("[metadata]\nname = \"expired-tool\"\n")
	if err := reg.CacheRecipe("expired-tool", expiredContent); err != nil {
		t.Fatalf("CacheRecipe failed: %v", err)
	}
	expiredMeta := &CacheMetadata{
		CachedAt:    time.Now().Add(-2 * time.Hour),
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
		LastAccess:  time.Now().Add(-2 * time.Hour),
		Size:        int64(len(expiredContent)),
		ContentHash: computeContentHash(expiredContent),
	}
	if err := reg.WriteMeta("expired-tool", expiredMeta); err != nil {
		t.Fatalf("WriteMeta failed: %v", err)
	}

	cached := NewCachedRegistry(reg, 1*time.Hour)

	// Check fresh recipe status
	freshStatus, err := cached.GetCacheStatus("fresh-tool")
	if err != nil {
		t.Fatalf("GetCacheStatus failed for fresh-tool: %v", err)
	}
	if freshStatus.Status != "already fresh" {
		t.Errorf("expected 'already fresh', got %q", freshStatus.Status)
	}

	// Check expired recipe status
	expiredStatus, err := cached.GetCacheStatus("expired-tool")
	if err != nil {
		t.Fatalf("GetCacheStatus failed for expired-tool: %v", err)
	}
	if expiredStatus.Status != "expired" {
		t.Errorf("expected 'expired', got %q", expiredStatus.Status)
	}
	// Age should be around 2 hours
	if expiredStatus.Age < 1*time.Hour || expiredStatus.Age > 3*time.Hour {
		t.Errorf("expected age around 2 hours, got %v", expiredStatus.Age)
	}

	// Check non-cached recipe
	notCachedStatus, err := cached.GetCacheStatus("not-cached")
	if err != nil {
		t.Fatalf("GetCacheStatus failed for not-cached: %v", err)
	}
	if notCachedStatus != nil {
		t.Error("expected nil for non-cached recipe")
	}
}
