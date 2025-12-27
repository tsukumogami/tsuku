package version

import (
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestDetectRedundantVersion(t *testing.T) {
	tests := []struct {
		name    string
		recipe  *recipe.Recipe
		wantLen int
		wantMsg string // substring to look for in first message
	}{
		{
			name: "no redundancy - no version section",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{{
					Action: "cargo_install",
					Params: map[string]interface{}{"crate": "cargo-audit"},
				}},
			},
			wantLen: 0,
		},
		{
			name: "redundant - go_install with goproxy",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "goproxy"},
				Steps: []recipe.Step{{
					Action: "go_install",
					Params: map[string]interface{}{"module": "mvdan.cc/gofumpt"},
				}},
			},
			wantLen: 1,
			wantMsg: "source=\"goproxy\" is redundant",
		},
		{
			name: "redundant - cargo_install with crates_io",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "crates_io"},
				Steps: []recipe.Step{{
					Action: "cargo_install",
					Params: map[string]interface{}{"crate": "cargo-audit"},
				}},
			},
			wantLen: 1,
			wantMsg: "source=\"crates_io\" is redundant",
		},
		{
			name: "redundant - pipx_install with pypi",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "pypi"},
				Steps: []recipe.Step{{
					Action: "pipx_install",
					Params: map[string]interface{}{"package": "black"},
				}},
			},
			wantLen: 1,
			wantMsg: "source=\"pypi\" is redundant",
		},
		{
			name: "redundant - npm_install with npm",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "npm"},
				Steps: []recipe.Step{{
					Action: "npm_install",
					Params: map[string]interface{}{"package": "serve"},
				}},
			},
			wantLen: 1,
			wantMsg: "source=\"npm\" is redundant",
		},
		{
			name: "redundant - gem_install with rubygems",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "rubygems"},
				Steps: []recipe.Step{{
					Action: "gem_install",
					Params: map[string]interface{}{"gem": "bundler"},
				}},
			},
			wantLen: 1,
			wantMsg: "source=\"rubygems\" is redundant",
		},
		{
			name: "redundant - cpan_install with metacpan",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "metacpan"},
				Steps: []recipe.Step{{
					Action: "cpan_install",
					Params: map[string]interface{}{"distribution": "ack"},
				}},
			},
			wantLen: 1,
			wantMsg: "source=\"metacpan\" is redundant",
		},
		{
			name: "redundant - github_archive with matching github_repo",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{GitHubRepo: "muesli/duf"},
				Steps: []recipe.Step{{
					Action: "github_archive",
					Params: map[string]interface{}{"repo": "muesli/duf"},
				}},
			},
			wantLen: 1,
			wantMsg: "github_repo=\"muesli/duf\" is redundant",
		},
		{
			name: "not redundant - github_archive with different github_repo",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{GitHubRepo: "owner/main-repo"},
				Steps: []recipe.Step{{
					Action: "github_archive",
					Params: map[string]interface{}{"repo": "owner/assets-repo"},
				}},
			},
			wantLen: 0, // Different repos - explicit version is intentional
		},
		{
			name: "not redundant - override source different from action",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "github_releases", GitHubRepo: "foo/bar"},
				Steps: []recipe.Step{{
					Action: "cargo_install",
					Params: map[string]interface{}{"crate": "some-crate"},
				}},
			},
			wantLen: 0, // Explicit override - version from GitHub, install from crates.io
		},
		{
			name: "not redundant - download_archive action",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Source: "nodejs_dist"},
				Steps: []recipe.Step{{
					Action: "download_archive",
					Params: map[string]interface{}{"url": "https://nodejs.org/..."},
				}},
			},
			wantLen: 0, // download_archive has no inference
		},
		// Module redundancy tests
		{
			name: "redundant module - github pattern",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Module: "github.com/go-delve/delve"},
				Steps: []recipe.Step{{
					Action: "go_install",
					Params: map[string]interface{}{"module": "github.com/go-delve/delve/cmd/dlv"},
				}},
			},
			wantLen: 1,
			wantMsg: "module=\"github.com/go-delve/delve\" is redundant",
		},
		{
			name: "redundant module - cmd pattern",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Module: "honnef.co/go/tools"},
				Steps: []recipe.Step{{
					Action: "go_install",
					Params: map[string]interface{}{"module": "honnef.co/go/tools/cmd/staticcheck"},
				}},
			},
			wantLen: 1,
			wantMsg: "module=\"honnef.co/go/tools\" is redundant",
		},
		{
			name: "redundant module - exact match",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Module: "mvdan.cc/gofumpt"},
				Steps: []recipe.Step{{
					Action: "go_install",
					Params: map[string]interface{}{"module": "mvdan.cc/gofumpt"},
				}},
			},
			wantLen: 1,
			wantMsg: "module=\"mvdan.cc/gofumpt\" is redundant",
		},
		{
			name: "not redundant module - non-matching pattern",
			recipe: &recipe.Recipe{
				Version: recipe.VersionSection{Module: "go.uber.org/mock"},
				Steps: []recipe.Step{{
					Action: "go_install",
					Params: map[string]interface{}{"module": "go.uber.org/mock/mockgen"},
				}},
			},
			wantLen: 0, // mockgen path doesn't match either pattern - explicit module is needed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectRedundantVersion(tt.recipe)
			if len(result) != tt.wantLen {
				t.Errorf("DetectRedundantVersion() returned %d items, want %d", len(result), tt.wantLen)
				for _, r := range result {
					t.Logf("  - %s", r.Message)
				}
				return
			}
			if tt.wantLen > 0 && tt.wantMsg != "" {
				found := false
				for _, r := range result {
					if strings.Contains(r.Message, tt.wantMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("DetectRedundantVersion() message does not contain %q, got %q", tt.wantMsg, result[0].Message)
				}
			}
		})
	}
}
