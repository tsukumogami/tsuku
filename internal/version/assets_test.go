package version

import (
	"fmt"
	"testing"
)

func TestMatchAssetPattern(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		assets     []string
		want       string
		wantErr    bool
		errContains string
	}{
		// Basic wildcard matching
		{
			name:    "single wildcard matches version",
			pattern: "app-*.tar.gz",
			assets: []string{
				"app-1.2.3.tar.gz",
				"app-1.2.2.tar.gz",
				"app-1.2.1.tar.gz",
			},
			want:    "app-1.2.3.tar.gz", // First match (newest from GitHub API)
			wantErr: false,
		},
		{
			name:    "wildcard matches complex version",
			pattern: "cpython-*+20251120-x86_64-unknown-linux-gnu-install_only.tar.gz",
			assets: []string{
				"cpython-3.13.1+20251120-x86_64-unknown-linux-gnu-install_only.tar.gz",
				"cpython-3.12.8+20251120-x86_64-unknown-linux-gnu-install_only.tar.gz",
				"cpython-3.11.11+20251120-x86_64-unknown-linux-gnu-install_only.tar.gz",
			},
			want:    "cpython-3.13.1+20251120-x86_64-unknown-linux-gnu-install_only.tar.gz",
			wantErr: false,
		},
		{
			name:    "multiple wildcards",
			pattern: "cpython-*+*-x86_64-*.tar.gz",
			assets: []string{
				"cpython-3.13.1+20251120-x86_64-unknown-linux-gnu-install_only.tar.gz",
				"cpython-3.12.8+20251119-x86_64-apple-darwin-install_only.tar.gz",
			},
			want:    "cpython-3.13.1+20251120-x86_64-unknown-linux-gnu-install_only.tar.gz",
			wantErr: false,
		},

		// Question mark wildcard
		{
			name:    "question mark matches single char",
			pattern: "app-1.2.?.tar.gz",
			assets: []string{
				"app-1.2.3.tar.gz",
				"app-1.2.10.tar.gz", // Won't match - 10 is two chars
			},
			want:    "app-1.2.3.tar.gz",
			wantErr: false,
		},

		// Exact match (no wildcards)
		{
			name:    "exact match without wildcards",
			pattern: "app-1.2.3.tar.gz",
			assets: []string{
				"app-1.2.3.tar.gz",
				"app-1.2.2.tar.gz",
			},
			want:    "app-1.2.3.tar.gz",
			wantErr: false,
		},

		// No matches
		{
			name:    "no match found",
			pattern: "app-*.zip",
			assets: []string{
				"app-1.2.3.tar.gz",
				"app-1.2.2.tar.gz",
			},
			wantErr:     true,
			errContains: "no asset matched pattern",
		},

		// Edge cases
		{
			name:        "empty pattern",
			pattern:     "",
			assets:      []string{"app-1.2.3.tar.gz"},
			wantErr:     true,
			errContains: "pattern cannot be empty",
		},
		{
			name:    "empty asset list",
			pattern: "app-*.tar.gz",
			assets:  []string{},
			wantErr: true,
			errContains: "no asset matched pattern",
		},
		{
			name:    "pattern matches multiple assets - returns first",
			pattern: "app-*",
			assets: []string{
				"app-3.0.0",
				"app-2.0.0",
				"app-1.0.0",
			},
			want:    "app-3.0.0", // First in list (GitHub API order = newest)
			wantErr: false,
		},

		// Complex patterns
		{
			name:    "platform and arch wildcards",
			pattern: "tool-*-linux-*.tar.gz",
			assets: []string{
				"tool-1.2.3-linux-amd64.tar.gz",
				"tool-1.2.3-darwin-amd64.tar.gz",
				"tool-1.2.3-linux-arm64.tar.gz",
			},
			want:    "tool-1.2.3-linux-amd64.tar.gz",
			wantErr: false,
		},
		{
			name:    "beginning and end wildcards",
			pattern: "*-linux-*",
			assets: []string{
				"mytool-v1.2.3-linux-amd64.tar.gz",
				"mytool-v1.2.2-darwin-amd64.tar.gz",
			},
			want:    "mytool-v1.2.3-linux-amd64.tar.gz",
			wantErr: false,
		},

		// Special characters that should work
		{
			name:    "pattern with dots and dashes",
			pattern: "app-*.*.*.tar.gz",
			assets: []string{
				"app-1.2.3.tar.gz",
			},
			want:    "app-1.2.3.tar.gz",
			wantErr: false,
		},
		{
			name:    "pattern with plus sign",
			pattern: "app-*+*.tar.gz",
			assets: []string{
				"app-1.2.3+build123.tar.gz",
			},
			want:    "app-1.2.3+build123.tar.gz",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MatchAssetPattern(tt.pattern, tt.assets)

			// Check error expectations
			if tt.wantErr {
				if err == nil {
					t.Errorf("MatchAssetPattern() expected error but got none")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("MatchAssetPattern() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			// Check success expectations
			if err != nil {
				t.Errorf("MatchAssetPattern() unexpected error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("MatchAssetPattern() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchAssetPattern_OrderPreservation(t *testing.T) {
	// Verify that GitHub API order is preserved (newest first)
	assets := []string{
		"app-3.0.0-linux-amd64.tar.gz", // Newest (uploaded last)
		"app-2.5.0-linux-amd64.tar.gz",
		"app-2.0.0-linux-amd64.tar.gz",
		"app-1.0.0-linux-amd64.tar.gz", // Oldest
	}

	pattern := "app-*-linux-amd64.tar.gz"
	got, err := MatchAssetPattern(pattern, assets)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return first match (newest version)
	want := "app-3.0.0-linux-amd64.tar.gz"
	if got != want {
		t.Errorf("MatchAssetPattern() = %v, want %v (newest version)", got, want)
	}
}

func TestMatchAssetPattern_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		assets  []string
		want    string
	}{
		{
			name:    "python-standalone pattern",
			pattern: "cpython-*+20251120-x86_64-unknown-linux-gnu-install_only.tar.gz",
			assets: []string{
				// Simulate real python-standalone release assets (partial list)
				"cpython-3.13.1+20251120-x86_64-unknown-linux-gnu-install_only.tar.gz",
				"cpython-3.12.8+20251120-x86_64-unknown-linux-gnu-install_only.tar.gz",
				"cpython-3.11.11+20251120-x86_64-unknown-linux-gnu-install_only.tar.gz",
				"cpython-3.10.19+20251120-x86_64-unknown-linux-gnu-install_only.tar.gz",
			},
			want: "cpython-3.13.1+20251120-x86_64-unknown-linux-gnu-install_only.tar.gz",
		},
		{
			name:    "terraform provider with platform",
			pattern: "terraform-provider-aws_*_linux_amd64.zip",
			assets: []string{
				"terraform-provider-aws_5.75.0_linux_amd64.zip",
				"terraform-provider-aws_5.75.0_darwin_amd64.zip",
				"terraform-provider-aws_5.75.0_windows_amd64.zip",
			},
			want: "terraform-provider-aws_5.75.0_linux_amd64.zip",
		},
		{
			name:    "kubectl with version prefix",
			pattern: "kubectl-*-linux-amd64",
			assets: []string{
				"kubectl-v1.31.0-linux-amd64",
				"kubectl-v1.31.0-darwin-amd64",
				"kubectl-v1.31.0-windows-amd64.exe",
			},
			want: "kubectl-v1.31.0-linux-amd64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MatchAssetPattern(tt.pattern, tt.assets)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("MatchAssetPattern() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseRepo(t *testing.T) {
	tests := []struct {
		name      string
		repo      string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "valid repo format",
			repo:      "owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
			wantErr:   false,
		},
		{
			name:      "repo with dashes",
			repo:      "my-org/my-repo",
			wantOwner: "my-org",
			wantRepo:  "my-repo",
			wantErr:   false,
		},
		{
			name:    "invalid format - no slash",
			repo:    "invalid",
			wantErr: true,
		},
		{
			name:    "invalid format - multiple slashes",
			repo:    "owner/repo/extra",
			wantErr: true,
		},
		{
			name:    "empty string",
			repo:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotRepo, err := parseRepo(tt.repo)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseRepo() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("parseRepo() unexpected error = %v", err)
				return
			}

			if gotOwner != tt.wantOwner {
				t.Errorf("parseRepo() owner = %v, want %v", gotOwner, tt.wantOwner)
			}
			if gotRepo != tt.wantRepo {
				t.Errorf("parseRepo() repo = %v, want %v", gotRepo, tt.wantRepo)
			}
		})
	}
}

func TestContainsWildcards(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    bool
	}{
		{
			name:    "contains asterisk",
			pattern: "app-*.tar.gz",
			want:    true,
		},
		{
			name:    "contains question mark",
			pattern: "app-1.2.?.tar.gz",
			want:    true,
		},
		{
			name:    "contains brackets",
			pattern: "app-[0-9].tar.gz",
			want:    true,
		},
		{
			name:    "no wildcards",
			pattern: "app-1.2.3.tar.gz",
			want:    false,
		},
		{
			name:    "empty string",
			pattern: "",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsWildcards(tt.pattern)
			if got != tt.want {
				t.Errorf("ContainsWildcards() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMatchAssetPattern_BracketWildcards tests [] bracket wildcard patterns
func TestMatchAssetPattern_BracketWildcards(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		assets     []string
		want       string
		wantErr    bool
	}{
		{
			name:    "bracket range matches single digit",
			pattern: "app-1.2.[0-9].tar.gz",
			assets: []string{
				"app-1.2.3.tar.gz",
				"app-1.2.10.tar.gz", // Won't match - 10 is two digits
			},
			want: "app-1.2.3.tar.gz",
		},
		{
			name:    "bracket list matches specific characters",
			pattern: "app-[abc].tar.gz",
			assets: []string{
				"app-b.tar.gz",
				"app-d.tar.gz", // Won't match
			},
			want: "app-b.tar.gz",
		},
		{
			name:    "bracket negation with caret",
			pattern: "app-[^0-9].tar.gz",
			assets: []string{
				"app-3.tar.gz",  // Won't match
				"app-x.tar.gz",  // Will match
			},
			want: "app-x.tar.gz",
		},
		{
			name:    "complex bracket pattern with version",
			pattern: "tool-v[0-9].[0-9].[0-9]-linux.tar.gz",
			assets: []string{
				"tool-v1.2.3-linux.tar.gz",
				"tool-v10.2.3-linux.tar.gz", // Won't match
			},
			want: "tool-v1.2.3-linux.tar.gz",
		},
		{
			name:    "multiple bracket patterns",
			pattern: "app-[0-9][0-9][0-9].tar.gz",
			assets: []string{
				"app-123.tar.gz",
				"app-12.tar.gz",  // Won't match
			},
			want: "app-123.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MatchAssetPattern(tt.pattern, tt.assets)

			if tt.wantErr {
				if err == nil {
					t.Errorf("MatchAssetPattern() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("MatchAssetPattern() unexpected error = %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("MatchAssetPattern() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMatchAssetPattern_SecurityEdgeCases tests security-related edge cases
func TestMatchAssetPattern_SecurityEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		assets     []string
		wantErr    bool
		errContains string
	}{
		{
			name:    "extremely long pattern",
			pattern: "app-" + string(make([]byte, 10000)) + ".tar.gz",
			assets:  []string{"app-1.0.tar.gz"},
			wantErr: true, // Will error on no match (pattern is valid but won't match)
			errContains: "no asset matched pattern",
		},
		{
			name:    "pattern with many wildcards still works",
			pattern: "*-*-*-*-*-*-*-*",
			assets:  []string{"a-b-c-d-e-f-g-h"},
			wantErr: false,
		},
		{
			name:    "invalid glob pattern unclosed bracket",
			pattern: "app-[.tar.gz",
			assets:  []string{"app-1.tar.gz"},
			wantErr: true,
			errContains: "invalid glob pattern",
		},
		{
			name:    "pattern with null bytes is invalid",
			pattern: "app-\x00.tar.gz",
			assets:  []string{"app-1.tar.gz"},
			wantErr: true, // Will either be invalid or no match
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := MatchAssetPattern(tt.pattern, tt.assets)

			if tt.wantErr {
				if err == nil {
					t.Errorf("MatchAssetPattern() expected error but got none")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("MatchAssetPattern() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("MatchAssetPattern() unexpected error = %v", err)
			}
		})
	}
}

// TestMatchAssetPattern_Unicode tests unicode handling
func TestMatchAssetPattern_Unicode(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		assets  []string
		want    string
	}{
		{
			name:    "unicode in asset name",
			pattern: "app-*-测试.tar.gz",
			assets:  []string{"app-1.0-测试.tar.gz"},
			want:    "app-1.0-测试.tar.gz",
		},
		{
			name:    "emoji in asset name",
			pattern: "app-*.tar.gz",
			assets:  []string{"app-🚀.tar.gz"},
			want:    "app-🚀.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MatchAssetPattern(tt.pattern, tt.assets)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("MatchAssetPattern() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMatchAssetPattern_LargeAssetLists tests behavior with large asset lists
func TestMatchAssetPattern_LargeAssetLists(t *testing.T) {
	// Create a large asset list
	largeAssetList := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		largeAssetList[i] = fmt.Sprintf("app-%d.tar.gz", i)
	}

	// Pattern that matches the first asset
	got, err := MatchAssetPattern("app-0.tar.gz", largeAssetList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "app-0.tar.gz" {
		t.Errorf("MatchAssetPattern() = %v, want app-0.tar.gz", got)
	}

	// Pattern that matches last asset (verifies iteration completes)
	got, err = MatchAssetPattern("app-999.tar.gz", largeAssetList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "app-999.tar.gz" {
		t.Errorf("MatchAssetPattern() = %v, want app-999.tar.gz", got)
	}

	// Wildcard that matches first (verifies early return optimization)
	got, err = MatchAssetPattern("app-*.tar.gz", largeAssetList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "app-0.tar.gz" {
		t.Errorf("MatchAssetPattern() = %v, want app-0.tar.gz (first match)", got)
	}
}

// TestFormatNoMatchError tests error message formatting
func TestFormatNoMatchError(t *testing.T) {
	tests := []struct {
		name         string
		pattern      string
		assetCount   int
		wantContains []string
	}{
		{
			name:       "few assets shows all",
			pattern:    "nonexistent",
			assetCount: 5,
			wantContains: []string{
				"no asset matched pattern 'nonexistent'",
				"asset-0", "asset-1", "asset-2", "asset-3", "asset-4",
			},
		},
		{
			name:       "many assets shows first 10 only",
			pattern:    "nonexistent",
			assetCount: 100,
			wantContains: []string{
				"no asset matched pattern 'nonexistent'",
				"asset-0", "asset-9",
				"... and 90 more",
			},
		},
		{
			name:       "exactly 10 assets shows all without truncation",
			pattern:    "nonexistent",
			assetCount: 10,
			wantContains: []string{
				"no asset matched pattern 'nonexistent'",
				"asset-0", "asset-9",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assets := make([]string, tt.assetCount)
			for i := 0; i < tt.assetCount; i++ {
				assets[i] = fmt.Sprintf("asset-%d", i)
			}

			err := formatNoMatchError(tt.pattern, assets)
			if err == nil {
				t.Fatal("formatNoMatchError() returned nil")
			}

			errMsg := err.Error()
			for _, want := range tt.wantContains {
				if !contains(errMsg, want) {
					t.Errorf("error message missing %q\nGot: %s", want, errMsg)
				}
			}

			// For exactly 10 assets, verify no truncation message
			if tt.assetCount == 10 {
				if contains(errMsg, "and") && contains(errMsg, "more") {
					t.Errorf("error message should not have truncation for exactly 10 assets\nGot: %s", errMsg)
				}
			}
		})
	}
}

// TestParseRepo_EdgeCases tests additional parseRepo edge cases
func TestParseRepo_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		repo    string
		wantErr bool
	}{
		{
			name:    "whitespace-only owner",
			repo:    "   /repo",
			wantErr: true,
		},
		{
			name:    "whitespace-only repo",
			repo:    "owner/   ",
			wantErr: true,
		},
		{
			name:    "leading slash",
			repo:    "/owner/repo",
			wantErr: true,
		},
		{
			name:    "trailing slash",
			repo:    "owner/repo/",
			wantErr: true,
		},
		{
			name:    "dots and underscores allowed",
			repo:    "my.org_name/my.repo_name",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseRepo(tt.repo)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRepo() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Helper function for substring matching in errors
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
