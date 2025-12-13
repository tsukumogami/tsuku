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
		wantSourceArg string
		wantLLMType   LLMBuilderType
	}{
		{
			name:          "ecosystem crates.io",
			from:          "crates.io",
			wantBuilder:   "crates.io",
			wantSourceArg: "",
			wantLLMType:   LLMBuilderNone,
		},
		{
			name:          "ecosystem pypi",
			from:          "pypi",
			wantBuilder:   "pypi",
			wantSourceArg: "",
			wantLLMType:   LLMBuilderNone,
		},
		{
			name:          "ecosystem npm",
			from:          "npm",
			wantBuilder:   "npm",
			wantSourceArg: "",
			wantLLMType:   LLMBuilderNone,
		},
		{
			name:          "ecosystem rubygems",
			from:          "rubygems",
			wantBuilder:   "rubygems",
			wantSourceArg: "",
			wantLLMType:   LLMBuilderNone,
		},
		{
			name:          "ecosystem cargo alias",
			from:          "cargo",
			wantBuilder:   "crates.io",
			wantSourceArg: "",
			wantLLMType:   LLMBuilderNone,
		},
		{
			name:          "github with lowercase",
			from:          "github:cli/cli",
			wantBuilder:   "github",
			wantSourceArg: "cli/cli",
			wantLLMType:   LLMBuilderGitHub,
		},
		{
			name:          "github with uppercase",
			from:          "GitHub:FiloSottile/age",
			wantBuilder:   "github",
			wantSourceArg: "FiloSottile/age",
			wantLLMType:   LLMBuilderGitHub,
		},
		{
			name:          "github with mixed case",
			from:          "GITHUB:stern/stern",
			wantBuilder:   "github",
			wantSourceArg: "stern/stern",
			wantLLMType:   LLMBuilderGitHub,
		},
		{
			name:          "github preserves sourceArg case",
			from:          "github:BurntSushi/ripgrep",
			wantBuilder:   "github",
			wantSourceArg: "BurntSushi/ripgrep",
			wantLLMType:   LLMBuilderGitHub,
		},
		{
			name:          "homebrew with lowercase",
			from:          "homebrew:jq",
			wantBuilder:   "homebrew",
			wantSourceArg: "jq",
			wantLLMType:   LLMBuilderHomebrew,
		},
		{
			name:          "homebrew with uppercase",
			from:          "HOMEBREW:ripgrep",
			wantBuilder:   "homebrew",
			wantSourceArg: "ripgrep",
			wantLLMType:   LLMBuilderHomebrew,
		},
		{
			name:          "homebrew preserves sourceArg case",
			from:          "Homebrew:PostgreSQL",
			wantBuilder:   "homebrew",
			wantSourceArg: "PostgreSQL",
			wantLLMType:   LLMBuilderHomebrew,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder, sourceArg, llmType := parseFromFlag(tt.from)
			if builder != tt.wantBuilder {
				t.Errorf("parseFromFlag(%q) builder = %q, want %q", tt.from, builder, tt.wantBuilder)
			}
			if sourceArg != tt.wantSourceArg {
				t.Errorf("parseFromFlag(%q) sourceArg = %q, want %q", tt.from, sourceArg, tt.wantSourceArg)
			}
			if llmType != tt.wantLLMType {
				t.Errorf("parseFromFlag(%q) llmType = %v, want %v", tt.from, llmType, tt.wantLLMType)
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
			name: "hashicorp_release",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{{
					Action: "hashicorp_release",
					Params: map[string]interface{}{
						"product": "terraform",
					},
				}},
			},
			want: []string{"releases.hashicorp.com/terraform/..."},
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
