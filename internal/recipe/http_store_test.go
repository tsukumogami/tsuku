package recipe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPStore_CacheMiss(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
		_, _ = w.Write([]byte("recipe-content"))
	}))
	defer server.Close()

	store := NewHTTPStore(HTTPStoreConfig{
		BaseURL:  server.URL,
		CacheDir: t.TempDir(),
		TTL:      1 * time.Hour,
		MaxSize:  10 * 1024 * 1024,
	})

	data, err := store.Get(context.Background(), "test.toml")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(data) != "recipe-content" {
		t.Errorf("expected 'recipe-content', got %q", string(data))
	}

	// Verify it was cached with HTTP metadata
	meta, err := store.Cache().ReadMeta("test.toml")
	if err != nil {
		t.Fatalf("ReadMeta failed: %v", err)
	}
	if meta.ETag != `"abc123"` {
		t.Errorf("expected ETag '\"abc123\"', got %q", meta.ETag)
	}
	if meta.LastModified != "Wed, 21 Oct 2015 07:28:00 GMT" {
		t.Errorf("unexpected LastModified: %q", meta.LastModified)
	}
}

func TestHTTPStore_CacheHit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		_, _ = w.Write([]byte("recipe-content"))
	}))
	defer server.Close()

	store := NewHTTPStore(HTTPStoreConfig{
		BaseURL:  server.URL,
		CacheDir: t.TempDir(),
		TTL:      1 * time.Hour,
		MaxSize:  10 * 1024 * 1024,
	})

	ctx := context.Background()

	// First call: cache miss, hits server
	_, err := store.Get(ctx, "test.toml")
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 server call, got %d", callCount)
	}

	// Second call: cache hit, no server call
	data, err := store.Get(ctx, "test.toml")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "recipe-content" {
		t.Errorf("expected 'recipe-content', got %q", string(data))
	}
	if callCount != 1 {
		t.Errorf("expected 1 server call (cached), got %d", callCount)
	}
}

func TestHTTPStore_ConditionalRequest304(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Header.Get("If-None-Match") == `"etag-value"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"etag-value"`)
		_, _ = w.Write([]byte("original-content"))
	}))
	defer server.Close()

	store := NewHTTPStore(HTTPStoreConfig{
		BaseURL:  server.URL,
		CacheDir: t.TempDir(),
		TTL:      50 * time.Millisecond,
		MaxSize:  10 * 1024 * 1024,
	})

	ctx := context.Background()

	// First call: populates cache
	data, err := store.Get(ctx, "test.toml")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original-content" {
		t.Fatalf("expected 'original-content', got %q", string(data))
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Second call: conditional request returns 304
	data, err = store.Get(ctx, "test.toml")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original-content" {
		t.Errorf("expected 'original-content' from cache after 304, got %q", string(data))
	}
	if callCount != 2 {
		t.Errorf("expected 2 server calls, got %d", callCount)
	}
}

func TestHTTPStore_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	store := NewHTTPStore(HTTPStoreConfig{
		BaseURL:  server.URL,
		CacheDir: t.TempDir(),
		TTL:      1 * time.Hour,
		MaxSize:  10 * 1024 * 1024,
	})

	_, err := store.Get(context.Background(), "test.toml")
	if err == nil {
		t.Fatal("expected error for rate limit")
	}

	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != 429 {
		t.Errorf("expected status 429, got %d", httpErr.StatusCode)
	}
	if httpErr.RetryAfter != 60*time.Second {
		t.Errorf("expected RetryAfter 60s, got %s", httpErr.RetryAfter)
	}
	if httpErr.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHTTPStore_StaleIfError(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			_, _ = w.Write([]byte("cached-content"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	store := NewHTTPStore(HTTPStoreConfig{
		BaseURL:  server.URL,
		CacheDir: t.TempDir(),
		TTL:      50 * time.Millisecond,
		MaxSize:  10 * 1024 * 1024,
		MaxStale: 1 * time.Hour,
	})

	ctx := context.Background()

	// First call: populates cache
	_, err := store.Get(ctx, "test.toml")
	if err != nil {
		t.Fatal(err)
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Second call: server returns 500 but stale data is usable
	data, err := store.Get(ctx, "test.toml")
	if err != nil {
		t.Fatalf("expected stale fallback, got error: %v", err)
	}
	if string(data) != "cached-content" {
		t.Errorf("expected stale 'cached-content', got %q", string(data))
	}
}

func TestHTTPStore_StaleIfError_TooOld(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			_, _ = w.Write([]byte("cached-content"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	store := NewHTTPStore(HTTPStoreConfig{
		BaseURL:  server.URL,
		CacheDir: t.TempDir(),
		TTL:      10 * time.Millisecond,
		MaxSize:  10 * 1024 * 1024,
		MaxStale: 20 * time.Millisecond, // very short
	})

	ctx := context.Background()

	// First call: populates cache
	_, err := store.Get(ctx, "test.toml")
	if err != nil {
		t.Fatal(err)
	}

	// Wait for TTL + MaxStale to expire
	time.Sleep(50 * time.Millisecond)

	// Second call: server returns 500 and stale data is too old
	_, err = store.Get(ctx, "test.toml")
	if err == nil {
		t.Fatal("expected error when stale data is too old")
	}
}

func TestHTTPStore_List(t *testing.T) {
	store := NewHTTPStore(HTTPStoreConfig{
		BaseURL:  "http://unused",
		CacheDir: t.TempDir(),
		TTL:      1 * time.Hour,
		MaxSize:  10 * 1024 * 1024,
	})

	// Populate cache directly
	_ = store.Cache().Put("a.toml", []byte("a"), nil)
	_ = store.Cache().Put("b.toml", []byte("b"), nil)
	_ = store.Cache().Put("c.txt", []byte("c"), nil)    // not .toml
	_ = store.Cache().Put("d/e.toml", []byte("e"), nil) // subdirectory

	paths, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	tomlCount := 0
	for _, p := range paths {
		if p == "c.txt" {
			t.Error("List should not include non-.toml files")
		}
		tomlCount++
	}
	if tomlCount != 3 {
		t.Errorf("expected 3 .toml paths, got %d: %v", tomlCount, paths)
	}
}

func TestHTTPStore_CacheStats(t *testing.T) {
	store := NewHTTPStore(HTTPStoreConfig{
		BaseURL:  "http://unused",
		CacheDir: t.TempDir(),
		TTL:      1 * time.Hour,
		MaxSize:  50 * 1024 * 1024,
	})

	_ = store.Cache().Put("a.toml", []byte("aaaa"), nil)

	stats, err := store.CacheStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.EntryCount != 1 {
		t.Errorf("expected 1 entry, got %d", stats.EntryCount)
	}
	if stats.SizeLimit != 50*1024*1024 {
		t.Errorf("expected 50MB limit, got %d", stats.SizeLimit)
	}
}

func TestHTTPStore_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	store := NewHTTPStore(HTTPStoreConfig{
		BaseURL:  server.URL,
		CacheDir: t.TempDir(),
		TTL:      1 * time.Hour,
		MaxSize:  10 * 1024 * 1024,
	})

	_, err := store.Get(context.Background(), "missing.toml")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", httpErr.StatusCode)
	}
}

func TestHTTPError_Messages(t *testing.T) {
	tests := []struct {
		name     string
		err      HTTPError
		contains string
	}{
		{
			name:     "rate limit with retry",
			err:      HTTPError{StatusCode: 429, URL: "https://example.com", RetryAfter: 60 * time.Second},
			contains: "rate limited",
		},
		{
			name:     "rate limit without retry",
			err:      HTTPError{StatusCode: 429, URL: "https://example.com"},
			contains: "rate limited",
		},
		{
			name:     "server error",
			err:      HTTPError{StatusCode: 500, Status: "Internal Server Error", URL: "https://example.com"},
			contains: "HTTP 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if msg == "" {
				t.Error("expected non-empty error message")
			}
			if tt.contains != "" {
				found := false
				for _, part := range []string{tt.contains} {
					if len(msg) >= len(part) {
						for i := range msg {
							if i+len(part) <= len(msg) && msg[i:i+len(part)] == part {
								found = true
								break
							}
						}
					}
				}
				if !found {
					t.Errorf("expected message to contain %q, got %q", tt.contains, msg)
				}
			}
		})
	}
}
