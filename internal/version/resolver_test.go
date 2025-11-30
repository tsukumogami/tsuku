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
			result := compareVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
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

func TestNewWithNpmRegistry(t *testing.T) {
	resolver := NewWithNpmRegistry("express")
	if resolver == nil {
		t.Fatal("NewWithNpmRegistry() returned nil")
	}
	if resolver.httpClient == nil {
		t.Error("NewWithNpmRegistry() did not initialize httpClient")
	}
}

func TestNewWithCratesIORegistry(t *testing.T) {
	resolver := NewWithCratesIORegistry("ripgrep")
	if resolver == nil {
		t.Fatal("NewWithCratesIORegistry() returned nil")
	}
	if resolver.httpClient == nil {
		t.Error("NewWithCratesIORegistry() did not initialize httpClient")
	}
}

func TestNewWithRubyGemsRegistry(t *testing.T) {
	resolver := NewWithRubyGemsRegistry("rails")
	if resolver == nil {
		t.Fatal("NewWithRubyGemsRegistry() returned nil")
	}
	if resolver.httpClient == nil {
		t.Error("NewWithRubyGemsRegistry() did not initialize httpClient")
	}
}

func TestNewWithPyPIRegistry(t *testing.T) {
	resolver := NewWithPyPIRegistry("requests")
	if resolver == nil {
		t.Fatal("NewWithPyPIRegistry() returned nil")
	}
	if resolver.httpClient == nil {
		t.Error("NewWithPyPIRegistry() did not initialize httpClient")
	}
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
