package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/distributed"
	"github.com/tsukumogami/tsuku/internal/install"
)

func TestSearchDistributedCaches_FindsMatchingRecipes(t *testing.T) {
	// Set up a temporary $TSUKU_HOME with a distributed cache
	tmpHome := t.TempDir()
	t.Setenv("TSUKU_HOME", tmpHome)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	// Write a config.toml with a registered distributed source
	configDir := filepath.Dir(cfg.ConfigFile)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll config dir: %v", err)
	}
	configContent := `
[registries]
[registries."acme/tools"]
url = "https://github.com/acme/tools"
`
	if err := os.WriteFile(cfg.ConfigFile, []byte(configContent), 0600); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	// Populate the distributed cache with source metadata
	cacheDir := filepath.Join(cfg.CacheDir, "distributed")
	cache := distributed.NewCacheManager(cacheDir, distributed.DefaultCacheTTL)

	meta := &distributed.SourceMeta{
		Branch: "main",
		Files: map[string]string{
			"my-tool":      "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/my-tool.toml",
			"other-widget": "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/other-widget.toml",
		},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	// Search for "my-tool" -- should find it
	alreadySeen := make(map[string]bool)
	results := searchDistributedCaches(cfg, "my-tool", alreadySeen, nil)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "my-tool" {
		t.Errorf("expected name 'my-tool', got %q", results[0].Name)
	}
	if results[0].Source != "acme/tools" {
		t.Errorf("expected source 'acme/tools', got %q", results[0].Source)
	}
}

func TestSearchDistributedCaches_SkipsAlreadySeen(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("TSUKU_HOME", tmpHome)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	configDir := filepath.Dir(cfg.ConfigFile)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	configContent := `
[registries]
[registries."acme/tools"]
url = "https://github.com/acme/tools"
`
	if err := os.WriteFile(cfg.ConfigFile, []byte(configContent), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cacheDir := filepath.Join(cfg.CacheDir, "distributed")
	cache := distributed.NewCacheManager(cacheDir, distributed.DefaultCacheTTL)
	meta := &distributed.SourceMeta{
		Branch: "main",
		Files: map[string]string{
			"my-tool": "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/my-tool.toml",
		},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	// Mark "my-tool" as already seen (e.g., from the loader)
	alreadySeen := map[string]bool{"my-tool": true}
	results := searchDistributedCaches(cfg, "my-tool", alreadySeen, nil)

	if len(results) != 0 {
		t.Errorf("expected 0 results (already seen), got %d", len(results))
	}
}

func TestSearchDistributedCaches_EmptyQuery(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("TSUKU_HOME", tmpHome)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	configDir := filepath.Dir(cfg.ConfigFile)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	configContent := `
[registries]
[registries."acme/tools"]
url = "https://github.com/acme/tools"
`
	if err := os.WriteFile(cfg.ConfigFile, []byte(configContent), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cacheDir := filepath.Join(cfg.CacheDir, "distributed")
	cache := distributed.NewCacheManager(cacheDir, distributed.DefaultCacheTTL)
	meta := &distributed.SourceMeta{
		Branch: "main",
		Files: map[string]string{
			"tool-a": "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/tool-a.toml",
			"tool-b": "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/tool-b.toml",
		},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	// Empty query should return all recipes from the source
	alreadySeen := make(map[string]bool)
	results := searchDistributedCaches(cfg, "", alreadySeen, nil)

	if len(results) != 2 {
		t.Errorf("expected 2 results for empty query, got %d", len(results))
	}
}

func TestSearchDistributedCaches_ShowsInstalledVersion(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("TSUKU_HOME", tmpHome)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	configDir := filepath.Dir(cfg.ConfigFile)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	configContent := `
[registries]
[registries."acme/tools"]
url = "https://github.com/acme/tools"
`
	if err := os.WriteFile(cfg.ConfigFile, []byte(configContent), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cacheDir := filepath.Join(cfg.CacheDir, "distributed")
	cache := distributed.NewCacheManager(cacheDir, distributed.DefaultCacheTTL)
	meta := &distributed.SourceMeta{
		Branch: "main",
		Files: map[string]string{
			"my-tool": "https://raw.githubusercontent.com/acme/tools/main/.tsuku-recipes/my-tool.toml",
		},
		FetchedAt: time.Now(),
	}
	if err := cache.PutSourceMeta("acme", "tools", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	installedTools := []install.InstalledTool{
		{Name: "my-tool", Version: "1.2.3"},
	}

	alreadySeen := make(map[string]bool)
	results := searchDistributedCaches(cfg, "my-tool", alreadySeen, installedTools)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Installed != "1.2.3" {
		t.Errorf("expected installed version '1.2.3', got %q", results[0].Installed)
	}
}

func TestSearchDistributedCaches_NoRegistries(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("TSUKU_HOME", tmpHome)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	// No config file at all -- should return nil gracefully
	alreadySeen := make(map[string]bool)
	results := searchDistributedCaches(cfg, "anything", alreadySeen, nil)

	if results != nil {
		t.Errorf("expected nil results when no registries configured, got %d", len(results))
	}
}
