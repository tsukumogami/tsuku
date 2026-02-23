package builders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// --- BinaryNameProvider tests for CargoBuilder ---

func TestCargoBuilder_AuthoritativeBinaryNames_AfterBuild(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "sqlx-cli",
			"description": "Command-line utility for SQLx",
			"homepage": "",
			"repository": "https://github.com/launchbadge/sqlx"
		},
		"versions": [
			{"bin_names": ["sqlx", "cargo-sqlx"], "yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(crateResponse))
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)

	// Before Build, should return nil
	if names := builder.AuthoritativeBinaryNames(); names != nil {
		t.Errorf("before Build(), AuthoritativeBinaryNames() = %v, want nil", names)
	}

	_, err := builder.Build(context.Background(), BuildRequest{Package: "sqlx-cli"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	names := builder.AuthoritativeBinaryNames()
	if len(names) != 2 {
		t.Fatalf("AuthoritativeBinaryNames() returned %d names, want 2: %v", len(names), names)
	}
	if names[0] != "sqlx" || names[1] != "cargo-sqlx" {
		t.Errorf("AuthoritativeBinaryNames() = %v, want [sqlx, cargo-sqlx]", names)
	}
}

func TestCargoBuilder_AuthoritativeBinaryNames_EmptyBinNames(t *testing.T) {
	crateResponse := `{
		"crate": {"name": "some-lib", "description": "lib", "homepage": "", "repository": ""},
		"versions": [{"bin_names": [], "yanked": false}]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(crateResponse))
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Build(context.Background(), BuildRequest{Package: "some-lib"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Empty bin_names should return an empty slice (not nil, but no entries)
	names := builder.AuthoritativeBinaryNames()
	if len(names) != 0 {
		t.Errorf("AuthoritativeBinaryNames() = %v, want empty", names)
	}
}

func TestCargoBuilder_AuthoritativeBinaryNames_SkipsYanked(t *testing.T) {
	crateResponse := `{
		"crate": {"name": "evolving", "description": "test", "homepage": "", "repository": ""},
		"versions": [
			{"bin_names": ["yanked-bin"], "yanked": true},
			{"bin_names": ["correct-bin"], "yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(crateResponse))
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Build(context.Background(), BuildRequest{Package: "evolving"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	names := builder.AuthoritativeBinaryNames()
	if len(names) != 1 || names[0] != "correct-bin" {
		t.Errorf("AuthoritativeBinaryNames() = %v, want [correct-bin]", names)
	}
}

func TestCargoBuilder_AuthoritativeBinaryNames_FiltersInvalid(t *testing.T) {
	crateResponse := `{
		"crate": {"name": "mixed", "description": "test", "homepage": "", "repository": ""},
		"versions": [
			{"bin_names": ["good-bin", "; evil", "also-good"], "yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(crateResponse))
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Build(context.Background(), BuildRequest{Package: "mixed"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	names := builder.AuthoritativeBinaryNames()
	if len(names) != 2 {
		t.Fatalf("AuthoritativeBinaryNames() returned %d names, want 2: %v", len(names), names)
	}
	if names[0] != "good-bin" || names[1] != "also-good" {
		t.Errorf("AuthoritativeBinaryNames() = %v, want [good-bin, also-good]", names)
	}
}

// --- BinaryNameProvider tests for NpmBuilder ---

func TestNpmBuilder_AuthoritativeBinaryNames_MapBin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "typescript",
			"description": "TypeScript language",
			"dist-tags": {"latest": "5.0.0"},
			"versions": {"5.0.0": {"bin": {"tsc": "bin/tsc", "tsserver": "bin/tsserver"}}}
		}`))
	}))
	defer server.Close()

	builder := NewNpmBuilderWithBaseURL(nil, server.URL)

	// Before Build, should return nil
	if names := builder.AuthoritativeBinaryNames(); names != nil {
		t.Errorf("before Build(), AuthoritativeBinaryNames() = %v, want nil", names)
	}

	_, err := builder.Build(context.Background(), BuildRequest{Package: "typescript"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	names := builder.AuthoritativeBinaryNames()
	if len(names) != 2 {
		t.Fatalf("AuthoritativeBinaryNames() returned %d names, want 2: %v", len(names), names)
	}

	sort.Strings(names)
	if names[0] != "tsc" || names[1] != "tsserver" {
		t.Errorf("AuthoritativeBinaryNames() = %v, want [tsc, tsserver]", names)
	}
}

func TestNpmBuilder_AuthoritativeBinaryNames_StringBin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "my-tool",
			"description": "A tool",
			"dist-tags": {"latest": "1.0.0"},
			"versions": {"1.0.0": {"bin": "./bin/tool.js"}}
		}`))
	}))
	defer server.Close()

	builder := NewNpmBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Build(context.Background(), BuildRequest{Package: "my-tool"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	names := builder.AuthoritativeBinaryNames()
	if len(names) != 1 || names[0] != "my-tool" {
		t.Errorf("AuthoritativeBinaryNames() = %v, want [my-tool]", names)
	}
}

func TestNpmBuilder_AuthoritativeBinaryNames_ScopedStringBin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "@scope/tool",
			"description": "A scoped tool",
			"dist-tags": {"latest": "2.0.0"},
			"versions": {"2.0.0": {"bin": "./bin/tool.js"}}
		}`))
	}))
	defer server.Close()

	builder := NewNpmBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Build(context.Background(), BuildRequest{Package: "@scope/tool"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	names := builder.AuthoritativeBinaryNames()
	if len(names) != 1 || names[0] != "tool" {
		t.Errorf("AuthoritativeBinaryNames() = %v, want [tool]", names)
	}
}

func TestNpmBuilder_AuthoritativeBinaryNames_NoBin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "no-bin",
			"description": "No bin",
			"dist-tags": {"latest": "1.0.0"},
			"versions": {"1.0.0": {}}
		}`))
	}))
	defer server.Close()

	builder := NewNpmBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Build(context.Background(), BuildRequest{Package: "no-bin"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	names := builder.AuthoritativeBinaryNames()
	if len(names) != 0 {
		t.Errorf("AuthoritativeBinaryNames() = %v, want empty", names)
	}
}

func TestNpmBuilder_AuthoritativeBinaryNames_NoLatestVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"name": "no-latest",
			"description": "No latest tag",
			"dist-tags": {},
			"versions": {}
		}`))
	}))
	defer server.Close()

	builder := NewNpmBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Build(context.Background(), BuildRequest{Package: "no-latest"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	names := builder.AuthoritativeBinaryNames()
	if names != nil {
		t.Errorf("AuthoritativeBinaryNames() = %v, want nil", names)
	}
}

// --- Interface compliance ---

func TestCargoBuilder_ImplementsBinaryNameProvider(t *testing.T) {
	var _ BinaryNameProvider = (*CargoBuilder)(nil)
}

func TestNpmBuilder_ImplementsBinaryNameProvider(t *testing.T) {
	var _ BinaryNameProvider = (*NpmBuilder)(nil)
}

// --- validateBinaryNames tests ---

func TestValidateBinaryNames_Match_NoChange(t *testing.T) {
	orch := NewOrchestrator()
	result := &BuildResult{
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{Name: "ripgrep"},
			Steps: []recipe.Step{
				{
					Action: "cargo_install",
					Params: map[string]interface{}{
						"crate":       "ripgrep",
						"executables": []string{"rg"},
					},
				},
			},
			Verify: &recipe.VerifySection{Command: "rg --version"},
		},
		Warnings: []string{},
	}

	provider := &mockBinaryNameProvider{names: []string{"rg"}}
	meta := orch.validateBinaryNames(provider, result, "crates.io")

	if meta != nil {
		t.Errorf("validateBinaryNames() returned metadata %+v, want nil (no change needed)", meta)
	}

	// Recipe should be unchanged
	executables := result.Recipe.Steps[0].Params["executables"].([]string)
	if len(executables) != 1 || executables[0] != "rg" {
		t.Errorf("executables = %v, want [rg]", executables)
	}

	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
}

func TestValidateBinaryNames_Mismatch_Correction(t *testing.T) {
	orch := NewOrchestrator()
	result := &BuildResult{
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{Name: "sqlx-cli"},
			Steps: []recipe.Step{
				{
					Action: "cargo_install",
					Params: map[string]interface{}{
						"crate":       "sqlx-cli",
						"executables": []string{"sqlx-cli"},
					},
				},
			},
			Verify: &recipe.VerifySection{Command: "sqlx-cli --version"},
		},
		Warnings: []string{},
	}

	provider := &mockBinaryNameProvider{names: []string{"sqlx", "cargo-sqlx"}}
	meta := orch.validateBinaryNames(provider, result, "crates.io")

	if meta == nil {
		t.Fatal("validateBinaryNames() returned nil, want correction metadata")
	}

	if len(meta.OldNames) != 1 || meta.OldNames[0] != "sqlx-cli" {
		t.Errorf("meta.OldNames = %v, want [sqlx-cli]", meta.OldNames)
	}
	if len(meta.NewNames) != 2 || meta.NewNames[0] != "sqlx" {
		t.Errorf("meta.NewNames = %v, want [sqlx, cargo-sqlx]", meta.NewNames)
	}
	if meta.Builder != "crates.io" {
		t.Errorf("meta.Builder = %q, want %q", meta.Builder, "crates.io")
	}

	// Recipe executables should be corrected
	executables := result.Recipe.Steps[0].Params["executables"].([]string)
	if len(executables) != 2 || executables[0] != "sqlx" {
		t.Errorf("corrected executables = %v, want [sqlx, cargo-sqlx]", executables)
	}

	// Verify command should be updated
	if result.Recipe.Verify.Command != "sqlx --version" {
		t.Errorf("Verify.Command = %q, want %q", result.Recipe.Verify.Command, "sqlx --version")
	}

	// Warning should be added
	if len(result.Warnings) == 0 {
		t.Error("expected warning about correction")
	}
}

func TestValidateBinaryNames_EmptyProvider_Skip(t *testing.T) {
	orch := NewOrchestrator()
	result := &BuildResult{
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{Name: "some-tool"},
			Steps: []recipe.Step{
				{
					Action: "cargo_install",
					Params: map[string]interface{}{
						"crate":       "some-tool",
						"executables": []string{"some-tool"},
					},
				},
			},
			Verify: &recipe.VerifySection{Command: "some-tool --version"},
		},
		Warnings: []string{},
	}

	provider := &mockBinaryNameProvider{names: []string{}}
	meta := orch.validateBinaryNames(provider, result, "crates.io")

	if meta != nil {
		t.Errorf("validateBinaryNames() returned metadata %+v, want nil (empty provider)", meta)
	}

	// Recipe should be unchanged
	executables := result.Recipe.Steps[0].Params["executables"].([]string)
	if executables[0] != "some-tool" {
		t.Errorf("executables should be unchanged, got %v", executables)
	}
}

func TestValidateBinaryNames_NilProvider_Skip(t *testing.T) {
	orch := NewOrchestrator()
	result := &BuildResult{
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{Name: "some-tool"},
			Steps: []recipe.Step{
				{
					Action: "cargo_install",
					Params: map[string]interface{}{
						"crate":       "some-tool",
						"executables": []string{"some-tool"},
					},
				},
			},
			Verify: &recipe.VerifySection{Command: "some-tool --version"},
		},
		Warnings: []string{},
	}

	provider := &mockBinaryNameProvider{names: nil}
	meta := orch.validateBinaryNames(provider, result, "crates.io")

	if meta != nil {
		t.Errorf("validateBinaryNames() returned metadata %+v, want nil (nil names)", meta)
	}
}

func TestValidateBinaryNames_NoExecutablesParam_Skip(t *testing.T) {
	orch := NewOrchestrator()
	result := &BuildResult{
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{Name: "some-tool"},
			Steps: []recipe.Step{
				{
					Action: "install_binaries",
					Params: map[string]interface{}{
						"binaries": []string{"bin/tool"},
					},
				},
			},
			Verify: &recipe.VerifySection{Command: "tool --version"},
		},
		Warnings: []string{},
	}

	provider := &mockBinaryNameProvider{names: []string{"tool"}}
	meta := orch.validateBinaryNames(provider, result, "crates.io")

	if meta != nil {
		t.Errorf("validateBinaryNames() returned metadata %+v, want nil (no executables param)", meta)
	}
}

func TestValidateBinaryNames_SameNamesOrderDiffers_NoChange(t *testing.T) {
	orch := NewOrchestrator()
	result := &BuildResult{
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{Name: "multi-bin"},
			Steps: []recipe.Step{
				{
					Action: "cargo_install",
					Params: map[string]interface{}{
						"crate":       "multi-bin",
						"executables": []string{"b", "a", "c"},
					},
				},
			},
			Verify: &recipe.VerifySection{Command: "b --version"},
		},
		Warnings: []string{},
	}

	// Same names but different order
	provider := &mockBinaryNameProvider{names: []string{"c", "a", "b"}}
	meta := orch.validateBinaryNames(provider, result, "crates.io")

	if meta != nil {
		t.Errorf("validateBinaryNames() returned metadata %+v, want nil (same set)", meta)
	}
}

func TestValidateBinaryNames_InvalidProviderNames_Skip(t *testing.T) {
	orch := NewOrchestrator()
	result := &BuildResult{
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{Name: "tool"},
			Steps: []recipe.Step{
				{
					Action: "cargo_install",
					Params: map[string]interface{}{
						"crate":       "tool",
						"executables": []string{"tool"},
					},
				},
			},
			Verify: &recipe.VerifySection{Command: "tool --version"},
		},
		Warnings: []string{},
	}

	// All provider names are invalid
	provider := &mockBinaryNameProvider{names: []string{"; rm -rf /", "$(whoami)"}}
	meta := orch.validateBinaryNames(provider, result, "crates.io")

	if meta != nil {
		t.Errorf("validateBinaryNames() returned metadata %+v, want nil (all invalid)", meta)
	}
}

func TestValidateBinaryNames_InterfaceExtractExecutables(t *testing.T) {
	orch := NewOrchestrator()
	// Test with []interface{} type (as TOML deserialization would produce)
	result := &BuildResult{
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{Name: "tool"},
			Steps: []recipe.Step{
				{
					Action: "npm_install",
					Params: map[string]interface{}{
						"package":     "tool",
						"executables": []interface{}{"old-name"},
					},
				},
			},
			Verify: &recipe.VerifySection{Command: "old-name --version"},
		},
		Warnings: []string{},
	}

	provider := &mockBinaryNameProvider{names: []string{"new-name"}}
	meta := orch.validateBinaryNames(provider, result, "npm")

	if meta == nil {
		t.Fatal("validateBinaryNames() returned nil, want correction metadata")
	}

	if meta.OldNames[0] != "old-name" {
		t.Errorf("meta.OldNames = %v, want [old-name]", meta.OldNames)
	}
}

// --- Helper: extractExecutablesFromStep ---

func TestExtractExecutablesFromStep_StringSlice(t *testing.T) {
	step := recipe.Step{
		Params: map[string]interface{}{
			"executables": []string{"a", "b"},
		},
	}
	got := extractExecutablesFromStep(step)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("extractExecutablesFromStep() = %v, want [a, b]", got)
	}
}

func TestExtractExecutablesFromStep_InterfaceSlice(t *testing.T) {
	step := recipe.Step{
		Params: map[string]interface{}{
			"executables": []interface{}{"x", "y"},
		},
	}
	got := extractExecutablesFromStep(step)
	if len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Errorf("extractExecutablesFromStep() = %v, want [x, y]", got)
	}
}

func TestExtractExecutablesFromStep_Missing(t *testing.T) {
	step := recipe.Step{
		Params: map[string]interface{}{
			"binaries": []string{"a"},
		},
	}
	got := extractExecutablesFromStep(step)
	if got != nil {
		t.Errorf("extractExecutablesFromStep() = %v, want nil", got)
	}
}

// --- Helper: executableSetsEqual ---

func TestExecutableSetsEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want bool
	}{
		{"equal same order", []string{"a", "b"}, []string{"a", "b"}, true},
		{"equal diff order", []string{"b", "a"}, []string{"a", "b"}, true},
		{"different length", []string{"a"}, []string{"a", "b"}, false},
		{"different content", []string{"a", "b"}, []string{"a", "c"}, false},
		{"both empty", []string{}, []string{}, true},
		{"single equal", []string{"x"}, []string{"x"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := executableSetsEqual(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("executableSetsEqual(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// --- Orchestrator.Create integration with BinaryNameProvider ---

func TestOrchestratorCreate_TypeAssertsToProvider(t *testing.T) {
	// Verify that Create() correctly type-asserts the builder before session creation.
	// We use a mock builder that implements BinaryNameProvider and produces wrong names,
	// and verify the correction happens.
	builder := &mockProviderBuilder{
		buildFunc: func(ctx context.Context, req BuildRequest) (*BuildResult, error) {
			return &BuildResult{
				Recipe: &recipe.Recipe{
					Metadata: recipe.MetadataSection{Name: "test-tool"},
					Steps: []recipe.Step{
						{
							Action: "cargo_install",
							Params: map[string]interface{}{
								"crate":       "test-tool",
								"executables": []string{"wrong-name"},
							},
						},
					},
					Verify: &recipe.VerifySection{Command: "wrong-name --version"},
				},
				Warnings: []string{},
			}, nil
		},
		authoritativeNames: []string{"correct-name"},
	}

	orch := NewOrchestrator(
		WithOrchestratorConfig(OrchestratorConfig{SkipSandbox: true}),
	)

	result, err := orch.Create(context.Background(), builder, BuildRequest{Package: "test-tool"}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	executables := result.Recipe.Steps[0].Params["executables"].([]string)
	if len(executables) != 1 || executables[0] != "correct-name" {
		t.Errorf("executables after correction = %v, want [correct-name]", executables)
	}

	if result.Recipe.Verify.Command != "correct-name --version" {
		t.Errorf("Verify.Command = %q, want %q", result.Recipe.Verify.Command, "correct-name --version")
	}
}

func TestOrchestratorCreate_NonProviderBuilder_SkipsValidation(t *testing.T) {
	// Verify that builders not implementing BinaryNameProvider don't trigger validation.
	builder := &mockNonProviderBuilder{
		buildFunc: func(ctx context.Context, req BuildRequest) (*BuildResult, error) {
			return &BuildResult{
				Recipe: &recipe.Recipe{
					Metadata: recipe.MetadataSection{Name: "test-tool"},
					Steps: []recipe.Step{
						{
							Action: "install_binaries",
							Params: map[string]interface{}{
								"binaries": []string{"bin/tool"},
							},
						},
					},
					Verify: &recipe.VerifySection{Command: "tool --version"},
				},
				Warnings: []string{},
			}, nil
		},
	}

	orch := NewOrchestrator(
		WithOrchestratorConfig(OrchestratorConfig{SkipSandbox: true}),
	)

	result, err := orch.Create(context.Background(), builder, BuildRequest{Package: "test-tool"}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// No correction should have happened
	if len(result.BuildResult.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.BuildResult.Warnings)
	}
}

// --- Mock types ---

type mockBinaryNameProvider struct {
	names []string
}

func (m *mockBinaryNameProvider) AuthoritativeBinaryNames() []string {
	return m.names
}

// mockProviderBuilder implements both SessionBuilder and BinaryNameProvider.
type mockProviderBuilder struct {
	buildFunc          func(ctx context.Context, req BuildRequest) (*BuildResult, error)
	authoritativeNames []string
}

func (m *mockProviderBuilder) Name() string      { return "mock-provider" }
func (m *mockProviderBuilder) RequiresLLM() bool { return false }
func (m *mockProviderBuilder) CanBuild(ctx context.Context, req BuildRequest) (bool, error) {
	return true, nil
}
func (m *mockProviderBuilder) NewSession(ctx context.Context, req BuildRequest, opts *SessionOptions) (BuildSession, error) {
	return NewDeterministicSession(m.buildFunc, req), nil
}
func (m *mockProviderBuilder) AuthoritativeBinaryNames() []string {
	return m.authoritativeNames
}

// mockNonProviderBuilder implements SessionBuilder but NOT BinaryNameProvider.
type mockNonProviderBuilder struct {
	buildFunc func(ctx context.Context, req BuildRequest) (*BuildResult, error)
}

func (m *mockNonProviderBuilder) Name() string      { return "mock-non-provider" }
func (m *mockNonProviderBuilder) RequiresLLM() bool { return false }
func (m *mockNonProviderBuilder) CanBuild(ctx context.Context, req BuildRequest) (bool, error) {
	return true, nil
}
func (m *mockNonProviderBuilder) NewSession(ctx context.Context, req BuildRequest, opts *SessionOptions) (BuildSession, error) {
	return NewDeterministicSession(m.buildFunc, req), nil
}
