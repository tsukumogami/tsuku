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

func TestTavilyProvider_Name(t *testing.T) {
	p := NewTavilyProvider("test-key")
	if p.Name() != "tavily" {
		t.Errorf("expected name 'tavily', got %q", p.Name())
	}
}

func TestTavilyProvider_Search_Success(t *testing.T) {
	content, err := os.ReadFile("testdata/tavily_success.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and content type
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	// Create provider with test server
	p := NewTavilyProviderWithOptions(TavilyOptions{
		APIKey:     "test-key",
		HTTPClient: newTestClient(server.URL),
		Logger:     log.NewNoop(),
	})

	ctx := context.Background()
	resp, err := p.Search(ctx, "stripe cli github")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Results) != 3 {
		t.Errorf("expected 3 results, got %d", len(resp.Results))
	}

	if len(resp.Results) > 0 {
		if resp.Results[0].Title != "Stripe CLI - GitHub" {
			t.Errorf("expected title 'Stripe CLI - GitHub', got %q", resp.Results[0].Title)
		}
		if resp.Results[0].URL != "https://github.com/stripe/stripe-cli" {
			t.Errorf("unexpected URL: %s", resp.Results[0].URL)
		}
	}
}

func TestTavilyProvider_Search_Empty(t *testing.T) {
	content, err := os.ReadFile("testdata/tavily_empty.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	p := NewTavilyProviderWithOptions(TavilyOptions{
		APIKey:     "test-key",
		HTTPClient: newTestClient(server.URL),
		Logger:     log.NewNoop(),
	})

	ctx := context.Background()
	resp, err := p.Search(ctx, "nonexistent-tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Results))
	}
}

func TestTavilyProvider_Search_RetryOnError(t *testing.T) {
	content, err := os.ReadFile("testdata/tavily_success.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count <= 2 {
			// First two requests return 500
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal Server Error"))
			return
		}
		// Third request succeeds
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	p := NewTavilyProviderWithOptions(TavilyOptions{
		APIKey:     "test-key",
		HTTPClient: newTestClient(server.URL),
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

func TestTavilyProvider_Search_MaxRetriesExceeded(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	p := NewTavilyProviderWithOptions(TavilyOptions{
		APIKey:     "test-key",
		HTTPClient: newTestClient(server.URL),
		MaxRetries: 2,
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

func TestTavilyProvider_Search_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Error"))
	}))
	defer server.Close()

	p := NewTavilyProviderWithOptions(TavilyOptions{
		APIKey:     "test-key",
		HTTPClient: newTestClient(server.URL),
		MaxRetries: 10,
		Logger:     log.NewNoop(),
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := p.Search(ctx, "test query")
		done <- err
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled error, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("search did not respond to context cancellation within timeout")
	}
}

func TestTavilyProvider_Search_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "Invalid API key"}`))
	}))
	defer server.Close()

	p := NewTavilyProviderWithOptions(TavilyOptions{
		APIKey:     "bad-key",
		HTTPClient: newTestClient(server.URL),
		MaxRetries: 0, // No retries for this test
		Logger:     log.NewNoop(),
	})

	ctx := context.Background()
	_, err := p.Search(ctx, "test query")
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}

	if !strings.Contains(err.Error(), "status 401") {
		t.Errorf("expected 'status 401' error, got: %v", err)
	}
}

func TestTavilyProviderWithOptions_Defaults(t *testing.T) {
	p := NewTavilyProviderWithOptions(TavilyOptions{APIKey: "test"})

	if p.maxRetries != DefaultMaxRetries {
		t.Errorf("expected default maxRetries %d, got %d", DefaultMaxRetries, p.maxRetries)
	}

	if p.client == nil {
		t.Error("expected default HTTP client to be set")
	}

	if p.logger == nil {
		t.Error("expected default logger to be set")
	}

	if p.apiKey != "test" {
		t.Errorf("expected apiKey 'test', got %q", p.apiKey)
	}
}

// newTestClient creates an HTTP client that redirects all requests to the test server.
func newTestClient(serverURL string) *http.Client {
	return &http.Client{
		Transport: &tavilyTestTransport{targetURL: serverURL},
	}
}

type tavilyTestTransport struct {
	targetURL string
}

func (t *tavilyTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.targetURL, "http://")
	return http.DefaultTransport.RoundTrip(req)
}
