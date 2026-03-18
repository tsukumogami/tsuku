package recipe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BackingStore is the raw byte storage behind a RegistryProvider.
// Implementations handle different transport mechanisms (in-memory, filesystem, HTTP).
type BackingStore interface {
	// Get retrieves raw bytes at the given path relative to the store root.
	Get(ctx context.Context, path string) ([]byte, error)

	// List returns all recipe file paths available in the store.
	List(ctx context.Context) ([]string, error)
}

// MemoryStore is a BackingStore backed by an in-memory map.
// Used for go:embed recipes compiled into the binary.
type MemoryStore struct {
	files map[string][]byte
}

// NewMemoryStore creates a MemoryStore from a map of path -> content.
func NewMemoryStore(files map[string][]byte) *MemoryStore {
	return &MemoryStore{files: files}
}

// NewMemoryStoreFromEmbedded creates a MemoryStore from an EmbeddedRegistry.
// Recipe names are stored as flat paths (e.g., "go.toml").
func NewMemoryStoreFromEmbedded(er *EmbeddedRegistry) *MemoryStore {
	files := make(map[string][]byte, len(er.recipes))
	for name, data := range er.recipes {
		files[name+".toml"] = data
	}
	return &MemoryStore{files: files}
}

func (s *MemoryStore) Get(_ context.Context, path string) ([]byte, error) {
	data, ok := s.files[path]
	if !ok {
		return nil, fmt.Errorf("recipe %q not found in memory store", path)
	}
	return data, nil
}

func (s *MemoryStore) List(_ context.Context) ([]string, error) {
	paths := make([]string, 0, len(s.files))
	for p := range s.files {
		paths = append(paths, p)
	}
	return paths, nil
}

// FSStore is a BackingStore backed by a filesystem directory.
type FSStore struct {
	dir string
}

// NewFSStore creates an FSStore rooted at the given directory.
func NewFSStore(dir string) *FSStore {
	return &FSStore{dir: dir}
}

func (s *FSStore) Get(_ context.Context, path string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("recipe %q not found in %s", path, s.dir)
		}
		return nil, err
	}
	return data, nil
}

func (s *FSStore) List(_ context.Context) ([]string, error) {
	if s.dir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".toml") {
			continue
		}
		paths = append(paths, name)
	}
	return paths, nil
}

// Dir returns the root directory of the FSStore.
func (s *FSStore) Dir() string {
	return s.dir
}
