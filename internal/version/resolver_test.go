package version

import (
	"fmt"
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"kustomize/v5.7.1", "5.7.1"},
		{"Release_1_15_0", "1.15.0"},
		{"go1.21.5", "1.21.5"},
		{"v2.0.0-rc1", "2.0.0-rc1"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeVersion(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeVersion(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"1.2.3", true},
		{"v1.0.0", true},
		{"0.1.0", true},
		{"", false},
		{"abc", false},
		{"latest", false},
		{"1.2.3-beta", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isValidVersion(tt.input)
			if result != tt.expected {
				t.Errorf("isValidVersion(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"1.21.5", "1.20.1", 1},
		{"1.20.1", "1.21.5", -1},
		{"1.0", "1.0.0", 0},
		{"2.0", "1.9.9", 1},
		{"10.0.0", "9.0.0", 1},
	}

	for _, tt := range tests {
		name := tt.v1 + "_vs_" + tt.v2
		t.Run(name, func(t *testing.T) {
			result := CompareVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestNew(t *testing.T) {
	resolver := New()
	if resolver == nil {
		t.Fatal("New() returned nil")
	}
	if resolver.httpClient == nil {
		t.Error("New() did not initialize httpClient")
	}
}

func TestNewWithOptions(t *testing.T) {
	t.Run("WithNpmRegistry", func(t *testing.T) {
		resolver := New(WithNpmRegistry("https://custom.npm.registry"))
		if resolver == nil {
			t.Fatal("New(WithNpmRegistry()) returned nil")
		}
		if resolver.httpClient == nil {
			t.Error("New(WithNpmRegistry()) did not initialize httpClient")
		}
		if resolver.npmRegistryURL != "https://custom.npm.registry" {
			t.Errorf("Expected npm registry URL 'https://custom.npm.registry', got '%s'", resolver.npmRegistryURL)
		}
	})

	t.Run("WithCratesIORegistry", func(t *testing.T) {
		resolver := New(WithCratesIORegistry("https://custom.crates.io"))
		if resolver == nil {
			t.Fatal("New(WithCratesIORegistry()) returned nil")
		}
		if resolver.httpClient == nil {
			t.Error("New(WithCratesIORegistry()) did not initialize httpClient")
		}
		if resolver.cratesIORegistryURL != "https://custom.crates.io" {
			t.Errorf("Expected crates.io registry URL 'https://custom.crates.io', got '%s'", resolver.cratesIORegistryURL)
		}
	})

	t.Run("WithRubyGemsRegistry", func(t *testing.T) {
		resolver := New(WithRubyGemsRegistry("https://custom.rubygems.org"))
		if resolver == nil {
			t.Fatal("New(WithRubyGemsRegistry()) returned nil")
		}
		if resolver.httpClient == nil {
			t.Error("New(WithRubyGemsRegistry()) did not initialize httpClient")
		}
		if resolver.rubygemsRegistryURL != "https://custom.rubygems.org" {
			t.Errorf("Expected RubyGems registry URL 'https://custom.rubygems.org', got '%s'", resolver.rubygemsRegistryURL)
		}
	})

	t.Run("WithPyPIRegistry", func(t *testing.T) {
		resolver := New(WithPyPIRegistry("https://custom.pypi.org"))
		if resolver == nil {
			t.Fatal("New(WithPyPIRegistry()) returned nil")
		}
		if resolver.httpClient == nil {
			t.Error("New(WithPyPIRegistry()) did not initialize httpClient")
		}
		if resolver.pypiRegistryURL != "https://custom.pypi.org" {
			t.Errorf("Expected PyPI registry URL 'https://custom.pypi.org', got '%s'", resolver.pypiRegistryURL)
		}
	})

	t.Run("MultipleOptions", func(t *testing.T) {
		resolver := New(
			WithNpmRegistry("https://npm.example.com"),
			WithGoDevURL("https://go.example.com"),
		)
		if resolver == nil {
			t.Fatal("New() with multiple options returned nil")
		}
		if resolver.npmRegistryURL != "https://npm.example.com" {
			t.Errorf("Expected npm registry URL 'https://npm.example.com', got '%s'", resolver.npmRegistryURL)
		}
		if resolver.goDevURL != "https://go.example.com" {
			t.Errorf("Expected go.dev URL 'https://go.example.com', got '%s'", resolver.goDevURL)
		}
	})
}

func TestWrapGitHubRateLimitError(t *testing.T) {
	resolver := New()

	t.Run("non-rate-limit error returns nil", func(t *testing.T) {
		err := fmt.Errorf("some other error")
		result := resolver.wrapGitHubRateLimitError(err, GitHubContextVersionResolution)
		if result != nil {
			t.Errorf("wrapGitHubRateLimitError() = %v, want nil for non-rate-limit error", result)
		}
	})

	t.Run("nil error returns nil", func(t *testing.T) {
		result := resolver.wrapGitHubRateLimitError(nil, GitHubContextVersionResolution)
		if result != nil {
			t.Errorf("wrapGitHubRateLimitError() = %v, want nil for nil error", result)
		}
	})

	t.Run("wrapped non-rate-limit error returns nil", func(t *testing.T) {
		err := fmt.Errorf("wrapped: %w", fmt.Errorf("inner error"))
		result := resolver.wrapGitHubRateLimitError(err, GitHubContextVersionResolution)
		if result != nil {
			t.Errorf("wrapGitHubRateLimitError() = %v, want nil for wrapped non-rate-limit error", result)
		}
	})
}
