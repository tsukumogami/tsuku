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

	canBuild, err := builder.CanBuild(ctx, "github.com/jesseduffield/lazygit")
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

	canBuild, err := builder.CanBuild(ctx, "github.com/nonexistent/module")
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
	canBuild, err := builder.CanBuild(ctx, "invalid;module")
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

	canBuild, err := builder.CanBuild(ctx, "github.com/retracted/module")
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

	result, err := builder.Build(ctx, "github.com/jesseduffield/lazygit", "")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Verify recipe structure
	if result.Recipe == nil {
		t.Fatal("Build() result.Recipe is nil")
	}

	// Name should be inferred executable (lazygit)
	if result.Recipe.Metadata.Name != "lazygit" {
		t.Errorf("Recipe.Metadata.Name = %q, want %q", result.Recipe.Metadata.Name, "lazygit")
	}

	if result.Recipe.Metadata.Description != "Go CLI tool from github.com/jesseduffield/lazygit" {
		t.Errorf("Recipe.Metadata.Description = %q", result.Recipe.Metadata.Description)
	}

	if result.Recipe.Metadata.Homepage != "https://pkg.go.dev/github.com/jesseduffield/lazygit" {
		t.Errorf("Recipe.Metadata.Homepage = %q", result.Recipe.Metadata.Homepage)
	}

	// Check dependencies
	if len(result.Recipe.Metadata.Dependencies) != 1 || result.Recipe.Metadata.Dependencies[0] != "go" {
		t.Errorf("Recipe.Metadata.Dependencies = %v, want [go]", result.Recipe.Metadata.Dependencies)
	}

	// Check version source
	if result.Recipe.Version.Source != "goproxy" {
		t.Errorf("Recipe.Version.Source = %q, want %q", result.Recipe.Version.Source, "goproxy")
	}

	// Check steps
	if len(result.Recipe.Steps) != 1 {
		t.Fatalf("len(Recipe.Steps) = %d, want 1", len(result.Recipe.Steps))
	}

	if result.Recipe.Steps[0].Action != "go_install" {
		t.Errorf("Recipe.Steps[0].Action = %q, want %q", result.Recipe.Steps[0].Action, "go_install")
	}

	// Check module param
	module, ok := result.Recipe.Steps[0].Params["module"].(string)
	if !ok || module != "github.com/jesseduffield/lazygit" {
		t.Errorf("module param = %v, want github.com/jesseduffield/lazygit", result.Recipe.Steps[0].Params["module"])
	}

	// Check executables param
	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok || len(executables) != 1 || executables[0] != "lazygit" {
		t.Errorf("executables param = %v, want [lazygit]", result.Recipe.Steps[0].Params["executables"])
	}

	// Check verify command
	if result.Recipe.Verify.Command != "lazygit --version" {
		t.Errorf("Verify.Command = %q, want %q", result.Recipe.Verify.Command, "lazygit --version")
	}

	// Check source
	if result.Source != "goproxy:github.com/jesseduffield/lazygit" {
		t.Errorf("result.Source = %q, want %q", result.Source, "goproxy:github.com/jesseduffield/lazygit")
	}

	// Should not have warnings for simple module
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

	result, err := builder.Build(ctx, "github.com/golangci/golangci-lint/cmd/golangci-lint", "")
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

	result, err := builder.Build(ctx, "mvdan.cc/gofumpt", "")
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

	_, err := builder.Build(ctx, "github.com/nonexistent/module", "")
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

	_, err := builder.Build(ctx, "github.com/jesseduffield/lazygit", "")
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

	_, err := builder.Build(ctx, "invalid;module", "")
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

	canBuild, err := builder.CanBuild(ctx, "github.com/User/Repo")
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
