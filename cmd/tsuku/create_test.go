package main

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestParseFromFlag(t *testing.T) {
	tests := []struct {
		name          string
		from          string
		wantBuilder   string
		wantRemainder string
	}{
		{
			name:          "ecosystem crates.io",
			from:          "crates.io",
			wantBuilder:   "crates.io",
			wantRemainder: "",
		},
		{
			name:          "ecosystem pypi",
			from:          "pypi",
			wantBuilder:   "pypi",
			wantRemainder: "",
		},
		{
			name:          "ecosystem npm",
			from:          "npm",
			wantBuilder:   "npm",
			wantRemainder: "",
		},
		{
			name:          "ecosystem rubygems",
			from:          "rubygems",
			wantBuilder:   "rubygems",
			wantRemainder: "",
		},
		{
			name:          "ecosystem cargo (not normalized here)",
			from:          "cargo",
			wantBuilder:   "cargo",
			wantRemainder: "",
		},
		{
			name:          "github with lowercase",
			from:          "github:cli/cli",
			wantBuilder:   "github",
			wantRemainder: "cli/cli",
		},
		{
			name:          "github with uppercase",
			from:          "GitHub:FiloSottile/age",
			wantBuilder:   "github",
			wantRemainder: "FiloSottile/age",
		},
		{
			name:          "github with mixed case",
			from:          "GITHUB:stern/stern",
			wantBuilder:   "github",
			wantRemainder: "stern/stern",
		},
		{
			name:          "github preserves remainder case",
			from:          "github:BurntSushi/ripgrep",
			wantBuilder:   "github",
			wantRemainder: "BurntSushi/ripgrep",
		},
		{
			name:          "homebrew with lowercase",
			from:          "homebrew:jq",
			wantBuilder:   "homebrew",
			wantRemainder: "jq",
		},
		{
			name:          "homebrew with uppercase",
			from:          "HOMEBREW:ripgrep",
			wantBuilder:   "homebrew",
			wantRemainder: "ripgrep",
		},
		{
			name:          "homebrew preserves remainder case",
			from:          "Homebrew:PostgreSQL",
			wantBuilder:   "homebrew",
			wantRemainder: "PostgreSQL",
		},
		{
			name:          "homebrew source passes full remainder",
			from:          "homebrew:jq:source",
			wantBuilder:   "homebrew",
			wantRemainder: "jq:source",
		},
		{
			name:          "homebrew source with mixed case",
			from:          "Homebrew:tmux:SOURCE",
			wantBuilder:   "homebrew",
			wantRemainder: "tmux:SOURCE",
		},
		{
			name:          "homebrew source preserves formula case",
			from:          "homebrew:PostgreSQL@17:source",
			wantBuilder:   "homebrew",
			wantRemainder: "PostgreSQL@17:source",
		},
		// Edge cases - PFF-1 through PFF-6
		{
			name:          "github trailing colon (empty remainder)",
			from:          "github:",
			wantBuilder:   "github",
			wantRemainder: "",
		},
		{
			name:          "homebrew trailing colon (empty remainder)",
			from:          "homebrew:",
			wantBuilder:   "homebrew",
			wantRemainder: "",
		},
		{
			name:          "colon only returns empty builder",
			from:          ":",
			wantBuilder:   "",
			wantRemainder: "",
		},
		{
			name:          "github double colon",
			from:          "github::cli",
			wantBuilder:   "github",
			wantRemainder: ":cli",
		},
		{
			name:          "homebrew multiple colons",
			from:          "homebrew:pg@15:source",
			wantBuilder:   "homebrew",
			wantRemainder: "pg@15:source",
		},
		{
			name:          "homebrew versioned formula",
			from:          "homebrew:postgresql@15",
			wantBuilder:   "homebrew",
			wantRemainder: "postgresql@15",
		},
		{
			name:          "empty string treated as ecosystem",
			from:          "",
			wantBuilder:   "",
			wantRemainder: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder, remainder := parseFromFlag(tt.from)
			if builder != tt.wantBuilder {
				t.Errorf("parseFromFlag(%q) builder = %q, want %q", tt.from, builder, tt.wantBuilder)
			}
			if remainder != tt.wantRemainder {
				t.Errorf("parseFromFlag(%q) remainder = %q, want %q", tt.from, remainder, tt.wantRemainder)
			}
		})
	}
}

func TestNormalizeEcosystem(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"crates.io", "crates.io", "crates.io"},
		{"crates_io", "crates_io", "crates.io"},
		{"crates", "crates", "crates.io"},
		{"cargo", "cargo", "crates.io"},
		{"rubygems", "rubygems", "rubygems"},
		{"rubygems.org", "rubygems.org", "rubygems"},
		{"gems", "gems", "rubygems"},
		{"gem", "gem", "rubygems"},
		{"pypi", "pypi", "pypi"},
		{"pypi.org", "pypi.org", "pypi"},
		{"pip", "pip", "pypi"},
		{"python", "python", "pypi"},
		{"npm", "npm", "npm"},
		{"npmjs", "npmjs", "npm"},
		{"npmjs.com", "npmjs.com", "npm"},
		{"node", "node", "npm"},
		{"nodejs", "nodejs", "npm"},
		{"unknown", "unknown", "unknown"},
		{"uppercase", "NPM", "npm"},
		{"mixed case", "PyPI", "pypi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeEcosystem(tt.input)
			if got != tt.want {
				t.Errorf("normalizeEcosystem(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDescribeStep(t *testing.T) {
	tests := []struct {
		name string
		step recipe.Step
		want string
	}{
		{
			name: "github_archive tar.gz",
			step: recipe.Step{
				Action: "github_archive",
				Params: map[string]interface{}{
					"archive_format": "tar.gz",
				},
			},
			want: "Download and extract tar.gz archive from GitHub",
		},
		{
			name: "github_archive zip",
			step: recipe.Step{
				Action: "github_archive",
				Params: map[string]interface{}{
					"archive_format": "zip",
				},
			},
			want: "Download and extract zip archive from GitHub",
		},
		{
			name: "github_archive default format",
			step: recipe.Step{
				Action: "github_archive",
				Params: map[string]interface{}{},
			},
			want: "Download and extract tar.gz archive from GitHub",
		},
		{
			name: "github_file",
			step: recipe.Step{
				Action: "github_file",
			},
			want: "Download binary from GitHub releases",
		},
		{
			name: "homebrew_bottle with formula",
			step: recipe.Step{
				Action: "homebrew_bottle",
				Params: map[string]interface{}{
					"formula": "jq",
				},
			},
			want: "Download Homebrew bottle for jq",
		},
		{
			name: "homebrew_bottle without formula",
			step: recipe.Step{
				Action: "homebrew_bottle",
				Params: map[string]interface{}{},
			},
			want: "Download Homebrew bottle",
		},
		{
			name: "npm_install",
			step: recipe.Step{
				Action: "npm_install",
			},
			want: "Install via npm",
		},
		{
			name: "run with command",
			step: recipe.Step{
				Action: "run",
				Params: map[string]interface{}{
					"command": "make install",
				},
			},
			want: "Run: make install",
		},
		{
			name: "run with long command truncated",
			step: recipe.Step{
				Action: "run",
				Params: map[string]interface{}{
					"command": "this is a very long command that should be truncated to fit",
				},
			},
			want: "Run: this is a very long command that shou...",
		},
		{
			name: "unknown action with description",
			step: recipe.Step{
				Action:      "custom_action",
				Description: "Custom description",
			},
			want: "Custom description",
		},
		{
			name: "unknown action without description",
			step: recipe.Step{
				Action: "custom_action",
			},
			want: "custom_action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := describeStep(tt.step)
			if got != tt.want {
				t.Errorf("describeStep() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractDownloadURLs(t *testing.T) {
	tests := []struct {
		name   string
		recipe *recipe.Recipe
		want   []string
	}{
		{
			name: "github_archive",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{{
					Action: "github_archive",
					Params: map[string]interface{}{
						"repo":          "cli/cli",
						"asset_pattern": "gh_{version}_{os}_{arch}.tar.gz",
					},
				}},
			},
			want: []string{"github.com/cli/cli/releases/.../gh_{version}_{os}_{arch}.tar.gz"},
		},
		{
			name: "github_file",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{{
					Action: "github_file",
					Params: map[string]interface{}{
						"repo":          "FiloSottile/age",
						"asset_pattern": "age-{version}-{os}-{arch}.tar.gz",
					},
				}},
			},
			want: []string{"github.com/FiloSottile/age/releases/.../age-{version}-{os}-{arch}.tar.gz"},
		},
		{
			name: "homebrew_bottle",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{{
					Action: "homebrew_bottle",
					Params: map[string]interface{}{
						"formula": "jq",
					},
				}},
			},
			want: []string{"ghcr.io/homebrew/core/jq:..."},
		},
		{
			name: "download",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{{
					Action: "download",
					Params: map[string]interface{}{
						"url": "https://example.com/file.tar.gz",
					},
				}},
			},
			want: []string{"https://example.com/file.tar.gz"},
		},
		{
			name: "no download steps",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{{
					Action: "run",
					Params: map[string]interface{}{
						"command": "make install",
					},
				}},
			},
			want: nil,
		},
		{
			name: "multiple steps",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{
						Action: "github_archive",
						Params: map[string]interface{}{
							"repo":          "owner/repo",
							"asset_pattern": "app.tar.gz",
						},
					},
					{
						Action: "run",
						Params: map[string]interface{}{
							"command": "make install",
						},
					},
				},
			},
			want: []string{"github.com/owner/repo/releases/.../app.tar.gz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDownloadURLs(tt.recipe)
			if len(got) != len(tt.want) {
				t.Errorf("extractDownloadURLs() returned %d URLs, want %d", len(got), len(tt.want))
				return
			}
			for i, url := range got {
				if url != tt.want[i] {
					t.Errorf("extractDownloadURLs()[%d] = %q, want %q", i, url, tt.want[i])
				}
			}
		})
	}
}

func TestFormatRecipeTOML(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "test-tool",
			Description: "A test tool",
		},
		Version: recipe.VersionSection{
			Source:     "github_releases",
			GitHubRepo: "owner/repo",
		},
		Steps: []recipe.Step{{
			Action: "github_archive",
			Params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "test.tar.gz",
			},
		}},
		Verify: recipe.VerifySection{
			Command: "test-tool --version",
			Pattern: "{version}",
		},
	}

	got, err := formatRecipeTOML(r)
	if err != nil {
		t.Fatalf("formatRecipeTOML() error = %v", err)
	}

	// Check that essential parts are present
	checks := []string{
		"[metadata]",
		`name = "test-tool"`,
		"[version]",
		`source = "github_releases"`,
		"[[steps]]",
		`action = "github_archive"`,
		"[verify]",
		`command = "test-tool --version"`,
	}

	for _, check := range checks {
		if !containsString(got, check) {
			t.Errorf("formatRecipeTOML() missing %q in output:\n%s", check, got)
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
