package distributed

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheManager_SourceMeta_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cm := NewCacheManager(dir, 1*time.Hour)

	meta := &SourceMeta{
		Branch: "main",
		Files: map[string]string{
			"foo": "https://raw.githubusercontent.com/owner/repo/main/.tsuku-recipes/foo.toml",
		},
		FetchedAt: time.Now().Truncate(time.Second),
	}

	if err := cm.PutSourceMeta("owner", "repo", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	got, err := cm.GetSourceMeta("owner", "repo")
	if err != nil {
		t.Fatalf("GetSourceMeta: %v", err)
	}
	if got == nil {
		t.Fatal("GetSourceMeta returned nil")
	}
	if got.Branch != "main" {
		t.Errorf("branch = %q, want %q", got.Branch, "main")
	}
	if len(got.Files) != 1 {
		t.Errorf("files count = %d, want 1", len(got.Files))
	}
}

func TestCacheManager_SourceMeta_NotFound(t *testing.T) {
	dir := t.TempDir()
	cm := NewCacheManager(dir, 1*time.Hour)

	got, err := cm.GetSourceMeta("owner", "repo")
	if err != nil {
		t.Fatalf("GetSourceMeta: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing source meta, got %+v", got)
	}
}

func TestCacheManager_IsSourceFresh(t *testing.T) {
	cm := NewCacheManager(t.TempDir(), 1*time.Hour)

	t.Run("nil meta", func(t *testing.T) {
		if cm.IsSourceFresh(nil) {
			t.Error("nil meta should not be fresh")
		}
	})

	t.Run("fresh", func(t *testing.T) {
		meta := &SourceMeta{FetchedAt: time.Now()}
		if !cm.IsSourceFresh(meta) {
			t.Error("recent meta should be fresh")
		}
	})

	t.Run("stale", func(t *testing.T) {
		meta := &SourceMeta{FetchedAt: time.Now().Add(-2 * time.Hour)}
		if cm.IsSourceFresh(meta) {
			t.Error("old meta should be stale")
		}
	})
}

func TestCacheManager_Recipe_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cm := NewCacheManager(dir, 1*time.Hour)

	data := []byte(`[tool]\nname = "foo"`)
	meta := &RecipeMeta{
		ETag:      `"abc123"`,
		FetchedAt: time.Now().Truncate(time.Second),
	}

	if err := cm.PutRecipe("owner", "repo", "foo", data, meta); err != nil {
		t.Fatalf("PutRecipe: %v", err)
	}

	got, err := cm.GetRecipe("owner", "repo", "foo")
	if err != nil {
		t.Fatalf("GetRecipe: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("recipe data mismatch")
	}

	gotMeta, err := cm.GetRecipeMeta("owner", "repo", "foo")
	if err != nil {
		t.Fatalf("GetRecipeMeta: %v", err)
	}
	if gotMeta.ETag != `"abc123"` {
		t.Errorf("etag = %q, want %q", gotMeta.ETag, `"abc123"`)
	}
}

func TestCacheManager_Recipe_NotFound(t *testing.T) {
	dir := t.TempDir()
	cm := NewCacheManager(dir, 1*time.Hour)

	got, err := cm.GetRecipe("owner", "repo", "missing")
	if err != nil {
		t.Fatalf("GetRecipe: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing recipe")
	}
}

func TestCacheManager_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	cm := NewCacheManager(dir, 1*time.Hour)

	tests := []struct {
		name  string
		owner string
		repo  string
	}{
		{"owner traversal", "../etc", "repo"},
		{"repo traversal", "owner", "../etc"},
		{"owner slash", "ow/ner", "repo"},
		{"repo slash", "owner", "re/po"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cm.GetSourceMeta(tt.owner, tt.repo)
			if err == nil {
				t.Error("expected error for path traversal")
			}
		})
	}
}

func TestCacheManager_InvalidRecipeName(t *testing.T) {
	dir := t.TempDir()
	cm := NewCacheManager(dir, 1*time.Hour)

	for _, name := range []string{"../evil", "foo/bar", "a..b"} {
		_, err := cm.GetRecipe("owner", "repo", name)
		if err == nil {
			t.Errorf("expected error for recipe name %q", name)
		}
	}
}

func TestCacheManager_FilesOnDisk(t *testing.T) {
	dir := t.TempDir()
	cm := NewCacheManager(dir, 1*time.Hour)

	data := []byte("test recipe content")
	meta := &RecipeMeta{FetchedAt: time.Now()}

	if err := cm.PutRecipe("myowner", "myrepo", "tool", data, meta); err != nil {
		t.Fatalf("PutRecipe: %v", err)
	}

	// Verify files exist at expected paths
	tomlPath := filepath.Join(dir, "myowner", "myrepo", "tool.toml")
	metaPath := filepath.Join(dir, "myowner", "myrepo", "tool.meta.json")

	if _, err := os.Stat(tomlPath); err != nil {
		t.Errorf("expected TOML file at %s: %v", tomlPath, err)
	}
	if _, err := os.Stat(metaPath); err != nil {
		t.Errorf("expected meta file at %s: %v", metaPath, err)
	}
}

func TestCacheManager_DefaultTTL(t *testing.T) {
	cm := NewCacheManager(t.TempDir(), 0) // 0 should use default
	if cm.ttl != DefaultCacheTTL {
		t.Errorf("ttl = %v, want %v", cm.ttl, DefaultCacheTTL)
	}

	cm2 := NewCacheManager(t.TempDir(), -1*time.Hour) // negative should use default
	if cm2.ttl != DefaultCacheTTL {
		t.Errorf("ttl = %v, want %v", cm2.ttl, DefaultCacheTTL)
	}
}
