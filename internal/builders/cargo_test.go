package builders

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCargoBuilder_Name(t *testing.T) {
	builder := NewCargoBuilder(nil)
	if builder.Name() != "crates.io" {
		t.Errorf("Name() = %q, want %q", builder.Name(), "crates.io")
	}
}

func TestCargoBuilder_CanBuild_ValidCrate(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "ripgrep",
			"description": "ripgrep recursively searches directories for a regex pattern",
			"homepage": "https://github.com/BurntSushi/ripgrep",
			"repository": "https://github.com/BurntSushi/ripgrep"
		},
		"versions": [{"bin_names": ["rg"], "yanked": false}]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/ripgrep" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	canBuild, err := builder.CanBuild(ctx, BuildRequest{Package: "ripgrep"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if !canBuild {
		t.Error("CanBuild() = false, want true")
	}
}

func TestCargoBuilder_CanBuild_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	canBuild, err := builder.CanBuild(ctx, BuildRequest{Package: "nonexistent-crate"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Error("CanBuild() = true, want false for nonexistent crate")
	}
}

func TestCargoBuilder_CanBuild_InvalidCrateName(t *testing.T) {
	builder := NewCargoBuilder(nil)
	ctx := context.Background()

	canBuild, err := builder.CanBuild(ctx, BuildRequest{Package: "invalid crate name!"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Error("CanBuild() = true, want false for invalid crate name")
	}
}

// TestCargoBuilder_Build_WithBinNames verifies that the builder reads bin_names
// from the crates.io version API response to discover executables.
func TestCargoBuilder_Build_WithBinNames(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "ripgrep",
			"description": "ripgrep recursively searches directories for a regex pattern",
			"homepage": "",
			"repository": "https://github.com/BurntSushi/ripgrep"
		},
		"versions": [
			{"bin_names": ["rg"], "yanked": false},
			{"bin_names": ["rg"], "yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/ripgrep" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Build(ctx, BuildRequest{Package: "ripgrep"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if result.Recipe == nil {
		t.Fatal("Build() result.Recipe is nil")
	}

	if result.Recipe.Metadata.Name != "ripgrep" {
		t.Errorf("Recipe.Metadata.Name = %q, want %q", result.Recipe.Metadata.Name, "ripgrep")
	}

	if result.Recipe.Metadata.Description != "ripgrep recursively searches directories for a regex pattern" {
		t.Errorf("Recipe.Metadata.Description = %q", result.Recipe.Metadata.Description)
	}

	if result.Recipe.Version.Source != "" {
		t.Errorf("Recipe.Version.Source = %q, want empty (inferred from action)", result.Recipe.Version.Source)
	}

	if len(result.Recipe.Steps) != 1 {
		t.Fatalf("len(Recipe.Steps) = %d, want 1", len(result.Recipe.Steps))
	}

	if result.Recipe.Steps[0].Action != "cargo_install" {
		t.Errorf("Recipe.Steps[0].Action = %q, want %q", result.Recipe.Steps[0].Action, "cargo_install")
	}

	// Verify executables come from bin_names
	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "rg" {
		t.Errorf("executables = %v, want [\"rg\"]", executables)
	}

	// Verify command uses the binary name from bin_names, not the crate name
	if result.Recipe.Verify.Command != "rg --version" {
		t.Errorf("Verify.Command = %q, want %q", result.Recipe.Verify.Command, "rg --version")
	}

	if result.Source != "crates.io:ripgrep" {
		t.Errorf("result.Source = %q, want %q", result.Source, "crates.io:ripgrep")
	}
}

// TestCargoBuilder_Build_SqlxCli verifies that the workspace monorepo crate
// sqlx-cli produces the correct binary names from the crates.io bin_names API.
func TestCargoBuilder_Build_SqlxCli(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "sqlx-cli",
			"description": "Command-line utility for SQLx",
			"homepage": "",
			"repository": "https://github.com/launchbadge/sqlx"
		},
		"versions": [
			{"bin_names": ["sqlx", "cargo-sqlx"], "yanked": false},
			{"bin_names": ["sqlx", "cargo-sqlx"], "yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/sqlx-cli" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "sqlx-cli"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}

	// sqlx-cli produces "sqlx" and "cargo-sqlx", not "sqlx-cli"
	if len(executables) != 2 {
		t.Fatalf("expected 2 executables, got %d: %v", len(executables), executables)
	}
	if executables[0] != "sqlx" {
		t.Errorf("executables[0] = %q, want %q", executables[0], "sqlx")
	}
	if executables[1] != "cargo-sqlx" {
		t.Errorf("executables[1] = %q, want %q", executables[1], "cargo-sqlx")
	}

	// Verify command should use the first binary (sqlx), not the crate name
	if result.Recipe.Verify.Command != "sqlx --version" {
		t.Errorf("Verify.Command = %q, want %q", result.Recipe.Verify.Command, "sqlx --version")
	}
}

// TestCargoBuilder_Build_ProbeRsTools verifies a multi-binary workspace crate.
func TestCargoBuilder_Build_ProbeRsTools(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "probe-rs-tools",
			"description": "Probe-rs tools for embedded development",
			"homepage": "",
			"repository": "https://github.com/probe-rs/probe-rs"
		},
		"versions": [
			{"bin_names": ["probe-rs", "cargo-flash", "cargo-embed"], "yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/probe-rs-tools" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "probe-rs-tools"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}

	if len(executables) != 3 {
		t.Fatalf("expected 3 executables, got %d: %v", len(executables), executables)
	}
	want := []string{"probe-rs", "cargo-flash", "cargo-embed"}
	for i, w := range want {
		if executables[i] != w {
			t.Errorf("executables[%d] = %q, want %q", i, executables[i], w)
		}
	}

	// First executable is probe-rs, which is a regular binary
	if result.Recipe.Verify.Command != "probe-rs --version" {
		t.Errorf("Verify.Command = %q, want %q", result.Recipe.Verify.Command, "probe-rs --version")
	}
}

// TestCargoBuilder_Build_FdFind verifies that fd-find produces binary "fd".
func TestCargoBuilder_Build_FdFind(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "fd-find",
			"description": "A simple, fast and user-friendly alternative to find",
			"homepage": "",
			"repository": "https://github.com/sharkdp/fd"
		},
		"versions": [
			{"bin_names": ["fd"], "yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/fd-find" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "fd-find"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}

	// fd-find produces "fd", not "fd-find"
	if len(executables) != 1 || executables[0] != "fd" {
		t.Errorf("executables = %v, want [\"fd\"]", executables)
	}

	if result.Recipe.Verify.Command != "fd --version" {
		t.Errorf("Verify.Command = %q, want %q", result.Recipe.Verify.Command, "fd --version")
	}
}

// TestCargoBuilder_Build_EmptyBinNamesFallbackToCrateName verifies that when
// bin_names is empty (library-only crate), the builder falls back to the crate name.
func TestCargoBuilder_Build_EmptyBinNamesFallbackToCrateName(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "some-tool",
			"description": "A tool",
			"homepage": "",
			"repository": ""
		},
		"versions": [
			{"bin_names": [], "yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/some-tool" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Build(ctx, BuildRequest{Package: "some-tool"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should have warning about empty bin_names
	hasWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "bin_names") {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Error("Expected warning about empty bin_names")
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "some-tool" {
		t.Errorf("executables = %v, want [\"some-tool\"]", executables)
	}

	if result.Recipe.Verify.Command != "some-tool --version" {
		t.Errorf("Verify.Command = %q, want %q", result.Recipe.Verify.Command, "some-tool --version")
	}
}

// TestCargoBuilder_Build_NullBinNamesFallbackToCrateName verifies that when
// bin_names is null (not present in JSON), the builder falls back to the crate name.
func TestCargoBuilder_Build_NullBinNamesFallbackToCrateName(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "some-lib",
			"description": "A library",
			"homepage": "",
			"repository": ""
		},
		"versions": [
			{"yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/some-lib" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "some-lib"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "some-lib" {
		t.Errorf("executables = %v, want [\"some-lib\"]", executables)
	}
}

// TestCargoBuilder_Build_AllVersionsYankedFallbackToCrateName verifies that when
// all versions are yanked, the builder falls back to the crate name.
func TestCargoBuilder_Build_AllVersionsYankedFallbackToCrateName(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "yanked-crate",
			"description": "A yanked crate",
			"homepage": "",
			"repository": ""
		},
		"versions": [
			{"bin_names": ["bad-binary"], "yanked": true},
			{"bin_names": ["old-binary"], "yanked": true}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/yanked-crate" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "yanked-crate"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should have warning about all versions yanked
	hasYankedWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "yanked") || strings.Contains(w, "Yanked") {
			hasYankedWarning = true
			break
		}
	}
	if !hasYankedWarning {
		t.Errorf("Expected warning about yanked versions, got warnings: %v", result.Warnings)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "yanked-crate" {
		t.Errorf("executables = %v, want [\"yanked-crate\"]", executables)
	}
}

// TestCargoBuilder_Build_InvalidBinNamesFiltered verifies that bin_names entries
// containing invalid executable names are filtered out.
func TestCargoBuilder_Build_InvalidBinNamesFiltered(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "mixed-bins",
			"description": "A crate with some invalid bin names",
			"homepage": "",
			"repository": ""
		},
		"versions": [
			{"bin_names": ["good-binary", "; rm -rf /", "also-good", "$(whoami)"], "yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/mixed-bins" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "mixed-bins"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}

	// Only valid names should pass through
	if len(executables) != 2 {
		t.Fatalf("expected 2 valid executables, got %d: %v", len(executables), executables)
	}
	if executables[0] != "good-binary" || executables[1] != "also-good" {
		t.Errorf("executables = %v, want [\"good-binary\", \"also-good\"]", executables)
	}

	// Should have warnings about the filtered names
	invalidWarnings := 0
	for _, w := range result.Warnings {
		if strings.Contains(w, "invalid executable") || strings.Contains(w, "Skipping invalid") {
			invalidWarnings++
		}
	}
	if invalidWarnings != 2 {
		t.Errorf("expected 2 warnings about invalid executable names, got %d. Warnings: %v", invalidWarnings, result.Warnings)
	}
}

// TestCargoBuilder_Build_AllBinNamesInvalidFallbackToCrateName verifies that
// when all bin_names are invalid, the builder falls back to the crate name.
func TestCargoBuilder_Build_AllBinNamesInvalidFallbackToCrateName(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "bad-names",
			"description": "A crate with all invalid bin names",
			"homepage": "",
			"repository": ""
		},
		"versions": [
			{"bin_names": ["; rm -rf /", "$(whoami)"], "yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/bad-names" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "bad-names"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "bad-names" {
		t.Errorf("executables = %v, want [\"bad-names\"]", executables)
	}
}

// TestCargoBuilder_Build_SkipsYankedVersionForBinNames verifies that the builder
// uses bin_names from the latest NON-yanked version, skipping yanked ones.
func TestCargoBuilder_Build_SkipsYankedVersionForBinNames(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "evolving-crate",
			"description": "A crate that changed binary names",
			"homepage": "",
			"repository": ""
		},
		"versions": [
			{"bin_names": ["new-bad-name"], "yanked": true},
			{"bin_names": ["correct-binary"], "yanked": false},
			{"bin_names": ["old-name"], "yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/evolving-crate" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "evolving-crate"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}

	// Should use the first non-yanked version's bin_names
	if len(executables) != 1 || executables[0] != "correct-binary" {
		t.Errorf("executables = %v, want [\"correct-binary\"]", executables)
	}
}

func TestCargoBuilder_Build_CargoSubcommand(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "cargo-hack",
			"description": "A cargo subcommand for testing each feature flag",
			"homepage": "",
			"repository": ""
		},
		"versions": [
			{"bin_names": ["cargo-hack"], "yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/cargo-hack" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Build(ctx, BuildRequest{Package: "cargo-hack"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Verify command should use "cargo <subcommand>" invocation
	if result.Recipe.Verify.Command != "cargo hack --version" {
		t.Errorf("Verify.Command = %q, want %q", result.Recipe.Verify.Command, "cargo hack --version")
	}
	if result.Recipe.Verify.Pattern != "{version}" {
		t.Errorf("Verify.Pattern = %q, want %q", result.Recipe.Verify.Pattern, "{version}")
	}
}

// TestCargoBuilder_Build_NoVersionsFallbackToCrateName verifies fallback when
// the API returns no versions at all.
func TestCargoBuilder_Build_NoVersionsFallbackToCrateName(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "no-versions",
			"description": "A crate with no versions",
			"homepage": "",
			"repository": ""
		},
		"versions": []
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/no-versions" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	result, err := builder.Build(context.Background(), BuildRequest{Package: "no-versions"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "no-versions" {
		t.Errorf("executables = %v, want [\"no-versions\"]", executables)
	}
}

// TestCargoBuilder_Build_CachesCrateInfo verifies that Build() caches the
// API response for downstream use by BinaryNameProvider (#1938).
func TestCargoBuilder_Build_CachesCrateInfo(t *testing.T) {
	crateResponse := `{
		"crate": {
			"name": "cached-crate",
			"description": "A crate",
			"homepage": "",
			"repository": ""
		},
		"versions": [
			{"bin_names": ["cached-bin"], "yanked": false}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/crates/cached-crate" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)

	// Before Build, cachedCrateInfo should be nil
	if builder.cachedCrateInfo != nil {
		t.Fatal("cachedCrateInfo should be nil before Build()")
	}

	_, err := builder.Build(context.Background(), BuildRequest{Package: "cached-crate"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// After Build, cachedCrateInfo should be populated
	if builder.cachedCrateInfo == nil {
		t.Fatal("cachedCrateInfo should be populated after Build()")
	}
	if builder.cachedCrateInfo.Crate.Name != "cached-crate" {
		t.Errorf("cachedCrateInfo.Crate.Name = %q, want %q", builder.cachedCrateInfo.Crate.Name, "cached-crate")
	}
	if len(builder.cachedCrateInfo.Versions) != 1 {
		t.Fatalf("cachedCrateInfo.Versions has %d entries, want 1", len(builder.cachedCrateInfo.Versions))
	}
	if len(builder.cachedCrateInfo.Versions[0].BinNames) != 1 || builder.cachedCrateInfo.Versions[0].BinNames[0] != "cached-bin" {
		t.Errorf("cachedCrateInfo.Versions[0].BinNames = %v, want [\"cached-bin\"]", builder.cachedCrateInfo.Versions[0].BinNames)
	}
}

func TestCargoVerifySection(t *testing.T) {
	tests := []struct {
		executable  string
		wantCommand string
	}{
		{"ripgrep", "ripgrep --version"},
		{"rg", "rg --version"},
		{"cargo-hack", "cargo hack --version"},
		{"cargo-llvm-cov", "cargo llvm-cov --version"},
		{"cargo-audit", "cargo audit --version"},
	}

	for _, tc := range tests {
		t.Run(tc.executable, func(t *testing.T) {
			vs := cargoVerifySection(tc.executable)
			if vs.Command != tc.wantCommand {
				t.Errorf("cargoVerifySection(%q).Command = %q, want %q", tc.executable, vs.Command, tc.wantCommand)
			}
			if vs.Pattern != "{version}" {
				t.Errorf("cargoVerifySection(%q).Pattern = %q, want %q", tc.executable, vs.Pattern, "{version}")
			}
		})
	}
}

func TestCargoBuilder_Build_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	_, err := builder.Build(ctx, BuildRequest{Package: "nonexistent"})
	if err == nil {
		t.Error("Build() should fail for nonexistent crate")
	}
}

func TestIsValidCrateName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"ripgrep", true},
		{"cargo-audit", true},
		{"some_tool", true},
		{"a", true},
		{"A", true},
		{"", false},
		{"1invalid", false},
		{"-invalid", false},
		{"_invalid", false},
		{"has spaces", false},
		{"has@special", false},
		// 65 characters (too long)
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidCrateName(tc.name)
			if got != tc.valid {
				t.Errorf("isValidCrateName(%q) = %v, want %v", tc.name, got, tc.valid)
			}
		})
	}
}

func TestIsValidExecutableName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"rg", true},
		{"cargo-audit", true},
		{"my_tool", true},
		{"tool.exe", true},
		{"_internal", true},
		{"1tool", true},
		{"", false},
		{"; rm -rf /", false},
		{"$(whoami)", false},
		{"`id`", false},
		{"a|b", false},
		{"a&b", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidExecutableName(tc.name)
			if got != tc.valid {
				t.Errorf("isValidExecutableName(%q) = %v, want %v", tc.name, got, tc.valid)
			}
		})
	}
}

func TestCargoBuilder_Discover_Pagination(t *testing.T) {
	response := `{
		"crates": [
			{"name": "ripgrep", "recent_downloads": 5000000},
			{"name": "fd-find", "recent_downloads": 2000000},
			{"name": "bat", "recent_downloads": 1500000}
		],
		"meta": {"total": 3}
	}`

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	candidates, err := builder.Discover(context.Background(), 3)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(candidates))
	}

	if candidates[0].Name != "ripgrep" || candidates[0].Downloads != 5000000 {
		t.Errorf("candidates[0] = %+v, want ripgrep/5000000", candidates[0])
	}
	if candidates[2].Name != "bat" || candidates[2].Downloads != 1500000 {
		t.Errorf("candidates[2] = %+v, want bat/1500000", candidates[2])
	}

	if requestCount != 1 {
		t.Errorf("expected 1 request, got %d", requestCount)
	}
}

func TestCargoBuilder_Discover_LimitRespected(t *testing.T) {
	page := `{
		"crates": [
			{"name": "a", "recent_downloads": 100},
			{"name": "b", "recent_downloads": 90},
			{"name": "c", "recent_downloads": 80}
		],
		"meta": {"total": 3}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(page))
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	candidates, err := builder.Discover(context.Background(), 2)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
}

func TestCargoBuilder_Discover_ZeroLimit(t *testing.T) {
	builder := NewCargoBuilder(nil)
	candidates, err := builder.Discover(context.Background(), 0)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates for limit=0, got %d", len(candidates))
	}
}

func TestCargoBuilder_Discover_ErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Discover(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestCargoBuilder_Discover_RateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Discover(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
}

func TestCargoBuilder_Discover_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		crates := `{"crates": [`
		for i := range 100 {
			if i > 0 {
				crates += ","
			}
			crates += fmt.Sprintf(`{"name": "crate-%d", "recent_downloads": 100}`, i)
		}
		crates += `], "meta": {"total": 500}}`
		_, _ = w.Write([]byte(crates))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Discover(ctx, 500)
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestRegistry_Operations(t *testing.T) {
	reg := NewRegistry()

	// Test empty registry
	if len(reg.List()) != 0 {
		t.Error("New registry should be empty")
	}

	// Register a builder
	builder := NewCargoBuilder(nil)
	reg.Register(builder)

	// Test Get
	got, ok := reg.Get("crates.io")
	if !ok {
		t.Error("Get(\"crates.io\") should return true")
	}
	if got != builder {
		t.Error("Get should return the registered builder")
	}

	// Test Get nonexistent
	_, ok = reg.Get("nonexistent")
	if ok {
		t.Error("Get(\"nonexistent\") should return false")
	}

	// Test List
	names := reg.List()
	if len(names) != 1 || names[0] != "crates.io" {
		t.Errorf("List() = %v, want [\"crates.io\"]", names)
	}
}
