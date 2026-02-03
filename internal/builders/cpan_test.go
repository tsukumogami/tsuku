package builders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCPANBuilder_Name(t *testing.T) {
	builder := NewCPANBuilder(nil)
	if builder.Name() != "cpan" {
		t.Errorf("Name() = %q, want %q", builder.Name(), "cpan")
	}
}

func TestCPANBuilder_CanBuild_ValidDistribution(t *testing.T) {
	// Mock MetaCPAN API response
	response := `{
		"distribution": "App-Ack",
		"version": "3.7.0",
		"abstract": "grep-like text finder"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/release/App-Ack" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCPANBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	canBuild, err := builder.CanBuild(ctx, BuildRequest{Package: "App-Ack"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if !canBuild {
		t.Error("CanBuild() = false, want true")
	}
}

func TestCPANBuilder_CanBuild_ModuleName(t *testing.T) {
	// Test that module names (App::Ack) are normalized to distribution names (App-Ack)
	response := `{
		"distribution": "App-Ack",
		"version": "3.7.0",
		"abstract": "grep-like text finder"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/release/App-Ack" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCPANBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	// Pass module name with ::
	canBuild, err := builder.CanBuild(ctx, BuildRequest{Package: "App::Ack"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if !canBuild {
		t.Error("CanBuild() = false, want true for module name")
	}
}

func TestCPANBuilder_CanBuild_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	builder := NewCPANBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	canBuild, err := builder.CanBuild(ctx, BuildRequest{Package: "Nonexistent-Distribution"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Error("CanBuild() = true, want false for nonexistent distribution")
	}
}

func TestCPANBuilder_CanBuild_InvalidName(t *testing.T) {
	builder := NewCPANBuilder(nil)
	ctx := context.Background()

	// Invalid distribution name should return false without making any HTTP requests
	canBuild, err := builder.CanBuild(ctx, BuildRequest{Package: "invalid distribution!"})
	if err != nil {
		t.Fatalf("CanBuild() error = %v", err)
	}
	if canBuild {
		t.Error("CanBuild() = true, want false for invalid distribution name")
	}
}

func TestCPANBuilder_Build_AppDistribution(t *testing.T) {
	response := `{
		"distribution": "App-Ack",
		"version": "3.7.0",
		"abstract": "grep-like text finder"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/release/App-Ack" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCPANBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Build(ctx, BuildRequest{Package: "App-Ack"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Verify recipe structure
	if result.Recipe == nil {
		t.Fatal("Build() result.Recipe is nil")
	}

	// Name should be inferred executable (ack)
	if result.Recipe.Metadata.Name != "ack" {
		t.Errorf("Recipe.Metadata.Name = %q, want %q", result.Recipe.Metadata.Name, "ack")
	}

	if result.Recipe.Metadata.Description != "grep-like text finder" {
		t.Errorf("Recipe.Metadata.Description = %q", result.Recipe.Metadata.Description)
	}

	if result.Recipe.Metadata.Homepage != "https://metacpan.org/dist/App-Ack" {
		t.Errorf("Recipe.Metadata.Homepage = %q", result.Recipe.Metadata.Homepage)
	}

	// Check dependencies
	if len(result.Recipe.Metadata.Dependencies) != 1 || result.Recipe.Metadata.Dependencies[0] != "perl" {
		t.Errorf("Recipe.Metadata.Dependencies = %v, want [perl]", result.Recipe.Metadata.Dependencies)
	}

	// Check version source
	if result.Recipe.Version.Source != "metacpan:App-Ack" {
		t.Errorf("Recipe.Version.Source = %q, want %q", result.Recipe.Version.Source, "metacpan:App-Ack")
	}

	// Check steps
	if len(result.Recipe.Steps) != 1 {
		t.Fatalf("len(Recipe.Steps) = %d, want 1", len(result.Recipe.Steps))
	}

	if result.Recipe.Steps[0].Action != "cpan_install" {
		t.Errorf("Recipe.Steps[0].Action = %q, want %q", result.Recipe.Steps[0].Action, "cpan_install")
	}

	// Check distribution param
	dist, ok := result.Recipe.Steps[0].Params["distribution"].(string)
	if !ok || dist != "App-Ack" {
		t.Errorf("distribution param = %v, want App-Ack", result.Recipe.Steps[0].Params["distribution"])
	}

	// Check executables param
	executables, ok := result.Recipe.Steps[0].Params["executables"].([]string)
	if !ok || len(executables) != 1 || executables[0] != "ack" {
		t.Errorf("executables param = %v, want [ack]", result.Recipe.Steps[0].Params["executables"])
	}

	// Check verify command
	if result.Recipe.Verify.Command != "ack --version" {
		t.Errorf("Verify.Command = %q, want %q", result.Recipe.Verify.Command, "ack --version")
	}

	// Check source
	if result.Source != "metacpan:App-Ack" {
		t.Errorf("result.Source = %q, want %q", result.Source, "metacpan:App-Ack")
	}

	// App-* distributions should not have warnings
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings for App-* distribution, got %v", result.Warnings)
	}
}

func TestCPANBuilder_Build_NonAppDistribution(t *testing.T) {
	response := `{
		"distribution": "Perl-Critic",
		"version": "1.152",
		"abstract": "Critique Perl source code for best-practices"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/release/Perl-Critic" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCPANBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Build(ctx, BuildRequest{Package: "Perl-Critic"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Name should be inferred (perlcritic)
	if result.Recipe.Metadata.Name != "perlcritic" {
		t.Errorf("Recipe.Metadata.Name = %q, want %q", result.Recipe.Metadata.Name, "perlcritic")
	}

	// Non-App distributions should have a warning
	if len(result.Warnings) == 0 {
		t.Error("expected warning for non-App distribution")
	}
}

func TestCPANBuilder_Build_ModuleName(t *testing.T) {
	response := `{
		"distribution": "App-Ack",
		"version": "3.7.0",
		"abstract": "grep-like text finder"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/release/App-Ack" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCPANBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	// Build with module name (App::Ack)
	result, err := builder.Build(ctx, BuildRequest{Package: "App::Ack"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Distribution should be normalized
	dist, ok := result.Recipe.Steps[0].Params["distribution"].(string)
	if !ok || dist != "App-Ack" {
		t.Errorf("distribution param = %v, want App-Ack", result.Recipe.Steps[0].Params["distribution"])
	}
}

func TestCPANBuilder_Build_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	builder := NewCPANBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	_, err := builder.Build(ctx, BuildRequest{Package: "Nonexistent-Distribution"})
	if err == nil {
		t.Error("Build() should fail for nonexistent distribution")
	}
}

func TestCPANBuilder_Build_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	builder := NewCPANBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	_, err := builder.Build(ctx, BuildRequest{Package: "App-Ack"})
	if err == nil {
		t.Error("Build() should fail on rate limit")
	}
	if err.Error() != "failed to fetch distribution info: metacpan.org rate limit exceeded" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIsValidCPANDistribution(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"App-Ack", true},
		{"Perl-Critic", true},
		{"File-Slurp", true},
		{"A", true},
		{"App123", true},
		{"App-123", true},
		{"", false},
		{"1App", false},     // starts with number
		{"-App", false},     // starts with hyphen
		{"App--Ack", false}, // double hyphen
		{"App-", false},     // ends with hyphen
		{"App::Ack", false}, // module name format (contains ::)
		{"has spaces", false},
		{"has@special", false},
		// 129 characters (too long)
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidCPANDistribution(tc.name)
			if got != tc.valid {
				t.Errorf("isValidCPANDistribution(%q) = %v, want %v", tc.name, got, tc.valid)
			}
		})
	}
}

func TestNormalizeToDistribution(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"App::Ack", "App-Ack"},
		{"Perl::Critic", "Perl-Critic"},
		{"App::Cmd::Simple", "App-Cmd-Simple"},
		{"App-Ack", "App-Ack"}, // already distribution format
		{"simple", "simple"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeToDistribution(tc.input)
			if got != tc.want {
				t.Errorf("normalizeToDistribution(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestInferExecutableName(t *testing.T) {
	tests := []struct {
		distribution string
		wantExe      string
		wantWarning  bool
	}{
		{"App-Ack", "ack", false},
		{"App-cpanminus", "cpanminus", false},
		{"App-perlbrew", "perlbrew", false},
		{"App-Prove", "prove", false},
		{"Perl-Critic", "perlcritic", true}, // non-App, should warn
		{"File-Slurp", "fileslurp", true},   // non-App, should warn
		{"Dist-Zilla", "distzilla", true},   // non-App, should warn
	}

	for _, tc := range tests {
		t.Run(tc.distribution, func(t *testing.T) {
			exe, warning := inferExecutableName(tc.distribution)
			if exe != tc.wantExe {
				t.Errorf("inferExecutableName(%q) = %q, want %q", tc.distribution, exe, tc.wantExe)
			}
			hasWarning := warning != ""
			if hasWarning != tc.wantWarning {
				t.Errorf("inferExecutableName(%q) warning = %v, want warning = %v", tc.distribution, hasWarning, tc.wantWarning)
			}
		})
	}
}

func TestCPANBuilder_Build_InvalidContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	defer server.Close()

	builder := NewCPANBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	_, err := builder.Build(ctx, BuildRequest{Package: "App-Ack"})
	if err == nil {
		t.Error("Build() should fail on invalid content type")
	}
}

func TestCPANBuilder_Probe_ReturnsQualityMetadata(t *testing.T) {
	// Mock MetaCPAN API responses for both endpoints
	releaseResponse := `{
		"distribution": "App-Ack",
		"version": "3.7.0",
		"abstract": "grep-like text finder"
	}`
	distributionResponse := `{
		"name": "App-Ack",
		"river": {
			"total": 42,
			"bucket": "5",
			"immediate": 10
		},
		"resources": {
			"repository": {
				"url": "https://github.com/beyondgrep/ack3",
				"type": "git"
			}
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/release/App-Ack":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(releaseResponse))
		case "/distribution/App-Ack":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(distributionResponse))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCPANBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Probe(ctx, "App-Ack")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result == nil {
		t.Fatal("Probe() returned nil result")
	}

	if result.Source != "App-Ack" {
		t.Errorf("Source = %q, want %q", result.Source, "App-Ack")
	}
	if result.Downloads != 42 {
		t.Errorf("Downloads = %d, want 42", result.Downloads)
	}
	if !result.HasRepository {
		t.Error("HasRepository = false, want true")
	}
}

func TestCPANBuilder_Probe_GracefulDegradation(t *testing.T) {
	// Distribution endpoint fails but release endpoint succeeds
	releaseResponse := `{
		"distribution": "App-Ack",
		"version": "3.7.0",
		"abstract": "grep-like text finder"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release/App-Ack":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(releaseResponse))
		case "/distribution/App-Ack":
			// Simulate distribution endpoint failure
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	builder := NewCPANBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Probe(ctx, "App-Ack")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if result == nil {
		t.Fatal("Probe() returned nil result - should succeed with graceful degradation")
	}

	// Graceful degradation: Downloads should be 0, HasRepository false
	if result.Downloads != 0 {
		t.Errorf("Downloads = %d, want 0 (graceful degradation)", result.Downloads)
	}
	if result.HasRepository {
		t.Error("HasRepository = true, want false (graceful degradation)")
	}
}

func TestCPANBuilder_Probe_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	builder := NewCPANBuilderWithBaseURL(nil, server.URL)
	ctx := context.Background()

	result, err := builder.Probe(ctx, "Nonexistent-Distribution")
	if err != nil {
		t.Fatalf("Probe() error = %v, want nil", err)
	}
	if result != nil {
		t.Errorf("Probe() result = %v, want nil for not found", result)
	}
}
