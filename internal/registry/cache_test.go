package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheMetadata_WriteMeta(t *testing.T) {
	cacheDir := t.TempDir()
	r := NewRegistry(cacheDir, "", nil, nil)

	meta := &CacheMetadata{
		CachedAt:    time.Now().Truncate(time.Second),
		ExpiresAt:   time.Now().Add(24 * time.Hour).Truncate(time.Second),
		LastAccess:  time.Now().Truncate(time.Second),
		Size:        1234,
		ContentHash: "abc123",
	}

	if err := r.WriteMeta("fzf", meta); err != nil {
		t.Fatalf("WriteMeta failed: %v", err)
	}

	// Verify file was created
	path := filepath.Join(cacheDir, "f", "fzf.meta.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("metadata file was not created")
	}

	// Verify content is valid JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read metadata file: %v", err)
	}

	var readMeta CacheMetadata
	if err := json.Unmarshal(data, &readMeta); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	if readMeta.Size != meta.Size {
		t.Errorf("Size mismatch: got %d, want %d", readMeta.Size, meta.Size)
	}
	if readMeta.ContentHash != meta.ContentHash {
		t.Errorf("ContentHash mismatch: got %s, want %s", readMeta.ContentHash, meta.ContentHash)
	}
}

func TestCacheMetadata_ReadMeta(t *testing.T) {
	cacheDir := t.TempDir()
	r := NewRegistry(cacheDir, "", nil, nil)

	// Test reading non-existent metadata
	meta, err := r.ReadMeta("nonexistent")
	if err != nil {
		t.Fatalf("ReadMeta should not error for non-existent file: %v", err)
	}
	if meta != nil {
		t.Error("expected nil metadata for non-existent file")
	}

	// Write and read back metadata
	originalMeta := &CacheMetadata{
		CachedAt:    time.Now().Truncate(time.Second),
		ExpiresAt:   time.Now().Add(24 * time.Hour).Truncate(time.Second),
		LastAccess:  time.Now().Truncate(time.Second),
		Size:        5678,
		ContentHash: "def456",
	}

	if err := r.WriteMeta("ripgrep", originalMeta); err != nil {
		t.Fatalf("WriteMeta failed: %v", err)
	}

	readMeta, err := r.ReadMeta("ripgrep")
	if err != nil {
		t.Fatalf("ReadMeta failed: %v", err)
	}
	if readMeta == nil {
		t.Fatal("expected metadata, got nil")
	}

	if readMeta.Size != originalMeta.Size {
		t.Errorf("Size mismatch: got %d, want %d", readMeta.Size, originalMeta.Size)
	}
	if readMeta.ContentHash != originalMeta.ContentHash {
		t.Errorf("ContentHash mismatch: got %s, want %s", readMeta.ContentHash, originalMeta.ContentHash)
	}
}

func TestCacheMetadata_ReadMeta_InvalidJSON(t *testing.T) {
	cacheDir := t.TempDir()
	r := NewRegistry(cacheDir, "", nil, nil)

	// Create invalid JSON metadata file
	metaDir := filepath.Join(cacheDir, "b")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "bad.meta.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	meta, err := r.ReadMeta("bad")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
	if meta != nil {
		t.Error("expected nil metadata for invalid JSON")
	}
}

func TestCacheRecipe_WritesMetadata(t *testing.T) {
	cacheDir := t.TempDir()
	r := NewRegistry(cacheDir, "", nil, nil)

	content := []byte(`[metadata]
name = "test-tool"
`)

	if err := r.CacheRecipe("test-tool", content); err != nil {
		t.Fatalf("CacheRecipe failed: %v", err)
	}

	// Verify recipe file exists
	recipePath := filepath.Join(cacheDir, "t", "test-tool.toml")
	if _, err := os.Stat(recipePath); os.IsNotExist(err) {
		t.Fatal("recipe file was not created")
	}

	// Verify metadata file exists
	metaPath := filepath.Join(cacheDir, "t", "test-tool.meta.json")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Fatal("metadata file was not created")
	}

	// Verify metadata content
	meta, err := r.ReadMeta("test-tool")
	if err != nil {
		t.Fatalf("ReadMeta failed: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}

	if meta.Size != int64(len(content)) {
		t.Errorf("Size mismatch: got %d, want %d", meta.Size, len(content))
	}

	expectedHash := computeContentHash(content)
	if meta.ContentHash != expectedHash {
		t.Errorf("ContentHash mismatch: got %s, want %s", meta.ContentHash, expectedHash)
	}

	// Verify CachedAt and ExpiresAt are set correctly
	if meta.CachedAt.IsZero() {
		t.Error("CachedAt should not be zero")
	}
	if meta.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should not be zero")
	}
	if meta.ExpiresAt.Sub(meta.CachedAt) != DefaultCacheTTL {
		t.Errorf("TTL mismatch: got %v, want %v", meta.ExpiresAt.Sub(meta.CachedAt), DefaultCacheTTL)
	}
}

func TestGetCached_MigratesMetadata(t *testing.T) {
	cacheDir := t.TempDir()
	r := NewRegistry(cacheDir, "", nil, nil)

	// Create a cached recipe without metadata (simulating pre-existing cache)
	content := []byte(`[metadata]
name = "old-tool"
`)
	recipeDir := filepath.Join(cacheDir, "o")
	if err := os.MkdirAll(recipeDir, 0755); err != nil {
		t.Fatal(err)
	}
	recipePath := filepath.Join(recipeDir, "old-tool.toml")
	if err := os.WriteFile(recipePath, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Verify no metadata exists yet
	metaPath := filepath.Join(recipeDir, "old-tool.meta.json")
	if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
		t.Fatal("metadata should not exist yet")
	}

	// Call GetCached - should create metadata
	readContent, err := r.GetCached("old-tool")
	if err != nil {
		t.Fatalf("GetCached failed: %v", err)
	}
	if string(readContent) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", readContent, content)
	}

	// Verify metadata was created
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Fatal("metadata should have been created during migration")
	}

	// Verify metadata content
	meta, err := r.ReadMeta("old-tool")
	if err != nil {
		t.Fatalf("ReadMeta failed: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}

	if meta.Size != int64(len(content)) {
		t.Errorf("Size mismatch: got %d, want %d", meta.Size, len(content))
	}

	expectedHash := computeContentHash(content)
	if meta.ContentHash != expectedHash {
		t.Errorf("ContentHash mismatch: got %s, want %s", meta.ContentHash, expectedHash)
	}
}

func TestGetCached_UpdatesLastAccess(t *testing.T) {
	cacheDir := t.TempDir()
	r := NewRegistry(cacheDir, "", nil, nil)

	content := []byte(`[metadata]
name = "access-test"
`)

	// Cache the recipe (this creates metadata)
	if err := r.CacheRecipe("access-test", content); err != nil {
		t.Fatalf("CacheRecipe failed: %v", err)
	}

	// Get initial metadata
	meta1, err := r.ReadMeta("access-test")
	if err != nil {
		t.Fatalf("ReadMeta failed: %v", err)
	}

	// Wait a bit and read the cached recipe
	time.Sleep(10 * time.Millisecond)

	_, err = r.GetCached("access-test")
	if err != nil {
		t.Fatalf("GetCached failed: %v", err)
	}

	// Get updated metadata
	meta2, err := r.ReadMeta("access-test")
	if err != nil {
		t.Fatalf("ReadMeta failed: %v", err)
	}

	// LastAccess should be updated
	if !meta2.LastAccess.After(meta1.LastAccess) {
		t.Errorf("LastAccess should be updated: original=%v, updated=%v", meta1.LastAccess, meta2.LastAccess)
	}

	// CachedAt should remain the same
	if !meta2.CachedAt.Equal(meta1.CachedAt) {
		t.Errorf("CachedAt should not change: original=%v, updated=%v", meta1.CachedAt, meta2.CachedAt)
	}
}

func TestComputeContentHash(t *testing.T) {
	content := []byte("hello world")
	hash := computeContentHash(content)

	// SHA256 of "hello world" is known
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if hash != expected {
		t.Errorf("hash mismatch: got %s, want %s", hash, expected)
	}

	// Same content should produce same hash
	hash2 := computeContentHash(content)
	if hash != hash2 {
		t.Error("hash should be deterministic")
	}

	// Different content should produce different hash
	hash3 := computeContentHash([]byte("different content"))
	if hash == hash3 {
		t.Error("different content should produce different hash")
	}
}

func TestMetaPath(t *testing.T) {
	cacheDir := "/test/cache"
	r := NewRegistry(cacheDir, "", nil, nil)

	tests := []struct {
		name string
		want string
	}{
		{"fzf", "/test/cache/f/fzf.meta.json"},
		{"ripgrep", "/test/cache/r/ripgrep.meta.json"},
		{"123tool", "/test/cache/_/123tool.meta.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.metaPath(tt.name)
			if got != tt.want {
				t.Errorf("metaPath(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestDeleteMeta(t *testing.T) {
	cacheDir := t.TempDir()
	r := NewRegistry(cacheDir, "", nil, nil)

	// Create metadata
	meta := &CacheMetadata{
		CachedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(24 * time.Hour),
		LastAccess:  time.Now(),
		Size:        100,
		ContentHash: "test",
	}
	if err := r.WriteMeta("delete-test", meta); err != nil {
		t.Fatalf("WriteMeta failed: %v", err)
	}

	// Verify it exists
	if _, err := r.ReadMeta("delete-test"); err != nil {
		t.Fatalf("metadata should exist: %v", err)
	}

	// Delete it
	if err := r.DeleteMeta("delete-test"); err != nil {
		t.Fatalf("DeleteMeta failed: %v", err)
	}

	// Verify it's gone
	meta, err := r.ReadMeta("delete-test")
	if err != nil {
		t.Fatalf("ReadMeta should not error for deleted file: %v", err)
	}
	if meta != nil {
		t.Error("metadata should be nil after deletion")
	}

	// Deleting non-existent should not error
	if err := r.DeleteMeta("nonexistent"); err != nil {
		t.Errorf("DeleteMeta should not error for non-existent file: %v", err)
	}
}

func TestListCachedWithMeta(t *testing.T) {
	cacheDir := t.TempDir()
	r := NewRegistry(cacheDir, "", nil, nil)

	// Cache some recipes
	if err := r.CacheRecipe("tool-a", []byte("content a")); err != nil {
		t.Fatal(err)
	}
	if err := r.CacheRecipe("tool-b", []byte("content b")); err != nil {
		t.Fatal(err)
	}

	// Create a recipe without metadata (simulating old cache)
	oldDir := filepath.Join(cacheDir, "o")
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "old-recipe.toml"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	// List cached with metadata
	result, err := r.ListCachedWithMeta()
	if err != nil {
		t.Fatalf("ListCachedWithMeta failed: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("expected 3 recipes, got %d", len(result))
	}

	// tool-a and tool-b should have metadata
	if result["tool-a"] == nil {
		t.Error("tool-a should have metadata")
	}
	if result["tool-b"] == nil {
		t.Error("tool-b should have metadata")
	}

	// old-recipe should not have metadata (nil value in map)
	if meta, exists := result["old-recipe"]; !exists {
		t.Error("old-recipe should be in the result")
	} else if meta != nil {
		t.Error("old-recipe should not have metadata")
	}
}
