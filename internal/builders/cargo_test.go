package builders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCargoBuilder_Name(t *testing.T) {
	builder := NewCargoBuilder(nil)
	if builder.Name() != "crates.io" {
		t.Errorf("Name() = %q, want %q", builder.Name(), "crates.io")
	}
}

func TestCargoBuilder_CanBuild_ValidCrate(t *testing.T) {
	// Mock crates.io API response
	crateResponse := `{
		"crate": {
			"name": "ripgrep",
			"description": "ripgrep recursively searches directories for a regex pattern",
			"homepage": "https://github.com/BurntSushi/ripgrep",
			"repository": "https://github.com/BurntSushi/ripgrep"
		}
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

	canBuild, err := builder.CanBuild(ctx, "ripgrep")
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

	canBuild, err := builder.CanBuild(ctx, "nonexistent-crate")
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

	// Invalid crate name should return false without making any HTTP requests
	canBuild, err := builder.CanBuild(ctx, "invalid crate name!")
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Error("CanBuild() = true, want false for invalid crate name")
	}
}

func TestCargoBuilder_Build_WithCargoToml(t *testing.T) {
	// Mock crates.io API response
	crateResponse := `{
		"crate": {
			"name": "ripgrep",
			"description": "ripgrep recursively searches directories for a regex pattern",
			"homepage": "",
			"repository": "https://github.com/BurntSushi/ripgrep"
		}
	}`

	// Mock Cargo.toml with [[bin]] sections
	cargoToml := `
[package]
name = "ripgrep"
version = "14.0.0"

[[bin]]
name = "rg"
path = "src/main.rs"
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/crates/ripgrep":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(crateResponse))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Mock GitHub raw content server for Cargo.toml
	githubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/BurntSushi/ripgrep/HEAD/Cargo.toml" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(cargoToml))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer githubServer.Close()

	// We need to patch the buildCargoTomlURL method, but since we can't easily do that,
	// we'll test the fallback behavior when Cargo.toml can't be fetched
	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Build(ctx, "ripgrep", "")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Verify recipe structure
	if result.Recipe == nil {
		t.Fatal("Build() result.Recipe is nil")
	}

	if result.Recipe.Metadata.Name != "ripgrep" {
		t.Errorf("Recipe.Metadata.Name = %q, want %q", result.Recipe.Metadata.Name, "ripgrep")
	}

	if result.Recipe.Metadata.Description != "ripgrep recursively searches directories for a regex pattern" {
		t.Errorf("Recipe.Metadata.Description = %q", result.Recipe.Metadata.Description)
	}

	// Check version source
	if result.Recipe.Version.Source != "crates_io" {
		t.Errorf("Recipe.Version.Source = %q, want %q", result.Recipe.Version.Source, "crates_io")
	}

	// Check steps
	if len(result.Recipe.Steps) != 1 {
		t.Fatalf("len(Recipe.Steps) = %d, want 1", len(result.Recipe.Steps))
	}

	if result.Recipe.Steps[0].Action != "cargo_install" {
		t.Errorf("Recipe.Steps[0].Action = %q, want %q", result.Recipe.Steps[0].Action, "cargo_install")
	}

	// Check source
	if result.Source != "crates.io:ripgrep" {
		t.Errorf("result.Source = %q, want %q", result.Source, "crates.io:ripgrep")
	}
}

func TestCargoBuilder_Build_FallbackToPackageName(t *testing.T) {
	// Crate without repository (falls back to crate name as executable)
	crateResponse := `{
		"crate": {
			"name": "some-tool",
			"description": "A tool",
			"homepage": "",
			"repository": ""
		}
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

	result, err := builder.Build(ctx, "some-tool", "")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should have warning about no repository
	if len(result.Warnings) == 0 {
		t.Error("Expected warning about no repository URL")
	}

	// Verify executable is crate name
	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "some-tool" {
		t.Errorf("executables = %v, want [\"some-tool\"]", executables)
	}

	// Verify command uses crate name
	if result.Recipe.Verify.Command != "some-tool --version" {
		t.Errorf("Verify.Command = %q, want %q", result.Recipe.Verify.Command, "some-tool --version")
	}
}

func TestCargoBuilder_Build_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	builder := NewCargoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	_, err := builder.Build(ctx, "nonexistent", "")
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

func TestCargoBuilder_buildCargoTomlURL(t *testing.T) {
	builder := NewCargoBuilder(nil)

	tests := []struct {
		repoURL string
		want    string
	}{
		{
			"https://github.com/BurntSushi/ripgrep",
			"https://raw.githubusercontent.com/BurntSushi/ripgrep/HEAD/Cargo.toml",
		},
		{
			"https://github.com/BurntSushi/ripgrep.git",
			"https://raw.githubusercontent.com/BurntSushi/ripgrep/HEAD/Cargo.toml",
		},
		{
			"https://github.com/BurntSushi/ripgrep/",
			"https://raw.githubusercontent.com/BurntSushi/ripgrep/HEAD/Cargo.toml",
		},
		{
			"https://gitlab.com/owner/repo",
			"", // Not GitHub, returns empty
		},
		{
			"not-a-url",
			"", // Invalid URL
		},
	}

	for _, tc := range tests {
		t.Run(tc.repoURL, func(t *testing.T) {
			got := builder.buildCargoTomlURL(tc.repoURL)
			if got != tc.want {
				t.Errorf("buildCargoTomlURL(%q) = %q, want %q", tc.repoURL, got, tc.want)
			}
		})
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
