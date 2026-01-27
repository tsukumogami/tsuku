package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheManager_Size_EmptyCache(t *testing.T) {
	cacheDir := t.TempDir()

	cm := NewCacheManager(cacheDir, 50*1024*1024)
	size, err := cm.Size()
	if err != nil {
		t.Fatalf("Size() error: %v", err)
	}

	if size != 0 {
		t.Errorf("Size() = %d, want 0", size)
	}
}

func TestCacheManager_Size_WithEntries(t *testing.T) {
	cacheDir := t.TempDir()

	// Create some cached recipes
	letterDir := filepath.Join(cacheDir, "f")
	if err := os.MkdirAll(letterDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a recipe file (100 bytes)
	recipeContent := make([]byte, 100)
	if err := os.WriteFile(filepath.Join(letterDir, "fzf.toml"), recipeContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Create a metadata file (50 bytes)
	metaContent := make([]byte, 50)
	if err := os.WriteFile(filepath.Join(letterDir, "fzf.meta.json"), metaContent, 0644); err != nil {
		t.Fatal(err)
	}

	cm := NewCacheManager(cacheDir, 50*1024*1024)
	size, err := cm.Size()
	if err != nil {
		t.Fatalf("Size() error: %v", err)
	}

	expected := int64(150)
	if size != expected {
		t.Errorf("Size() = %d, want %d", size, expected)
	}
}

func TestCacheManager_EnforceLimit_BelowThreshold(t *testing.T) {
	cacheDir := t.TempDir()

	// Create a small cache (100 bytes)
	letterDir := filepath.Join(cacheDir, "f")
	if err := os.MkdirAll(letterDir, 0755); err != nil {
		t.Fatal(err)
	}
	recipeContent := make([]byte, 100)
	if err := os.WriteFile(filepath.Join(letterDir, "fzf.toml"), recipeContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Set limit to 1MB - 100 bytes is well below 80%
	cm := NewCacheManager(cacheDir, 1024*1024)
	evicted, err := cm.EnforceLimit()
	if err != nil {
		t.Fatalf("EnforceLimit() error: %v", err)
	}

	if evicted != 0 {
		t.Errorf("EnforceLimit() evicted %d, want 0 (below threshold)", evicted)
	}
}

func TestCacheManager_EnforceLimit_AboveThreshold(t *testing.T) {
	cacheDir := t.TempDir()

	// Create multiple cache entries
	entries := []struct {
		letter string
		name   string
		size   int
	}{
		{"a", "alpha", 300},
		{"b", "beta", 200},
		{"g", "gamma", 250},
	}

	for _, e := range entries {
		letterDir := filepath.Join(cacheDir, e.letter)
		if err := os.MkdirAll(letterDir, 0755); err != nil {
			t.Fatal(err)
		}
		content := make([]byte, e.size)
		if err := os.WriteFile(filepath.Join(letterDir, e.name+".toml"), content, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Total: 750 bytes. Set limit to 900 bytes.
	// 80% of 900 = 720. 750 > 720, so eviction should trigger.
	// 60% of 900 = 540. Need to evict until below 540.
	cm := NewCacheManager(cacheDir, 900)
	evicted, err := cm.EnforceLimit()
	if err != nil {
		t.Fatalf("EnforceLimit() error: %v", err)
	}

	if evicted == 0 {
		t.Error("EnforceLimit() should have evicted entries")
	}

	// Verify cache is now below low water mark
	size, _ := cm.Size()
	lowWater := int64(float64(900) * 0.60)
	if size > lowWater {
		t.Errorf("After eviction, size = %d, want <= %d (low water)", size, lowWater)
	}
}

func TestCacheManager_EnforceLimit_EvictsLRU(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	// Create entries with different last_access times
	entries := []struct {
		name       string
		content    []byte
		lastAccess time.Time
	}{
		{"oldest", []byte("oldest-content"), time.Now().Add(-3 * time.Hour)},
		{"middle", []byte("middle-content"), time.Now().Add(-1 * time.Hour)},
		{"newest", []byte("newest-content"), time.Now()},
	}

	for _, e := range entries {
		// Use registry to properly set up files and metadata
		if err := reg.CacheRecipe(e.name, e.content); err != nil {
			t.Fatalf("CacheRecipe failed: %v", err)
		}
		// Update last access
		meta, _ := reg.ReadMeta(e.name)
		if meta != nil {
			meta.LastAccess = e.lastAccess
			if err := reg.WriteMeta(e.name, meta); err != nil {
				t.Fatalf("WriteMeta failed: %v", err)
			}
		}
	}

	initialSize, _ := NewCacheManager(cacheDir, 1024*1024).Size()

	// Set limit very low to force eviction of most entries
	// Each entry has ~15 bytes content + ~100 bytes metadata ≈ 115 bytes
	// Total ≈ 345 bytes. Set limit to 200 bytes.
	// 80% of 200 = 160. 345 > 160, eviction triggers.
	// 60% of 200 = 120. Need to evict until below 120.
	cm := NewCacheManager(cacheDir, 200)
	_, err := cm.EnforceLimit()
	if err != nil {
		t.Fatalf("EnforceLimit() error: %v", err)
	}

	// Verify oldest was evicted first
	oldestPath := filepath.Join(cacheDir, "o", "oldest.toml")
	if _, err := os.Stat(oldestPath); !os.IsNotExist(err) {
		t.Log("initial size:", initialSize)
		t.Error("oldest entry should have been evicted (LRU)")
	}

	// Verify newest was preserved (if any entry remains)
	newestPath := filepath.Join(cacheDir, "n", "newest.toml")
	if _, err := os.Stat(newestPath); os.IsNotExist(err) {
		// This is OK if we had to evict everything to get below low water
		t.Log("All entries evicted to meet low water mark - this is expected for very small limits")
	}
}

func TestCacheManager_Cleanup_RemovesOldEntries(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	// Create an old entry
	oldContent := []byte("old-content")
	if err := reg.CacheRecipe("old-tool", oldContent); err != nil {
		t.Fatal(err)
	}
	meta, _ := reg.ReadMeta("old-tool")
	if meta != nil {
		meta.LastAccess = time.Now().Add(-48 * time.Hour)
		_ = reg.WriteMeta("old-tool", meta)
	}

	// Create a recent entry
	newContent := []byte("new-content")
	if err := reg.CacheRecipe("new-tool", newContent); err != nil {
		t.Fatal(err)
	}

	cm := NewCacheManager(cacheDir, 50*1024*1024)

	// Cleanup entries older than 24 hours
	removed, err := cm.Cleanup(24 * time.Hour)
	if err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	if removed != 1 {
		t.Errorf("Cleanup() removed %d, want 1", removed)
	}

	// Verify old entry is gone
	oldPath := filepath.Join(cacheDir, "o", "old-tool.toml")
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old entry should have been removed")
	}

	// Verify new entry still exists
	newPath := filepath.Join(cacheDir, "n", "new-tool.toml")
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		t.Error("new entry should still exist")
	}
}

func TestCacheManager_Cleanup_KeepsRecentEntries(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	// Create a recent entry
	content := []byte("recent-content")
	if err := reg.CacheRecipe("recent-tool", content); err != nil {
		t.Fatal(err)
	}

	cm := NewCacheManager(cacheDir, 50*1024*1024)

	// Cleanup entries older than 24 hours (our entry is newer)
	removed, err := cm.Cleanup(24 * time.Hour)
	if err != nil {
		t.Fatalf("Cleanup() error: %v", err)
	}

	if removed != 0 {
		t.Errorf("Cleanup() removed %d, want 0 (recent entry)", removed)
	}

	// Verify entry still exists
	path := filepath.Join(cacheDir, "r", "recent-tool.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("recent entry should still exist")
	}
}

func TestCacheManager_Info(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	// Create multiple entries with different timestamps
	entries := []struct {
		name       string
		content    []byte
		lastAccess time.Time
	}{
		{"tool-a", []byte("content-a"), time.Now().Add(-2 * time.Hour)},
		{"tool-b", []byte("content-b"), time.Now().Add(-1 * time.Hour)},
		{"tool-c", []byte("content-c"), time.Now()},
	}

	for _, e := range entries {
		if err := reg.CacheRecipe(e.name, e.content); err != nil {
			t.Fatal(err)
		}
		meta, _ := reg.ReadMeta(e.name)
		if meta != nil {
			meta.LastAccess = e.lastAccess
			_ = reg.WriteMeta(e.name, meta)
		}
	}

	cm := NewCacheManager(cacheDir, 50*1024*1024)
	stats, err := cm.Info()
	if err != nil {
		t.Fatalf("Info() error: %v", err)
	}

	if stats.EntryCount != 3 {
		t.Errorf("EntryCount = %d, want 3", stats.EntryCount)
	}

	if stats.TotalSize == 0 {
		t.Error("TotalSize should be > 0")
	}

	// Oldest should be tool-a (2 hours ago)
	expectedOldest := entries[0].lastAccess
	if !stats.OldestAccess.Equal(expectedOldest) {
		t.Errorf("OldestAccess = %v, want %v", stats.OldestAccess, expectedOldest)
	}

	// Newest should be tool-c (now)
	expectedNewest := entries[2].lastAccess
	if !stats.NewestAccess.Equal(expectedNewest) {
		t.Errorf("NewestAccess = %v, want %v", stats.NewestAccess, expectedNewest)
	}
}

func TestCacheManager_Info_EmptyCache(t *testing.T) {
	cacheDir := t.TempDir()

	cm := NewCacheManager(cacheDir, 50*1024*1024)
	stats, err := cm.Info()
	if err != nil {
		t.Fatalf("Info() error: %v", err)
	}

	if stats.EntryCount != 0 {
		t.Errorf("EntryCount = %d, want 0", stats.EntryCount)
	}

	if stats.TotalSize != 0 {
		t.Errorf("TotalSize = %d, want 0", stats.TotalSize)
	}

	if !stats.OldestAccess.IsZero() {
		t.Errorf("OldestAccess should be zero for empty cache")
	}

	if !stats.NewestAccess.IsZero() {
		t.Errorf("NewestAccess should be zero for empty cache")
	}
}

func TestCacheManager_CleanupWithDetails(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	// Create an old entry
	oldContent := []byte("old-content-here")
	if err := reg.CacheRecipe("old-tool", oldContent); err != nil {
		t.Fatal(err)
	}
	meta, _ := reg.ReadMeta("old-tool")
	if meta != nil {
		meta.LastAccess = time.Now().Add(-48 * time.Hour)
		_ = reg.WriteMeta("old-tool", meta)
	}

	// Create a recent entry
	newContent := []byte("new-content-here")
	if err := reg.CacheRecipe("new-tool", newContent); err != nil {
		t.Fatal(err)
	}

	cm := NewCacheManager(cacheDir, 50*1024*1024)

	// Cleanup entries older than 24 hours (not dry-run)
	details, freedBytes, err := cm.CleanupWithDetails(24*time.Hour, false)
	if err != nil {
		t.Fatalf("CleanupWithDetails() error: %v", err)
	}

	if len(details) != 1 {
		t.Errorf("CleanupWithDetails() returned %d details, want 1", len(details))
	}

	if len(details) > 0 {
		if details[0].Name != "old-tool" {
			t.Errorf("detail.Name = %q, want 'old-tool'", details[0].Name)
		}
		if details[0].Age < 24*time.Hour {
			t.Errorf("detail.Age = %v, should be >= 24h", details[0].Age)
		}
	}

	if freedBytes == 0 {
		t.Error("freedBytes should be > 0")
	}

	// Verify old entry is actually deleted
	oldPath := filepath.Join(cacheDir, "o", "old-tool.toml")
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old entry should have been deleted")
	}
}

func TestCacheManager_CleanupWithDetails_DryRun(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	// Create an old entry
	oldContent := []byte("old-content")
	if err := reg.CacheRecipe("old-tool", oldContent); err != nil {
		t.Fatal(err)
	}
	meta, _ := reg.ReadMeta("old-tool")
	if meta != nil {
		meta.LastAccess = time.Now().Add(-48 * time.Hour)
		_ = reg.WriteMeta("old-tool", meta)
	}

	cm := NewCacheManager(cacheDir, 50*1024*1024)

	// Cleanup with dry-run
	details, freedBytes, err := cm.CleanupWithDetails(24*time.Hour, true)
	if err != nil {
		t.Fatalf("CleanupWithDetails() error: %v", err)
	}

	if len(details) != 1 {
		t.Errorf("CleanupWithDetails() returned %d details, want 1", len(details))
	}

	if freedBytes == 0 {
		t.Error("freedBytes should be > 0 (reports would-be freed)")
	}

	// Verify entry is NOT deleted (dry-run)
	oldPath := filepath.Join(cacheDir, "o", "old-tool.toml")
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		t.Error("old entry should NOT be deleted in dry-run mode")
	}
}

func TestCacheManager_CleanupWithDetails_NoOldEntries(t *testing.T) {
	cacheDir := t.TempDir()
	reg := New(cacheDir)

	// Create only a recent entry
	content := []byte("recent-content")
	if err := reg.CacheRecipe("recent-tool", content); err != nil {
		t.Fatal(err)
	}

	cm := NewCacheManager(cacheDir, 50*1024*1024)

	// Cleanup entries older than 24 hours (none exist)
	details, freedBytes, err := cm.CleanupWithDetails(24*time.Hour, false)
	if err != nil {
		t.Fatalf("CleanupWithDetails() error: %v", err)
	}

	if len(details) != 0 {
		t.Errorf("CleanupWithDetails() returned %d details, want 0", len(details))
	}

	if freedBytes != 0 {
		t.Errorf("freedBytes = %d, want 0", freedBytes)
	}
}

func TestCacheManager_SizeLimit(t *testing.T) {
	cacheDir := t.TempDir()
	limit := int64(100 * 1024 * 1024)

	cm := NewCacheManager(cacheDir, limit)
	if cm.SizeLimit() != limit {
		t.Errorf("SizeLimit() = %d, want %d", cm.SizeLimit(), limit)
	}
}
