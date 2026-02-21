package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/discover"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/registry"
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
			name: "homebrew with formula",
			step: recipe.Step{
				Action: "homebrew",
				Params: map[string]interface{}{
					"formula": "jq",
				},
			},
			want: "Download Homebrew bottle for jq",
		},
		{
			name: "homebrew without formula",
			step: recipe.Step{
				Action: "homebrew",
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
			name: "homebrew",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{{
					Action: "homebrew",
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

func TestFormatDaysAgo(t *testing.T) {
	tests := []struct {
		days int
		want string
	}{
		{0, "today"},
		{1, "1 day ago"},
		{3, "3 days ago"},
		{6, "6 days ago"},
		{7, "1 week ago"},
		{13, "1 week ago"},
		{14, "2 weeks ago"},
		{21, "3 weeks ago"},
		{28, "4 weeks ago"},
		{30, "1 month ago"},
		{59, "1 month ago"},
		{60, "2 months ago"},
		{90, "3 months ago"},
		{180, "6 months ago"},
		{364, "12 months ago"},
		{365, "1 year ago"},
		{729, "1 year ago"},
		{730, "2 years ago"},
		{1095, "3 years ago"},
		{1825, "5 years ago"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDaysAgo(tt.days)
			if got != tt.want {
				t.Errorf("formatDaysAgo(%d) = %q, want %q", tt.days, got, tt.want)
			}
		})
	}
}

func TestFormatDownloadCount(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1.0K"},
		{1500, "1.5K"},
		{45000, "45.0K"},
		{999999, "1000.0K"},
		{1000000, "1.0M"},
		{1200000, "1.2M"},
		{50000000, "50.0M"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDownloadCount(tt.count)
			if got != tt.want {
				t.Errorf("formatDownloadCount(%d) = %q, want %q", tt.count, got, tt.want)
			}
		})
	}
}

func TestFormatDisambiguationPrompt(t *testing.T) {
	tests := []struct {
		name    string
		matches []discover.ProbeMatch
		checks  []string
	}{
		{
			name: "single match",
			matches: []discover.ProbeMatch{
				{Builder: "npm", Source: "bat", Downloads: 45000, VersionCount: 12, HasRepository: true},
			},
			checks: []string{
				"Multiple sources found:",
				"1. npm: bat (recommended)",
				"Downloads: 45.0K",
				"Versions: 12",
				"Has repository",
			},
		},
		{
			name: "two matches with different metadata",
			matches: []discover.ProbeMatch{
				{Builder: "crates.io", Source: "bat", Downloads: 1200000, VersionCount: 50, HasRepository: true},
				{Builder: "npm", Source: "bat-cli", Downloads: 5000, VersionCount: 3, HasRepository: false},
			},
			checks: []string{
				"1. crates.io: bat (recommended)",
				"Downloads: 1.2M",
				"Versions: 50",
				"2. npm: bat-cli",
				"Downloads: 5.0K",
				"Versions: 3",
				"No repository",
			},
		},
		{
			name: "match with no downloads",
			matches: []discover.ProbeMatch{
				{Builder: "pypi", Source: "mytool", Downloads: 0, VersionCount: 5, HasRepository: true},
			},
			checks: []string{
				"1. pypi: mytool (recommended)",
				"Downloads: N/A",
				"Versions: 5",
			},
		},
		{
			name: "match with no version count",
			matches: []discover.ProbeMatch{
				{Builder: "rubygems", Source: "gemtool", Downloads: 8000, VersionCount: 0, HasRepository: false},
			},
			checks: []string{
				"Downloads: 8.0K",
				"No repository",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDisambiguationPrompt(tt.matches)
			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("formatDisambiguationPrompt() missing %q in output:\n%s", check, got)
				}
			}
		})
	}

	// Test that only first match gets "(recommended)" suffix
	t.Run("only first is recommended", func(t *testing.T) {
		matches := []discover.ProbeMatch{
			{Builder: "npm", Source: "first"},
			{Builder: "pypi", Source: "second"},
			{Builder: "crates.io", Source: "third"},
		}
		got := formatDisambiguationPrompt(matches)

		// Count occurrences of "(recommended)"
		count := strings.Count(got, "(recommended)")
		if count != 1 {
			t.Errorf("expected 1 occurrence of '(recommended)', got %d in:\n%s", count, got)
		}

		// Verify it's attached to the first match
		if !strings.Contains(got, "1. npm: first (recommended)") {
			t.Errorf("'(recommended)' not attached to first match in:\n%s", got)
		}
	})
}

// --- checkExistingRecipe Tests ---

// newTestLoader creates a loader with embedded recipes and a registry that
// returns 404 for all requests. This isolates tests from the real registry,
// ensuring the satisfies fallback is exercised instead of an exact name
// match from a remote registry recipe.
func newTestLoader(t *testing.T) *recipe.Loader {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	reg := registry.New(t.TempDir())
	reg.BaseURL = server.URL

	return recipe.NewWithLocalRecipes(reg, t.TempDir())
}

func TestCheckExistingRecipe_SatisfiesMatchPreventsGeneration(t *testing.T) {
	// The embedded openssl recipe satisfies "openssl@3". With the registry
	// returning 404 for everything, exact name lookup for "openssl@3" fails
	// and the satisfies fallback resolves it to "openssl".
	l := newTestLoader(t)

	canonicalName, found := checkExistingRecipe(l, "openssl@3")
	if !found {
		t.Fatal("expected checkExistingRecipe to find openssl@3 via satisfies fallback")
	}
	if canonicalName != "openssl" {
		t.Errorf("expected canonical name 'openssl', got %q", canonicalName)
	}
}

func TestCheckExistingRecipe_DirectNameMatchLocal(t *testing.T) {
	// A local recipe exists with the exact name requested.
	recipesDir := t.TempDir()
	recipeContent := `[metadata]
name = "my-tool"

[[steps]]
action = "download"
url = "https://example.com/tool.tar.gz"

[verify]
command = "my-tool --version"
`
	if err := os.WriteFile(filepath.Join(recipesDir, "my-tool.toml"), []byte(recipeContent), 0644); err != nil {
		t.Fatalf("Failed to write local recipe: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	reg := registry.New(t.TempDir())
	reg.BaseURL = server.URL
	l := recipe.NewWithoutEmbedded(reg, recipesDir)

	canonicalName, found := checkExistingRecipe(l, "my-tool")
	if !found {
		t.Fatal("expected checkExistingRecipe to find 'my-tool' directly")
	}
	if canonicalName != "my-tool" {
		t.Errorf("expected canonical name 'my-tool', got %q", canonicalName)
	}
}

func TestCheckExistingRecipe_DirectNameMatchEmbedded(t *testing.T) {
	// The embedded openssl recipe should be found by direct name lookup.
	l := newTestLoader(t)

	canonicalName, found := checkExistingRecipe(l, "openssl")
	if !found {
		t.Fatal("expected checkExistingRecipe to find embedded 'openssl'")
	}
	if canonicalName != "openssl" {
		t.Errorf("expected canonical name 'openssl', got %q", canonicalName)
	}
}

func TestCheckExistingRecipe_NoMatchAllowsGeneration(t *testing.T) {
	// When no recipe matches, checkExistingRecipe should return false.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	reg := registry.New(t.TempDir())
	reg.BaseURL = server.URL
	l := recipe.NewWithoutEmbedded(reg, t.TempDir())

	_, found := checkExistingRecipe(l, "nonexistent-tool")
	if found {
		t.Error("expected checkExistingRecipe to return false for nonexistent tool")
	}
}

func TestCheckExistingRecipe_NilLoader(t *testing.T) {
	// When the loader is nil, checkExistingRecipe should return false gracefully.
	_, found := checkExistingRecipe(nil, "anything")
	if found {
		t.Error("expected checkExistingRecipe to return false for nil loader")
	}
}

func TestCheckExistingRecipe_AlwaysReportsMatch(t *testing.T) {
	// checkExistingRecipe is a pure lookup helper: it always reports whether
	// a recipe exists, regardless of any flags. The --force bypass lives at
	// the call site in runCreate (create.go:485), not inside the helper.
	//
	// This test confirms the helper returns a match for both direct and
	// satisfies lookups, which is the correct behavior -- callers decide
	// whether to act on the result.
	l := newTestLoader(t)

	// Direct name match should always be reported.
	if canonicalName, found := checkExistingRecipe(l, "openssl"); !found {
		t.Fatal("expected checkExistingRecipe to find 'openssl'")
	} else if canonicalName != "openssl" {
		t.Errorf("expected canonical name 'openssl', got %q", canonicalName)
	}

	// Satisfies fallback match should always be reported.
	if canonicalName, found := checkExistingRecipe(l, "openssl@3"); !found {
		t.Fatal("expected checkExistingRecipe to find 'openssl@3' via satisfies")
	} else if canonicalName != "openssl" {
		t.Errorf("expected canonical name 'openssl', got %q", canonicalName)
	}
}
