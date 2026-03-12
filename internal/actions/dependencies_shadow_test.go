package actions

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

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
