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

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello, World!"))
	}))
	defer server.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := client.fetchFile(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetchFile error: %v", err)
	}

	if content != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", content)
	}
}

func TestFetchFile_HTTPError(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	// Create a test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = client.fetchFile(context.Background(), server.URL)
	if err == nil {
		t.Error("expected error for HTTP 404")
	}
}

func TestInspectArchive_Stub(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	client, err := NewClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// inspectArchive is a stub for Slice 1
	result, err := client.inspectArchive(context.Background(), "https://example.com/archive.tar.gz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty result from stub")
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
