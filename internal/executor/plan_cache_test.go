package executor

import (
	"strings"
	"testing"
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
	if key.Tool != "" || key.Version != "" || key.Platform != "" {
		t.Error("PlanCacheKey zero value should have empty strings")
	}
}
