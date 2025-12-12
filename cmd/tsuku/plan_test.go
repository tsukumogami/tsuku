package main

import (
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{"zero bytes", 0, "0 bytes"},
		{"bytes", 500, "500 bytes"},
		{"kilobytes", 1024, "1.00 KB"},
		{"kilobytes with decimal", 1536, "1.50 KB"},
		{"megabytes", 1048576, "1.00 MB"},
		{"megabytes with decimal", 12582912, "12.00 MB"},
		{"gigabytes", 1073741824, "1.00 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestTruncateHash(t *testing.T) {
	tests := []struct {
		name     string
		hash     string
		expected string
	}{
		{"empty hash", "", ""},
		{"short hash", "abc123", "abc123"},
		{"exactly 12 chars", "123456789012", "123456789012"},
		{"long hash", "sha256:abcdef123456789", "sha256:abcde..."},
		{"very long hash", "deadbeefcafebabedeadbeefcafebabedeadbeef", "deadbeefcafe..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateHash(tt.hash)
			if result != tt.expected {
				t.Errorf("truncateHash(%q) = %q, want %q", tt.hash, result, tt.expected)
			}
		})
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{"short string", "hello", "hello"},
		{"long string truncated", "this is a very long string that should be truncated because it exceeds fifty characters", "this is a very long string that should be trunc..."},
		{"empty slice", []interface{}{}, "[]"},
		{"small slice", []interface{}{"a", "b"}, "[a, b]"},
		{"large slice truncated", []interface{}{"a", "b", "c", "d", "e"}, "[a, b, c, ...+2 more]"},
		{"integer", 42, "42"},
		{"boolean", true, "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatValue(tt.value)
			if result != tt.expected {
				t.Errorf("formatValue(%v) = %q, want %q", tt.value, result, tt.expected)
			}
		})
	}
}

func TestFormatParams(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]interface{}
		contains []string // Check that output contains these substrings
		excludes []string // Check that output does NOT contain these substrings
	}{
		{
			name:     "empty params",
			params:   map[string]interface{}{},
			contains: nil,
		},
		{
			name:     "url only is empty",
			params:   map[string]interface{}{"url": "https://example.com"},
			contains: nil,
		},
		{
			name:     "single param",
			params:   map[string]interface{}{"format": "tar.gz"},
			contains: []string{"format=tar.gz"},
		},
		{
			name:     "url excluded from output",
			params:   map[string]interface{}{"url": "https://example.com", "format": "zip"},
			contains: []string{"format=zip"},
			excludes: []string{"url="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatParams(tt.params)
			for _, s := range tt.contains {
				if !containsSubstring(result, s) {
					t.Errorf("formatParams() = %q, want it to contain %q", result, s)
				}
			}
			for _, s := range tt.excludes {
				if containsSubstring(result, s) {
					t.Errorf("formatParams() = %q, want it NOT to contain %q", result, s)
				}
			}
		})
	}
}

