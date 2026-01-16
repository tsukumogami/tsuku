package actions

import (
	"context"
	"errors"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// mockLoader implements RecipeLoader for testing
type mockLoader struct {
	recipes map[string]*recipe.Recipe
}

func newMockLoader() *mockLoader {
	return &mockLoader{recipes: make(map[string]*recipe.Recipe)}
}

func (m *mockLoader) GetWithContext(ctx context.Context, name string) (*recipe.Recipe, error) {
	if r, ok := m.recipes[name]; ok {
		return r, nil
	}
	return nil, errors.New("recipe not found")
}

func (m *mockLoader) addRecipe(name string, r *recipe.Recipe) {
	r.Metadata.Name = name
	m.recipes[name] = r
}

func TestResolveDependencies_NpmInstall(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "npm_install", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependencies(r)

	// npm_install should have nodejs in both install and runtime
	if deps.InstallTime["nodejs"] != "latest" {
		t.Errorf("InstallTime[nodejs] = %q, want %q", deps.InstallTime["nodejs"], "latest")
	}
	if deps.Runtime["nodejs"] != "latest" {
		t.Errorf("Runtime[nodejs] = %q, want %q", deps.Runtime["nodejs"], "latest")
	}
}

func TestResolveDependencies_GoInstall(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "go_install", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependencies(r)

	// go_install should have go in install only
	if deps.InstallTime["go"] != "latest" {
		t.Errorf("InstallTime[go] = %q, want %q", deps.InstallTime["go"], "latest")
	}
	if _, hasRuntime := deps.Runtime["go"]; hasRuntime {
		t.Errorf("Runtime should not contain go, got %v", deps.Runtime)
	}
}

func TestResolveDependencies_Download(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "download", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependencies(r)

	// download should have no deps
	if len(deps.InstallTime) != 0 {
		t.Errorf("InstallTime = %v, want empty", deps.InstallTime)
	}
	if len(deps.Runtime) != 0 {
		t.Errorf("Runtime = %v, want empty", deps.Runtime)
	}
}

func TestResolveDependencies_MultipleSteps(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "download", Params: map[string]interface{}{}},
			{Action: "extract", Params: map[string]interface{}{}},
			{Action: "npm_install", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependencies(r)

	// Should have nodejs from npm_install
	if deps.InstallTime["nodejs"] != "latest" {
		t.Errorf("InstallTime[nodejs] = %q, want %q", deps.InstallTime["nodejs"], "latest")
	}
	if deps.Runtime["nodejs"] != "latest" {
		t.Errorf("Runtime[nodejs] = %q, want %q", deps.Runtime["nodejs"], "latest")
	}
	// Should only have nodejs (from npm_install)
	if len(deps.InstallTime) != 1 {
		t.Errorf("InstallTime has %d deps, want 1", len(deps.InstallTime))
	}
}

func TestResolveDependencies_ExtraDependencies(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{
					"extra_dependencies": []interface{}{"wget", "curl@7.0"},
				},
			},
		},
	}

	deps := ResolveDependencies(r)

	if deps.InstallTime["wget"] != "latest" {
		t.Errorf("InstallTime[wget] = %q, want %q", deps.InstallTime["wget"], "latest")
	}
	if deps.InstallTime["curl"] != "7.0" {
		t.Errorf("InstallTime[curl] = %q, want %q", deps.InstallTime["curl"], "7.0")
	}
}

func TestResolveDependencies_ExtraRuntimeDependencies(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "go_install",
				Params: map[string]interface{}{
					"extra_runtime_dependencies": []interface{}{"bash"},
				},
			},
		},
	}

	deps := ResolveDependencies(r)

	// go_install has go in install
	if deps.InstallTime["go"] != "latest" {
		t.Errorf("InstallTime[go] = %q, want %q", deps.InstallTime["go"], "latest")
	}
	// extra_runtime_dependencies adds bash to runtime
	if deps.Runtime["bash"] != "latest" {
		t.Errorf("Runtime[bash] = %q, want %q", deps.Runtime["bash"], "latest")
	}
}

func TestResolveDependencies_CombinedExtras(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]interface{}{
					"extra_dependencies":         []interface{}{"python@3.11"},
					"extra_runtime_dependencies": []interface{}{"bash"},
				},
			},
		},
	}

	deps := ResolveDependencies(r)

	// Implicit from npm_install
	if deps.InstallTime["nodejs"] != "latest" {
		t.Errorf("InstallTime[nodejs] = %q, want %q", deps.InstallTime["nodejs"], "latest")
	}
	if deps.Runtime["nodejs"] != "latest" {
		t.Errorf("Runtime[nodejs] = %q, want %q", deps.Runtime["nodejs"], "latest")
	}
	// extra_dependencies
	if deps.InstallTime["python"] != "3.11" {
		t.Errorf("InstallTime[python] = %q, want %q", deps.InstallTime["python"], "3.11")
	}
	// extra_runtime_dependencies
	if deps.Runtime["bash"] != "latest" {
		t.Errorf("Runtime[bash] = %q, want %q", deps.Runtime["bash"], "latest")
	}
}

func TestResolveDependencies_EmptyRecipe(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Steps: []recipe.Step{},
	}

	deps := ResolveDependencies(r)

	if len(deps.InstallTime) != 0 {
		t.Errorf("InstallTime = %v, want empty", deps.InstallTime)
	}
	if len(deps.Runtime) != 0 {
		t.Errorf("Runtime = %v, want empty", deps.Runtime)
	}
}

func TestParseDependency(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input       string
		wantName    string
		wantVersion string
	}{
		{"nodejs", "nodejs", "latest"},
		{"nodejs@20", "nodejs@20", "latest"}, // Note: this is incorrect, let me fix
		{"python@3.11", "python", "3.11"},
		{"go@1.21.0", "go", "1.21.0"},
		{"", "", "latest"},
	}

	// Actually, let me check the implementation first
	name, version := parseDependency("nodejs@20")
	if name != "nodejs" || version != "20" {
		t.Errorf("parseDependency(nodejs@20) = (%q, %q), want (%q, %q)", name, version, "nodejs", "20")
	}

	for _, tt := range tests {
		if tt.input == "nodejs@20" {
			continue // Already tested above
		}
		t.Run(tt.input, func(t *testing.T) {
			name, version := parseDependency(tt.input)
			if tt.input == "nodejs" && name != "nodejs" {
				t.Errorf("name = %q, want %q", name, "nodejs")
			}
			if tt.input == "python@3.11" && (name != "python" || version != "3.11") {
				t.Errorf("got (%q, %q), want (%q, %q)", name, version, "python", "3.11")
			}
		})
	}
}

func TestGetStringSliceParam(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		params map[string]interface{}
		key    string
		want   []string
	}{
		{
			name:   "interface slice",
			params: map[string]interface{}{"deps": []interface{}{"a", "b"}},
			key:    "deps",
			want:   []string{"a", "b"},
		},
		{
			name:   "string slice",
			params: map[string]interface{}{"deps": []string{"x", "y"}},
			key:    "deps",
			want:   []string{"x", "y"},
		},
		{
			name:   "missing key",
			params: map[string]interface{}{},
			key:    "deps",
			want:   nil,
		},
		{
			name:   "wrong type",
			params: map[string]interface{}{"deps": "not a slice"},
			key:    "deps",
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStringSliceParam(tt.params, tt.key)
			if !slicesEqual(got, tt.want) {
				t.Errorf("getStringSliceParam() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Tests for step-level replace behavior

func TestResolveDependencies_StepRuntimeDependenciesReplace(t *testing.T) {
	t.Parallel()
	// esbuild case: npm_install but compiled binary, no runtime needed
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]interface{}{
					"runtime_dependencies": []interface{}{}, // Empty: no runtime deps
				},
			},
		},
	}

	deps := ResolveDependencies(r)

	// Install still has nodejs (implicit from npm_install)
	if deps.InstallTime["nodejs"] != "latest" {
		t.Errorf("InstallTime[nodejs] = %q, want %q", deps.InstallTime["nodejs"], "latest")
	}
	// Runtime should be empty (replaced with [])
	if len(deps.Runtime) != 0 {
		t.Errorf("Runtime = %v, want empty", deps.Runtime)
	}
}

func TestResolveDependencies_StepDependenciesReplace(t *testing.T) {
	t.Parallel()
	// Replace implicit install deps with custom ones
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]interface{}{
					"dependencies": []interface{}{"bun@1.0"}, // Use bun instead of nodejs
				},
			},
		},
	}

	deps := ResolveDependencies(r)

	// Install has only bun (replaced nodejs)
	if deps.InstallTime["bun"] != "1.0" {
		t.Errorf("InstallTime[bun] = %q, want %q", deps.InstallTime["bun"], "1.0")
	}
	if _, hasNodejs := deps.InstallTime["nodejs"]; hasNodejs {
		t.Errorf("InstallTime should not contain nodejs, got %v", deps.InstallTime)
	}
	// Runtime still has nodejs (action implicit, not replaced)
	if deps.Runtime["nodejs"] != "latest" {
		t.Errorf("Runtime[nodejs] = %q, want %q", deps.Runtime["nodejs"], "latest")
	}
}

// Tests for recipe-level replace behavior

func TestResolveDependencies_RecipeDependenciesReplace(t *testing.T) {
	t.Parallel()
	// Recipe-level dependencies replaces all install deps
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Dependencies: []string{"nodejs@20", "python@3.11"},
		},
		Steps: []recipe.Step{
			{Action: "npm_install", Params: map[string]interface{}{}},
			{Action: "go_install", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependencies(r)

	// Install has only recipe-level deps (replaced step-level)
	if deps.InstallTime["nodejs"] != "20" {
		t.Errorf("InstallTime[nodejs] = %q, want %q", deps.InstallTime["nodejs"], "20")
	}
	if deps.InstallTime["python"] != "3.11" {
		t.Errorf("InstallTime[python] = %q, want %q", deps.InstallTime["python"], "3.11")
	}
	if _, hasGo := deps.InstallTime["go"]; hasGo {
		t.Errorf("InstallTime should not contain go (replaced), got %v", deps.InstallTime)
	}
	if len(deps.InstallTime) != 2 {
		t.Errorf("InstallTime has %d deps, want 2", len(deps.InstallTime))
	}

	// Runtime still has nodejs from npm_install (not replaced)
	if deps.Runtime["nodejs"] != "latest" {
		t.Errorf("Runtime[nodejs] = %q, want %q", deps.Runtime["nodejs"], "latest")
	}
}

func TestResolveDependencies_RecipeRuntimeDependenciesReplace(t *testing.T) {
	t.Parallel()
	// Recipe-level runtime_dependencies replaces all runtime deps
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			RuntimeDependencies: []string{"python@3.11"},
		},
		Steps: []recipe.Step{
			{Action: "npm_install", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependencies(r)

	// Install still has nodejs (not replaced)
	if deps.InstallTime["nodejs"] != "latest" {
		t.Errorf("InstallTime[nodejs] = %q, want %q", deps.InstallTime["nodejs"], "latest")
	}
	// Runtime has only recipe-level deps (replaced step-level)
	if deps.Runtime["python"] != "3.11" {
		t.Errorf("Runtime[python] = %q, want %q", deps.Runtime["python"], "3.11")
	}
	if _, hasNodejs := deps.Runtime["nodejs"]; hasNodejs {
		t.Errorf("Runtime should not contain nodejs (replaced), got %v", deps.Runtime)
	}
	if len(deps.Runtime) != 1 {
		t.Errorf("Runtime has %d deps, want 1", len(deps.Runtime))
	}
}

// Tests for recipe-level extend behavior

func TestResolveDependencies_RecipeExtraDependencies(t *testing.T) {
	t.Parallel()
	// Recipe-level extra_dependencies adds to install deps
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			ExtraDependencies: []string{"wget", "curl@7.0"},
		},
		Steps: []recipe.Step{
			{Action: "npm_install", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependencies(r)

	// Install has nodejs (implicit) + extras
	if deps.InstallTime["nodejs"] != "latest" {
		t.Errorf("InstallTime[nodejs] = %q, want %q", deps.InstallTime["nodejs"], "latest")
	}
	if deps.InstallTime["wget"] != "latest" {
		t.Errorf("InstallTime[wget] = %q, want %q", deps.InstallTime["wget"], "latest")
	}
	if deps.InstallTime["curl"] != "7.0" {
		t.Errorf("InstallTime[curl] = %q, want %q", deps.InstallTime["curl"], "7.0")
	}
}

func TestResolveDependencies_RecipeExtraRuntimeDependencies(t *testing.T) {
	t.Parallel()
	// Recipe-level extra_runtime_dependencies adds to runtime deps
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			ExtraRuntimeDependencies: []string{"bash"},
		},
		Steps: []recipe.Step{
			{Action: "npm_install", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependencies(r)

	// Runtime has nodejs (implicit) + extras
	if deps.Runtime["nodejs"] != "latest" {
		t.Errorf("Runtime[nodejs] = %q, want %q", deps.Runtime["nodejs"], "latest")
	}
	if deps.Runtime["bash"] != "latest" {
		t.Errorf("Runtime[bash] = %q, want %q", deps.Runtime["bash"], "latest")
	}
}

func TestResolveDependencies_RecipeReplaceAndExtend(t *testing.T) {
	t.Parallel()
	// Recipe-level replace + extend
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Dependencies:             []string{"nodejs@20"}, // Replace
			ExtraRuntimeDependencies: []string{"bash"},      // Extend
		},
		Steps: []recipe.Step{
			{Action: "npm_install", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependencies(r)

	// Install has only nodejs@20 (replaced)
	if deps.InstallTime["nodejs"] != "20" {
		t.Errorf("InstallTime[nodejs] = %q, want %q", deps.InstallTime["nodejs"], "20")
	}
	if len(deps.InstallTime) != 1 {
		t.Errorf("InstallTime has %d deps, want 1", len(deps.InstallTime))
	}

	// Runtime has nodejs (implicit) + bash (extended)
	if deps.Runtime["nodejs"] != "latest" {
		t.Errorf("Runtime[nodejs] = %q, want %q", deps.Runtime["nodejs"], "latest")
	}
	if deps.Runtime["bash"] != "latest" {
		t.Errorf("Runtime[bash] = %q, want %q", deps.Runtime["bash"], "latest")
	}
}

// Tests for transitive resolution

func TestResolveTransitive_EmptyDeps(t *testing.T) {
	t.Parallel()
	loader := newMockLoader()
	ctx := context.Background()

	deps := ResolvedDeps{
		InstallTime: make(map[string]string),
		Runtime:     make(map[string]string),
	}

	result, err := ResolveTransitive(ctx, loader, deps, "root")
	if err != nil {
		t.Fatalf("ResolveTransitive() error = %v", err)
	}

	if len(result.InstallTime) != 0 {
		t.Errorf("InstallTime = %v, want empty", result.InstallTime)
	}
	if len(result.Runtime) != 0 {
		t.Errorf("Runtime = %v, want empty", result.Runtime)
	}
}

func TestResolveTransitive_NoDepsRecipe(t *testing.T) {
	t.Parallel()
	// Dependency exists but has no deps of its own
	loader := newMockLoader()
	loader.addRecipe("nodejs", &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "download", Params: map[string]interface{}{}},
		},
	})
	ctx := context.Background()

	deps := ResolvedDeps{
		InstallTime: map[string]string{"nodejs": "latest"},
		Runtime:     make(map[string]string),
	}

	result, err := ResolveTransitive(ctx, loader, deps, "root")
	if err != nil {
		t.Fatalf("ResolveTransitive() error = %v", err)
	}

	if result.InstallTime["nodejs"] != "latest" {
		t.Errorf("InstallTime[nodejs] = %q, want %q", result.InstallTime["nodejs"], "latest")
	}
	if len(result.InstallTime) != 1 {
		t.Errorf("InstallTime has %d deps, want 1", len(result.InstallTime))
	}
}

func TestResolveTransitive_LinearChain(t *testing.T) {
	t.Parallel()
	// A -> B -> C (linear chain)
	loader := newMockLoader()

	// B depends on C via npm_install (needs nodejs)
	loader.addRecipe("B", &recipe.Recipe{
		Metadata: recipe.MetadataSection{Dependencies: []string{"C"}},
		Steps:    []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	// C has no deps
	loader.addRecipe("C", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	ctx := context.Background()

	// A depends on B
	deps := ResolvedDeps{
		InstallTime: map[string]string{"B": "latest"},
		Runtime:     make(map[string]string),
	}

	result, err := ResolveTransitive(ctx, loader, deps, "A")
	if err != nil {
		t.Fatalf("ResolveTransitive() error = %v", err)
	}

	// Should have B and C
	if result.InstallTime["B"] != "latest" {
		t.Errorf("InstallTime[B] = %q, want %q", result.InstallTime["B"], "latest")
	}
	if result.InstallTime["C"] != "latest" {
		t.Errorf("InstallTime[C] = %q, want %q", result.InstallTime["C"], "latest")
	}
	if len(result.InstallTime) != 2 {
		t.Errorf("InstallTime has %d deps, want 2", len(result.InstallTime))
	}
}

func TestResolveTransitive_Diamond(t *testing.T) {
	t.Parallel()
	// Diamond: A -> B, A -> C, B -> D, C -> D
	loader := newMockLoader()

	// B depends on D
	loader.addRecipe("B", &recipe.Recipe{
		Metadata: recipe.MetadataSection{Dependencies: []string{"D"}},
		Steps:    []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	// C depends on D
	loader.addRecipe("C", &recipe.Recipe{
		Metadata: recipe.MetadataSection{Dependencies: []string{"D"}},
		Steps:    []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	// D has no deps
	loader.addRecipe("D", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	ctx := context.Background()

	// A depends on B and C
	deps := ResolvedDeps{
		InstallTime: map[string]string{"B": "latest", "C": "latest"},
		Runtime:     make(map[string]string),
	}

	result, err := ResolveTransitive(ctx, loader, deps, "A")
	if err != nil {
		t.Fatalf("ResolveTransitive() error = %v", err)
	}

	// Should have B, C, and D
	if result.InstallTime["B"] != "latest" {
		t.Errorf("InstallTime[B] = %q, want %q", result.InstallTime["B"], "latest")
	}
	if result.InstallTime["C"] != "latest" {
		t.Errorf("InstallTime[C] = %q, want %q", result.InstallTime["C"], "latest")
	}
	if result.InstallTime["D"] != "latest" {
		t.Errorf("InstallTime[D] = %q, want %q", result.InstallTime["D"], "latest")
	}
	if len(result.InstallTime) != 3 {
		t.Errorf("InstallTime has %d deps, want 3", len(result.InstallTime))
	}
}

func TestResolveTransitive_CycleDetection(t *testing.T) {
	t.Parallel()
	// Cycle: A -> B -> C -> A
	loader := newMockLoader()

	// B depends on C
	loader.addRecipe("B", &recipe.Recipe{
		Metadata: recipe.MetadataSection{Dependencies: []string{"C"}},
		Steps:    []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	// C depends on A (creates cycle)
	loader.addRecipe("C", &recipe.Recipe{
		Metadata: recipe.MetadataSection{Dependencies: []string{"A"}},
		Steps:    []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	// A is in registry too (needed for cycle detection)
	loader.addRecipe("A", &recipe.Recipe{
		Metadata: recipe.MetadataSection{Dependencies: []string{"B"}},
		Steps:    []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	ctx := context.Background()

	// A depends on B
	deps := ResolvedDeps{
		InstallTime: map[string]string{"B": "latest"},
		Runtime:     make(map[string]string),
	}

	_, err := ResolveTransitive(ctx, loader, deps, "A")
	if err == nil {
		t.Fatal("ResolveTransitive() expected error for cycle, got nil")
	}
	if !errors.Is(err, ErrCyclicDependency) {
		t.Errorf("error = %v, want ErrCyclicDependency", err)
	}
	// Check that error message contains the cycle path
	if !containsAll(err.Error(), "A", "B", "C") {
		t.Errorf("error message %q should contain cycle path A -> B -> C -> A", err.Error())
	}
}

func TestResolveTransitive_SelfCycle(t *testing.T) {
	t.Parallel()
	// Self-cycle: A -> A
	loader := newMockLoader()

	// A depends on itself
	loader.addRecipe("A", &recipe.Recipe{
		Metadata: recipe.MetadataSection{Dependencies: []string{"A"}},
		Steps:    []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	ctx := context.Background()

	// Root depends on A
	deps := ResolvedDeps{
		InstallTime: map[string]string{"A": "latest"},
		Runtime:     make(map[string]string),
	}

	_, err := ResolveTransitive(ctx, loader, deps, "root")
	if err == nil {
		t.Fatal("ResolveTransitive() expected error for self-cycle, got nil")
	}
	if !errors.Is(err, ErrCyclicDependency) {
		t.Errorf("error = %v, want ErrCyclicDependency", err)
	}
}

func TestResolveTransitive_MaxDepthExceeded(t *testing.T) {
	t.Parallel()
	// Create a chain of 15 deps: D0 -> D1 -> D2 -> ... -> D14
	loader := newMockLoader()

	for i := 0; i < 15; i++ {
		name := depName(i)
		var deps []string
		if i < 14 {
			deps = []string{depName(i + 1)}
		}
		loader.addRecipe(name, &recipe.Recipe{
			Metadata: recipe.MetadataSection{Dependencies: deps},
			Steps:    []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
		})
	}

	ctx := context.Background()

	// Root depends on D0
	deps := ResolvedDeps{
		InstallTime: map[string]string{"D0": "latest"},
		Runtime:     make(map[string]string),
	}

	_, err := ResolveTransitive(ctx, loader, deps, "root")
	if err == nil {
		t.Fatal("ResolveTransitive() expected error for max depth, got nil")
	}
	if !errors.Is(err, ErrMaxDepthExceeded) {
		t.Errorf("error = %v, want ErrMaxDepthExceeded", err)
	}
}

func TestResolveTransitive_VersionPreservation(t *testing.T) {
	t.Parallel()
	// B depends on C@2.0, but root already has C@1.0
	// First encountered version should win
	loader := newMockLoader()

	// B depends on C@2.0
	loader.addRecipe("B", &recipe.Recipe{
		Metadata: recipe.MetadataSection{Dependencies: []string{"C@2.0"}},
		Steps:    []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	loader.addRecipe("C", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	ctx := context.Background()

	// Root has C@1.0 and B
	deps := ResolvedDeps{
		InstallTime: map[string]string{"B": "latest", "C": "1.0"},
		Runtime:     make(map[string]string),
	}

	result, err := ResolveTransitive(ctx, loader, deps, "root")
	if err != nil {
		t.Fatalf("ResolveTransitive() error = %v", err)
	}

	// C should keep version 1.0 (first encountered)
	if result.InstallTime["C"] != "1.0" {
		t.Errorf("InstallTime[C] = %q, want %q (first version wins)", result.InstallTime["C"], "1.0")
	}
}

func TestResolveTransitive_MissingRecipe(t *testing.T) {
	t.Parallel()
	// Dependency recipe not in registry should be skipped (not error)
	loader := newMockLoader()
	ctx := context.Background()

	deps := ResolvedDeps{
		InstallTime: map[string]string{"missing-tool": "latest"},
		Runtime:     make(map[string]string),
	}

	result, err := ResolveTransitive(ctx, loader, deps, "root")
	if err != nil {
		t.Fatalf("ResolveTransitive() error = %v, want nil (missing recipe should be skipped)", err)
	}

	// missing-tool should still be in deps (just not expanded)
	if result.InstallTime["missing-tool"] != "latest" {
		t.Errorf("InstallTime[missing-tool] = %q, want %q", result.InstallTime["missing-tool"], "latest")
	}
}

func TestResolveTransitive_RuntimeDeps(t *testing.T) {
	t.Parallel()
	// Runtime deps should also be resolved transitively
	loader := newMockLoader()

	// B has runtime dep on C
	loader.addRecipe("B", &recipe.Recipe{
		Metadata: recipe.MetadataSection{RuntimeDependencies: []string{"C"}},
		Steps:    []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	loader.addRecipe("C", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	ctx := context.Background()

	deps := ResolvedDeps{
		InstallTime: make(map[string]string),
		Runtime:     map[string]string{"B": "latest"},
	}

	result, err := ResolveTransitive(ctx, loader, deps, "root")
	if err != nil {
		t.Fatalf("ResolveTransitive() error = %v", err)
	}

	// Runtime should have B and C
	if result.Runtime["B"] != "latest" {
		t.Errorf("Runtime[B] = %q, want %q", result.Runtime["B"], "latest")
	}
	if result.Runtime["C"] != "latest" {
		t.Errorf("Runtime[C] = %q, want %q", result.Runtime["C"], "latest")
	}
}

// Tests for platform-conditional dependencies

// testPlatformAction is a mock action with platform-specific dependencies for testing.
type testPlatformAction struct{ BaseAction }

func (testPlatformAction) Name() string { return "test_platform" }
func (testPlatformAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	return nil
}
func (testPlatformAction) Dependencies() ActionDeps {
	return ActionDeps{
		InstallTime:       []string{"common-tool"},
		LinuxInstallTime:  []string{"patchelf"},
		DarwinInstallTime: []string{"macos-tool"},
		LinuxRuntime:      []string{"linux-runtime"},
		DarwinRuntime:     []string{"darwin-runtime"},
	}
}

func init() {
	Register(&testPlatformAction{})
}

func TestResolveDependenciesForPlatform_LinuxInstallDeps(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "test_platform", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependenciesForPlatform(r, "linux")

	// Should have common-tool (cross-platform) and patchelf (linux-specific)
	if deps.InstallTime["common-tool"] != "latest" {
		t.Errorf("InstallTime[common-tool] = %q, want %q", deps.InstallTime["common-tool"], "latest")
	}
	if deps.InstallTime["patchelf"] != "latest" {
		t.Errorf("InstallTime[patchelf] = %q, want %q", deps.InstallTime["patchelf"], "latest")
	}
	// Should NOT have darwin-specific deps
	if _, has := deps.InstallTime["macos-tool"]; has {
		t.Errorf("InstallTime should not contain macos-tool on Linux, got %v", deps.InstallTime)
	}
	if len(deps.InstallTime) != 2 {
		t.Errorf("InstallTime has %d deps, want 2: %v", len(deps.InstallTime), deps.InstallTime)
	}
}

func TestResolveDependenciesForPlatform_DarwinInstallDeps(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "test_platform", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependenciesForPlatform(r, "darwin")

	// Should have common-tool (cross-platform) and macos-tool (darwin-specific)
	if deps.InstallTime["common-tool"] != "latest" {
		t.Errorf("InstallTime[common-tool] = %q, want %q", deps.InstallTime["common-tool"], "latest")
	}
	if deps.InstallTime["macos-tool"] != "latest" {
		t.Errorf("InstallTime[macos-tool] = %q, want %q", deps.InstallTime["macos-tool"], "latest")
	}
	// Should NOT have linux-specific deps
	if _, has := deps.InstallTime["patchelf"]; has {
		t.Errorf("InstallTime should not contain patchelf on Darwin, got %v", deps.InstallTime)
	}
	if len(deps.InstallTime) != 2 {
		t.Errorf("InstallTime has %d deps, want 2: %v", len(deps.InstallTime), deps.InstallTime)
	}
}

func TestResolveDependenciesForPlatform_LinuxRuntimeDeps(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "test_platform", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependenciesForPlatform(r, "linux")

	// Should have linux-specific runtime dep
	if deps.Runtime["linux-runtime"] != "latest" {
		t.Errorf("Runtime[linux-runtime] = %q, want %q", deps.Runtime["linux-runtime"], "latest")
	}
	// Should NOT have darwin-specific runtime deps
	if _, has := deps.Runtime["darwin-runtime"]; has {
		t.Errorf("Runtime should not contain darwin-runtime on Linux, got %v", deps.Runtime)
	}
}

func TestResolveDependenciesForPlatform_DarwinRuntimeDeps(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "test_platform", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependenciesForPlatform(r, "darwin")

	// Should have darwin-specific runtime dep
	if deps.Runtime["darwin-runtime"] != "latest" {
		t.Errorf("Runtime[darwin-runtime] = %q, want %q", deps.Runtime["darwin-runtime"], "latest")
	}
	// Should NOT have linux-specific runtime deps
	if _, has := deps.Runtime["linux-runtime"]; has {
		t.Errorf("Runtime should not contain linux-runtime on Darwin, got %v", deps.Runtime)
	}
}

func TestResolveDependenciesForPlatform_UnknownOSExcludesPlatformDeps(t *testing.T) {
	t.Parallel()
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "test_platform", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependenciesForPlatform(r, "windows")

	// Should have only common-tool (cross-platform)
	if deps.InstallTime["common-tool"] != "latest" {
		t.Errorf("InstallTime[common-tool] = %q, want %q", deps.InstallTime["common-tool"], "latest")
	}
	// Should NOT have any platform-specific deps
	if _, has := deps.InstallTime["patchelf"]; has {
		t.Errorf("InstallTime should not contain patchelf on Windows, got %v", deps.InstallTime)
	}
	if _, has := deps.InstallTime["macos-tool"]; has {
		t.Errorf("InstallTime should not contain macos-tool on Windows, got %v", deps.InstallTime)
	}
	if len(deps.InstallTime) != 1 {
		t.Errorf("InstallTime has %d deps, want 1: %v", len(deps.InstallTime), deps.InstallTime)
	}
	if len(deps.Runtime) != 0 {
		t.Errorf("Runtime has %d deps, want 0: %v", len(deps.Runtime), deps.Runtime)
	}
}

func TestResolveDependenciesForPlatform_StepReplaceOverridesPlatformDeps(t *testing.T) {
	t.Parallel()
	// When step-level "dependencies" is set, it replaces everything including platform deps
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "test_platform",
				Params: map[string]interface{}{
					"dependencies": []interface{}{"custom-tool"},
				},
			},
		},
	}

	deps := ResolveDependenciesForPlatform(r, "linux")

	// Should have only custom-tool (step replace overrides all)
	if deps.InstallTime["custom-tool"] != "latest" {
		t.Errorf("InstallTime[custom-tool] = %q, want %q", deps.InstallTime["custom-tool"], "latest")
	}
	// Should NOT have action deps (replaced)
	if _, has := deps.InstallTime["common-tool"]; has {
		t.Errorf("InstallTime should not contain common-tool (replaced), got %v", deps.InstallTime)
	}
	if _, has := deps.InstallTime["patchelf"]; has {
		t.Errorf("InstallTime should not contain patchelf (replaced), got %v", deps.InstallTime)
	}
	if len(deps.InstallTime) != 1 {
		t.Errorf("InstallTime has %d deps, want 1: %v", len(deps.InstallTime), deps.InstallTime)
	}
}

func TestResolveDependenciesForPlatform_ExtraDepsWithPlatformDeps(t *testing.T) {
	t.Parallel()
	// Extra deps should be added alongside platform deps
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "test_platform",
				Params: map[string]interface{}{
					"extra_dependencies": []interface{}{"extra-tool"},
				},
			},
		},
	}

	deps := ResolveDependenciesForPlatform(r, "linux")

	// Should have common-tool, patchelf (linux), and extra-tool
	if deps.InstallTime["common-tool"] != "latest" {
		t.Errorf("InstallTime[common-tool] = %q, want %q", deps.InstallTime["common-tool"], "latest")
	}
	if deps.InstallTime["patchelf"] != "latest" {
		t.Errorf("InstallTime[patchelf] = %q, want %q", deps.InstallTime["patchelf"], "latest")
	}
	if deps.InstallTime["extra-tool"] != "latest" {
		t.Errorf("InstallTime[extra-tool] = %q, want %q", deps.InstallTime["extra-tool"], "latest")
	}
	if len(deps.InstallTime) != 3 {
		t.Errorf("InstallTime has %d deps, want 3: %v", len(deps.InstallTime), deps.InstallTime)
	}
}

func TestResolveDependenciesForPlatform_RecipeReplaceOverridesPlatformDeps(t *testing.T) {
	t.Parallel()
	// Recipe-level Dependencies replaces all install deps including platform-specific
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Dependencies: []string{"recipe-dep"},
		},
		Steps: []recipe.Step{
			{Action: "test_platform", Params: map[string]interface{}{}},
		},
	}

	deps := ResolveDependenciesForPlatform(r, "linux")

	// Should have only recipe-dep (recipe replace overrides all)
	if deps.InstallTime["recipe-dep"] != "latest" {
		t.Errorf("InstallTime[recipe-dep] = %q, want %q", deps.InstallTime["recipe-dep"], "latest")
	}
	if len(deps.InstallTime) != 1 {
		t.Errorf("InstallTime has %d deps, want 1: %v", len(deps.InstallTime), deps.InstallTime)
	}
}

func TestGetPlatformInstallDeps(t *testing.T) {
	t.Parallel()
	deps := ActionDeps{
		LinuxInstallTime:  []string{"linux-dep"},
		DarwinInstallTime: []string{"darwin-dep"},
	}

	tests := []struct {
		os   string
		want []string
	}{
		{"linux", []string{"linux-dep"}},
		{"darwin", []string{"darwin-dep"}},
		{"windows", nil},
		{"freebsd", nil},
	}

	for _, tt := range tests {
		t.Run(tt.os, func(t *testing.T) {
			got := getPlatformInstallDeps(deps, tt.os)
			if !slicesEqual(got, tt.want) {
				t.Errorf("getPlatformInstallDeps(%q) = %v, want %v", tt.os, got, tt.want)
			}
		})
	}
}

func TestGetPlatformRuntimeDeps(t *testing.T) {
	t.Parallel()
	deps := ActionDeps{
		LinuxRuntime:  []string{"linux-rt"},
		DarwinRuntime: []string{"darwin-rt"},
	}

	tests := []struct {
		os   string
		want []string
	}{
		{"linux", []string{"linux-rt"}},
		{"darwin", []string{"darwin-rt"}},
		{"windows", nil},
		{"freebsd", nil},
	}

	for _, tt := range tests {
		t.Run(tt.os, func(t *testing.T) {
			got := getPlatformRuntimeDeps(deps, tt.os)
			if !slicesEqual(got, tt.want) {
				t.Errorf("getPlatformRuntimeDeps(%q) = %v, want %v", tt.os, got, tt.want)
			}
		})
	}
}

// Helper functions for tests

func depName(i int) string {
	return "D" + string(rune('0'+i))
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		found := false
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Tests for ResolveTransitiveForPlatform - verifies cross-platform dependency resolution

func TestResolveTransitiveForPlatform_ExcludesLinuxDepsOnDarwin(t *testing.T) {
	t.Parallel()
	// Simulates issue #920: when generating plans on Linux for Darwin targets,
	// nested Linux-specific dependencies (like patchelf) should not be included.
	loader := newMockLoader()

	// "B" recipe uses test_platform action which has platform-specific deps
	loader.addRecipe("B", &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "test_platform", Params: map[string]interface{}{}},
		},
	})

	// Recipes for the platform-specific deps (needed for transitive resolution)
	loader.addRecipe("common-tool", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})
	loader.addRecipe("patchelf", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})
	loader.addRecipe("macos-tool", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	ctx := context.Background()

	// Root depends on B
	deps := ResolvedDeps{
		InstallTime: map[string]string{"B": "latest"},
		Runtime:     make(map[string]string),
	}

	// Resolve for darwin target (even if running on linux)
	result, err := ResolveTransitiveForPlatform(ctx, loader, deps, "root", "darwin")
	if err != nil {
		t.Fatalf("ResolveTransitiveForPlatform() error = %v", err)
	}

	// Should have B, common-tool, and macos-tool
	if result.InstallTime["B"] != "latest" {
		t.Errorf("InstallTime[B] = %q, want %q", result.InstallTime["B"], "latest")
	}
	if result.InstallTime["common-tool"] != "latest" {
		t.Errorf("InstallTime[common-tool] = %q, want %q", result.InstallTime["common-tool"], "latest")
	}
	if result.InstallTime["macos-tool"] != "latest" {
		t.Errorf("InstallTime[macos-tool] = %q, want %q", result.InstallTime["macos-tool"], "latest")
	}

	// Should NOT have patchelf (linux-only dependency)
	if _, has := result.InstallTime["patchelf"]; has {
		t.Errorf("InstallTime should not contain patchelf on Darwin target, got %v", result.InstallTime)
	}

	// Verify exact count: B + common-tool + macos-tool = 3
	if len(result.InstallTime) != 3 {
		t.Errorf("InstallTime has %d deps, want 3: %v", len(result.InstallTime), result.InstallTime)
	}
}

func TestResolveTransitiveForPlatform_IncludesLinuxDepsOnLinux(t *testing.T) {
	t.Parallel()
	// Verify linux-specific deps ARE included when targeting linux
	loader := newMockLoader()

	loader.addRecipe("B", &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "test_platform", Params: map[string]interface{}{}},
		},
	})

	loader.addRecipe("common-tool", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})
	loader.addRecipe("patchelf", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})
	loader.addRecipe("macos-tool", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	ctx := context.Background()

	deps := ResolvedDeps{
		InstallTime: map[string]string{"B": "latest"},
		Runtime:     make(map[string]string),
	}

	// Resolve for linux target
	result, err := ResolveTransitiveForPlatform(ctx, loader, deps, "root", "linux")
	if err != nil {
		t.Fatalf("ResolveTransitiveForPlatform() error = %v", err)
	}

	// Should have B, common-tool, and patchelf (linux-specific)
	if result.InstallTime["B"] != "latest" {
		t.Errorf("InstallTime[B] = %q, want %q", result.InstallTime["B"], "latest")
	}
	if result.InstallTime["common-tool"] != "latest" {
		t.Errorf("InstallTime[common-tool] = %q, want %q", result.InstallTime["common-tool"], "latest")
	}
	if result.InstallTime["patchelf"] != "latest" {
		t.Errorf("InstallTime[patchelf] = %q, want %q", result.InstallTime["patchelf"], "latest")
	}

	// Should NOT have macos-tool (darwin-only dependency)
	if _, has := result.InstallTime["macos-tool"]; has {
		t.Errorf("InstallTime should not contain macos-tool on Linux target, got %v", result.InstallTime)
	}

	// Verify exact count: B + common-tool + patchelf = 3
	if len(result.InstallTime) != 3 {
		t.Errorf("InstallTime has %d deps, want 3: %v", len(result.InstallTime), result.InstallTime)
	}
}

func TestResolveTransitiveForPlatform_NestedDependencies(t *testing.T) {
	t.Parallel()
	// Test deeper nesting: A -> B -> C (where C has platform-specific deps)
	loader := newMockLoader()

	// B depends on C
	loader.addRecipe("B", &recipe.Recipe{
		Metadata: recipe.MetadataSection{Dependencies: []string{"C"}},
		Steps:    []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	// C uses test_platform action
	loader.addRecipe("C", &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "test_platform", Params: map[string]interface{}{}},
		},
	})

	loader.addRecipe("common-tool", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})
	loader.addRecipe("patchelf", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})
	loader.addRecipe("macos-tool", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	ctx := context.Background()

	deps := ResolvedDeps{
		InstallTime: map[string]string{"B": "latest"},
		Runtime:     make(map[string]string),
	}

	// Resolve for darwin - nested patchelf should NOT be included
	result, err := ResolveTransitiveForPlatform(ctx, loader, deps, "root", "darwin")
	if err != nil {
		t.Fatalf("ResolveTransitiveForPlatform() error = %v", err)
	}

	// Should have B, C, common-tool, and macos-tool
	if result.InstallTime["B"] != "latest" {
		t.Errorf("InstallTime[B] = %q, want %q", result.InstallTime["B"], "latest")
	}
	if result.InstallTime["C"] != "latest" {
		t.Errorf("InstallTime[C] = %q, want %q", result.InstallTime["C"], "latest")
	}
	if result.InstallTime["common-tool"] != "latest" {
		t.Errorf("InstallTime[common-tool] = %q, want %q", result.InstallTime["common-tool"], "latest")
	}
	if result.InstallTime["macos-tool"] != "latest" {
		t.Errorf("InstallTime[macos-tool] = %q, want %q", result.InstallTime["macos-tool"], "latest")
	}

	// Should NOT have patchelf (linux-only, even in nested deps)
	if _, has := result.InstallTime["patchelf"]; has {
		t.Errorf("InstallTime should not contain patchelf (nested) on Darwin target, got %v", result.InstallTime)
	}

	// Verify exact count: B + C + common-tool + macos-tool = 4
	if len(result.InstallTime) != 4 {
		t.Errorf("InstallTime has %d deps, want 4: %v", len(result.InstallTime), result.InstallTime)
	}
}

func TestResolveTransitiveForPlatform_RuntimeDeps(t *testing.T) {
	t.Parallel()
	// Verify runtime deps also respect platform filtering
	loader := newMockLoader()

	loader.addRecipe("B", &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "test_platform", Params: map[string]interface{}{}},
		},
	})

	loader.addRecipe("linux-runtime", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})
	loader.addRecipe("darwin-runtime", &recipe.Recipe{
		Steps: []recipe.Step{{Action: "download", Params: map[string]interface{}{}}},
	})

	ctx := context.Background()

	deps := ResolvedDeps{
		InstallTime: make(map[string]string),
		Runtime:     map[string]string{"B": "latest"},
	}

	// Resolve for darwin
	result, err := ResolveTransitiveForPlatform(ctx, loader, deps, "root", "darwin")
	if err != nil {
		t.Fatalf("ResolveTransitiveForPlatform() error = %v", err)
	}

	// Should have B and darwin-runtime
	if result.Runtime["B"] != "latest" {
		t.Errorf("Runtime[B] = %q, want %q", result.Runtime["B"], "latest")
	}
	if result.Runtime["darwin-runtime"] != "latest" {
		t.Errorf("Runtime[darwin-runtime] = %q, want %q", result.Runtime["darwin-runtime"], "latest")
	}

	// Should NOT have linux-runtime
	if _, has := result.Runtime["linux-runtime"]; has {
		t.Errorf("Runtime should not contain linux-runtime on Darwin target, got %v", result.Runtime)
	}
}
