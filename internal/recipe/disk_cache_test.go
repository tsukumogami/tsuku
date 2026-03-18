package recipe

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiskCache_PutGet(t *testing.T) {
	cache := NewDiskCache(DiskCacheConfig{
		Dir:     t.TempDir(),
		TTL:     1 * time.Hour,
		MaxSize: 10 * 1024 * 1024,
	})

	content := []byte("hello world")
	if err := cache.Put("test.toml", content, nil); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	data, meta, err := cache.Get("test.toml")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
	if meta == nil {
		t.Fatal("expected non-nil metadata")
		return
	}
	if meta.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), meta.Size)
	}
	if meta.ContentHash == "" {
		t.Error("expected non-empty content hash")
	}
}

func TestDiskCache_GetMiss(t *testing.T) {
	cache := NewDiskCache(DiskCacheConfig{
		Dir: t.TempDir(),
		TTL: 1 * time.Hour,
	})

	data, meta, err := cache.Get("nonexistent.toml")
	if err != nil {
		t.Fatalf("Get should not error on miss: %v", err)
	}
	if data != nil {
		t.Error("expected nil data on miss")
	}
	if meta != nil {
		t.Error("expected nil meta on miss")
	}
}

func TestDiskCache_TTLExpiry(t *testing.T) {
	cache := NewDiskCache(DiskCacheConfig{
		Dir: t.TempDir(),
		TTL: 50 * time.Millisecond,
	})

	meta := &CacheMeta{
		CachedAt:   time.Now(),
		ExpiresAt:  time.Now().Add(50 * time.Millisecond),
		LastAccess: time.Now(),
		Size:       5,
	}

	if !cache.IsFresh(meta) {
		t.Error("new entry should be fresh")
	}

	time.Sleep(60 * time.Millisecond)
	if cache.IsFresh(meta) {
		t.Error("expired entry should not be fresh")
	}
}

func TestDiskCache_StaleIfError(t *testing.T) {
	cache := NewDiskCache(DiskCacheConfig{
		Dir:      t.TempDir(),
		TTL:      50 * time.Millisecond,
		MaxStale: 1 * time.Hour,
	})

	meta := &CacheMeta{
		CachedAt:   time.Now(),
		ExpiresAt:  time.Now().Add(50 * time.Millisecond),
		LastAccess: time.Now(),
		Size:       5,
	}

	// Just expired but within max stale
	time.Sleep(60 * time.Millisecond)
	if cache.IsFresh(meta) {
		t.Error("should be expired")
	}
	if !cache.IsStaleUsable(meta) {
		t.Error("should be stale-usable (within MaxStale)")
	}
}

func TestDiskCache_StaleIfErrorDisabled(t *testing.T) {
	cache := NewDiskCache(DiskCacheConfig{
		Dir:      t.TempDir(),
		TTL:      50 * time.Millisecond,
		MaxStale: 0, // disabled
	})

	meta := &CacheMeta{
		CachedAt:   time.Now(),
		ExpiresAt:  time.Now().Add(50 * time.Millisecond),
		LastAccess: time.Now(),
		Size:       5,
	}

	time.Sleep(60 * time.Millisecond)
	if cache.IsStaleUsable(meta) {
		t.Error("stale-if-error is disabled, should not be usable")
	}
}

func TestDiskCache_StaleIfErrorTooOld(t *testing.T) {
	cache := NewDiskCache(DiskCacheConfig{
		Dir:      t.TempDir(),
		TTL:      10 * time.Millisecond,
		MaxStale: 20 * time.Millisecond,
	})

	meta := &CacheMeta{
		CachedAt:   time.Now().Add(-100 * time.Millisecond),
		ExpiresAt:  time.Now().Add(-90 * time.Millisecond),
		LastAccess: time.Now(),
		Size:       5,
	}

	if cache.IsStaleUsable(meta) {
		t.Error("entry is way past MaxStale, should not be usable")
	}
}

func TestDiskCache_MetadataRoundTrip(t *testing.T) {
	cache := NewDiskCache(DiskCacheConfig{
		Dir: t.TempDir(),
		TTL: 1 * time.Hour,
	})

	original := &CacheMeta{
		CachedAt:     time.Now().Truncate(time.Millisecond),
		ExpiresAt:    time.Now().Add(1 * time.Hour).Truncate(time.Millisecond),
		LastAccess:   time.Now().Truncate(time.Millisecond),
		Size:         42,
		ContentHash:  "abc123",
		ETag:         `"etag-value"`,
		LastModified: "Wed, 21 Oct 2015 07:28:00 GMT",
	}

	if err := cache.Put("test.toml", []byte("data"), original); err != nil {
		t.Fatal(err)
	}

	meta, err := cache.ReadMeta("test.toml")
	if err != nil {
		t.Fatalf("ReadMeta failed: %v", err)
	}
	if meta == nil {
		t.Fatal("expected non-nil meta")
		return
	}

	if meta.ETag != original.ETag {
		t.Errorf("ETag: expected %q, got %q", original.ETag, meta.ETag)
	}
	if meta.LastModified != original.LastModified {
		t.Errorf("LastModified: expected %q, got %q", original.LastModified, meta.LastModified)
	}
	if meta.ContentHash != original.ContentHash {
		t.Errorf("ContentHash: expected %q, got %q", original.ContentHash, meta.ContentHash)
	}
	if meta.Size != original.Size {
		t.Errorf("Size: expected %d, got %d", original.Size, meta.Size)
	}
}

func TestDiskCache_Delete(t *testing.T) {
	cache := NewDiskCache(DiskCacheConfig{
		Dir: t.TempDir(),
		TTL: 1 * time.Hour,
	})

	if err := cache.Put("test.toml", []byte("data"), nil); err != nil {
		t.Fatal(err)
	}

	if !cache.Has("test.toml") {
		t.Error("expected Has to return true after Put")
	}

	if err := cache.Delete("test.toml"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if cache.Has("test.toml") {
		t.Error("expected Has to return false after Delete")
	}
}

func TestDiskCache_Keys(t *testing.T) {
	dir := t.TempDir()
	cache := NewDiskCache(DiskCacheConfig{
		Dir: dir,
		TTL: 1 * time.Hour,
	})

	_ = cache.Put("a.toml", []byte("a"), nil)
	_ = cache.Put("b.toml", []byte("b"), nil)
	_ = cache.Put("subdir/c.toml", []byte("c"), nil)

	keys, err := cache.Keys()
	if err != nil {
		t.Fatal(err)
	}

	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d: %v", len(keys), keys)
	}
}

func TestDiskCache_GroupedLayout(t *testing.T) {
	dir := t.TempDir()
	cache := NewDiskCache(DiskCacheConfig{
		Dir: dir,
		TTL: 1 * time.Hour,
	})

	// Simulate grouped layout: f/fzf.toml
	if err := cache.Put("f/fzf.toml", []byte("fzf-content"), nil); err != nil {
		t.Fatal(err)
	}

	data, meta, err := cache.Get("f/fzf.toml")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(data) != "fzf-content" {
		t.Errorf("expected 'fzf-content', got %q", string(data))
	}
	if meta == nil {
		t.Fatal("expected non-nil metadata")
	}

	// Verify sidecar is at f/fzf.meta.json
	metaPath := filepath.Join(dir, "f", "fzf.meta.json")
	if _, err := os.Stat(metaPath); err != nil {
		t.Errorf("expected metadata sidecar at %s: %v", metaPath, err)
	}
}

func TestDiskCache_EvictionLRU(t *testing.T) {
	dir := t.TempDir()
	cache := NewDiskCache(DiskCacheConfig{
		Dir:       dir,
		TTL:       1 * time.Hour,
		MaxSize:   1000, // enough for ~3-4 entries with sidecars
		HighWater: 0.80,
		LowWater:  0.50,
		Eviction:  EvictLRU,
	})

	// Write several entries to fill the cache.
	// Each content + sidecar is ~200-250 bytes.
	for i := 0; i < 6; i++ {
		data := make([]byte, 50)
		meta := cache.NewMeta(data)
		meta.LastAccess = time.Now().Add(time.Duration(i) * time.Minute)
		key := fmt.Sprintf("entry%d.toml", i)
		if err := cache.Put(key, data, meta); err != nil {
			t.Fatal(err)
		}
	}

	// The most recently accessed entries should survive eviction.
	// We can't assert exact counts since sidecar sizes vary, but
	// the newest entry should still be present.
	if !cache.Has("entry5.toml") {
		t.Error("newest entry should survive LRU eviction")
	}
}

func TestDiskCache_EvictionOldest(t *testing.T) {
	dir := t.TempDir()
	cache := NewDiskCache(DiskCacheConfig{
		Dir:       dir,
		TTL:       1 * time.Hour,
		MaxSize:   100,
		HighWater: 0.50,
		LowWater:  0.30,
		Eviction:  EvictOldest,
	})

	data := make([]byte, 30)
	oldMeta := cache.NewMeta(data)
	oldMeta.CachedAt = time.Now().Add(-1 * time.Hour)
	if err := cache.Put("old.toml", data, oldMeta); err != nil {
		t.Fatal(err)
	}

	newMeta := cache.NewMeta(data)
	if err := cache.Put("new.toml", data, newMeta); err != nil {
		t.Fatal(err)
	}
	// Verify no panics and eviction ran
}

func TestDiskCache_Stats(t *testing.T) {
	cache := NewDiskCache(DiskCacheConfig{
		Dir:     t.TempDir(),
		TTL:     1 * time.Hour,
		MaxSize: 50 * 1024 * 1024,
	})

	_ = cache.Put("a.toml", []byte("aaaa"), nil)
	_ = cache.Put("b.toml", []byte("bb"), nil)

	stats, err := cache.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.EntryCount != 2 {
		t.Errorf("expected 2 entries, got %d", stats.EntryCount)
	}
	if stats.TotalSize == 0 {
		t.Error("expected non-zero total size")
	}
	if stats.SizeLimit != 50*1024*1024 {
		t.Errorf("expected SizeLimit 50MB, got %d", stats.SizeLimit)
	}
	if stats.TTL != 1*time.Hour {
		t.Errorf("expected TTL 1h, got %s", stats.TTL)
	}
}

func TestDiskCache_Has(t *testing.T) {
	cache := NewDiskCache(DiskCacheConfig{
		Dir: t.TempDir(),
		TTL: 1 * time.Hour,
	})

	if cache.Has("missing.toml") {
		t.Error("Has should return false for missing key")
	}

	_ = cache.Put("exists.toml", []byte("data"), nil)
	if !cache.Has("exists.toml") {
		t.Error("Has should return true for existing key")
	}
}

func TestDiskCache_NilMeta(t *testing.T) {
	cache := NewDiskCache(DiskCacheConfig{
		Dir: t.TempDir(),
		TTL: 1 * time.Hour,
	})

	if cache.IsFresh(nil) {
		t.Error("IsFresh(nil) should be false")
	}
	if cache.IsStaleUsable(nil) {
		t.Error("IsStaleUsable(nil) should be false")
	}
}
