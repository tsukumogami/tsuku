package builders

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tsukumogami/tsuku/internal/llm"
	"github.com/tsukumogami/tsuku/internal/telemetry"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// mockProvider is a simple mock implementation of llm.Provider for testing.
type mockProvider struct {
	name      string
	responses []*llm.CompletionResponse
	callCount int
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.CompletionResponse, error) {
	if m.callCount < len(m.responses) {
		resp := m.responses[m.callCount]
		m.callCount++
		return resp, nil
	}
	// Default response with extract_pattern tool call
	return &llm.CompletionResponse{
		Content:    "Analyzing the release...",
		StopReason: "tool_use",
		ToolCalls: []llm.ToolCall{
			{
				ID:   "call_1",
				Name: llm.ToolExtractPattern,
				Arguments: map[string]any{
					"mappings": []map[string]any{
						{"os": "linux", "arch": "amd64", "asset": "test_linux_amd64.tar.gz", "format": "tar.gz"},
					},
					"executable":     "test",
					"verify_command": "test --version",
				},
			},
		},
		Usage: llm.Usage{InputTokens: 100, OutputTokens: 50},
	}, nil
}

// createMockFactory creates a factory with a mock provider for testing.
func createMockFactory(provider llm.Provider) *llm.Factory {
	providers := map[string]llm.Provider{
		provider.Name(): provider,
	}
	return llm.NewFactoryWithProviders(providers)
}

func TestParseRepo(t *testing.T) {
	tests := []struct {
		name      string
		sourceArg string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "valid owner/repo",
			sourceArg: "cli/cli",
			wantOwner: "cli",
			wantRepo:  "cli",
			wantErr:   false,
		},
		{
			name:      "valid with org",
			sourceArg: "FiloSottile/age",
			wantOwner: "FiloSottile",
			wantRepo:  "age",
			wantErr:   false,
		},
		{
			name:      "empty string",
			sourceArg: "",
			wantErr:   true,
		},
		{
			name:      "no slash",
			sourceArg: "cli",
			wantErr:   true,
		},
		{
			name:      "empty owner",
			sourceArg: "/repo",
			wantErr:   true,
		},
		{
			name:      "empty repo",
			sourceArg: "owner/",
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, err := parseRepo(tc.sourceArg)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if owner != tc.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tc.wantOwner)
			}
			if repo != tc.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tc.wantRepo)
			}
		})
	}
}

func TestDeriveAssetPattern(t *testing.T) {
	tests := []struct {
		name     string
		mappings []llm.PlatformMapping
		want     string
	}{
		{
			name: "go style with version",
			mappings: []llm.PlatformMapping{
				{Asset: "gh_2.42.0_linux_amd64.tar.gz", OS: "linux", Arch: "amd64", Format: "tar.gz"},
			},
			want: "gh_2.42.0_{os}_{arch}.tar.gz",
		},
		{
			name: "rust style",
			mappings: []llm.PlatformMapping{
				{Asset: "ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz", OS: "x86_64-unknown-linux-musl", Arch: "", Format: "tar.gz"},
			},
			want: "ripgrep-14.1.0-{os}.tar.gz",
		},
		{
			name: "binary format",
			mappings: []llm.PlatformMapping{
				{Asset: "k3d-linux-amd64", OS: "linux", Arch: "amd64", Format: "binary"},
			},
			want: "k3d-{os}-{arch}",
		},
		{
			name:     "empty mappings",
			mappings: []llm.PlatformMapping{},
			want:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveAssetPattern(tc.mappings)
			if got != tc.want {
				t.Errorf("deriveAssetPattern() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGitHubReleaseBuilder_Name(t *testing.T) {
	ctx := context.Background()
	mockProv := &mockProvider{name: "mock"}
	factory := createMockFactory(mockProv)

	b, err := NewGitHubReleaseBuilder(ctx, WithFactory(factory))
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	if b.Name() != "github" {
		t.Errorf("Name() = %q, want %q", b.Name(), "github")
	}
}

func TestGitHubReleaseBuilder_CanBuild(t *testing.T) {
	ctx := context.Background()
	mockProv := &mockProvider{name: "mock"}
	factory := createMockFactory(mockProv)

	b, err := NewGitHubReleaseBuilder(ctx, WithFactory(factory))
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	// CanBuild always returns false because this builder needs SourceArg
	can, err := b.CanBuild(context.Background(), "some-package")
	if err != nil {
		t.Errorf("CanBuild error: %v", err)
	}
	if can {
		t.Error("CanBuild should return false for github builder")
	}
}

func TestGitHubReleaseBuilder_FetchReleases(t *testing.T) {
	ctx := context.Background()
	mockProv := &mockProvider{name: "mock"}
	factory := createMockFactory(mockProv)

	// Create mock GitHub API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/cli/cli/releases" {
			releases := []githubRelease{
				{
					TagName: "v2.42.0",
					Assets: []githubAsset{
						{Name: "gh_2.42.0_linux_amd64.tar.gz"},
						{Name: "gh_2.42.0_darwin_arm64.zip"},
					},
				},
				{
					TagName: "v2.41.0",
					Assets: []githubAsset{
						{Name: "gh_2.41.0_linux_amd64.tar.gz"},
						{Name: "gh_2.41.0_darwin_arm64.zip"},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(releases)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	b, err := NewGitHubReleaseBuilder(ctx, WithFactory(factory), WithGitHubBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	releases, err := b.fetchReleases(context.Background(), "cli", "cli")
	if err != nil {
		t.Fatalf("fetchReleases error: %v", err)
	}

	if len(releases) != 2 {
		t.Errorf("expected 2 releases, got %d", len(releases))
	}

	if releases[0].Tag != "v2.42.0" {
		t.Errorf("first release tag = %q, want %q", releases[0].Tag, "v2.42.0")
	}

	if len(releases[0].Assets) != 2 {
		t.Errorf("expected 2 assets, got %d", len(releases[0].Assets))
	}
}

func TestGitHubReleaseBuilder_FetchReleases_NotFound(t *testing.T) {
	ctx := context.Background()
	mockProv := &mockProvider{name: "mock"}
	factory := createMockFactory(mockProv)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	b, err := NewGitHubReleaseBuilder(ctx, WithFactory(factory), WithGitHubBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	_, err = b.fetchReleases(context.Background(), "nonexistent", "repo")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestGitHubReleaseBuilder_FetchRepoMeta(t *testing.T) {
	ctx := context.Background()
	mockProv := &mockProvider{name: "mock"}
	factory := createMockFactory(mockProv)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/cli/cli" {
			repo := githubRepo{
				Description: "GitHub's official command line tool",
				Homepage:    "https://cli.github.com",
				HTMLURL:     "https://github.com/cli/cli",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(repo)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	b, err := NewGitHubReleaseBuilder(ctx, WithFactory(factory), WithGitHubBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	meta, err := b.fetchRepoMeta(context.Background(), "cli", "cli")
	if err != nil {
		t.Fatalf("fetchRepoMeta error: %v", err)
	}

	if meta.Description != "GitHub's official command line tool" {
		t.Errorf("description = %q, want GitHub's official command line tool", meta.Description)
	}

	if meta.Homepage != "https://cli.github.com" {
		t.Errorf("homepage = %q, want https://cli.github.com", meta.Homepage)
	}
}

func TestGitHubReleaseBuilder_FetchRepoMeta_FallbackHomepage(t *testing.T) {
	ctx := context.Background()
	mockProv := &mockProvider{name: "mock"}
	factory := createMockFactory(mockProv)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repo := githubRepo{
			Description: "A tool without homepage",
			Homepage:    "", // No homepage
			HTMLURL:     "https://github.com/owner/repo",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repo)
	}))
	defer server.Close()

	b, err := NewGitHubReleaseBuilder(ctx, WithFactory(factory), WithGitHubBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	meta, err := b.fetchRepoMeta(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("fetchRepoMeta error: %v", err)
	}

	// Should fall back to GitHub URL
	if meta.Homepage != "https://github.com/owner/repo" {
		t.Errorf("homepage = %q, want https://github.com/owner/repo", meta.Homepage)
	}
}

func TestGenerateRecipe_Archive(t *testing.T) {
	meta := &repoMeta{
		Description: "Test tool",
		Homepage:    "https://example.com",
	}

	pattern := &llm.AssetPattern{
		Mappings: []llm.PlatformMapping{
			{Asset: "tool_1.0.0_linux_amd64.tar.gz", OS: "linux", Arch: "amd64", Format: "tar.gz"},
			{Asset: "tool_1.0.0_darwin_arm64.tar.gz", OS: "darwin", Arch: "arm64", Format: "tar.gz"},
		},
		Executable:    "tool",
		VerifyCommand: "tool --version",
	}

	r, err := generateRecipe("tool", "owner/tool", meta, pattern)
	if err != nil {
		t.Fatalf("generateRecipe error: %v", err)
	}

	if r.Metadata.Name != "tool" {
		t.Errorf("name = %q, want %q", r.Metadata.Name, "tool")
	}

	if len(r.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(r.Steps))
	}

	if r.Steps[0].Action != "github_archive" {
		t.Errorf("action = %q, want %q", r.Steps[0].Action, "github_archive")
	}

	if r.Steps[0].Params["archive_format"] != "tar.gz" {
		t.Errorf("archive_format = %v, want tar.gz", r.Steps[0].Params["archive_format"])
	}
}

func TestGenerateRecipe_Binary(t *testing.T) {
	meta := &repoMeta{
		Description: "Binary tool",
		Homepage:    "https://example.com",
	}

	pattern := &llm.AssetPattern{
		Mappings: []llm.PlatformMapping{
			{Asset: "k3d-linux-amd64", OS: "linux", Arch: "amd64", Format: "binary"},
			{Asset: "k3d-darwin-arm64", OS: "darwin", Arch: "arm64", Format: "binary"},
		},
		Executable:    "k3d",
		VerifyCommand: "k3d version",
	}

	r, err := generateRecipe("k3d", "k3d-io/k3d", meta, pattern)
	if err != nil {
		t.Fatalf("generateRecipe error: %v", err)
	}

	if len(r.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(r.Steps))
	}

	if r.Steps[0].Action != "github_file" {
		t.Errorf("action = %q, want %q", r.Steps[0].Action, "github_file")
	}

	if r.Steps[0].Params["binary"] != "k3d" {
		t.Errorf("binary = %v, want k3d", r.Steps[0].Params["binary"])
	}
}

func TestGenerateRecipe_EmptyMappings(t *testing.T) {
	meta := &repoMeta{
		Description: "Test",
		Homepage:    "https://example.com",
	}

	pattern := &llm.AssetPattern{
		Mappings:      []llm.PlatformMapping{},
		Executable:    "tool",
		VerifyCommand: "tool --version",
	}

	_, err := generateRecipe("tool", "owner/tool", meta, pattern)
	if err == nil {
		t.Error("expected error for empty mappings")
	}
}

func TestGitHubReleaseBuilder_Build_ValidationSkipped(t *testing.T) {
	ctx := context.Background()
	mockProv := &mockProvider{name: "mock"}
	factory := createMockFactory(mockProv)

	// Create mock GitHub API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/cli/cli/releases":
			releases := []githubRelease{
				{
					TagName: "v2.42.0",
					Assets: []githubAsset{
						{Name: "gh_2.42.0_linux_amd64.tar.gz"},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(releases)
		case "/repos/cli/cli":
			repo := githubRepo{
				Description: "GitHub CLI",
				Homepage:    "https://cli.github.com",
				HTMLURL:     "https://github.com/cli/cli",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(repo)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Build without executor - validation should be skipped
	b, err := NewGitHubReleaseBuilder(ctx, WithFactory(factory), WithGitHubBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	result, err := b.Build(ctx, BuildRequest{
		Package:   "gh",
		SourceArg: "cli/cli",
	})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	if result.Recipe == nil {
		t.Fatal("expected recipe, got nil")
	}

	if !result.ValidationSkipped {
		t.Error("expected ValidationSkipped to be true when no executor")
	}

	if result.Provider != "mock" {
		t.Errorf("Provider = %q, want %q", result.Provider, "mock")
	}
}

func TestGitHubReleaseBuilder_BuildRepairMessage(t *testing.T) {
	ctx := context.Background()
	mockProv := &mockProvider{name: "mock"}
	factory := createMockFactory(mockProv)

	b, err := NewGitHubReleaseBuilder(ctx, WithFactory(factory))
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	result := &validate.ValidationResult{
		Passed:   false,
		ExitCode: 127,
		Stdout:   "",
		Stderr:   "sh: mytool: not found",
	}

	msg := b.buildRepairMessage(result)

	// Should contain sanitized error
	if msg == "" {
		t.Error("expected non-empty repair message")
	}

	// Should contain error category
	if !contains(msg, "Error category:") {
		t.Error("expected repair message to contain error category")
	}

	// Should contain exit code
	if !contains(msg, "Exit code: 127") {
		t.Error("expected repair message to contain exit code")
	}
}

func TestGitHubReleaseBuilder_FormatValidationError(t *testing.T) {
	ctx := context.Background()
	mockProv := &mockProvider{name: "mock"}
	factory := createMockFactory(mockProv)

	b, err := NewGitHubReleaseBuilder(ctx, WithFactory(factory))
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	result := &validate.ValidationResult{
		Passed:   false,
		ExitCode: 1,
		Stdout:   "some output",
		Stderr:   "error message",
	}

	formatted := b.formatValidationError(result)

	if !contains(formatted, "exit code 1") {
		t.Error("expected formatted error to contain exit code")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestWithTelemetryClient(t *testing.T) {
	ctx := context.Background()
	mockProv := &mockProvider{name: "mock"}
	factory := createMockFactory(mockProv)

	// Create a telemetry client (disabled to avoid actual sends)
	telemetryClient := telemetry.NewClientWithOptions("http://unused", 0, true, false)

	b, err := NewGitHubReleaseBuilder(ctx, WithFactory(factory), WithTelemetryClient(telemetryClient))
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	// Verify the telemetry client was set correctly
	if b.telemetryClient != telemetryClient {
		t.Error("telemetry client not set correctly")
	}
}

// mockProgressReporter records progress events for testing.
type mockProgressReporter struct {
	stages []string
	dones  []string
	fails  int
}

func (m *mockProgressReporter) OnStageStart(stage string) {
	m.stages = append(m.stages, stage)
}

func (m *mockProgressReporter) OnStageDone(detail string) {
	m.dones = append(m.dones, detail)
}

func (m *mockProgressReporter) OnStageFailed() {
	m.fails++
}

func TestWithProgressReporter(t *testing.T) {
	ctx := context.Background()
	mockProv := &mockProvider{name: "mock"}
	factory := createMockFactory(mockProv)

	reporter := &mockProgressReporter{}

	b, err := NewGitHubReleaseBuilder(ctx, WithFactory(factory), WithProgressReporter(reporter))
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	// Verify the progress reporter was set correctly
	if b.progress != reporter {
		t.Error("progress reporter not set correctly")
	}
}

func TestProgressReporterHelpers(t *testing.T) {
	ctx := context.Background()
	mockProv := &mockProvider{name: "mock"}
	factory := createMockFactory(mockProv)

	reporter := &mockProgressReporter{}

	b, err := NewGitHubReleaseBuilder(ctx, WithFactory(factory), WithProgressReporter(reporter))
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	// Test reportStart
	b.reportStart("Fetching metadata")
	if len(reporter.stages) != 1 || reporter.stages[0] != "Fetching metadata" {
		t.Errorf("reportStart: got stages=%v, want [Fetching metadata]", reporter.stages)
	}

	// Test reportDone
	b.reportDone("v1.0.0, 5 assets")
	if len(reporter.dones) != 1 || reporter.dones[0] != "v1.0.0, 5 assets" {
		t.Errorf("reportDone: got dones=%v, want [v1.0.0, 5 assets]", reporter.dones)
	}

	// Test reportFailed
	b.reportFailed()
	if reporter.fails != 1 {
		t.Errorf("reportFailed: got fails=%d, want 1", reporter.fails)
	}
}

func TestProgressReporterNilSafe(t *testing.T) {
	ctx := context.Background()
	mockProv := &mockProvider{name: "mock"}
	factory := createMockFactory(mockProv)

	// No progress reporter set
	b, err := NewGitHubReleaseBuilder(ctx, WithFactory(factory))
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	// These should not panic when progress is nil
	b.reportStart("test")
	b.reportDone("test")
	b.reportFailed()
}

func TestProgressReporterCalledDuringBuild(t *testing.T) {
	// Set up mock GitHub server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/test/repo/releases":
			releases := []map[string]any{
				{
					"tag_name": "v1.0.0",
					"assets": []map[string]any{
						{"name": "test_linux_amd64.tar.gz", "browser_download_url": "http://example.com/test.tar.gz"},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(releases)
		case "/repos/test/repo":
			repo := map[string]any{
				"description": "Test tool",
				"homepage":    "https://example.com",
				"html_url":    "https://github.com/test/repo",
			}
			_ = json.NewEncoder(w).Encode(repo)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	mockProv := &mockProvider{name: "claude"}
	factory := createMockFactory(mockProv)

	reporter := &mockProgressReporter{}
	telemetryClient := telemetry.NewClientWithOptions("http://unused", 0, true, false)

	b, err := NewGitHubReleaseBuilder(ctx,
		WithFactory(factory),
		WithGitHubBaseURL(server.URL),
		WithProgressReporter(reporter),
		WithTelemetryClient(telemetryClient),
	)
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	// Run build
	_, err = b.Build(ctx, BuildRequest{
		Package:   "test",
		SourceArg: "test/repo",
	})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// Verify progress events were emitted
	// Should have: "Fetching release metadata" and "Analyzing assets with claude"
	if len(reporter.stages) < 2 {
		t.Errorf("expected at least 2 stages, got %d: %v", len(reporter.stages), reporter.stages)
	}

	// Check for expected stage names
	foundMetadata := false
	foundAnalysis := false
	for _, stage := range reporter.stages {
		if stage == "Fetching release metadata" {
			foundMetadata = true
		}
		if stage == "Analyzing assets with claude" {
			foundAnalysis = true
		}
	}

	if !foundMetadata {
		t.Error("expected 'Fetching release metadata' stage")
	}
	if !foundAnalysis {
		t.Error("expected 'Analyzing assets with claude' stage")
	}

	// Verify dones were called (at least for metadata and analysis)
	if len(reporter.dones) < 2 {
		t.Errorf("expected at least 2 done calls, got %d", len(reporter.dones))
	}

	// First done should have version and asset count
	if len(reporter.dones) > 0 && reporter.dones[0] != "v1.0.0, 1 assets" {
		t.Errorf("expected first done to be 'v1.0.0, 1 assets', got %q", reporter.dones[0])
	}
}

func TestGitHubReleaseBuilder_Build_VersionPlaceholders(t *testing.T) {
	ctx := context.Background()

	// Create mock provider that returns {version} placeholders in verify command and pattern
	mockProv := &mockProvider{
		name: "mock",
		responses: []*llm.CompletionResponse{
			{
				Content:    "Analyzing the release...",
				StopReason: "tool_use",
				ToolCalls: []llm.ToolCall{
					{
						ID:   "call_1",
						Name: llm.ToolExtractPattern,
						Arguments: map[string]any{
							"mappings": []map[string]any{
								{"os": "linux", "arch": "amd64", "asset": "tool_1.2.3_linux_amd64.tar.gz", "format": "tar.gz"},
							},
							"executable":     "tool",
							"verify_command": "tool --version | grep {version}",
							"verify_pattern": "tool {version}",
						},
					},
				},
				Usage: llm.Usage{InputTokens: 100, OutputTokens: 50},
			},
		},
	}
	factory := createMockFactory(mockProv)

	// Create mock GitHub API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/test/tool/releases":
			releases := []githubRelease{
				{
					TagName: "v1.2.3",
					Assets: []githubAsset{
						{Name: "tool_1.2.3_linux_amd64.tar.gz"},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(releases)
		case "/repos/test/tool":
			repo := githubRepo{
				Description: "A test tool",
				Homepage:    "https://example.com",
				HTMLURL:     "https://github.com/test/tool",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(repo)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Build without executor - the returned recipe retains {version} placeholders
	// since version substitution only happens internally during validation.
	// The placeholders are preserved so tsuku install can substitute them at runtime.
	b, err := NewGitHubReleaseBuilder(ctx,
		WithFactory(factory),
		WithGitHubBaseURL(server.URL),
	)
	if err != nil {
		t.Fatalf("NewGitHubReleaseBuilder error: %v", err)
	}

	result, err := b.Build(ctx, BuildRequest{
		Package:   "tool",
		SourceArg: "test/tool",
	})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	if result.Recipe == nil {
		t.Fatal("expected recipe, got nil")
	}

	// Verify the {version} placeholder is preserved in the output recipe
	// generateRecipe always uses "{version}" as the pattern (line 872 in github_release.go)
	if result.Recipe.Verify.Pattern != "{version}" {
		t.Errorf("Verify.Pattern = %q, want %q (placeholder preserved)", result.Recipe.Verify.Pattern, "{version}")
	}
	// The verify command from LLM should be preserved with {version} placeholder
	if result.Recipe.Verify.Command != "tool --version | grep {version}" {
		t.Errorf("Verify.Command = %q, want %q (placeholder preserved)", result.Recipe.Verify.Command, "tool --version | grep {version}")
	}
}
