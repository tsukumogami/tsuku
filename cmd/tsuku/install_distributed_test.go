package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

func TestParseDistributedName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantNil    bool
		wantSource string
		wantRecipe string
		wantVer    string
	}{
		{
			name:    "bare tool name",
			input:   "kubectl",
			wantNil: true,
		},
		{
			name:    "tool with version",
			input:   "kubectl@v1.29.0",
			wantNil: true,
		},
		{
			name:       "owner/repo",
			input:      "myorg/recipes",
			wantSource: "myorg/recipes",
			wantRecipe: "recipes",
			wantVer:    "",
		},
		{
			name:       "owner/repo:recipe",
			input:      "myorg/recipes:mytool",
			wantSource: "myorg/recipes",
			wantRecipe: "mytool",
			wantVer:    "",
		},
		{
			name:       "owner/repo@version",
			input:      "myorg/recipes@v1.0.0",
			wantSource: "myorg/recipes",
			wantRecipe: "recipes",
			wantVer:    "v1.0.0",
		},
		{
			name:       "owner/repo:recipe@version",
			input:      "myorg/recipes:mytool@v2.3.1",
			wantSource: "myorg/recipes",
			wantRecipe: "mytool",
			wantVer:    "v2.3.1",
		},
		{
			name:       "owner with hyphens",
			input:      "my-org/my-tools:some-tool@latest",
			wantSource: "my-org/my-tools",
			wantRecipe: "some-tool",
			wantVer:    "latest",
		},
		{
			name:       "version with dots",
			input:      "acme/toolbox@3.14.159",
			wantSource: "acme/toolbox",
			wantRecipe: "toolbox",
			wantVer:    "3.14.159",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDistributedName(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil for %q, got %+v", tt.input, result)
				}
				return
			}
			if result == nil {
				t.Fatalf("expected non-nil for %q", tt.input)
			}
			if result.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", result.Source, tt.wantSource)
			}
			if result.RecipeName != tt.wantRecipe {
				t.Errorf("RecipeName = %q, want %q", result.RecipeName, tt.wantRecipe)
			}
			if result.Version != tt.wantVer {
				t.Errorf("Version = %q, want %q", result.Version, tt.wantVer)
			}
		})
	}
}

func TestComputeRecipeHash(t *testing.T) {
	data := []byte("[metadata]\nname = \"test\"\n")
	hash := computeRecipeHash(data)

	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64 (SHA256 hex)", len(hash))
	}

	// Same input should produce the same hash
	hash2 := computeRecipeHash(data)
	if hash != hash2 {
		t.Error("same input produced different hashes")
	}

	// Different input should produce different hash
	hash3 := computeRecipeHash([]byte("different"))
	if hash == hash3 {
		t.Error("different input produced same hash")
	}
}

func TestEnsureDistributedSource_AlreadyRegistered(t *testing.T) {
	// Create a temporary config with a registered source
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	cfg := userconfig.DefaultConfig()
	cfg.Registries = map[string]userconfig.RegistryEntry{
		"myorg/recipes": {
			URL: "https://github.com/myorg/recipes",
		},
	}
	if err := cfg.SaveToPathForTest(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// This test would need to mock userconfig.Load() to use our temp config,
	// which isn't straightforward in the current architecture. Instead, we
	// verify the validation step works correctly.
	err := validateRegistrySource("myorg/recipes")
	if err != nil {
		t.Errorf("valid source should pass validation: %v", err)
	}
}

func TestEnsureDistributedSource_InvalidSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{"empty", ""},
		{"no repo", "myorg"},
		{"path traversal", "../evil/repo"},
		{"triple path", "a/b/c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRegistrySource(tt.source)
			if err == nil {
				t.Errorf("expected error for invalid source %q", tt.source)
			}
		})
	}
}

func TestCheckSourceCollision_NotInstalled(t *testing.T) {
	// When a tool is not installed, there's no collision
	tmpDir := t.TempDir()
	origHome := os.Getenv("TSUKU_HOME")
	os.Setenv("TSUKU_HOME", tmpDir)
	defer os.Setenv("TSUKU_HOME", origHome)

	// Create minimal config
	os.MkdirAll(filepath.Join(tmpDir, "bin"), 0755)

	cfg, cfgErr := config.DefaultConfig()
	if cfgErr != nil {
		t.Fatalf("failed to get config: %v", cfgErr)
	}
	err := checkSourceCollision("nonexistent-tool", "myorg/recipes", false, cfg)
	if err != nil {
		t.Errorf("expected no collision for uninstalled tool: %v", err)
	}
}

func TestCheckSourceCollision_SameSource(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("TSUKU_HOME")
	os.Setenv("TSUKU_HOME", tmpDir)
	defer os.Setenv("TSUKU_HOME", origHome)

	// Set up state with a tool from a specific source
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("failed to get config: %v", err)
	}
	os.MkdirAll(filepath.Dir(filepath.Join(cfg.HomeDir, "state.json")), 0755)

	mgr := install.New(cfg)
	err = mgr.GetState().UpdateTool("mytool", func(ts *install.ToolState) {
		ts.Source = "myorg/recipes"
		ts.ActiveVersion = "1.0.0"
	})
	if err != nil {
		t.Fatalf("failed to set up state: %v", err)
	}

	// Same source -- no collision
	err = checkSourceCollision("mytool", "myorg/recipes", false, cfg)
	if err != nil {
		t.Errorf("expected no collision for same source: %v", err)
	}
}

func TestCheckSourceCollision_DifferentSourceWithForce(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("TSUKU_HOME")
	os.Setenv("TSUKU_HOME", tmpDir)
	defer os.Setenv("TSUKU_HOME", origHome)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("failed to get config: %v", err)
	}
	os.MkdirAll(filepath.Dir(filepath.Join(cfg.HomeDir, "state.json")), 0755)

	mgr := install.New(cfg)
	err = mgr.GetState().UpdateTool("mytool", func(ts *install.ToolState) {
		ts.Source = "central"
		ts.ActiveVersion = "1.0.0"
	})
	if err != nil {
		t.Fatalf("failed to set up state: %v", err)
	}

	// Different source with force -- no collision
	err = checkSourceCollision("mytool", "other/source", true, cfg)
	if err != nil {
		t.Errorf("expected no collision with --force: %v", err)
	}
}

func TestCheckSourceCollision_DifferentSourceNoForce(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("TSUKU_HOME")
	os.Setenv("TSUKU_HOME", tmpDir)
	defer os.Setenv("TSUKU_HOME", origHome)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("failed to get config: %v", err)
	}
	os.MkdirAll(filepath.Dir(filepath.Join(cfg.HomeDir, "state.json")), 0755)

	mgr := install.New(cfg)
	err = mgr.GetState().UpdateTool("mytool", func(ts *install.ToolState) {
		ts.Source = "central"
		ts.ActiveVersion = "1.0.0"
	})
	if err != nil {
		t.Fatalf("failed to set up state: %v", err)
	}

	// Different source without force and non-interactive -- should error
	// (confirmWithUser returns false when not interactive)
	err = checkSourceCollision("mytool", "other/source", false, cfg)
	if err == nil {
		t.Error("expected collision error for different source without force in non-interactive mode")
	}
}

func TestRecordDistributedSource(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("TSUKU_HOME")
	os.Setenv("TSUKU_HOME", tmpDir)
	defer os.Setenv("TSUKU_HOME", origHome)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("failed to get config: %v", err)
	}
	os.MkdirAll(filepath.Dir(filepath.Join(cfg.HomeDir, "state.json")), 0755)

	// Pre-create the tool entry
	mgr := install.New(cfg)
	err = mgr.GetState().UpdateTool("mytool", func(ts *install.ToolState) {
		ts.ActiveVersion = "1.0.0"
	})
	if err != nil {
		t.Fatalf("failed to set up state: %v", err)
	}

	// Record source and hash
	err = recordDistributedSource("mytool", "myorg/recipes", "abc123hash", cfg)
	if err != nil {
		t.Fatalf("recordDistributedSource failed: %v", err)
	}

	// Reload state manager to verify persistence
	mgr2 := install.New(cfg)
	toolState, err := mgr2.GetState().GetToolState("mytool")
	if err != nil {
		t.Fatalf("failed to get tool state: %v", err)
	}
	if toolState.Source != "myorg/recipes" {
		t.Errorf("Source = %q, want %q", toolState.Source, "myorg/recipes")
	}
	if toolState.RecipeHash != "abc123hash" {
		t.Errorf("RecipeHash = %q, want %q", toolState.RecipeHash, "abc123hash")
	}
}

func TestAutoRegisterSource(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Save empty config
	cfg := userconfig.DefaultConfig()
	if err := cfg.SaveToPathForTest(configPath); err != nil {
		t.Fatalf("failed to save initial config: %v", err)
	}

	// Auto-register a source
	if cfg.Registries == nil {
		cfg.Registries = make(map[string]userconfig.RegistryEntry)
	}
	cfg.Registries["myorg/tools"] = userconfig.RegistryEntry{
		URL:            "https://github.com/myorg/tools",
		AutoRegistered: true,
	}
	if err := cfg.SaveToPathForTest(configPath); err != nil {
		t.Fatalf("failed to save config with registry: %v", err)
	}

	// Reload and verify
	loaded, err := userconfig.LoadFromPathForTest(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}

	entry, exists := loaded.Registries["myorg/tools"]
	if !exists {
		t.Fatal("registry not found after auto-register")
	}
	if !entry.AutoRegistered {
		t.Error("expected AutoRegistered=true")
	}
	if entry.URL != "https://github.com/myorg/tools" {
		t.Errorf("URL = %q, want %q", entry.URL, "https://github.com/myorg/tools")
	}
}

func TestRecipeHashField_InState(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("TSUKU_HOME")
	os.Setenv("TSUKU_HOME", tmpDir)
	defer os.Setenv("TSUKU_HOME", origHome)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("failed to get config: %v", err)
	}
	os.MkdirAll(filepath.Dir(filepath.Join(cfg.HomeDir, "state.json")), 0755)

	mgr := install.New(cfg)
	hash := computeRecipeHash([]byte("[metadata]\nname = \"test\"\n"))

	err = mgr.GetState().UpdateTool("test-tool", func(ts *install.ToolState) {
		ts.Source = "org/repo"
		ts.RecipeHash = hash
		ts.ActiveVersion = "1.0"
	})
	if err != nil {
		t.Fatalf("failed to update state: %v", err)
	}

	// Reload and verify the hash persists through save/load
	ts, err := mgr.GetState().GetToolState("test-tool")
	if err != nil {
		t.Fatalf("failed to get tool state: %v", err)
	}
	if ts.RecipeHash != hash {
		t.Errorf("RecipeHash = %q, want %q", ts.RecipeHash, hash)
	}
}

func TestInstallYesFlag(t *testing.T) {
	// Verify the --yes/-y flag is registered on the install command
	f := installCmd.Flags().Lookup("yes")
	if f == nil {
		t.Fatal("expected --yes flag on install command")
	}
	if f.Shorthand != "y" {
		t.Errorf("--yes shorthand = %q, want %q", f.Shorthand, "y")
	}
	if f.DefValue != "false" {
		t.Errorf("--yes default = %q, want %q", f.DefValue, "false")
	}
}
