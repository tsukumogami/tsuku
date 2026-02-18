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
			canBuild, err := b.CanBuild(ctx, BuildRequest{Package: tc.pkg})
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
		result, err := builder.Build(ctx, BuildRequest{Package: "ruff"})
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
		result, err := builder.Build(ctx, BuildRequest{Package: "no-source"})
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
		result, err := builder.Build(ctx, BuildRequest{Package: "with-homepage"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if result.Recipe.Metadata.Homepage != "https://example.com" {
			t.Errorf("Homepage = %q, want %q", result.Recipe.Metadata.Homepage, "https://example.com")
		}
	})

	t.Run("fallback homepage to Repository", func(t *testing.T) {
		result, err := builder.Build(ctx, BuildRequest{Package: "repo-homepage"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if result.Recipe.Metadata.Homepage != "https://github.com/test/repo" {
			t.Errorf("Homepage = %q, want %q", result.Recipe.Metadata.Homepage, "https://github.com/test/repo")
		}
	})

	t.Run("fallback homepage to Source", func(t *testing.T) {
		result, err := builder.Build(ctx, BuildRequest{Package: "source-homepage"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if result.Recipe.Metadata.Homepage != "https://github.com/test/source" {
			t.Errorf("Homepage = %q, want %q", result.Recipe.Metadata.Homepage, "https://github.com/test/source")
		}
	})

	t.Run("discovers source from Source Code key", func(t *testing.T) {
		result, err := builder.Build(ctx, BuildRequest{Package: "source-code-url"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		// Should have warning about fetching pyproject.toml failure (since test server doesn't serve it)
		if len(result.Warnings) == 0 {
			t.Error("expected warning")
		}
	})

	t.Run("fallback when non-GitHub source URL", func(t *testing.T) {
		result, err := builder.Build(ctx, BuildRequest{Package: "gitlab-source"})
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
		_, err := builder.Build(ctx, BuildRequest{Package: "../invalid"})
		if err == nil {
			t.Error("Build() should fail for invalid package name")
		}
	})

	t.Run("not found returns error", func(t *testing.T) {
		_, err := builder.Build(ctx, BuildRequest{Package: "nonexistent"})
		if err == nil {
			t.Error("Build() should fail for nonexistent package")
		}
	})
}

//nolint:dupl // Test structure similar to other builder tests by design
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

func TestPyPIBuilder_Discover_Success(t *testing.T) {
	topPackages := `{
		"last_update": "2026-02-01",
		"rows": [
			{"project": "httpie", "download_count": 500000},
			{"project": "requests", "download_count": 400000},
			{"project": "black", "download_count": 300000}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/top-pypi-packages-30-days.min.json":
			_, _ = w.Write([]byte(topPackages))
		case "/pypi/httpie/json":
			_, _ = w.Write([]byte(`{"info": {"name": "httpie", "summary": "CLI HTTP client", "classifiers": ["Environment :: Console"]}}`))
		case "/pypi/requests/json":
			// No console classifier -- should be filtered out.
			_, _ = w.Write([]byte(`{"info": {"name": "requests", "summary": "HTTP library", "classifiers": ["Development Status :: 5 - Production/Stable"]}}`))
		case "/pypi/black/json":
			_, _ = w.Write([]byte(`{"info": {"name": "black", "summary": "Code formatter", "classifiers": ["Environment :: Console"]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	candidates, err := builder.Discover(context.Background(), 10)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates (httpie, black), got %d: %+v", len(candidates), candidates)
	}

	// Verify filtered results.
	names := make(map[string]bool)
	for _, c := range candidates {
		names[c.Name] = true
		if c.Downloads != 0 {
			t.Errorf("expected Downloads=0 for PyPI candidate %q, got %d", c.Name, c.Downloads)
		}
	}
	if !names["httpie"] {
		t.Error("expected httpie in candidates")
	}
	if !names["black"] {
		t.Error("expected black in candidates")
	}
	if names["requests"] {
		t.Error("requests should be filtered out (no Console classifier)")
	}
}

func TestPyPIBuilder_Discover_LimitRespected(t *testing.T) {
	topPackages := `{
		"rows": [
			{"project": "a", "download_count": 500},
			{"project": "b", "download_count": 400},
			{"project": "c", "download_count": 300}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/top-pypi-packages-30-days.min.json" {
			_, _ = w.Write([]byte(topPackages))
		} else {
			// All have Console classifier.
			_, _ = w.Write([]byte(`{"info": {"name": "x", "classifiers": ["Environment :: Console"]}}`))
		}
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	candidates, err := builder.Discover(context.Background(), 1)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
}

func TestPyPIBuilder_Discover_ZeroLimit(t *testing.T) {
	builder := NewPyPIBuilder(nil)
	candidates, err := builder.Discover(context.Background(), 0)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(candidates))
	}
}

func TestPyPIBuilder_Discover_TopPackagesUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Discover(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error when top packages are unavailable")
	}
}

func TestPyPIBuilder_Discover_MalformedTopPackages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Discover(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error for malformed top packages JSON")
	}
}

func TestPyPIBuilder_Discover_EmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rows": []}`))
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	candidates, err := builder.Discover(context.Background(), 10)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(candidates))
	}
}

func TestPyPIBuilder_Discover_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rows": [{"project": "a", "download_count": 100}]}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	_, err := builder.Discover(ctx, 10)
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestPyPIBuilder_Discover_SkipsUnfetchablePackages(t *testing.T) {
	topPackages := `{
		"rows": [
			{"project": "good-pkg", "download_count": 500},
			{"project": "bad-pkg", "download_count": 400}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/top-pypi-packages-30-days.min.json":
			_, _ = w.Write([]byte(topPackages))
		case "/pypi/good-pkg/json":
			_, _ = w.Write([]byte(`{"info": {"name": "good-pkg", "classifiers": ["Environment :: Console"]}}`))
		case "/pypi/bad-pkg/json":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewPyPIBuilderWithBaseURL(nil, server.URL)
	candidates, err := builder.Discover(context.Background(), 10)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Name != "good-pkg" {
		t.Errorf("candidates[0].Name = %q, want %q", candidates[0].Name, "good-pkg")
	}
}

func TestHasConsoleClassifier(t *testing.T) {
	tests := []struct {
		name        string
		classifiers []string
		want        bool
	}{
		{"has console", []string{"Environment :: Console", "Development Status :: 5"}, true},
		{"no console", []string{"Development Status :: 5"}, false},
		{"empty", nil, false},
		{"exact match only", []string{"Environment :: Console :: Curses"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hasConsoleClassifier(tc.classifiers)
			if got != tc.want {
				t.Errorf("hasConsoleClassifier(%v) = %v, want %v", tc.classifiers, got, tc.want)
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
