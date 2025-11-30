package builders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGemBuilder_Name(t *testing.T) {
	builder := NewGemBuilder(nil)
	if builder.Name() != "rubygems" {
		t.Errorf("Name() = %q, want %q", builder.Name(), "rubygems")
	}
}

func TestGemBuilder_CanBuild_ValidGem(t *testing.T) {
	// Mock RubyGems API response
	gemResponse := `{
		"name": "jekyll",
		"info": "Jekyll is a simple, blog aware, static site generator.",
		"homepage_uri": "https://jekyllrb.com",
		"source_code_uri": "https://github.com/jekyll/jekyll"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/gems/jekyll.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(gemResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGemBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	canBuild, err := builder.CanBuild(ctx, "jekyll")
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if !canBuild {
		t.Error("CanBuild() = false, want true")
	}
}

func TestGemBuilder_CanBuild_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	builder := NewGemBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	canBuild, err := builder.CanBuild(ctx, "nonexistent-gem")
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Error("CanBuild() = true, want false for nonexistent gem")
	}
}

func TestGemBuilder_CanBuild_InvalidGemName(t *testing.T) {
	builder := NewGemBuilder(nil)
	ctx := context.Background()

	// Invalid gem name should return false without making any HTTP requests
	canBuild, err := builder.CanBuild(ctx, "invalid gem name!")
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Error("CanBuild() = true, want false for invalid gem name")
	}
}

func TestGemBuilder_Build_WithGemspec(t *testing.T) {
	// Mock RubyGems API response
	gemResponse := `{
		"name": "jekyll",
		"info": "Jekyll is a simple, blog aware, static site generator.",
		"homepage_uri": "https://jekyllrb.com",
		"source_code_uri": "https://github.com/jekyll/jekyll"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/gems/jekyll.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(gemResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Note: The test won't be able to fetch from GitHub, so it will fall back to gem name
	builder := NewGemBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Build(ctx, "jekyll", "")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Verify recipe structure
	if result.Recipe == nil {
		t.Fatal("Build() result.Recipe is nil")
	}

	if result.Recipe.Metadata.Name != "jekyll" {
		t.Errorf("Recipe.Metadata.Name = %q, want %q", result.Recipe.Metadata.Name, "jekyll")
	}

	if result.Recipe.Metadata.Description != "Jekyll is a simple, blog aware, static site generator." {
		t.Errorf("Recipe.Metadata.Description = %q", result.Recipe.Metadata.Description)
	}

	if result.Recipe.Metadata.Homepage != "https://jekyllrb.com" {
		t.Errorf("Recipe.Metadata.Homepage = %q, want %q", result.Recipe.Metadata.Homepage, "https://jekyllrb.com")
	}

	// Check version source
	if result.Recipe.Version.Source != "rubygems" {
		t.Errorf("Recipe.Version.Source = %q, want %q", result.Recipe.Version.Source, "rubygems")
	}

	// Check steps
	if len(result.Recipe.Steps) != 1 {
		t.Fatalf("len(Recipe.Steps) = %d, want 1", len(result.Recipe.Steps))
	}

	if result.Recipe.Steps[0].Action != "gem_install" {
		t.Errorf("Recipe.Steps[0].Action = %q, want %q", result.Recipe.Steps[0].Action, "gem_install")
	}

	// Check source
	if result.Source != "rubygems:jekyll" {
		t.Errorf("result.Source = %q, want %q", result.Source, "rubygems:jekyll")
	}
}

func TestGemBuilder_Build_FallbackToGemName(t *testing.T) {
	// Gem without source_code_uri (falls back to gem name as executable)
	gemResponse := `{
		"name": "some-tool",
		"info": "A tool",
		"homepage_uri": "",
		"source_code_uri": ""
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/gems/some-tool.json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(gemResponse))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGemBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Build(ctx, "some-tool", "")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Should have warning about no source code URL
	if len(result.Warnings) == 0 {
		t.Error("Expected warning about no source code URL")
	}

	// Verify executable is gem name
	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok {
		t.Fatal("executables param is not []string")
	}
	if len(executables) != 1 || executables[0] != "some-tool" {
		t.Errorf("executables = %v, want [\"some-tool\"]", executables)
	}

	// Verify command uses gem name
	if result.Recipe.Verify.Command != "some-tool --version" {
		t.Errorf("Verify.Command = %q, want %q", result.Recipe.Verify.Command, "some-tool --version")
	}
}

func TestGemBuilder_Build_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	builder := NewGemBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	_, err := builder.Build(ctx, "nonexistent", "")
	if err == nil {
		t.Error("Build() should fail for nonexistent gem")
	}
}

func TestIsValidGemName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"jekyll", true},
		{"rails", true},
		{"some_gem", true},
		{"gem-name", true},
		{"a", true},
		{"A", true},
		{"", false},
		{"1invalid", false},
		{"-invalid", false},
		{"_invalid", false},
		{"has spaces", false},
		{"has@special", false},
		// 101 characters (too long)
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidGemName(tc.name)
			if got != tc.valid {
				t.Errorf("isValidGemName(%q) = %v, want %v", tc.name, got, tc.valid)
			}
		})
	}
}

func TestParseGemspecExecutables(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "array literal",
			content: `spec.executables = ["jekyll", "safe_yaml"]`,
			want:    []string{"jekyll", "safe_yaml"},
		},
		{
			name:    "single element",
			content: `s.executables = ["bundler"]`,
			want:    []string{"bundler"},
		},
		{
			name:    "word array",
			content: `spec.executables = %w[thor thor-parallel]`,
			want:    []string{"thor", "thor-parallel"},
		},
		{
			name:    "word array with parens",
			content: `s.executables = %w(rake)`,
			want:    []string{"rake"},
		},
		{
			name:    "no executables",
			content: `spec.name = "some-lib"`,
			want:    nil,
		},
		{
			name:    "empty array",
			content: `spec.executables = []`,
			want:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseGemspecExecutables(tc.content)
			if len(got) != len(tc.want) {
				t.Errorf("parseGemspecExecutables() = %v, want %v", got, tc.want)
				return
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("parseGemspecExecutables()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestGemBuilder_buildGemspecURL(t *testing.T) {
	builder := NewGemBuilder(nil)

	tests := []struct {
		sourceURL string
		gemName   string
		want      string
	}{
		{
			"https://github.com/jekyll/jekyll",
			"jekyll",
			"https://raw.githubusercontent.com/jekyll/jekyll/HEAD/jekyll.gemspec",
		},
		{
			"https://github.com/jekyll/jekyll.git",
			"jekyll",
			"https://raw.githubusercontent.com/jekyll/jekyll/HEAD/jekyll.gemspec",
		},
		{
			"https://github.com/jekyll/jekyll/",
			"jekyll",
			"https://raw.githubusercontent.com/jekyll/jekyll/HEAD/jekyll.gemspec",
		},
		{
			"https://gitlab.com/owner/repo",
			"somegem",
			"", // Not GitHub, returns empty
		},
		{
			"not-a-url",
			"somegem",
			"", // Invalid URL
		},
	}

	for _, tc := range tests {
		t.Run(tc.sourceURL, func(t *testing.T) {
			got := builder.buildGemspecURL(tc.sourceURL, tc.gemName)
			if got != tc.want {
				t.Errorf("buildGemspecURL(%q, %q) = %q, want %q", tc.sourceURL, tc.gemName, got, tc.want)
			}
		})
	}
}
