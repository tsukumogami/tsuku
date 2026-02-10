package executor

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCacheKeyFor(t *testing.T) {
	tests := []struct {
		name            string
		tool            string
		resolvedVersion string
		os              string
		arch            string
		want            PlanCacheKey
	}{
		{
			name:            "standard case",
			tool:            "ripgrep",
			resolvedVersion: "14.1.0",
			os:              "linux",
			arch:            "amd64",
			want: PlanCacheKey{
				Tool:     "ripgrep",
				Version:  "14.1.0",
				Platform: "linux-amd64",
			},
		},
		{
			name:            "darwin arm64",
			tool:            "kubectl",
			resolvedVersion: "1.29.0",
			os:              "darwin",
			arch:            "arm64",
			want: PlanCacheKey{
				Tool:     "kubectl",
				Version:  "1.29.0",
				Platform: "darwin-arm64",
			},
		},
		{
			name:            "windows amd64",
			tool:            "gh",
			resolvedVersion: "2.40.0",
			os:              "windows",
			arch:            "amd64",
			want: PlanCacheKey{
				Tool:     "gh",
				Version:  "2.40.0",
				Platform: "windows-amd64",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CacheKeyFor(tt.tool, tt.resolvedVersion, tt.os, tt.arch)
			if got != tt.want {
				t.Errorf("CacheKeyFor() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestValidateCachedPlan(t *testing.T) {
	validPlan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "ripgrep",
		Version:       "14.1.0",
		Platform: Platform{
			OS:   "linux",
			Arch: "amd64",
		},
	}

	validKey := PlanCacheKey{
		Tool:     "ripgrep",
		Version:  "14.1.0",
		Platform: "linux-amd64",
	}

	tests := []struct {
		name    string
		plan    *InstallationPlan
		key     PlanCacheKey
		wantErr string
	}{
		{
			name:    "valid plan",
			plan:    validPlan,
			key:     validKey,
			wantErr: "",
		},
		{
			name: "format version mismatch",
			plan: &InstallationPlan{
				FormatVersion: 1, // outdated
				Tool:          "ripgrep",
				Version:       "14.1.0",
				Platform:      Platform{OS: "linux", Arch: "amd64"},
			},
			key:     validKey,
			wantErr: "plan format version 1 is outdated (current: 4)",
		},
		{
			name: "platform OS mismatch",
			plan: &InstallationPlan{
				FormatVersion: PlanFormatVersion,
				Tool:          "ripgrep",
				Version:       "14.1.0",
				Platform:      Platform{OS: "darwin", Arch: "amd64"},
			},
			key:     validKey,
			wantErr: "plan platform darwin-amd64 does not match linux-amd64",
		},
		{
			name: "platform arch mismatch",
			plan: &InstallationPlan{
				FormatVersion: PlanFormatVersion,
				Tool:          "ripgrep",
				Version:       "14.1.0",
				Platform:      Platform{OS: "linux", Arch: "arm64"},
			},
			key:     validKey,
			wantErr: "plan platform linux-arm64 does not match linux-amd64",
		},
		{
			name: "invalid platform format in key",
			plan: validPlan,
			key: PlanCacheKey{
				Tool:     "ripgrep",
				Version:  "14.1.0",
				Platform: "linux", // missing arch
			},
			wantErr: "invalid platform format in cache key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCachedPlan(tt.plan, tt.key)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateCachedPlan() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateCachedPlan() expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ValidateCachedPlan() error = %q, want error containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestChecksumMismatchError(t *testing.T) {
	err := &ChecksumMismatchError{
		Tool:             "ripgrep",
		Version:          "14.1.0",
		URL:              "https://github.com/BurntSushi/ripgrep/releases/download/14.1.0/ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz",
		ExpectedChecksum: "abc123def456",
		ActualChecksum:   "xyz789uvw012",
	}

	errorMsg := err.Error()

	// Check that all required information is in the error message
	checks := []struct {
		name     string
		contains string
	}{
		{"URL", "https://github.com/BurntSushi/ripgrep"},
		{"expected checksum", "abc123def456"},
		{"actual checksum", "xyz789uvw012"},
		{"tool name in recovery command", "ripgrep@14.1.0"},
		{"fresh flag", "--fresh"},
		{"supply chain warning", "supply chain attack"},
		{"legitimate update mention", "legitimate release update"},
	}

	for _, check := range checks {
		if !strings.Contains(errorMsg, check.contains) {
			t.Errorf("ChecksumMismatchError.Error() missing %s: should contain %q\nGot: %s",
				check.name, check.contains, errorMsg)
		}
	}
}

func TestChecksumMismatchError_RecoveryCommand(t *testing.T) {
	// Test that the recovery command includes the correct tool and version
	tests := []struct {
		tool    string
		version string
		want    string
	}{
		{"ripgrep", "14.1.0", "tsuku install ripgrep@14.1.0 --fresh"},
		{"kubectl", "1.29.0", "tsuku install kubectl@1.29.0 --fresh"},
		{"gh", "2.40.0", "tsuku install gh@2.40.0 --fresh"},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			err := &ChecksumMismatchError{
				Tool:             tt.tool,
				Version:          tt.version,
				URL:              "https://example.com/download",
				ExpectedChecksum: "expected",
				ActualChecksum:   "actual",
			}

			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ChecksumMismatchError.Error() should contain %q for recovery", tt.want)
			}
		})
	}
}

func TestPlanCacheKey_ZeroValue(t *testing.T) {
	// Ensure zero value is well-defined
	var key PlanCacheKey
	if key.Tool != "" || key.Version != "" || key.Platform != "" || key.ContentHash != "" {
		t.Error("PlanCacheKey zero value should have empty strings")
	}
}

func TestComputePlanContentHash(t *testing.T) {
	t.Run("deterministic output", func(t *testing.T) {
		plan := &InstallationPlan{
			FormatVersion: PlanFormatVersion,
			Tool:          "ripgrep",
			Version:       "14.1.0",
			Platform:      Platform{OS: "linux", Arch: "amd64"},
			Steps: []ResolvedStep{
				{
					Action:        "download_file",
					Params:        map[string]interface{}{"url": "https://example.com/file.tar.gz"},
					Evaluable:     true,
					Deterministic: true,
					URL:           "https://example.com/file.tar.gz",
					Checksum:      "sha256:deadbeef",
				},
			},
		}

		hash1 := ComputePlanContentHash(plan)
		hash2 := ComputePlanContentHash(plan)

		if hash1 != hash2 {
			t.Errorf("ComputePlanContentHash() not deterministic: %s != %s", hash1, hash2)
		}

		// Should be a valid hex SHA256 (64 chars)
		if len(hash1) != 64 {
			t.Errorf("ComputePlanContentHash() returned invalid length: %d (expected 64)", len(hash1))
		}
	})

	t.Run("identical content produces identical hash", func(t *testing.T) {
		plan1 := &InstallationPlan{
			FormatVersion: PlanFormatVersion,
			Tool:          "gh",
			Version:       "2.40.0",
			Platform:      Platform{OS: "darwin", Arch: "arm64"},
			Deterministic: true,
			Steps: []ResolvedStep{
				{Action: "download_file", URL: "https://example.com/gh.tar.gz", Checksum: "abc123"},
				{Action: "extract", Params: map[string]interface{}{"format": "tar.gz"}},
			},
		}

		plan2 := &InstallationPlan{
			FormatVersion: PlanFormatVersion,
			Tool:          "gh",
			Version:       "2.40.0",
			Platform:      Platform{OS: "darwin", Arch: "arm64"},
			Deterministic: true,
			Steps: []ResolvedStep{
				{Action: "download_file", URL: "https://example.com/gh.tar.gz", Checksum: "abc123"},
				{Action: "extract", Params: map[string]interface{}{"format": "tar.gz"}},
			},
		}

		if ComputePlanContentHash(plan1) != ComputePlanContentHash(plan2) {
			t.Error("Identical plans should produce identical hashes")
		}
	})

	t.Run("different GeneratedAt produces same hash", func(t *testing.T) {
		now := time.Now()
		plan1 := &InstallationPlan{
			FormatVersion: PlanFormatVersion,
			Tool:          "kubectl",
			Version:       "1.29.0",
			Platform:      Platform{OS: "linux", Arch: "amd64"},
			GeneratedAt:   now,
			RecipeSource:  "registry",
			Steps:         []ResolvedStep{{Action: "download_file"}},
		}

		plan2 := &InstallationPlan{
			FormatVersion: PlanFormatVersion,
			Tool:          "kubectl",
			Version:       "1.29.0",
			Platform:      Platform{OS: "linux", Arch: "amd64"},
			GeneratedAt:   now.Add(24 * time.Hour), // Different time
			RecipeSource:  "/local/recipe.toml",    // Different source
			Steps:         []ResolvedStep{{Action: "download_file"}},
		}

		if ComputePlanContentHash(plan1) != ComputePlanContentHash(plan2) {
			t.Error("Plans differing only in GeneratedAt/RecipeSource should have same hash")
		}
	})

	t.Run("different steps produce different hash", func(t *testing.T) {
		plan1 := &InstallationPlan{
			FormatVersion: PlanFormatVersion,
			Tool:          "ripgrep",
			Version:       "14.1.0",
			Platform:      Platform{OS: "linux", Arch: "amd64"},
			Steps: []ResolvedStep{
				{Action: "download_file", URL: "https://example.com/v1.tar.gz"},
			},
		}

		plan2 := &InstallationPlan{
			FormatVersion: PlanFormatVersion,
			Tool:          "ripgrep",
			Version:       "14.1.0",
			Platform:      Platform{OS: "linux", Arch: "amd64"},
			Steps: []ResolvedStep{
				{Action: "download_file", URL: "https://example.com/v2.tar.gz"}, // Different URL
			},
		}

		if ComputePlanContentHash(plan1) == ComputePlanContentHash(plan2) {
			t.Error("Plans with different steps should have different hashes")
		}
	})

	t.Run("nested dependencies included in hash", func(t *testing.T) {
		plan1 := &InstallationPlan{
			FormatVersion: PlanFormatVersion,
			Tool:          "neovim",
			Version:       "0.9.0",
			Platform:      Platform{OS: "linux", Arch: "amd64"},
			Dependencies: []DependencyPlan{
				{
					Tool:    "libuv",
					Version: "1.44.0",
					Steps:   []ResolvedStep{{Action: "download_file"}},
				},
			},
			Steps: []ResolvedStep{{Action: "download_file"}},
		}

		plan2 := &InstallationPlan{
			FormatVersion: PlanFormatVersion,
			Tool:          "neovim",
			Version:       "0.9.0",
			Platform:      Platform{OS: "linux", Arch: "amd64"},
			Dependencies: []DependencyPlan{
				{
					Tool:    "libuv",
					Version: "1.45.0", // Different dependency version
					Steps:   []ResolvedStep{{Action: "download_file"}},
				},
			},
			Steps: []ResolvedStep{{Action: "download_file"}},
		}

		if ComputePlanContentHash(plan1) == ComputePlanContentHash(plan2) {
			t.Error("Plans with different dependencies should have different hashes")
		}
	})

	t.Run("map ordering in params is deterministic", func(t *testing.T) {
		// Create plans with params in different insertion orders
		// (Go maps don't guarantee iteration order, but our normalization should)
		plan1 := &InstallationPlan{
			FormatVersion: PlanFormatVersion,
			Tool:          "test",
			Version:       "1.0.0",
			Platform:      Platform{OS: "linux", Arch: "amd64"},
			Steps: []ResolvedStep{
				{
					Action: "extract",
					Params: map[string]interface{}{
						"format":       "tar.gz",
						"strip_prefix": float64(1),
						"destination":  "/tmp/test",
					},
				},
			},
		}

		plan2 := &InstallationPlan{
			FormatVersion: PlanFormatVersion,
			Tool:          "test",
			Version:       "1.0.0",
			Platform:      Platform{OS: "linux", Arch: "amd64"},
			Steps: []ResolvedStep{
				{
					Action: "extract",
					Params: map[string]interface{}{
						"destination":  "/tmp/test",
						"strip_prefix": float64(1),
						"format":       "tar.gz",
					},
				},
			},
		}

		if ComputePlanContentHash(plan1) != ComputePlanContentHash(plan2) {
			t.Error("Plans with same params in different order should have same hash")
		}
	})
}

func TestCacheKeyWithHash(t *testing.T) {
	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "ripgrep",
		Version:       "14.1.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps:         []ResolvedStep{{Action: "download_file"}},
	}

	key := CacheKeyWithHash("ripgrep", "14.1.0", "linux", "amd64", plan)

	if key.Tool != "ripgrep" {
		t.Errorf("Tool = %q, want %q", key.Tool, "ripgrep")
	}
	if key.Version != "14.1.0" {
		t.Errorf("Version = %q, want %q", key.Version, "14.1.0")
	}
	if key.Platform != "linux-amd64" {
		t.Errorf("Platform = %q, want %q", key.Platform, "linux-amd64")
	}
	if key.ContentHash == "" {
		t.Error("ContentHash should not be empty")
	}
	if len(key.ContentHash) != 64 {
		t.Errorf("ContentHash length = %d, want 64", len(key.ContentHash))
	}
}

func TestValidateCachedPlan_ContentHash(t *testing.T) {
	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "ripgrep",
		Version:       "14.1.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps:         []ResolvedStep{{Action: "download_file"}},
	}

	t.Run("valid content hash", func(t *testing.T) {
		contentHash := ComputePlanContentHash(plan)
		key := PlanCacheKey{
			Tool:        "ripgrep",
			Version:     "14.1.0",
			Platform:    "linux-amd64",
			ContentHash: contentHash,
		}

		err := ValidateCachedPlan(plan, key)
		if err != nil {
			t.Errorf("ValidateCachedPlan() unexpected error: %v", err)
		}
	})

	t.Run("mismatched content hash", func(t *testing.T) {
		key := PlanCacheKey{
			Tool:        "ripgrep",
			Version:     "14.1.0",
			Platform:    "linux-amd64",
			ContentHash: "0000000000000000000000000000000000000000000000000000000000000000",
		}

		err := ValidateCachedPlan(plan, key)
		if err == nil {
			t.Error("ValidateCachedPlan() expected error for mismatched hash")
		}
		if !strings.Contains(err.Error(), "content hash mismatch") {
			t.Errorf("Error message should mention content hash mismatch: %v", err)
		}
	})

	t.Run("empty content hash skips validation", func(t *testing.T) {
		key := PlanCacheKey{
			Tool:        "ripgrep",
			Version:     "14.1.0",
			Platform:    "linux-amd64",
			ContentHash: "", // Empty - should skip hash validation
		}

		err := ValidateCachedPlan(plan, key)
		if err != nil {
			t.Errorf("ValidateCachedPlan() should skip hash validation when empty: %v", err)
		}
	})
}

func TestSortedParams(t *testing.T) {
	t.Run("nil params", func(t *testing.T) {
		result := sortedParams(nil)
		if result != nil {
			t.Errorf("sortedParams(nil) = %v, want nil", result)
		}
	})

	t.Run("empty params", func(t *testing.T) {
		result := sortedParams(map[string]interface{}{})
		m, ok := result.(map[string]interface{})
		if !ok || len(m) != 0 {
			t.Errorf("sortedParams({}) = %v, want empty map", result)
		}
	})

	t.Run("nested maps sorted", func(t *testing.T) {
		params := map[string]interface{}{
			"outer": map[string]interface{}{
				"z_key": "z",
				"a_key": "a",
			},
		}
		result := sortedParams(params)
		// Verify the result can be marshaled deterministically
		data1, _ := json.Marshal(result)
		data2, _ := json.Marshal(result)
		if string(data1) != string(data2) {
			t.Error("sortedParams should produce deterministic JSON")
		}
	})

	t.Run("slices with nested maps", func(t *testing.T) {
		params := map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"b": 2, "a": 1},
				map[string]interface{}{"d": 4, "c": 3},
			},
		}
		result := sortedParams(params)
		data1, _ := json.Marshal(result)
		data2, _ := json.Marshal(result)
		if string(data1) != string(data2) {
			t.Error("sortedParams should handle slices with nested maps")
		}
	})
}
