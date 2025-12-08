package builders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNpmBuilder_Name(t *testing.T) {
	builder := NewNpmBuilder(nil)
	if builder.Name() != "npm" {
		t.Errorf("Name() = %q, want %q", builder.Name(), "npm")
	}
}

func TestNpmBuilder_CanBuild(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prettier":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"prettier","description":"Prettier is an opinionated code formatter"}`))
		case "/rate-limited":
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewNpmBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	tests := []struct {
		name    string
		pkg     string
		wantOK  bool
		wantErr bool
		useReal bool
	}{
		{"valid package", "prettier", true, false, false},
		{"not found", "nonexistent", false, false, false},
		{"rate limited", "rate-limited", false, true, false},
		{"invalid name uppercase", "Prettier", false, false, true},
		{"invalid name path traversal", "../etc/passwd", false, false, true},
		{"scoped package format valid", "@types/node", false, false, false}, // not found but valid format
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := builder
			if tc.useReal {
				b = NewNpmBuilder(nil)
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

func TestNpmBuilder_Build(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prettier":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "prettier",
				"description": "Prettier is an opinionated code formatter",
				"homepage": "https://prettier.io",
				"dist-tags": {"latest": "3.0.0"},
				"versions": {
					"3.0.0": {
						"bin": {"prettier": "./bin/prettier.cjs"}
					}
				}
			}`))
		case "/no-bin":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "no-bin",
				"description": "Package without bin",
				"dist-tags": {"latest": "1.0.0"},
				"versions": {"1.0.0": {}}
			}`))
		case "/with-repo-object":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "with-repo-object",
				"description": "Package with repo object",
				"repository": {"type": "git", "url": "git+https://github.com/test/repo.git"},
				"dist-tags": {"latest": "1.0.0"},
				"versions": {"1.0.0": {"bin": {"mytool": "./bin/mytool.js"}}}
			}`))
		case "/with-repo-string":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "with-repo-string",
				"description": "Package with repo string",
				"repository": "https://github.com/test/repo",
				"dist-tags": {"latest": "1.0.0"},
				"versions": {"1.0.0": {"bin": {"mytool": "./bin/mytool.js"}}}
			}`))
		case "/multiple-bins":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "multiple-bins",
				"description": "Package with multiple binaries",
				"dist-tags": {"latest": "1.0.0"},
				"versions": {"1.0.0": {"bin": {"tool1": "./bin/tool1.js", "tool2": "./bin/tool2.js"}}}
			}`))
		case "/no-latest":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"name": "no-latest",
				"description": "Package without latest tag",
				"dist-tags": {},
				"versions": {}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewNpmBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	t.Run("build prettier recipe", func(t *testing.T) {
		result, err := builder.Build(ctx, BuildRequest{Package: "prettier"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if result.Recipe == nil {
			t.Fatal("result.Recipe is nil")
		}
		if result.Recipe.Metadata.Name != "prettier" {
			t.Errorf("Metadata.Name = %q, want %q", result.Recipe.Metadata.Name, "prettier")
		}
		if result.Recipe.Version.Source != "npm" {
			t.Errorf("Version.Source = %q, want %q", result.Recipe.Version.Source, "npm")
		}
		if result.Recipe.Steps[0].Action != "npm_install" {
			t.Errorf("Steps[0].Action = %q, want %q", result.Recipe.Steps[0].Action, "npm_install")
		}
		if result.Source != "npm:prettier" {
			t.Errorf("Source = %q, want %q", result.Source, "npm:prettier")
		}
		if result.Recipe.Metadata.Homepage != "https://prettier.io" {
			t.Errorf("Homepage = %q, want %q", result.Recipe.Metadata.Homepage, "https://prettier.io")
		}
		executables := result.Recipe.Steps[0].Params["executables"].([]string)
		if len(executables) != 1 || executables[0] != "prettier" {
			t.Errorf("executables = %v, want [\"prettier\"]", executables)
		}
	})

	t.Run("fallback when no bin field", func(t *testing.T) {
		result, err := builder.Build(ctx, BuildRequest{Package: "no-bin"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if len(result.Warnings) == 0 {
			t.Error("expected warning about no bin field")
		}
		executables := result.Recipe.Steps[0].Params["executables"].([]string)
		if len(executables) != 1 || executables[0] != "no-bin" {
			t.Errorf("executables = %v, want [\"no-bin\"]", executables)
		}
	})

	t.Run("repository object URL", func(t *testing.T) {
		result, err := builder.Build(ctx, BuildRequest{Package: "with-repo-object"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if result.Recipe.Metadata.Homepage != "https://github.com/test/repo" {
			t.Errorf("Homepage = %q, want %q", result.Recipe.Metadata.Homepage, "https://github.com/test/repo")
		}
	})

	t.Run("repository string URL", func(t *testing.T) {
		result, err := builder.Build(ctx, BuildRequest{Package: "with-repo-string"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if result.Recipe.Metadata.Homepage != "https://github.com/test/repo" {
			t.Errorf("Homepage = %q, want %q", result.Recipe.Metadata.Homepage, "https://github.com/test/repo")
		}
	})

	t.Run("multiple binaries", func(t *testing.T) {
		result, err := builder.Build(ctx, BuildRequest{Package: "multiple-bins"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		executables := result.Recipe.Steps[0].Params["executables"].([]string)
		if len(executables) != 2 {
			t.Errorf("expected 2 executables, got %d: %v", len(executables), executables)
		}
	})

	t.Run("fallback when no latest tag", func(t *testing.T) {
		result, err := builder.Build(ctx, BuildRequest{Package: "no-latest"})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if len(result.Warnings) == 0 {
			t.Error("expected warning about no latest version")
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
func TestNpmBuilder_fetchPackageInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/valid":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"valid","description":"Test"}`))
		case "/rate-limited":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/server-error":
			w.WriteHeader(http.StatusInternalServerError)
		case "/wrong-content":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html>not json</html>`))
		case "/bad-json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{invalid json`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewNpmBuilderWithBaseURL(nil, server.URL)
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

func TestIsValidNpmPackageNameForBuilder(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"prettier", true},
		{"lodash", true},
		{"@types/node", true},
		{"@scope/package", true},
		{"package-name", true},
		{"package_name", true},
		{"package.name", true},
		{"a", true},
		{"", false},
		{"Prettier", false},  // uppercase not allowed
		{"PRETTIER", false},  // uppercase not allowed
		{"../path", false},   // path traversal
		{"path\\win", false}, // backslash
		// 215 characters (too long)
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidNpmPackageNameForBuilder(tc.name)
			if got != tc.valid {
				t.Errorf("isValidNpmPackageNameForBuilder(%q) = %v, want %v", tc.name, got, tc.valid)
			}
		})
	}
}

func TestParseBinField(t *testing.T) {
	tests := []struct {
		name string
		bin  any
		want int // expected number of executables
	}{
		{"nil", nil, 0},
		{"string", "./bin/tool.js", 0}, // string means package name is the command
		{"single map", map[string]any{"tool": "./bin/tool.js"}, 1},
		{"multiple map", map[string]any{"tool1": "./bin/tool1.js", "tool2": "./bin/tool2.js"}, 2},
		{"invalid type", 123, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseBinField(tc.bin)
			if len(got) != tc.want {
				t.Errorf("parseBinField() = %v (len %d), want len %d", got, len(got), tc.want)
			}
		})
	}
}

func TestExtractRepositoryURL(t *testing.T) {
	tests := []struct {
		name string
		repo any
		want string
	}{
		{"nil", nil, ""},
		{"string URL", "https://github.com/test/repo", "https://github.com/test/repo"},
		{"string with .git", "https://github.com/test/repo.git", "https://github.com/test/repo"},
		{"git+ prefix", "git+https://github.com/test/repo.git", "https://github.com/test/repo"},
		{"git:// protocol", "git://github.com/test/repo.git", "https://github.com/test/repo"},
		{"object with url", map[string]any{"type": "git", "url": "git+https://github.com/test/repo.git"}, "https://github.com/test/repo"},
		{"object without url", map[string]any{"type": "git"}, ""},
		{"invalid type", 123, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractRepositoryURL(tc.repo)
			if got != tc.want {
				t.Errorf("extractRepositoryURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCleanRepositoryURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://github.com/test/repo", "https://github.com/test/repo"},
		{"https://github.com/test/repo.git", "https://github.com/test/repo"},
		{"git+https://github.com/test/repo.git", "https://github.com/test/repo"},
		{"git://github.com/test/repo.git", "https://github.com/test/repo"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := cleanRepositoryURL(tc.input)
			if got != tc.want {
				t.Errorf("cleanRepositoryURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
