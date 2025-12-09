package version

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mockVersionLister is a test double for VersionLister
type mockVersionLister struct {
	versions    []string
	callCount   int
	sourceDesc  string
	shouldError bool
	errorMsg    string
}

func (m *mockVersionLister) ListVersions(ctx context.Context) ([]string, error) {
	m.callCount++
	if m.shouldError {
		return nil, &mockError{msg: m.errorMsg}
	}
	return m.versions, nil
}

func (m *mockVersionLister) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	if len(m.versions) > 0 {
		return &VersionInfo{Version: m.versions[0]}, nil
	}
	return nil, &mockError{msg: "no versions"}
}

func (m *mockVersionLister) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	return &VersionInfo{Version: version}, nil
}

func (m *mockVersionLister) SourceDescription() string {
	if m.sourceDesc != "" {
		return m.sourceDesc
	}
	return "mock:test"
}

type mockError struct {
	msg string
}

func (e *mockError) Error() string { return e.msg }

func TestCachedVersionLister_CacheMiss(t *testing.T) {
	cacheDir := t.TempDir()
	mock := &mockVersionLister{
		versions: []string{"1.0.0", "0.9.0", "0.8.0"},
	}

	cached := NewCachedVersionLister(mock, cacheDir, time.Hour)

	versions, fromCache, err := cached.ListVersionsWithCacheInfo(context.Background())
	if err != nil {
		t.Fatalf("ListVersionsWithCacheInfo() error = %v", err)
	}

	if fromCache {
		t.Error("expected fromCache=false on first call")
	}

	if mock.callCount != 1 {
		t.Errorf("expected 1 call to underlying, got %d", mock.callCount)
	}

	if len(versions) != 3 {
		t.Errorf("expected 3 versions, got %d", len(versions))
	}
}

func TestCachedVersionLister_CacheHit(t *testing.T) {
	cacheDir := t.TempDir()
	mock := &mockVersionLister{
		versions: []string{"1.0.0", "0.9.0"},
	}

	cached := NewCachedVersionLister(mock, cacheDir, time.Hour)

	// First call - cache miss
	_, _, err := cached.ListVersionsWithCacheInfo(context.Background())
	if err != nil {
		t.Fatalf("First call error = %v", err)
	}

	// Second call - should be cache hit
	versions, fromCache, err := cached.ListVersionsWithCacheInfo(context.Background())
	if err != nil {
		t.Fatalf("Second call error = %v", err)
	}

	if !fromCache {
		t.Error("expected fromCache=true on second call")
	}

	if mock.callCount != 1 {
		t.Errorf("expected only 1 call to underlying (cached), got %d", mock.callCount)
	}

	if len(versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(versions))
	}
}

func TestCachedVersionLister_CacheExpired(t *testing.T) {
	cacheDir := t.TempDir()
	mock := &mockVersionLister{
		versions: []string{"1.0.0"},
	}

	// Use very short TTL
	cached := NewCachedVersionLister(mock, cacheDir, 1*time.Millisecond)

	// First call
	_, _, err := cached.ListVersionsWithCacheInfo(context.Background())
	if err != nil {
		t.Fatalf("First call error = %v", err)
	}

	// Wait for cache to expire
	time.Sleep(10 * time.Millisecond)

	// Second call should be cache miss due to expiration
	_, fromCache, err := cached.ListVersionsWithCacheInfo(context.Background())
	if err != nil {
		t.Fatalf("Second call error = %v", err)
	}

	if fromCache {
		t.Error("expected fromCache=false after expiration")
	}

	if mock.callCount != 2 {
		t.Errorf("expected 2 calls (cache expired), got %d", mock.callCount)
	}
}

func TestCachedVersionLister_Refresh(t *testing.T) {
	cacheDir := t.TempDir()
	mock := &mockVersionLister{
		versions: []string{"1.0.0"},
	}

	cached := NewCachedVersionLister(mock, cacheDir, time.Hour)

	// First call to populate cache
	_, _, err := cached.ListVersionsWithCacheInfo(context.Background())
	if err != nil {
		t.Fatalf("First call error = %v", err)
	}

	// Refresh should bypass cache
	versions, err := cached.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	if mock.callCount != 2 {
		t.Errorf("expected 2 calls (refresh bypasses cache), got %d", mock.callCount)
	}

	if len(versions) != 1 {
		t.Errorf("expected 1 version, got %d", len(versions))
	}
}

func TestCachedVersionLister_ListVersions(t *testing.T) {
	cacheDir := t.TempDir()
	mock := &mockVersionLister{
		versions: []string{"2.0.0", "1.0.0"},
	}

	cached := NewCachedVersionLister(mock, cacheDir, time.Hour)

	// ListVersions (without cache info) should work
	versions, err := cached.ListVersions(context.Background())
	if err != nil {
		t.Fatalf("ListVersions() error = %v", err)
	}

	if len(versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(versions))
	}
}

func TestCachedVersionLister_SourceDescription(t *testing.T) {
	cacheDir := t.TempDir()
	mock := &mockVersionLister{
		sourceDesc: "GitHub:test/repo",
	}

	cached := NewCachedVersionLister(mock, cacheDir, time.Hour)

	desc := cached.SourceDescription()
	if desc != "GitHub:test/repo" {
		t.Errorf("SourceDescription() = %q, want %q", desc, "GitHub:test/repo")
	}
}

func TestCachedVersionLister_DelegatesResolve(t *testing.T) {
	cacheDir := t.TempDir()
	mock := &mockVersionLister{
		versions: []string{"3.0.0", "2.0.0"},
	}

	cached := NewCachedVersionLister(mock, cacheDir, time.Hour)

	// ResolveLatest should delegate
	latest, err := cached.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest() error = %v", err)
	}
	if latest.Version != "3.0.0" {
		t.Errorf("ResolveLatest() = %q, want %q", latest.Version, "3.0.0")
	}

	// ResolveVersion should delegate
	resolved, err := cached.ResolveVersion(context.Background(), "2.0.0")
	if err != nil {
		t.Fatalf("ResolveVersion() error = %v", err)
	}
	if resolved.Version != "2.0.0" {
		t.Errorf("ResolveVersion() = %q, want %q", resolved.Version, "2.0.0")
	}
}

func TestCachedVersionLister_GetCacheInfo(t *testing.T) {
	cacheDir := t.TempDir()
	mock := &mockVersionLister{
		versions: []string{"1.0.0"},
	}

	cached := NewCachedVersionLister(mock, cacheDir, time.Hour)

	// Before any call, cache should not exist
	info := cached.GetCacheInfo()
	if info.Exists {
		t.Error("expected cache to not exist initially")
	}

	// Populate cache
	_, _, err := cached.ListVersionsWithCacheInfo(context.Background())
	if err != nil {
		t.Fatalf("ListVersionsWithCacheInfo() error = %v", err)
	}

	// Cache should now exist
	info = cached.GetCacheInfo()
	if !info.Exists {
		t.Error("expected cache to exist after call")
	}
	if info.IsExpired {
		t.Error("expected cache to not be expired")
	}
	if info.CachedAt.IsZero() {
		t.Error("expected CachedAt to be set")
	}
	if info.ExpiresAt.IsZero() {
		t.Error("expected ExpiresAt to be set")
	}
}

func TestCachedVersionLister_CacheFilePerSource(t *testing.T) {
	cacheDir := t.TempDir()

	mock1 := &mockVersionLister{
		versions:   []string{"1.0.0"},
		sourceDesc: "GitHub:owner1/repo1",
	}
	mock2 := &mockVersionLister{
		versions:   []string{"2.0.0"},
		sourceDesc: "GitHub:owner2/repo2",
	}

	cached1 := NewCachedVersionLister(mock1, cacheDir, time.Hour)
	cached2 := NewCachedVersionLister(mock2, cacheDir, time.Hour)

	// Populate both caches
	_, _, _ = cached1.ListVersionsWithCacheInfo(context.Background())
	_, _, _ = cached2.ListVersionsWithCacheInfo(context.Background())

	// Should have 2 cache files
	files, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 cache files, got %d", len(files))
	}

	// Each cache should return its own data
	v1, _, _ := cached1.ListVersionsWithCacheInfo(context.Background())
	v2, _, _ := cached2.ListVersionsWithCacheInfo(context.Background())

	if len(v1) != 1 || v1[0] != "1.0.0" {
		t.Errorf("cached1 returned wrong data: %v", v1)
	}
	if len(v2) != 1 || v2[0] != "2.0.0" {
		t.Errorf("cached2 returned wrong data: %v", v2)
	}

	// Each underlying should only be called once
	if mock1.callCount != 1 {
		t.Errorf("mock1 call count = %d, want 1", mock1.callCount)
	}
	if mock2.callCount != 1 {
		t.Errorf("mock2 call count = %d, want 1", mock2.callCount)
	}
}

func TestCachedVersionLister_HandleCorruptCache(t *testing.T) {
	cacheDir := t.TempDir()
	mock := &mockVersionLister{
		versions:   []string{"1.0.0"},
		sourceDesc: "test:corrupt",
	}

	cached := NewCachedVersionLister(mock, cacheDir, time.Hour)

	// First call to create the cache file
	_, _, _ = cached.ListVersionsWithCacheInfo(context.Background())

	files, _ := filepath.Glob(filepath.Join(cacheDir, "*.json"))
	if len(files) > 0 {
		// Corrupt the cache file
		if err := os.WriteFile(files[0], []byte("not valid json"), 0644); err != nil {
			t.Fatalf("failed to corrupt cache file: %v", err)
		}
	}

	// Should handle corrupt cache gracefully
	versions, fromCache, err := cached.ListVersionsWithCacheInfo(context.Background())
	if err != nil {
		t.Fatalf("ListVersionsWithCacheInfo() should not error on corrupt cache: %v", err)
	}
	if fromCache {
		t.Error("expected cache miss on corrupt cache")
	}
	if len(versions) != 1 {
		t.Errorf("expected 1 version, got %d", len(versions))
	}
}

func TestCachedVersionLister_UnderlyingError(t *testing.T) {
	cacheDir := t.TempDir()
	mock := &mockVersionLister{
		shouldError: true,
		errorMsg:    "network error",
	}

	cached := NewCachedVersionLister(mock, cacheDir, time.Hour)

	_, _, err := cached.ListVersionsWithCacheInfo(context.Background())
	if err == nil {
		t.Error("expected error to propagate")
	}
}

func TestCache_InfoEmpty(t *testing.T) {
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir)

	info, err := cache.Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.EntryCount != 0 {
		t.Errorf("expected 0 entries, got %d", info.EntryCount)
	}
	if info.TotalSize != 0 {
		t.Errorf("expected 0 size, got %d", info.TotalSize)
	}
}

func TestCache_InfoNonexistent(t *testing.T) {
	cache := NewCache("/nonexistent/path/cache")

	info, err := cache.Info()
	if err != nil {
		t.Fatalf("Info() on nonexistent dir error = %v", err)
	}
	if info.EntryCount != 0 {
		t.Errorf("expected 0 entries, got %d", info.EntryCount)
	}
}

func TestCache_InfoWithEntries(t *testing.T) {
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir)

	// Create some cache entries
	content := []byte(`{"versions": ["v1.0.0"], "cached_at": "2024-01-01T00:00:00Z"}`)
	if err := os.WriteFile(filepath.Join(cacheDir, "test1.json"), content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "test2.json"), content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	// Create a non-JSON file (should be ignored)
	if err := os.WriteFile(filepath.Join(cacheDir, "test.txt"), []byte("not json"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	info, err := cache.Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.EntryCount != 2 {
		t.Errorf("expected 2 entries, got %d", info.EntryCount)
	}
	if info.TotalSize != int64(len(content)*2) {
		t.Errorf("expected size %d, got %d", len(content)*2, info.TotalSize)
	}
}

func TestCache_Clear(t *testing.T) {
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir)

	// Create some cache entries
	content := []byte(`{"versions": ["v1.0.0"]}`)
	if err := os.WriteFile(filepath.Join(cacheDir, "test1.json"), content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "test2.json"), content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Verify entries exist
	info, _ := cache.Info()
	if info.EntryCount != 2 {
		t.Fatalf("expected 2 entries before clear, got %d", info.EntryCount)
	}

	// Clear cache
	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	// Verify cleared
	info, _ = cache.Info()
	if info.EntryCount != 0 {
		t.Errorf("expected 0 entries after clear, got %d", info.EntryCount)
	}
}

func TestCache_ClearEmpty(t *testing.T) {
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir)

	// Clear on empty cache should not error
	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear() on empty cache error = %v", err)
	}
}

func TestCache_ClearNonexistent(t *testing.T) {
	cache := NewCache("/nonexistent/path/cache")

	// Clear on nonexistent directory should not error
	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear() on nonexistent dir error = %v", err)
	}
}
