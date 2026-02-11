package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/log"
)

func TestDDGProvider_Name(t *testing.T) {
	p := NewDDGProvider()
	if p.Name() != "ddg" {
		t.Errorf("expected name 'ddg', got %q", p.Name())
	}
}

func TestParseHTMLResults(t *testing.T) {
	html := `
<div class="result">
  <a class="result__a" href="https://github.com/stripe/stripe-cli">Stripe CLI</a>
  <a class="result__snippet">A command line tool for Stripe</a>
</div>
<div class="result">
  <a class="result__a" href="https://stripe.com/docs/cli">Stripe CLI Documentation</a>
  <a class="result__snippet">Official Stripe CLI documentation</a>
</div>
`
	results := parseHTMLResults(html)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	if len(results) > 0 {
		if results[0].Title != "Stripe CLI" {
			t.Errorf("expected title 'Stripe CLI', got %q", results[0].Title)
		}
		if results[0].URL != "https://github.com/stripe/stripe-cli" {
			t.Errorf("unexpected URL: %s", results[0].URL)
		}
	}
}

func TestParseHTMLResults_FromFixture(t *testing.T) {
	content, err := os.ReadFile("testdata/ddg_success.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	results := parseHTMLResults(string(content))
	if len(results) != 3 {
		t.Errorf("expected 3 results from fixture, got %d", len(results))
	}

	if len(results) > 0 {
		if results[0].Title != "Stripe CLI - GitHub" {
			t.Errorf("expected title 'Stripe CLI - GitHub', got %q", results[0].Title)
		}
		if results[0].URL != "https://github.com/stripe/stripe-cli" {
			t.Errorf("expected URL 'https://github.com/stripe/stripe-cli', got %q", results[0].URL)
		}
	}

	// Third result should have decoded redirect URL
	if len(results) >= 3 {
		expectedURL := "https://github.com/stripe/stripe-cli/releases"
		if results[2].URL != expectedURL {
			t.Errorf("expected decoded URL %q, got %q", expectedURL, results[2].URL)
		}
	}
}

func TestParseHTMLResults_EmptyFixture(t *testing.T) {
	content, err := os.ReadFile("testdata/ddg_empty.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	results := parseHTMLResults(string(content))
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty fixture, got %d", len(results))
	}
}

func TestDecodeRedirectURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "//duckduckgo.com/l/?uddg=https%3A%2F%2Fgithub.com%2Fstripe%2Fstripe-cli",
			expected: "https://github.com/stripe/stripe-cli",
		},
		{
			input:    "https://github.com/direct/link",
			expected: "https://github.com/direct/link",
		},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result, err := decodeRedirectURL(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.expected {
				t.Errorf("got %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestDDGProvider_Search_Success(t *testing.T) {
	content, err := os.ReadFile("testdata/ddg_success.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	// Create a custom transport that redirects to our test server
	transport := &testTransport{targetURL: server.URL}
	client := &http.Client{Transport: transport}

	p := NewDDGProviderWithOptions(DDGOptions{
		HTTPClient: client,
		Logger:     log.NewNoop(),
	})

	ctx := context.Background()
	resp, err := p.Search(ctx, "test query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Results) != 3 {
		t.Errorf("expected 3 results, got %d", len(resp.Results))
	}
}

func TestDDGProvider_Search_RetryOn202(t *testing.T) {
	content, err := os.ReadFile("testdata/ddg_success.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count <= 2 {
			// First two requests return 202 (rate limited)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("Rate limited"))
			return
		}
		// Third request succeeds
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	transport := &testTransport{targetURL: server.URL}
	client := &http.Client{Transport: transport}

	// Use a short test timeout - retries should be fast in tests
	// since we're using mock server
	p := NewDDGProviderWithOptions(DDGOptions{
		HTTPClient: client,
		MaxRetries: 3,
		Logger:     log.NewNoop(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Search(ctx, "test query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", atomic.LoadInt32(&attempts))
	}

	if len(resp.Results) != 3 {
		t.Errorf("expected 3 results, got %d", len(resp.Results))
	}
}

func TestDDGProvider_Search_MaxRetriesExceeded(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		// Always return 202 (rate limited)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("Rate limited"))
	}))
	defer server.Close()

	transport := &testTransport{targetURL: server.URL}
	client := &http.Client{Transport: transport}

	p := NewDDGProviderWithOptions(DDGOptions{
		HTTPClient: client,
		MaxRetries: 2, // Only 2 retries (3 total attempts)
		Logger:     log.NewNoop(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := p.Search(ctx, "test query")
	if err == nil {
		t.Fatal("expected error after max retries exceeded")
	}

	if !strings.Contains(err.Error(), "failed after 2 retries") {
		t.Errorf("expected 'failed after 2 retries' error, got: %v", err)
	}

	// Should have made 3 attempts (initial + 2 retries)
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestDDGProvider_Search_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return 202 to trigger retry loop
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("Rate limited"))
	}))
	defer server.Close()

	transport := &testTransport{targetURL: server.URL}
	client := &http.Client{Transport: transport}

	p := NewDDGProviderWithOptions(DDGOptions{
		HTTPClient: client,
		MaxRetries: 10, // High retry count to ensure we test cancellation
		Logger:     log.NewNoop(),
	})

	// Create a context that will be canceled quickly
	ctx, cancel := context.WithCancel(context.Background())

	// Start search in goroutine
	done := make(chan error, 1)
	go func() {
		_, err := p.Search(ctx, "test query")
		done <- err
	}()

	// Give the search a moment to start, then cancel
	time.Sleep(10 * time.Millisecond)
	cancel()

	// Wait for completion with timeout
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled error, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("search did not respond to context cancellation within timeout")
	}
}

func TestDDGProvider_Search_NonRetryableStatus(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		// Return 404 - not retryable
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not found"))
	}))
	defer server.Close()

	transport := &testTransport{targetURL: server.URL}
	client := &http.Client{Transport: transport}

	p := NewDDGProviderWithOptions(DDGOptions{
		HTTPClient: client,
		MaxRetries: 3,
		Logger:     log.NewNoop(),
	})

	ctx := context.Background()
	_, err := p.Search(ctx, "test query")
	if err == nil {
		t.Fatal("expected error for 404 status")
	}

	if !strings.Contains(err.Error(), "status 404") {
		t.Errorf("expected 'status 404' error, got: %v", err)
	}

	// Should only make 1 attempt for non-retryable status
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("expected 1 attempt for non-retryable status, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestDDGProviderWithOptions_Defaults(t *testing.T) {
	p := NewDDGProviderWithOptions(DDGOptions{})

	if p.maxRetries != DefaultMaxRetries {
		t.Errorf("expected default maxRetries %d, got %d", DefaultMaxRetries, p.maxRetries)
	}

	if p.client == nil {
		t.Error("expected default HTTP client to be set")
	}

	if p.logger == nil {
		t.Error("expected default logger to be set")
	}
}

func TestDDGProviderWithOptions_Custom(t *testing.T) {
	customClient := &http.Client{Timeout: 5 * time.Second}
	customLogger := log.NewNoop()

	p := NewDDGProviderWithOptions(DDGOptions{
		HTTPClient: customClient,
		MaxRetries: 5,
		Logger:     customLogger,
	})

	if p.maxRetries != 5 {
		t.Errorf("expected maxRetries 5, got %d", p.maxRetries)
	}

	if p.client != customClient {
		t.Error("expected custom HTTP client")
	}
}

func TestDDGProvider_Integration(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") == "" {
		t.Skip("Set INTEGRATION_TESTS=1 to run integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p := NewDDGProvider()
	resp, err := p.Search(ctx, "stripe cli github")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(resp.Results) == 0 {
		t.Error("expected results, got none")
	}

	t.Logf("Got %d results for 'stripe cli github'", len(resp.Results))
	for i, r := range resp.Results {
		if i >= 3 {
			break
		}
		t.Logf("  %d. %s - %s", i+1, r.Title, r.URL)
	}
}

func TestResponseFormatForLLM(t *testing.T) {
	resp := &Response{
		Query: "test query",
		Results: []Result{
			{Title: "Result 1", URL: "https://example.com/1", Snippet: "First result"},
			{Title: "Result 2", URL: "https://example.com/2", Snippet: "Second result"},
			{Title: "Result 3", URL: "https://example.com/3", Snippet: "Third result"},
		},
	}

	// Test with limit
	formatted := resp.FormatForLLM(2)
	if formatted == "" {
		t.Error("expected non-empty formatted output")
	}

	// Should contain first 2 results but not 3rd
	if !containsSubstr(formatted, "Result 1") || !containsSubstr(formatted, "Result 2") {
		t.Error("expected first two results in output")
	}

	// Test empty results
	emptyResp := &Response{Query: "empty query", Results: nil}
	emptyFormatted := emptyResp.FormatForLLM(10)
	if !containsSubstr(emptyFormatted, "No results found") {
		t.Errorf("expected 'No results found' message, got: %s", emptyFormatted)
	}
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// testTransport redirects all requests to the test server.
type testTransport struct {
	targetURL string
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect the request to our test server
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.targetURL, "http://")
	return http.DefaultTransport.RoundTrip(req)
}
