package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// TempDir creates a temporary directory and returns a cleanup function
func TempDir(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "tsuku-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	return dir, func() { os.RemoveAll(dir) }
}

// NewTestConfig creates a config with temporary directories for testing
func NewTestConfig(t *testing.T) (*config.Config, func()) {
	t.Helper()
	tmpDir, cleanup := TempDir(t)

	cfg := &config.Config{
		HomeDir:          tmpDir,
		ToolsDir:         filepath.Join(tmpDir, "tools"),
		CurrentDir:       filepath.Join(tmpDir, "tools", "current"),
		RecipesDir:       filepath.Join(tmpDir, "recipes"),
		RegistryDir:      filepath.Join(tmpDir, "registry"),
		LibsDir:          filepath.Join(tmpDir, "libs"),
		AppsDir:          filepath.Join(tmpDir, "apps"),
		CacheDir:         filepath.Join(tmpDir, "cache"),
		VersionCacheDir:  filepath.Join(tmpDir, "cache", "versions"),
		DownloadCacheDir: filepath.Join(tmpDir, "cache", "downloads"),
		KeyCacheDir:      filepath.Join(tmpDir, "cache", "keys"),
		TapCacheDir:      filepath.Join(tmpDir, "cache", "taps"),
		ConfigFile:       filepath.Join(tmpDir, "config.toml"),
	}

	// Create directories
	if err := os.MkdirAll(cfg.ToolsDir, 0755); err != nil {
		cleanup()
		t.Fatalf("failed to create tools dir: %v", err)
	}
	if err := os.MkdirAll(cfg.CurrentDir, 0755); err != nil {
		cleanup()
		t.Fatalf("failed to create current dir: %v", err)
	}
	if err := os.MkdirAll(cfg.RecipesDir, 0755); err != nil {
		cleanup()
		t.Fatalf("failed to create recipes dir: %v", err)
	}
	if err := os.MkdirAll(cfg.RegistryDir, 0755); err != nil {
		cleanup()
		t.Fatalf("failed to create registry dir: %v", err)
	}
	if err := os.MkdirAll(cfg.LibsDir, 0755); err != nil {
		cleanup()
		t.Fatalf("failed to create libs dir: %v", err)
	}
	if err := os.MkdirAll(cfg.AppsDir, 0755); err != nil {
		cleanup()
		t.Fatalf("failed to create apps dir: %v", err)
	}
	if err := os.MkdirAll(cfg.CacheDir, 0755); err != nil {
		cleanup()
		t.Fatalf("failed to create cache dir: %v", err)
	}
	if err := os.MkdirAll(cfg.VersionCacheDir, 0755); err != nil {
		cleanup()
		t.Fatalf("failed to create version cache dir: %v", err)
	}
	if err := os.MkdirAll(cfg.DownloadCacheDir, 0755); err != nil {
		cleanup()
		t.Fatalf("failed to create download cache dir: %v", err)
	}
	if err := os.MkdirAll(cfg.KeyCacheDir, 0755); err != nil {
		cleanup()
		t.Fatalf("failed to create key cache dir: %v", err)
	}
	if err := os.MkdirAll(cfg.TapCacheDir, 0755); err != nil {
		cleanup()
		t.Fatalf("failed to create tap cache dir: %v", err)
	}

	return cfg, cleanup
}

// NewTestRecipe creates a test recipe with common defaults
func NewTestRecipe(name string) *recipe.Recipe {
	return &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:          name,
			Description:   "Test recipe for " + name,
			VersionFormat: "semver",
			Dependencies:  []string{},
		},
		Steps: []recipe.Step{
			{
				Action: "run_command",
				Params: map[string]interface{}{
					"command": "echo 'test'",
				},
			},
		},
		Verify: recipe.VerifySection{
			Command: "echo 'verified'",
			Pattern: "verified",
		},
	}
}

// NewTestRecipeWithDeps creates a test recipe with dependencies
func NewTestRecipeWithDeps(name string, deps []string) *recipe.Recipe {
	r := NewTestRecipe(name)
	r.Metadata.Dependencies = deps
	return r
}

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// AssertFileExists checks if a file exists at the given path
func AssertFileExists(t *testing.T, path string) {
	t.Helper()
	if !FileExists(path) {
		t.Errorf("file does not exist: %s", path)
	}
}

// AssertFileNotExists checks if a file does NOT exist at the given path
func AssertFileNotExists(t *testing.T, path string) {
	t.Helper()
	if FileExists(path) {
		t.Errorf("file should not exist: %s", path)
	}
}
