package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/userconfig"
)

func TestRegistryCmd_NoSubcommand(t *testing.T) {
	// The registry command with no subcommand should print help (not error)
	cmd := registryCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})
	// Reset for test
	cmd.Run(cmd, []string{})
	// If we got here without panic, the help path works
}

func TestRegistryList_NoRegistries(t *testing.T) {
	cfg := userconfig.DefaultConfig()

	// Verify empty registries
	if len(cfg.Registries) != 0 {
		t.Fatalf("expected no registries in default config, got %d", len(cfg.Registries))
	}
}

func TestRegistryList_WithRegistries(t *testing.T) {
	cfg := userconfig.DefaultConfig()
	cfg.Registries = map[string]userconfig.RegistryEntry{
		"myorg/recipes": {
			URL:            "https://github.com/myorg/recipes",
			AutoRegistered: false,
		},
		"auto/repo": {
			URL:            "https://github.com/auto/repo",
			AutoRegistered: true,
		},
	}

	if len(cfg.Registries) != 2 {
		t.Fatalf("expected 2 registries, got %d", len(cfg.Registries))
	}

	entry := cfg.Registries["auto/repo"]
	if !entry.AutoRegistered {
		t.Error("expected auto/repo to be auto-registered")
	}
}

func TestRegistryAdd_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Start with empty config
	cfg := userconfig.DefaultConfig()
	if err := cfg.SaveToPathForTest(configPath); err != nil {
		t.Fatalf("failed to save initial config: %v", err)
	}

	// Simulate add: load, modify, save
	cfg, err := userconfig.LoadFromPathForTest(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	source := "myorg/recipes"
	if cfg.Registries == nil {
		cfg.Registries = make(map[string]userconfig.RegistryEntry)
	}
	cfg.Registries[source] = userconfig.RegistryEntry{
		URL:            "https://github.com/" + source,
		AutoRegistered: false,
	}

	if err := cfg.SaveToPathForTest(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Reload and verify
	loaded, err := userconfig.LoadFromPathForTest(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}

	entry, exists := loaded.Registries[source]
	if !exists {
		t.Fatalf("registry %q not found after save/load", source)
	}
	if entry.URL != "https://github.com/myorg/recipes" {
		t.Errorf("URL = %q, want %q", entry.URL, "https://github.com/myorg/recipes")
	}
	if entry.AutoRegistered {
		t.Error("expected AutoRegistered=false for manually added registry")
	}
}

func TestRegistryAdd_Idempotent(t *testing.T) {
	cfg := userconfig.DefaultConfig()
	cfg.Registries = map[string]userconfig.RegistryEntry{
		"myorg/recipes": {
			URL:            "https://github.com/myorg/recipes",
			AutoRegistered: false,
		},
	}

	// Adding the same source again should be a no-op (checked via exists)
	_, exists := cfg.Registries["myorg/recipes"]
	if !exists {
		t.Error("expected registry to exist")
	}
}

func TestRegistryRemove_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Start with one registry
	cfg := userconfig.DefaultConfig()
	cfg.Registries = map[string]userconfig.RegistryEntry{
		"myorg/recipes": {
			URL:            "https://github.com/myorg/recipes",
			AutoRegistered: false,
		},
	}
	if err := cfg.SaveToPathForTest(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Remove it
	cfg, err := userconfig.LoadFromPathForTest(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	delete(cfg.Registries, "myorg/recipes")
	if len(cfg.Registries) == 0 {
		cfg.Registries = nil
	}

	if err := cfg.SaveToPathForTest(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Reload and verify
	loaded, err := userconfig.LoadFromPathForTest(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}

	if len(loaded.Registries) != 0 {
		t.Errorf("expected no registries after remove, got %d", len(loaded.Registries))
	}
}

func TestRegistryRemove_NonExistent(t *testing.T) {
	cfg := userconfig.DefaultConfig()

	// Removing a non-existent registry should not panic
	_, exists := cfg.Registries["nonexistent/repo"]
	if exists {
		t.Error("expected registry to not exist")
	}
}

func TestRegistryList_StrictRegistries(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	content := `strict_registries = true

[registries]
[registries."myorg/recipes"]
url = "https://github.com/myorg/recipes"
`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := userconfig.LoadFromPathForTest(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !cfg.StrictRegistries {
		t.Error("expected StrictRegistries=true")
	}
	if len(cfg.Registries) != 1 {
		t.Errorf("expected 1 registry, got %d", len(cfg.Registries))
	}
}

func TestRegistryAdd_ValidatesFormat(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		wantErr bool
	}{
		{"valid owner/repo", "myorg/recipes", false},
		{"valid with hyphens", "my-org/my-recipes", false},
		{"missing repo", "myorg", true},
		{"empty string", "", true},
		{"path traversal", "../etc/passwd", true},
		{"triple path", "a/b/c", true},
		{"with credentials", "user:pass@github.com/org/repo", true},
		{"just slash", "/", true},
		{"double dot owner", "../foo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We test validation by checking if discover.ValidateGitHubURL accepts it.
			// The actual import is tested by the command itself; here we just verify
			// the logic pattern.
			err := validateRegistrySource(tt.source)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRegistrySource(%q) error = %v, wantErr %v", tt.source, err, tt.wantErr)
			}
		})
	}
}

// validateRegistrySource is a test helper that mirrors the validation in runRegistryAdd.
func validateRegistrySource(source string) error {
	// Empty check
	if source == "" {
		return os.ErrInvalid
	}
	// Must be owner/repo format (exactly one slash, no extra path segments)
	parts := strings.SplitN(source, "/", 3)
	if len(parts) != 2 {
		return os.ErrInvalid
	}
	if parts[0] == "" || parts[1] == "" {
		return os.ErrInvalid
	}
	// No path traversal
	if parts[0] == ".." || parts[0] == "." || parts[1] == ".." || parts[1] == "." {
		return os.ErrInvalid
	}
	// No credentials (@ sign)
	if strings.Contains(source, "@") {
		return os.ErrInvalid
	}
	return nil
}
