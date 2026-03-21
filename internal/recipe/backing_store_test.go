package recipe

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestMemoryStore_Get(t *testing.T) {
	store := NewMemoryStore(map[string][]byte{
		"foo.toml": []byte("content-foo"),
		"bar.toml": []byte("content-bar"),
	})

	data, err := store.Get(context.Background(), "foo.toml")
	if err != nil {
		t.Fatalf("Get(foo.toml) failed: %v", err)
	}
	if string(data) != "content-foo" {
		t.Errorf("expected content-foo, got %q", string(data))
	}
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	store := NewMemoryStore(map[string][]byte{})

	_, err := store.Get(context.Background(), "missing.toml")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestMemoryStore_List(t *testing.T) {
	store := NewMemoryStore(map[string][]byte{
		"a.toml": []byte("a"),
		"b.toml": []byte("b"),
	})

	paths, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	sort.Strings(paths)
	if len(paths) != 2 || paths[0] != "a.toml" || paths[1] != "b.toml" {
		t.Errorf("unexpected paths: %v", paths)
	}
}

func TestMemoryStoreFromEmbedded(t *testing.T) {
	er, err := NewEmbeddedRegistry()
	if err != nil {
		t.Fatalf("NewEmbeddedRegistry() failed: %v", err)
	}

	store := NewMemoryStoreFromEmbedded(er)

	// Should be able to get any embedded recipe by name.toml
	names := er.List()
	if len(names) == 0 {
		t.Fatal("expected at least one embedded recipe")
	}

	data, err := store.Get(context.Background(), names[0]+".toml")
	if err != nil {
		t.Fatalf("Get(%s.toml) failed: %v", names[0], err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty data")
	}
}

func TestFSStore_Get(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.toml"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewFSStore(dir)
	data, err := store.Get(context.Background(), "test.toml")
	if err != nil {
		t.Fatalf("Get(test.toml) failed: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected hello, got %q", string(data))
	}
}

func TestFSStore_GetNotFound(t *testing.T) {
	store := NewFSStore(t.TempDir())
	_, err := store.Get(context.Background(), "missing.toml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFSStore_List(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.toml"), []byte("a"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "b.toml"), []byte("b"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c"), 0644) // not .toml
	_ = os.Mkdir(filepath.Join(dir, "subdir"), 0755)                 // directory

	store := NewFSStore(dir)
	paths, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	sort.Strings(paths)
	if len(paths) != 2 || paths[0] != "a.toml" || paths[1] != "b.toml" {
		t.Errorf("expected [a.toml b.toml], got %v", paths)
	}
}

func TestFSStore_ListEmptyDir(t *testing.T) {
	store := NewFSStore("")
	paths, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if paths != nil {
		t.Errorf("expected nil for empty dir, got %v", paths)
	}
}

func TestFSStore_ListNonexistentDir(t *testing.T) {
	store := NewFSStore("/nonexistent/path")
	paths, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() should return nil for nonexistent dir, got error: %v", err)
	}
	if paths != nil {
		t.Errorf("expected nil, got %v", paths)
	}
}

func TestFSStore_Dir(t *testing.T) {
	store := NewFSStore("/some/path")
	if store.Dir() != "/some/path" {
		t.Errorf("expected /some/path, got %q", store.Dir())
	}
}
