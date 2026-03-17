package distributed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestServer creates a TLS test server and returns a client configured to trust it.
func newTestServer(t *testing.T, handler http.Handler) (*httptest.Server, *http.Client) {
	t.Helper()
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)
	client := ts.Client()
	return ts, client
}

// scenario-13: Auth token isolation - token only sent to api.github.com
func TestAuthTransport_TokenIsolation(t *testing.T) {
	var gotAuth string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})

	ts, baseClient := newTestServer(t, handler)

	transport := &authTransport{
		token: "ghp_test_token",
		base:  baseClient.Transport,
	}
	authedClient := &http.Client{Transport: transport}

	t.Run("token sent to api.github.com", func(t *testing.T) {
		// Use a capturing base transport to verify the Authorization header
		// is added by authTransport.RoundTrip for api.github.com requests.
		var capturedReq *http.Request
		capturingTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			capturedReq = req
			return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
		})
		at := &authTransport{token: "ghp_test_token", base: capturingTransport}

		req, _ := http.NewRequest("GET", "https://api.github.com/repos/test/test", nil)
		_, err := at.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip failed: %v", err)
		}
		if capturedReq == nil {
			t.Fatal("base transport was not called")
		}
		got := capturedReq.Header.Get("Authorization")
		if got != "Bearer ghp_test_token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer ghp_test_token")
		}
	})

	t.Run("transport does not add token for other hosts", func(t *testing.T) {
		gotAuth = ""
		req, _ := http.NewRequest("GET", ts.URL+"/test", nil)
		// Test server hostname is 127.0.0.1:PORT, not api.github.com
		resp, err := authedClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
		if gotAuth != "" {
			t.Errorf("token should not be sent to non-api.github.com hosts, got: %s", gotAuth)
		}
	})
}

// scenario-14: Contents API response parsing and filtering
func TestContentsAPIResponseParsing(t *testing.T) {
	entries := []contentsEntry{
		{Name: "foo.toml", Type: "file", DownloadURL: "https://raw.githubusercontent.com/owner/repo/main/.tsuku-recipes/foo.toml"},
		{Name: "bar.toml", Type: "file", DownloadURL: "https://raw.githubusercontent.com/owner/repo/main/.tsuku-recipes/bar.toml"},
		{Name: "README.md", Type: "file", DownloadURL: "https://raw.githubusercontent.com/owner/repo/main/.tsuku-recipes/README.md"},
		{Name: "subdir", Type: "dir", DownloadURL: ""},
	}

	// Simulate what listViaContentsAPI does: filter to .toml files with valid download URLs
	files := make(map[string]string)
	for _, entry := range entries {
		if entry.Type != "file" || !strings.HasSuffix(entry.Name, ".toml") {
			continue
		}
		if err := validateDownloadURL(entry.DownloadURL); err != nil {
			continue
		}
		name := strings.TrimSuffix(entry.Name, ".toml")
		files[name] = entry.DownloadURL
	}

	if len(files) != 2 {
		t.Errorf("expected 2 TOML files, got %d: %v", len(files), files)
	}
	if _, ok := files["foo"]; !ok {
		t.Error("expected foo in files")
	}
	if _, ok := files["bar"]; !ok {
		t.Error("expected bar in files")
	}

	// Verify non-TOML and directory entries are filtered out
	if _, ok := files["README"]; ok {
		t.Error("non-TOML file should be filtered")
	}
}

func TestGitHubClient_ListRecipes_CacheHit(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)
	meta := &SourceMeta{
		Branch: "main",
		Files: map[string]string{
			"cached-tool": "https://raw.githubusercontent.com/owner/repo/main/.tsuku-recipes/cached-tool.toml",
		},
		FetchedAt: time.Now(), // Fresh
	}
	if err := cache.PutSourceMeta("owner", "repo", meta); err != nil {
		t.Fatalf("PutSourceMeta: %v", err)
	}

	// API client that panics if called -- verifies cache is used
	panicClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatal("API should not be called when cache is fresh")
			return nil, nil
		}),
	}

	gc := newGitHubClientWithHTTP(panicClient, panicClient, cache, false)
	got, err := gc.ListRecipes(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatalf("ListRecipes: %v", err)
	}
	if got.Branch != "main" {
		t.Errorf("branch = %q, want %q", got.Branch, "main")
	}
	if _, ok := got.Files["cached-tool"]; !ok {
		t.Error("expected cached-tool in files")
	}
}

// scenario-15: Download URL hostname allowlist validation
func TestValidateDownloadURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid raw.githubusercontent.com", "https://raw.githubusercontent.com/owner/repo/main/file.toml", false},
		{"valid objects.githubusercontent.com", "https://objects.githubusercontent.com/some/path", false},
		{"empty URL", "", true},
		{"HTTP not HTTPS", "http://raw.githubusercontent.com/owner/repo/main/file.toml", true},
		{"disallowed host", "https://evil.com/owner/repo/main/file.toml", true},
		{"github.com not allowed", "https://github.com/owner/repo/raw/main/file.toml", true},
		{"api.github.com not allowed for download", "https://api.github.com/repos/owner/repo", true},
		{"FTP scheme", "ftp://raw.githubusercontent.com/file.toml", true},
		{"malformed", "://bad", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDownloadURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDownloadURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

// scenario-16: Cache read/write/expiry lifecycle and FetchRecipe URL validation
func TestCacheLifecycleAndFetchRecipeValidation(t *testing.T) {
	recipeContent := []byte(`[tool]\nname = "test-tool"`)
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)

	t.Run("cache round-trip", func(t *testing.T) {
		meta := &RecipeMeta{
			ETag:      `"etag-abc"`,
			FetchedAt: time.Now(),
		}
		if err := cache.PutRecipe("owner", "repo", "test-tool", recipeContent, meta); err != nil {
			t.Fatalf("PutRecipe: %v", err)
		}

		got, err := cache.GetRecipe("owner", "repo", "test-tool")
		if err != nil {
			t.Fatalf("GetRecipe: %v", err)
		}
		if string(got) != string(recipeContent) {
			t.Error("cached recipe content mismatch")
		}
	})

	t.Run("fresh cache returns immediately", func(t *testing.T) {
		freshMeta := &RecipeMeta{FetchedAt: time.Now()}
		if !cache.IsRecipeFresh(freshMeta) {
			t.Error("recent recipe meta should be fresh")
		}
	})

	t.Run("stale cache is not fresh", func(t *testing.T) {
		staleMeta := &RecipeMeta{FetchedAt: time.Now().Add(-2 * time.Hour)}
		if cache.IsRecipeFresh(staleMeta) {
			t.Error("old recipe meta should be stale")
		}
	})

	t.Run("FetchRecipe rejects invalid URLs", func(t *testing.T) {
		gc := newGitHubClientWithHTTP(&http.Client{}, &http.Client{}, cache, false)
		_, err := gc.FetchRecipe(context.Background(), "owner", "repo", "test-tool", "http://evil.com/recipe.toml")
		if err == nil {
			t.Error("expected error for invalid download URL")
		}
	})
}

// scenario-17: Rate limit error handling with remaining/reset headers
func TestGitHubClient_RateLimitHandling(t *testing.T) {
	resetTime := time.Now().Add(1 * time.Hour).Unix()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strings.TrimRight(strings.TrimRight(
			time.Unix(resetTime, 0).Format("2006010215040500"), "0"), ""))
		// Use the actual unix timestamp
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime))
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	})

	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)

	resp, err := ts.Client().Get(ts.URL + "/repos/owner/repo/contents/.tsuku-recipes")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	t.Run("parses rate limit headers", func(t *testing.T) {
		rlErr := parseRateLimitError(resp, false)
		if rlErr.Remaining != 0 {
			t.Errorf("remaining = %d, want 0", rlErr.Remaining)
		}
		if rlErr.ResetAt.Unix() != resetTime {
			t.Errorf("resetAt = %v, want %v", rlErr.ResetAt.Unix(), resetTime)
		}
		if rlErr.HasToken {
			t.Error("hasToken should be false")
		}
	})

	t.Run("error message includes guidance without token", func(t *testing.T) {
		rlErr := parseRateLimitError(resp, false)
		msg := rlErr.Error()
		if !strings.Contains(msg, "GITHUB_TOKEN") {
			t.Errorf("error message should guide user to set GITHUB_TOKEN: %s", msg)
		}
	})

	t.Run("error message omits guidance with token", func(t *testing.T) {
		rlErr := parseRateLimitError(resp, true)
		msg := rlErr.Error()
		if strings.Contains(msg, "GITHUB_TOKEN") {
			t.Errorf("error message should not mention GITHUB_TOKEN when already set: %s", msg)
		}
	})
}

func TestExtractBranchFromURL(t *testing.T) {
	entries := []contentsEntry{
		{
			Name:        "foo.toml",
			DownloadURL: "https://raw.githubusercontent.com/owner/repo/develop/.tsuku-recipes/foo.toml",
		},
	}

	branch := extractBranchFromURL(entries)
	if branch != "develop" {
		t.Errorf("branch = %q, want %q", branch, "develop")
	}
}

func TestExtractBranchFromURL_Empty(t *testing.T) {
	branch := extractBranchFromURL(nil)
	if branch != "" {
		t.Errorf("branch = %q, want empty", branch)
	}

	branch = extractBranchFromURL([]contentsEntry{{Name: "foo.toml", DownloadURL: ""}})
	if branch != "" {
		t.Errorf("branch = %q, want empty", branch)
	}
}

func TestGitHubClient_ListRecipes_ValidationRejectsInvalid(t *testing.T) {
	cache := NewCacheManager(t.TempDir(), 1*time.Hour)
	gc := newGitHubClientWithHTTP(&http.Client{}, &http.Client{}, cache, false)

	tests := []struct {
		name  string
		owner string
		repo  string
	}{
		{"path traversal", "../etc", "repo"},
		{"empty owner", "", "repo"},
		{"credentials in URL format", "user:pass@owner", "repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gc.ListRecipes(context.Background(), tt.owner, tt.repo)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestErrorMessages(t *testing.T) {
	t.Run("ErrRepoNotFound", func(t *testing.T) {
		e := &ErrRepoNotFound{Owner: "owner", Repo: "repo"}
		if !strings.Contains(e.Error(), "owner/repo") {
			t.Errorf("error should contain owner/repo: %s", e.Error())
		}
	})

	t.Run("ErrNoRecipeDir", func(t *testing.T) {
		e := &ErrNoRecipeDir{Owner: "owner", Repo: "repo"}
		if !strings.Contains(e.Error(), ".tsuku-recipes") {
			t.Errorf("error should mention .tsuku-recipes: %s", e.Error())
		}
	})

	t.Run("ErrInvalidDownloadURL", func(t *testing.T) {
		e := &ErrInvalidDownloadURL{URL: "http://evil.com", Reason: "must use HTTPS"}
		if !strings.Contains(e.Error(), "HTTPS") {
			t.Errorf("error should mention HTTPS: %s", e.Error())
		}
	})

	t.Run("ErrNetwork", func(t *testing.T) {
		inner := fmt.Errorf("connection refused")
		e := &ErrNetwork{Operation: "fetching", Err: inner}
		if !strings.Contains(e.Error(), "connection refused") {
			t.Errorf("error should contain inner error: %s", e.Error())
		}
		if e.Unwrap() != inner {
			t.Error("Unwrap should return inner error")
		}
	})
}

// roundTripFunc wraps a function to satisfy http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
