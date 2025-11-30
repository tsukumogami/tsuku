package builders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPyPIBuilder_Name(t *testing.T) {
	builder := NewPyPIBuilder(nil)
	if builder.Name() != "pypi" {
		t.Errorf("Name() = %q, want %q", builder.Name(), "pypi")
	}
}

func TestPyPIBuilder_CanBuild(t *testing.T) {
	// Test server that returns different responses per package
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/ruff/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"info":{"name":"ruff","summary":"Linter"}}`))
		case "/pypi/rate-limited/json":
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	tests := []struct {
		name    string
		pkg     string
		wantOK  bool
		wantErr bool
		useReal bool // use real builder (for invalid name check)
	}{
		{"valid package", "ruff", true, false, false},
		{"not found", "nonexistent", false, false, false},
		{"rate limited", "rate-limited", false, true, false},
		{"invalid name with spaces", "invalid name", false, false, true},
		{"invalid name path traversal", "../etc/passwd", false, false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := builder
			if tc.useReal {
				b = NewPyPIBuilder(nil)
			}
			canBuild, err := b.CanBuild(ctx, tc.pkg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("CanBuild() error = %v, wantErr %v", err, tc.wantErr)
			}
			if canBuild != tc.wantOK {
				t.Errorf("CanBuild() = %v, want %v", canBuild, tc.wantOK)
			}
		})
	}
}

func TestPyPIBuilder_Build(t *testing.T) {
	// Test server returning different responses per package
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/ruff/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"info": {
					"name": "ruff",
					"summary": "An extremely fast Python linter.",
					"home_page": "",
					"project_urls": {"Homepage": "https://docs.astral.sh/ruff", "Repository": "https://github.com/astral-sh/ruff"}
				}
			}`))
		case "/pypi/no-source/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"info":{"name":"no-source","summary":"Tool","home_page":"","project_urls":null}}`))
		case "/pypi/with-homepage/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"info":{"name":"with-homepage","summary":"Tool","home_page":"https://example.com","project_urls":null}}`))
		case "/pypi/repo-homepage/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"info":{"name":"repo-homepage","summary":"Tool","home_page":"","project_urls":{"Repository":"https://github.com/test/repo"}}}`))
		case "/pypi/source-homepage/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"info":{"name":"source-homepage","summary":"Tool","home_page":"","project_urls":{"Source":"https://github.com/test/source"}}}`))
		case "/pypi/source-code-url/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"info":{"name":"source-code-url","summary":"Tool","home_page":"","project_urls":{"Source Code":"https://github.com/test/code"}}}`))
		case "/pypi/gitlab-source/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"info":{"name":"gitlab-source","summary":"Tool","home_page":"","project_urls":{"Repository":"https://gitlab.com/test/repo"}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("build ruff recipe", func(t *testing.T) {
		result, err := builder.Build(ctx, "ruff", "")
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if result.Recipe == nil {
			t.Fatal("result.Recipe is nil")
		}
		if result.Recipe.Metadata.Name != "ruff" {
			t.Errorf("Metadata.Name = %q, want %q", result.Recipe.Metadata.Name, "ruff")
		}
		if result.Recipe.Version.Source != "pypi" {
			t.Errorf("Version.Source = %q, want %q", result.Recipe.Version.Source, "pypi")
		}
		if result.Recipe.Steps[0].Action != "pipx_install" {
			t.Errorf("Steps[0].Action = %q, want %q", result.Recipe.Steps[0].Action, "pipx_install")
		}
		if result.Source != "pypi:ruff" {
			t.Errorf("Source = %q, want %q", result.Source, "pypi:ruff")
		}
		// Homepage from project_urls.Homepage
		if result.Recipe.Metadata.Homepage != "https://docs.astral.sh/ruff" {
			t.Errorf("Homepage = %q, want %q", result.Recipe.Metadata.Homepage, "https://docs.astral.sh/ruff")
		}
	})

	t.Run("fallback to package name when no source URL", func(t *testing.T) {
		result, err := builder.Build(ctx, "no-source", "")
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if len(result.Warnings) == 0 {
			t.Error("expected warning about no source URL")
		}
		executables := result.Recipe.Steps[0].Params["executables"].([]string)
		if len(executables) != 1 || executables[0] != "no-source" {
			t.Errorf("executables = %v, want [\"no-source\"]", executables)
		}
	})

	t.Run("uses home_page when available", func(t *testing.T) {
		result, err := builder.Build(ctx, "with-homepage", "")
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if result.Recipe.Metadata.Homepage != "https://example.com" {
			t.Errorf("Homepage = %q, want %q", result.Recipe.Metadata.Homepage, "https://example.com")
		}
	})

	t.Run("fallback homepage to Repository", func(t *testing.T) {
		result, err := builder.Build(ctx, "repo-homepage", "")
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if result.Recipe.Metadata.Homepage != "https://github.com/test/repo" {
			t.Errorf("Homepage = %q, want %q", result.Recipe.Metadata.Homepage, "https://github.com/test/repo")
		}
	})

	t.Run("fallback homepage to Source", func(t *testing.T) {
		result, err := builder.Build(ctx, "source-homepage", "")
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if result.Recipe.Metadata.Homepage != "https://github.com/test/source" {
			t.Errorf("Homepage = %q, want %q", result.Recipe.Metadata.Homepage, "https://github.com/test/source")
		}
	})

	t.Run("discovers source from Source Code key", func(t *testing.T) {
		result, err := builder.Build(ctx, "source-code-url", "")
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		// Should have warning about fetching pyproject.toml failure (since test server doesn't serve it)
		if len(result.Warnings) == 0 {
			t.Error("expected warning")
		}
	})

	t.Run("fallback when non-GitHub source URL", func(t *testing.T) {
		result, err := builder.Build(ctx, "gitlab-source", "")
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		// Should have warning about unparseable source URL
		foundWarning := false
		for _, w := range result.Warnings {
			if w == "Could not parse source URL https://gitlab.com/test/repo; using package name as executable" {
				foundWarning = true
				break
			}
		}
		if !foundWarning {
			t.Errorf("expected warning about gitlab URL, got %v", result.Warnings)
		}
	})

	t.Run("invalid package name returns error", func(t *testing.T) {
		_, err := builder.Build(ctx, "../invalid", "")
		if err == nil {
			t.Error("Build() should fail for invalid package name")
		}
	})

	t.Run("not found returns error", func(t *testing.T) {
		_, err := builder.Build(ctx, "nonexistent", "")
		if err == nil {
			t.Error("Build() should fail for nonexistent package")
		}
	})
}

func TestPyPIBuilder_fetchPackageInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pypi/valid/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"info":{"name":"valid","summary":"Test"}}`))
		case "/pypi/rate-limited/json":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/pypi/server-error/json":
			w.WriteHeader(http.StatusInternalServerError)
		case "/pypi/wrong-content/json":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html>not json</html>`))
		case "/pypi/bad-json/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{invalid json`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	tests := []struct {
		name    string
		pkg     string
		wantErr string
	}{
		{"valid", "valid", ""},
		{"not found", "notfound", "not found"},
		{"rate limited", "rate-limited", "rate limit"},
		{"server error", "server-error", "status 500"},
		{"wrong content type", "wrong-content", "content-type"},
		{"invalid json", "bad-json", "parse response"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := builder.fetchPackageInfo(ctx, tc.pkg)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("fetchPackageInfo() unexpected error = %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("fetchPackageInfo() expected error containing %q", tc.wantErr)
				}
			}
		})
	}
}

func TestPyPIBuilder_fetchPyprojectExecutables(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/project-scripts":
			_, _ = w.Write([]byte(`
[project.scripts]
mytool = "mypackage:main"
othertool = "mypackage:other"
`))
		case "/poetry-scripts":
			_, _ = w.Write([]byte(`
[tool.poetry.scripts]
poetrytool = "pkg:main"
`))
		case "/no-scripts":
			_, _ = w.Write([]byte(`
[project]
name = "mypackage"
version = "1.0.0"
`))
		case "/invalid-toml":
			_, _ = w.Write([]byte(`{not valid toml`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("project.scripts", func(t *testing.T) {
		execs, err := builder.fetchPyprojectExecutables(ctx, server.URL+"/project-scripts")
		if err != nil {
			t.Fatalf("fetchPyprojectExecutables() error = %v", err)
		}
		if len(execs) != 2 {
			t.Errorf("expected 2 executables, got %d: %v", len(execs), execs)
		}
	})

	t.Run("tool.poetry.scripts fallback", func(t *testing.T) {
		execs, err := builder.fetchPyprojectExecutables(ctx, server.URL+"/poetry-scripts")
		if err != nil {
			t.Fatalf("fetchPyprojectExecutables() error = %v", err)
		}
		if len(execs) != 1 || execs[0] != "poetrytool" {
			t.Errorf("expected [poetrytool], got %v", execs)
		}
	})

	t.Run("no scripts returns empty", func(t *testing.T) {
		execs, err := builder.fetchPyprojectExecutables(ctx, server.URL+"/no-scripts")
		if err != nil {
			t.Fatalf("fetchPyprojectExecutables() error = %v", err)
		}
		if len(execs) != 0 {
			t.Errorf("expected empty, got %v", execs)
		}
	})

	t.Run("invalid TOML returns error", func(t *testing.T) {
		_, err := builder.fetchPyprojectExecutables(ctx, server.URL+"/invalid-toml")
		if err == nil {
			t.Error("expected error for invalid TOML")
		}
	})

	t.Run("not found returns error", func(t *testing.T) {
		_, err := builder.fetchPyprojectExecutables(ctx, server.URL+"/notfound")
		if err == nil {
			t.Error("expected error for not found")
		}
	})
}

func TestIsValidPyPIPackageName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"ruff", true},
		{"black", true},
		{"some_package", true},
		{"package-name", true},
		{"a", true},
		{"A", true},
		{"package.name", true},
		{"a1", true},
		{"1a", true},
		{"", false},
		{"-invalid", false},
		{"../path/traversal", false},
		{"path/traversal", false},
		{"path\\traversal", false},
		{"has spaces", false},
		// 215 characters (too long)
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidPyPIPackageName(tc.name)
			if got != tc.valid {
				t.Errorf("isValidPyPIPackageName(%q) = %v, want %v", tc.name, got, tc.valid)
			}
		})
	}
}

func TestPyPIBuilder_buildPyprojectURL(t *testing.T) {
	builder := NewPyPIBuilder(nil)

	tests := []struct {
		sourceURL string
		want      string
	}{
		{"https://github.com/astral-sh/ruff", "https://raw.githubusercontent.com/astral-sh/ruff/HEAD/pyproject.toml"},
		{"https://github.com/psf/black.git", "https://raw.githubusercontent.com/psf/black/HEAD/pyproject.toml"},
		{"https://github.com/owner/repo/", "https://raw.githubusercontent.com/owner/repo/HEAD/pyproject.toml"},
		{"https://www.github.com/owner/repo", "https://raw.githubusercontent.com/owner/repo/HEAD/pyproject.toml"},
		{"https://gitlab.com/owner/repo", ""}, // Not GitHub
		{"https://github.com/owner", ""},      // Too short path
		{"://invalid-url", ""},                // Invalid URL
	}

	for _, tc := range tests {
		t.Run(tc.sourceURL, func(t *testing.T) {
			got := builder.buildPyprojectURL(tc.sourceURL)
			if got != tc.want {
				t.Errorf("buildPyprojectURL(%q) = %q, want %q", tc.sourceURL, got, tc.want)
			}
		})
	}
}
