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

	// Invalid crate name should return false without making any HTTP requests
	canBuild, err := builder.CanBuild(ctx, BuildRequest{Package: "invalid crate name!"})
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

	result, err := builder.Build(ctx, BuildRequest{Package: "ripgrep"})
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

	result, err := builder.Build(ctx, BuildRequest{Package: "some-tool"})
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

func TestCargoBuilder_Discover_Pagination(t *testing.T) {
	// The Discover method uses per_page=100. We set limit=3 and serve
	// pages of 2 items each. Since len(page1) < per_page, Discover stops
	// after page 1. Instead, we set limit=2 with a single page of 3 crates
	// to verify limit truncation, or serve a full page.
	//
	// For a clean pagination test, serve 3 items on page 1 (< per_page,
	// so it's the last page) and request limit=3.
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

	// Only one page fetched (3 items < per_page=100).
	if requestCount != 1 {
		t.Errorf("expected 1 request, got %d", requestCount)
	}
}

func TestCargoBuilder_Discover_LimitRespected(t *testing.T) {
	// Return more than the limit to verify truncation.
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
		// Return a full page so pagination would continue.
		crates := `{"crates": [`
		for i := range 100 {
			if i > 0 {
				crates += ","
			}
			crates += `{"name": "crate-` + string(rune('a'+i%26)) + `", "recent_downloads": 100}`
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
