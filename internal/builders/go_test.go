package builders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoBuilder_Name(t *testing.T) {
	builder := NewGoBuilder(nil)
	if builder.Name() != "go" {
		t.Errorf("Name() = %q, want %q", builder.Name(), "go")
	}
}

func TestGoBuilder_CanBuild_ValidModule(t *testing.T) {
	// Mock Go proxy response
	response := `{"Version":"v0.40.0","Time":"2024-01-15T10:30:00Z"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/github.com/jesseduffield/lazygit/@latest" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	canBuild, err := builder.CanBuild(ctx, BuildRequest{Package: "github.com/jesseduffield/lazygit"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if !canBuild {
		t.Error("CanBuild() = false, want true")
	}
}

func TestGoBuilder_CanBuild_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	canBuild, err := builder.CanBuild(ctx, BuildRequest{Package: "github.com/nonexistent/module"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Error("CanBuild() = true, want false for nonexistent module")
	}
}

func TestGoBuilder_CanBuild_InvalidName(t *testing.T) {
	builder := NewGoBuilder(nil)
	ctx := context.Background()

	// Invalid module path should return false without making any HTTP requests
	canBuild, err := builder.CanBuild(ctx, BuildRequest{Package: "invalid;module"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Error("CanBuild() = true, want false for invalid module path")
	}
}

func TestGoBuilder_CanBuild_Gone(t *testing.T) {
	// Test 410 Gone (retracted module)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	canBuild, err := builder.CanBuild(ctx, BuildRequest{Package: "github.com/retracted/module"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Error("CanBuild() = true, want false for retracted module")
	}
}

func TestGoBuilder_Build_SimpleModule(t *testing.T) {
	response := `{"Version":"v0.40.0","Time":"2024-01-15T10:30:00Z"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/github.com/jesseduffield/lazygit/@latest" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Build(ctx, BuildRequest{Package: "github.com/jesseduffield/lazygit"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Verify recipe is not nil
	if result.Recipe == nil {
		t.Fatal("Build() result.Recipe is nil")
	}

	// Verify Go-specific recipe structure using helper
	verifyGoRecipe(t, result, "lazygit", "github.com/jesseduffield/lazygit")
}

// verifyGoRecipe is a helper that validates Go builder recipe structure
func verifyGoRecipe(t *testing.T, result *BuildResult, expectedExe, modulePath string) {
	t.Helper()

	r := result.Recipe

	// Check metadata
	if r.Metadata.Name != expectedExe {
		t.Errorf("Recipe.Metadata.Name = %q, want %q", r.Metadata.Name, expectedExe)
	}

	wantDesc := "Go CLI tool from " + modulePath
	if r.Metadata.Description != wantDesc {
		t.Errorf("Recipe.Metadata.Description = %q, want %q", r.Metadata.Description, wantDesc)
	}

	wantHomepage := "https://pkg.go.dev/" + modulePath
	if r.Metadata.Homepage != wantHomepage {
		t.Errorf("Recipe.Metadata.Homepage = %q, want %q", r.Metadata.Homepage, wantHomepage)
	}

	// Go tools require go dependency
	if len(r.Metadata.Dependencies) != 1 || r.Metadata.Dependencies[0] != "go" {
		t.Errorf("Recipe.Metadata.Dependencies = %v, want [go]", r.Metadata.Dependencies)
	}

	// Version source should be goproxy
	if r.Version.Source != "goproxy" {
		t.Errorf("Recipe.Version.Source = %q, want %q", r.Version.Source, "goproxy")
	}

	// Check single step with go_install action
	if len(r.Steps) != 1 {
		t.Fatalf("len(Recipe.Steps) = %d, want 1", len(r.Steps))
	}

	if r.Steps[0].Action != "go_install" {
		t.Errorf("Recipe.Steps[0].Action = %q, want %q", r.Steps[0].Action, "go_install")
	}

	// Verify module param
	module, ok := r.Steps[0].Params["module"].(string)
	if !ok || module != modulePath {
		t.Errorf("module param = %v, want %s", r.Steps[0].Params["module"], modulePath)
	}

	// Verify executables param
	executables, ok := r.Steps[0].Params["executables"].([]string)
	if !ok || len(executables) != 1 || executables[0] != expectedExe {
		t.Errorf("executables param = %v, want [%s]", r.Steps[0].Params["executables"], expectedExe)
	}

	// Verify command
	wantVerify := expectedExe + " --version"
	if r.Verify.Command != wantVerify {
		t.Errorf("Verify.Command = %q, want %q", r.Verify.Command, wantVerify)
	}

	// Check source
	wantSource := "goproxy:" + modulePath
	if result.Source != wantSource {
		t.Errorf("result.Source = %q, want %q", result.Source, wantSource)
	}

	// Should not have warnings for standard modules
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %v", result.Warnings)
	}
}

func TestGoBuilder_Build_CmdSubpath(t *testing.T) {
	response := `{"Version":"v1.55.0","Time":"2024-01-15T10:30:00Z"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/github.com/golangci/golangci-lint/cmd/golangci-lint/@latest" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Build(ctx, BuildRequest{Package: "github.com/golangci/golangci-lint/cmd/golangci-lint"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Name should be last path segment (golangci-lint)
	if result.Recipe.Metadata.Name != "golangci-lint" {
		t.Errorf("Recipe.Metadata.Name = %q, want %q", result.Recipe.Metadata.Name, "golangci-lint")
	}

	// Module should be the full path
	module, ok := result.Recipe.Steps[0].Params["module"].(string)
	if !ok || module != "github.com/golangci/golangci-lint/cmd/golangci-lint" {
		t.Errorf("module param = %v, want github.com/golangci/golangci-lint/cmd/golangci-lint", result.Recipe.Steps[0].Params["module"])
	}
}

func TestGoBuilder_Build_NonGitHubModule(t *testing.T) {
	response := `{"Version":"v0.6.0","Time":"2024-01-15T10:30:00Z"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mvdan.cc/gofumpt/@latest" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Build(ctx, BuildRequest{Package: "mvdan.cc/gofumpt"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Name should be gofumpt
	if result.Recipe.Metadata.Name != "gofumpt" {
		t.Errorf("Recipe.Metadata.Name = %q, want %q", result.Recipe.Metadata.Name, "gofumpt")
	}
}

func TestGoBuilder_Build_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	_, err := builder.Build(ctx, BuildRequest{Package: "github.com/nonexistent/module"})
	if err == nil {
		t.Error("Build() should fail for nonexistent module")
	}
}

func TestGoBuilder_Build_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	_, err := builder.Build(ctx, BuildRequest{Package: "github.com/jesseduffield/lazygit"})
	if err == nil {
		t.Error("Build() should fail on rate limit")
	}
	if !containsSubstr(err.Error(), "rate limit") {
		t.Errorf("error should mention rate limit: %v", err)
	}
}

func TestGoBuilder_Build_InvalidModule(t *testing.T) {
	builder := NewGoBuilder(nil)
	ctx := context.Background()

	_, err := builder.Build(ctx, BuildRequest{Package: "invalid;module"})
	if err == nil {
		t.Error("Build() should fail for invalid module path")
	}
}

func TestIsValidGoModule(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"github.com/user/repo", true},
		{"github.com/jesseduffield/lazygit", true},
		{"github.com/golangci/golangci-lint/cmd/golangci-lint", true},
		{"mvdan.cc/gofumpt", true},
		{"golang.org/x/tools", true},
		{"k8s.io/client-go", true},
		{"github.com/User/Repo", true}, // uppercase allowed
		{"github.com/user/repo-name", true},
		{"github.com/user/repo_name", true},
		{"github.com/user/repo.name", true},

		// Invalid
		{"", false},
		{"github.com", false},         // no path
		{"single", false},             // no slash
		{"github.com/", false},        // trailing slash only
		{"/github.com/user", false},   // leading slash
		{"123.com/user/repo", false},  // starts with number
		{"github.com//user", false},   // double slash
		{"github.com/../etc", false},  // path traversal
		{"github.com/user;rm", false}, // shell metachar
		{"github.com/user|cat", false},
		{"github.com/user$var", false},
		{"github.com/user`cmd`", false},
		{"github.com/user'quote", false},
		{"github.com/user\"quote", false},
		// Max length exceeded (257 chars)
		{"github.com/user/" + string(make([]byte, 250)), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidGoModule(tc.name)
			if got != tc.valid {
				t.Errorf("isValidGoModule(%q) = %v, want %v", tc.name, got, tc.valid)
			}
		})
	}
}

func TestInferGoExecutableName(t *testing.T) {
	tests := []struct {
		modulePath  string
		wantExe     string
		wantWarning bool
	}{
		{"github.com/jesseduffield/lazygit", "lazygit", false},
		{"github.com/golangci/golangci-lint/cmd/golangci-lint", "golangci-lint", false},
		{"mvdan.cc/gofumpt", "gofumpt", false},
		{"golang.org/x/tools/cmd/godoc", "godoc", false},
		{"github.com/derailed/k9s", "k9s", false},
		{"github.com/cli/cli/v2/cmd/gh", "gh", false},
	}

	for _, tc := range tests {
		t.Run(tc.modulePath, func(t *testing.T) {
			exe, warning := inferGoExecutableName(tc.modulePath)
			if exe != tc.wantExe {
				t.Errorf("inferGoExecutableName(%q) = %q, want %q", tc.modulePath, exe, tc.wantExe)
			}
			hasWarning := warning != ""
			if hasWarning != tc.wantWarning {
				t.Errorf("inferGoExecutableName(%q) warning = %v, want warning = %v", tc.modulePath, hasWarning, tc.wantWarning)
			}
		})
	}
}

func TestEncodeGoModulePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github.com/user/repo", "github.com/user/repo"},
		{"github.com/User/Repo", "github.com/!user/!repo"},
		{"github.com/UserName/RepoName", "github.com/!user!name/!repo!name"},
		{"ALLCAPS/path", "!a!l!l!c!a!p!s/path"},
		{"mvdan.cc/gofumpt", "mvdan.cc/gofumpt"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := encodeGoModulePath(tc.input)
			if got != tc.want {
				t.Errorf("encodeGoModulePath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestGoBuilder_CanBuild_UppercaseModule(t *testing.T) {
	// Test that uppercase module paths are encoded correctly
	response := `{"Version":"v1.0.0","Time":"2024-01-15T10:30:00Z"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The path should be encoded
		if r.URL.Path == "/github.com/!user/!repo/@latest" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	canBuild, err := builder.CanBuild(ctx, BuildRequest{Package: "github.com/User/Repo"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if !canBuild {
		t.Error("CanBuild() = false, want true for uppercase module")
	}
}

// Helper function defined in cargo_test.go but we need it here too
func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestGoBuilder_Probe_QualityMetadata(t *testing.T) {
	// Mock Go proxy responses with origin data and version list
	latestResponse := `{
		"Version": "v0.40.0",
		"Time": "2024-01-15T10:30:00Z",
		"Origin": {
			"VCS": "git",
			"URL": "https://github.com/jesseduffield/lazygit",
			"Hash": "abc123",
			"Ref": "refs/tags/v0.40.0"
		}
	}`

	versionList := `v0.36.0
v0.37.0
v0.38.0
v0.39.0
v0.40.0
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github.com/jesseduffield/lazygit/@latest":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(latestResponse))
		case "/github.com/jesseduffield/lazygit/@v/list":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(versionList))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Probe(ctx, "github.com/jesseduffield/lazygit")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result == nil {
		t.Fatal("Probe() returned nil for existing module")
	}

	if result.Source != "github.com/jesseduffield/lazygit" {
		t.Errorf("Probe().Source = %q, want %q", result.Source, "github.com/jesseduffield/lazygit")
	}
	if result.Downloads != 0 {
		t.Errorf("Probe().Downloads = %d, want 0 (Go has no download metrics)", result.Downloads)
	}
	if result.VersionCount != 5 {
		t.Errorf("Probe().VersionCount = %d, want %d", result.VersionCount, 5)
	}
	if !result.HasRepository {
		t.Error("Probe().HasRepository = false, want true (Origin.URL present)")
	}
}

func TestGoBuilder_Probe_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Probe(ctx, "github.com/nonexistent/module")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result != nil {
		t.Error("Probe() should return nil for nonexistent module")
	}
}

func TestGoBuilder_Probe_VersionListFetchFails(t *testing.T) {
	// Test graceful degradation when version list fetch fails
	latestResponse := `{
		"Version": "v1.0.0",
		"Time": "2024-01-15T10:30:00Z",
		"Origin": {
			"VCS": "git",
			"URL": "https://github.com/owner/repo"
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github.com/owner/repo/@latest":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(latestResponse))
		case "/github.com/owner/repo/@v/list":
			// Version list endpoint fails
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Probe(ctx, "github.com/owner/repo")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result == nil {
		t.Fatal("Probe() should return result even when version list fetch fails")
	}

	// Should have HasRepository but VersionCount should be 0
	if result.VersionCount != 0 {
		t.Errorf("Probe().VersionCount = %d, want 0 (version list fetch failed)", result.VersionCount)
	}
	if !result.HasRepository {
		t.Error("Probe().HasRepository = false, want true")
	}
}

func TestGoBuilder_Probe_NoOrigin(t *testing.T) {
	// Test module without Origin field (older modules may not have it)
	latestResponse := `{
		"Version": "v1.0.0",
		"Time": "2024-01-15T10:30:00Z"
	}`

	versionList := `v1.0.0
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github.com/old/module/@latest":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(latestResponse))
		case "/github.com/old/module/@v/list":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(versionList))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Probe(ctx, "github.com/old/module")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result == nil {
		t.Fatal("Probe() returned nil for existing module")
	}

	if result.HasRepository {
		t.Error("Probe().HasRepository = true, want false (no Origin field)")
	}
	if result.VersionCount != 1 {
		t.Errorf("Probe().VersionCount = %d, want 1", result.VersionCount)
	}
}

func TestGoBuilder_Probe_UppercaseModule(t *testing.T) {
	// Test that uppercase modules are encoded correctly for probe
	latestResponse := `{
		"Version": "v1.0.0",
		"Time": "2024-01-15T10:30:00Z",
		"Origin": {
			"VCS": "git",
			"URL": "https://github.com/User/Repo"
		}
	}`

	versionList := `v1.0.0
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github.com/!user/!repo/@latest":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(latestResponse))
		case "/github.com/!user/!repo/@v/list":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(versionList))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewGoBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Probe(ctx, "github.com/User/Repo")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result == nil {
		t.Fatal("Probe() returned nil for uppercase module")
	}

	if result.Source != "github.com/User/Repo" {
		t.Errorf("Probe().Source = %q, want %q", result.Source, "github.com/User/Repo")
	}
}
