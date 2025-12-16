package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// setTestAPIKey sets the ANTHROPIC_API_KEY for testing and returns a cleanup function.
func setTestAPIKey(t *testing.T, key string) func() {
	t.Helper()
	original := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		_ = os.Unsetenv("ANTHROPIC_API_KEY")
	} else {
		_ = os.Setenv("ANTHROPIC_API_KEY", key)
	}
	return func() {
		if original != "" {
			_ = os.Setenv("ANTHROPIC_API_KEY", original)
		} else {
			_ = os.Unsetenv("ANTHROPIC_API_KEY")
		}
	}
}

func TestNewClient_NoAPIKey(t *testing.T) {
	cleanup := setTestAPIKey(t, "")
	defer cleanup()

	_, err := NewClient()
	if err == nil {
		t.Error("expected error when ANTHROPIC_API_KEY is not set")
	}
}

func TestNewClient_WithAPIKey(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	client, err := NewClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Error("expected non-nil client")
	}
}

func TestNewClientWithHTTPClient(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	customClient := &http.Client{Timeout: 30 * time.Second}
	client, err := NewClientWithHTTPClient(customClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.httpClient != customClient {
		t.Error("expected custom HTTP client to be used")
	}
}

func TestFetchFile(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	// Create a test server that simulates raw.githubusercontent.com
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the path is constructed correctly
		expectedPath := "/test-owner/test-repo/v1.0.0/README.md"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %q, got %q", expectedPath, r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello, World!"))
	}))
	defer server.Close()

	// Create client with custom HTTP client that redirects to our test server
	customClient := &http.Client{
		Transport: &testTransport{server: server},
	}
	client, err := NewClientWithHTTPClient(customClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := client.fetchFile(context.Background(), "test-owner/test-repo", "v1.0.0", "README.md")
	if err != nil {
		t.Fatalf("fetchFile error: %v", err)
	}

	if content != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", content)
	}
}

// testTransport redirects requests to a test server while preserving the path
type testTransport struct {
	server *httptest.Server
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect to test server but keep the path
	testURL := t.server.URL + req.URL.Path
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, testURL, req.Body)
	if err != nil {
		return nil, err
	}
	return http.DefaultTransport.RoundTrip(newReq)
}

func TestFetchFile_NotFound(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	// Create a test server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	customClient := &http.Client{
		Transport: &testTransport{server: server},
	}
	client, err := NewClientWithHTTPClient(customClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = client.fetchFile(context.Background(), "owner/repo", "v1.0.0", "INSTALL.md")
	if err == nil {
		t.Error("expected error for HTTP 404")
	}

	// Should contain helpful error message
	errMsg := err.Error()
	if !containsSubstring(errMsg, "file not found") {
		t.Errorf("expected helpful error message containing 'file not found', got: %s", errMsg)
	}
	if !containsSubstring(errMsg, "INSTALL.md") {
		t.Errorf("expected error message to contain file path, got: %s", errMsg)
	}
}

func TestFetchFile_BinaryContentType(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	// Create a test server that returns binary content type
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0x00, 0x01, 0x02})
	}))
	defer server.Close()

	customClient := &http.Client{
		Transport: &testTransport{server: server},
	}
	client, err := NewClientWithHTTPClient(customClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = client.fetchFile(context.Background(), "owner/repo", "v1.0.0", "binary.exe")
	if err == nil {
		t.Error("expected error for binary content type")
	}

	errMsg := err.Error()
	if !containsSubstring(errMsg, "binary") {
		t.Errorf("expected error message about binary content, got: %s", errMsg)
	}
}

func TestIsTextContentType(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		{"text/plain", true},
		{"text/html", true},
		{"text/markdown", true},
		{"application/json", true},
		{"application/xml", true},
		{"application/javascript", true},
		{"application/x-yaml", true},
		{"application/toml", true},
		{"application/octet-stream", false},
		{"image/png", false},
		{"application/zip", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			got := isTextContentType(tt.contentType)
			if got != tt.want {
				t.Errorf("isTextContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

// containsSubstring checks if s contains substr
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFetchFile_ServerError(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	// Create a test server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	customClient := &http.Client{
		Transport: &testTransport{server: server},
	}
	client, err := NewClientWithHTTPClient(customClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = client.fetchFile(context.Background(), "owner/repo", "v1.0.0", "file.md")
	if err == nil {
		t.Error("expected error for HTTP 500")
	}

	errMsg := err.Error()
	if !containsSubstring(errMsg, "500") {
		t.Errorf("expected error message to contain status code, got: %s", errMsg)
	}
}

func TestFetchFile_EmptyContentType(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	// Create a test server that returns no content type (should be allowed)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No Content-Type header set
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content without content-type"))
	}))
	defer server.Close()

	customClient := &http.Client{
		Transport: &testTransport{server: server},
	}
	client, err := NewClientWithHTTPClient(customClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := client.fetchFile(context.Background(), "owner/repo", "v1.0.0", "file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if content != "content without content-type" {
		t.Errorf("expected content, got %q", content)
	}
}

func TestFetchFile_URLConstruction(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	tests := []struct {
		name         string
		repo         string
		tag          string
		path         string
		expectedPath string
	}{
		{
			name:         "simple path",
			repo:         "owner/repo",
			tag:          "v1.0.0",
			path:         "README.md",
			expectedPath: "/owner/repo/v1.0.0/README.md",
		},
		{
			name:         "nested path",
			repo:         "owner/repo",
			tag:          "v2.0.0",
			path:         "docs/install.md",
			expectedPath: "/owner/repo/v2.0.0/docs/install.md",
		},
		{
			name:         "deep nested path",
			repo:         "org/project",
			tag:          "release-1.0",
			path:         "docs/guides/getting-started.md",
			expectedPath: "/org/project/release-1.0/docs/guides/getting-started.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			}))
			defer server.Close()

			customClient := &http.Client{
				Transport: &testTransport{server: server},
			}
			client, err := NewClientWithHTTPClient(customClient)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			_, err = client.fetchFile(context.Background(), tt.repo, tt.tag, tt.path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if capturedPath != tt.expectedPath {
				t.Errorf("expected path %q, got %q", tt.expectedPath, capturedPath)
			}
		})
	}
}

func TestFetchFile_ContextCancellation(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	// Create a test server that blocks until context is canceled
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	customClient := &http.Client{
		Transport: &testTransport{server: server},
	}
	client, err := NewClientWithHTTPClient(customClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately
	cancel()

	_, err = client.fetchFile(ctx, "owner/repo", "v1.0.0", "file.md")
	if err == nil {
		t.Error("expected error for canceled context")
	}
}

func TestFetchFile_LargeFile(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	// Create content close to but under the 1MB limit
	largeContent := make([]byte, 500*1024) // 500KB
	for i := range largeContent {
		largeContent[i] = byte('a' + (i % 26))
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(largeContent)
	}))
	defer server.Close()

	customClient := &http.Client{
		Transport: &testTransport{server: server},
	}
	client, err := NewClientWithHTTPClient(customClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := client.fetchFile(context.Background(), "owner/repo", "v1.0.0", "large.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(content) != len(largeContent) {
		t.Errorf("expected content length %d, got %d", len(largeContent), len(content))
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	prompt := buildSystemPrompt()
	if prompt == "" {
		t.Error("expected non-empty system prompt")
	}

	// Check for key phrases that should be in the prompt
	keywords := []string{
		"GitHub releases",
		"tsuku",
		"fetch_file",
		"inspect_archive",
		"extract_pattern",
	}

	for _, keyword := range keywords {
		found := false
		for i := 0; i <= len(prompt)-len(keyword); i++ {
			if prompt[i:i+len(keyword)] == keyword {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected system prompt to contain %q", keyword)
		}
	}
}

func TestBuildUserMessage(t *testing.T) {
	req := &GenerateRequest{
		Repo:        "cli/cli",
		Description: "GitHub CLI",
		Releases: []Release{
			{
				Tag:    "v2.42.0",
				Assets: []string{"gh_2.42.0_linux_amd64.tar.gz", "gh_2.42.0_darwin_arm64.zip"},
			},
		},
		README: "# GitHub CLI\n\nThis is the GitHub CLI.",
	}

	msg := buildUserMessage(req)

	if msg == "" {
		t.Error("expected non-empty user message")
	}

	// Check for key content
	keywords := []string{
		"cli/cli",
		"GitHub CLI",
		"v2.42.0",
		"gh_2.42.0_linux_amd64.tar.gz",
		"README.md",
	}

	for _, keyword := range keywords {
		found := false
		for i := 0; i <= len(msg)-len(keyword); i++ {
			if msg[i:i+len(keyword)] == keyword {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected user message to contain %q", keyword)
		}
	}
}

func TestBuildUserMessage_LongREADME(t *testing.T) {
	// Create a README longer than 10000 characters
	longREADME := make([]byte, 15000)
	for i := range longREADME {
		longREADME[i] = 'a'
	}

	req := &GenerateRequest{
		Repo:        "test/repo",
		Description: "Test",
		Releases:    []Release{},
		README:      string(longREADME),
	}

	msg := buildUserMessage(req)

	// Message should be truncated
	if len(msg) > 12000 { // Allow some overhead for the other parts
		t.Errorf("expected truncated message, got length %d", len(msg))
	}

	// Should contain truncation indicator
	found := false
	indicator := "...(truncated)"
	for i := 0; i <= len(msg)-len(indicator); i++ {
		if msg[i:i+len(indicator)] == indicator {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected truncation indicator in long README")
	}
}

// TestGenerateRecipe_Integration is skipped unless ANTHROPIC_API_KEY is set.
// This test makes real API calls and incurs costs.
func TestGenerateRecipe_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping integration test")
	}

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	req := &GenerateRequest{
		Repo:        "FiloSottile/age",
		Description: "A simple, modern and secure encryption tool",
		Releases: []Release{
			{
				Tag: "v1.2.1",
				Assets: []string{
					"age-v1.2.1-darwin-amd64.tar.gz",
					"age-v1.2.1-darwin-arm64.tar.gz",
					"age-v1.2.1-linux-amd64.tar.gz",
					"age-v1.2.1-linux-arm64.tar.gz",
					"age-v1.2.1-windows-amd64.zip",
				},
			},
		},
		README: "# age\n\nA simple, modern and secure encryption tool.\n\n## Installation\n\nDownload the binary from the releases page.",
	}

	pattern, usage, err := client.GenerateRecipe(ctx, req)
	if err != nil {
		t.Fatalf("GenerateRecipe error: %v", err)
	}

	if pattern == nil {
		t.Fatal("expected non-nil pattern")
	}

	if len(pattern.Mappings) == 0 {
		t.Error("expected at least one platform mapping")
	}

	if pattern.Executable == "" {
		t.Error("expected non-empty executable name")
	}

	if pattern.VerifyCommand == "" {
		t.Error("expected non-empty verify command")
	}

	if usage == nil {
		t.Fatal("expected non-nil usage")
	}

	if usage.InputTokens == 0 {
		t.Error("expected non-zero input tokens")
	}

	if usage.OutputTokens == 0 {
		t.Error("expected non-zero output tokens")
	}

	t.Logf("Pattern: executable=%s, verify=%s, mappings=%d",
		pattern.Executable, pattern.VerifyCommand, len(pattern.Mappings))
	t.Logf("Usage: %s", usage.String())
}
