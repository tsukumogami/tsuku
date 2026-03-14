package actions

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestActionDependencies_EcosystemActions(t *testing.T) {
	t.Parallel()
	// Ecosystem actions should have both install-time and runtime dependencies
	tests := []struct {
		action          string
		wantInstallTime []string
		wantRuntime     []string
	}{
		{"npm_install", []string{"nodejs"}, []string{"nodejs"}},
		{"pipx_install", []string{"python-standalone"}, []string{"python-standalone"}},
		{"gem_install", []string{"ruby"}, []string{"ruby"}},
		{"cpan_install", []string{"perl"}, []string{"perl"}},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			deps := GetActionDeps(tt.action)

			if !slicesEqual(deps.InstallTime, tt.wantInstallTime) {
				t.Errorf("InstallTime = %v, want %v", deps.InstallTime, tt.wantInstallTime)
			}
			if !slicesEqual(deps.Runtime, tt.wantRuntime) {
				t.Errorf("Runtime = %v, want %v", deps.Runtime, tt.wantRuntime)
			}
		})
	}
}

func TestActionDependencies_BuildActions(t *testing.T) {
	t.Parallel()
	// Build actions should have install-time deps for build tools
	tests := []struct {
		action          string
		wantInstallTime []string
	}{
		{"configure_make", []string{"make", "zig", "pkg-config"}},
		{"cmake_build", []string{"cmake", "make", "zig", "pkg-config"}},
		// meson_build has cross-platform deps; patchelf is Linux-only
		{"meson_build", []string{"meson", "ninja", "zig"}},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			deps := GetActionDeps(tt.action)

			if !slicesEqual(deps.InstallTime, tt.wantInstallTime) {
				t.Errorf("InstallTime = %v, want %v", deps.InstallTime, tt.wantInstallTime)
			}
			if deps.Runtime != nil {
				t.Errorf("Runtime = %v, want nil", deps.Runtime)
			}
		})
	}
}

func TestActionDependencies_PlatformSpecific(t *testing.T) {
	t.Parallel()
	// Actions with platform-specific dependencies
	tests := []struct {
		action               string
		wantLinuxInstallTime []string
	}{
		{"meson_build", []string{"patchelf"}},
		{"homebrew", []string{"patchelf"}},
		{"homebrew_relocate", []string{"patchelf"}},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			deps := GetActionDeps(tt.action)

			if !slicesEqual(deps.LinuxInstallTime, tt.wantLinuxInstallTime) {
				t.Errorf("LinuxInstallTime = %v, want %v", deps.LinuxInstallTime, tt.wantLinuxInstallTime)
			}
		})
	}
}

func TestActionDependencies_CompiledBinaryActions(t *testing.T) {
	t.Parallel()
	// Compiled binary actions should have install-time deps but no runtime deps
	tests := []struct {
		action          string
		wantInstallTime []string
	}{
		{"go_install", []string{"go"}},
		{"cargo_install", []string{"rust"}},
		{"nix_install", []string{"nix-portable"}},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			deps := GetActionDeps(tt.action)

			if !slicesEqual(deps.InstallTime, tt.wantInstallTime) {
				t.Errorf("InstallTime = %v, want %v", deps.InstallTime, tt.wantInstallTime)
			}
			if deps.Runtime != nil {
				t.Errorf("Runtime = %v, want nil", deps.Runtime)
			}
		})
	}
}

func TestActionDependencies_NoDependencyActions(t *testing.T) {
	t.Parallel()
	// These actions should have no dependencies
	actions := []string{
		// Download/extract
		"download",
		"extract",
		"chmod",
		"install_binaries",
		"set_env",
		"run_command",
		// System package managers
		"apt_install",
		"apt_repo",
		"apt_ppa",
		"brew_install",
		"brew_cask",
		"dnf_install",
		"dnf_repo",
		"pacman_install",
		"apk_install",
		"zypper_install",
		// Composites
		"download_archive",
		"github_archive",
		"github_file",
	}

	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			deps := GetActionDeps(action)

			if deps.InstallTime != nil {
				t.Errorf("InstallTime = %v, want nil", deps.InstallTime)
			}
			if deps.Runtime != nil {
				t.Errorf("Runtime = %v, want nil", deps.Runtime)
			}
		})
	}
}

func TestGetActionDeps_UnknownAction(t *testing.T) {
	t.Parallel()
	deps := GetActionDeps("nonexistent_action")

	if deps.InstallTime != nil {
		t.Errorf("InstallTime = %v, want nil for unknown action", deps.InstallTime)
	}
	if deps.Runtime != nil {
		t.Errorf("Runtime = %v, want nil for unknown action", deps.Runtime)
	}
}

// slicesEqual compares two string slices for equality
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDetectShadowedDeps_NoShadowing(t *testing.T) {
	t.Parallel()

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Dependencies:        []string{"openssl"},
			RuntimeDependencies: []string{"ca-certificates"},
		},
		Steps: []recipe.Step{
			{
				Action: "download_archive",
				Params: map[string]any{
					"url":      "https://example.com/file.tar.gz",
					"binaries": []any{"bin/tool"},
				},
			},
		},
	}

	shadowed := DetectShadowedDeps(r)
	if len(shadowed) != 0 {
		t.Errorf("DetectShadowedDeps() returned %d shadowed deps, want 0: %v", len(shadowed), shadowed)
	}
}

func TestDetectShadowedDeps_RecipeLevelInstallShadowing(t *testing.T) {
	t.Parallel()

	// go_install inherits "go" as install-time dep.
	// Declaring "go" in recipe-level dependencies should be detected as shadowed.
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Dependencies: []string{"go"},
		},
		Steps: []recipe.Step{
			{
				Action: "go_install",
				Params: map[string]any{
					"module":      "github.com/example/tool",
					"executables": []any{"tool"},
				},
			},
		},
	}

	shadowed := DetectShadowedDeps(r)
	found := false
	for _, s := range shadowed {
		if s.Name == "go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'go' to be detected as shadowed, got %v", shadowed)
	}
}

func TestDetectShadowedDeps_RecipeLevelRuntimeShadowing(t *testing.T) {
	t.Parallel()

	// npm_install inherits "nodejs" as runtime dep.
	// Declaring "nodejs" in recipe-level runtime_dependencies should be detected.
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			RuntimeDependencies: []string{"nodejs"},
		},
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]any{
					"package":     "tool",
					"executables": []any{"tool"},
				},
			},
		},
	}

	shadowed := DetectShadowedDeps(r)
	found := false
	for _, s := range shadowed {
		if s.Name == "nodejs" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'nodejs' to be detected as shadowed, got %v", shadowed)
	}
}

func TestDetectShadowedDeps_ExtraDependencies(t *testing.T) {
	t.Parallel()

	// Extra dependencies that shadow inherited ones
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			ExtraDependencies: []string{"go"},
		},
		Steps: []recipe.Step{
			{
				Action: "go_install",
				Params: map[string]any{
					"module":      "github.com/example/tool",
					"executables": []any{"tool"},
				},
			},
		},
	}

	shadowed := DetectShadowedDeps(r)
	found := false
	for _, s := range shadowed {
		if s.Name == "go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'go' in extra_dependencies to be shadowed, got %v", shadowed)
	}
}

func TestDetectShadowedDeps_ExtraRuntimeDependencies(t *testing.T) {
	t.Parallel()

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			ExtraRuntimeDependencies: []string{"nodejs"},
		},
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]any{
					"package":     "tool",
					"executables": []any{"tool"},
				},
			},
		},
	}

	shadowed := DetectShadowedDeps(r)
	found := false
	for _, s := range shadowed {
		if s.Name == "nodejs" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'nodejs' in extra_runtime_dependencies to be shadowed, got %v", shadowed)
	}
}

func TestDetectShadowedDeps_StepLevelDependencies(t *testing.T) {
	t.Parallel()

	// Step-level dependencies that shadow the step's own inherited deps
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "go_install",
				Params: map[string]any{
					"module":       "github.com/example/tool",
					"executables":  []any{"tool"},
					"dependencies": []any{"go"},
				},
			},
		},
	}

	shadowed := DetectShadowedDeps(r)
	found := false
	for _, s := range shadowed {
		if s.Name == "go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'go' in step dependencies to be shadowed, got %v", shadowed)
	}
}

func TestDetectShadowedDeps_StepLevelExtraDependencies(t *testing.T) {
	t.Parallel()

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "go_install",
				Params: map[string]any{
					"module":             "github.com/example/tool",
					"executables":        []any{"tool"},
					"extra_dependencies": []any{"go"},
				},
			},
		},
	}

	shadowed := DetectShadowedDeps(r)
	found := false
	for _, s := range shadowed {
		if s.Name == "go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'go' in step extra_dependencies to be shadowed, got %v", shadowed)
	}
}

func TestDetectShadowedDeps_StepLevelRuntimeDependencies(t *testing.T) {
	t.Parallel()

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]any{
					"package":              "tool",
					"executables":          []any{"tool"},
					"runtime_dependencies": []any{"nodejs"},
				},
			},
		},
	}

	shadowed := DetectShadowedDeps(r)
	found := false
	for _, s := range shadowed {
		if s.Name == "nodejs" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'nodejs' in step runtime_dependencies to be shadowed, got %v", shadowed)
	}
}

func TestDetectShadowedDeps_StepLevelExtraRuntimeDependencies(t *testing.T) {
	t.Parallel()

	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]any{
					"package":                    "tool",
					"executables":                []any{"tool"},
					"extra_runtime_dependencies": []any{"nodejs"},
				},
			},
		},
	}

	shadowed := DetectShadowedDeps(r)
	found := false
	for _, s := range shadowed {
		if s.Name == "nodejs" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'nodejs' in step extra_runtime_dependencies to be shadowed, got %v", shadowed)
	}
}

func TestDetectShadowedDeps_EmptyRecipe(t *testing.T) {
	t.Parallel()

	r := &recipe.Recipe{}
	shadowed := DetectShadowedDeps(r)
	if len(shadowed) != 0 {
		t.Errorf("DetectShadowedDeps() on empty recipe returned %d, want 0", len(shadowed))
	}
}

func TestDetectShadowedDeps_DependencyWithVersion(t *testing.T) {
	t.Parallel()

	// Dependencies with version constraints (using @ separator) should still be detected
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Dependencies: []string{"go@1.21"},
		},
		Steps: []recipe.Step{
			{
				Action: "go_install",
				Params: map[string]any{
					"module":      "github.com/example/tool",
					"executables": []any{"tool"},
				},
			},
		},
	}

	shadowed := DetectShadowedDeps(r)
	found := false
	for _, s := range shadowed {
		if s.Name == "go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'go@1.21' (parsed as 'go') to be shadowed, got %v", shadowed)
	}
}
