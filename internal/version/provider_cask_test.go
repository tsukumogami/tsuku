package version

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// Mock cask responses for testing
var mockCaskResponses = map[string]homebrewCaskInfo{
	"iterm2": {
		Token:   "iterm2",
		Version: "3.6.6",
		SHA256:  "68293d89ddf2140407879a651b42d9d05e12f403f69633ec635a96a29d90b4f3",
		URL:     "https://iterm2.com/downloads/stable/iTerm2-3_6_6.zip",
	},
	"visual-studio-code": {
		Token:   "visual-studio-code",
		Version: "1.108.0",
		SHA256:  "316a301f2f7997c200f97700160d85649a1a97c8a0d9a6695a3cc54479c76c89",
		URL:     "https://update.code.visualstudio.com/1.108.0/darwin-arm64/stable",
		Variations: map[string]caskVariation{
			"arm64_sonoma": {
				URL:    "https://update.code.visualstudio.com/1.108.0/darwin-arm64/stable",
				SHA256: "316a301f2f7997c200f97700160d85649a1a97c8a0d9a6695a3cc54479c76c89",
			},
			"sonoma": {
				URL:    "https://update.code.visualstudio.com/1.108.0/darwin/stable",
				SHA256: "8866bde2169b77cad52cfd174372d982e4c0988934a1d8df303ecb954fb4b816",
			},
		},
	},
	"no-checksum-cask": {
		Token:   "no-checksum-cask",
		Version: "1.0.0",
		SHA256:  ":no_check",
		URL:     "https://example.com/app.zip",
	},
	"empty-checksum-cask": {
		Token:   "empty-checksum-cask",
		Version: "1.0.0",
		SHA256:  "",
		URL:     "https://example.com/app.zip",
	},
}

// createMockCaskServer creates a test server that responds to cask API requests
func createMockCaskServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract cask name from path: /api/cask/{name}.json
		path := r.URL.Path
		if !strings.HasPrefix(path, "/api/cask/") || !strings.HasSuffix(path, ".json") {
			t.Errorf("unexpected path: %s", path)
			http.NotFound(w, r)
			return
		}

		caskName := strings.TrimSuffix(strings.TrimPrefix(path, "/api/cask/"), ".json")

		caskInfo, ok := mockCaskResponses[caskName]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(caskInfo); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
}

func TestResolveCask_Success(t *testing.T) {
	server := createMockCaskServer(t)
	defer server.Close()

	resolver := New(WithCaskRegistry(server.URL))

	info, err := resolver.ResolveCask(context.Background(), "iterm2")
	if err != nil {
		t.Fatalf("ResolveCask() error = %v", err)
	}

	if info.Version != "3.6.6" {
		t.Errorf("Version = %q, want %q", info.Version, "3.6.6")
	}

	if info.Metadata["url"] != "https://iterm2.com/downloads/stable/iTerm2-3_6_6.zip" {
		t.Errorf("URL = %q, want %q", info.Metadata["url"], "https://iterm2.com/downloads/stable/iTerm2-3_6_6.zip")
	}

	if info.Metadata["checksum"] != "sha256:68293d89ddf2140407879a651b42d9d05e12f403f69633ec635a96a29d90b4f3" {
		t.Errorf("Checksum = %q, want sha256 prefix", info.Metadata["checksum"])
	}
}

func TestResolveCask_NotFound(t *testing.T) {
	server := createMockCaskServer(t)
	defer server.Close()

	resolver := New(WithCaskRegistry(server.URL))

	_, err := resolver.ResolveCask(context.Background(), "nonexistent-cask")
	if err == nil {
		t.Fatal("expected error for nonexistent cask")
	}

	var resolverErr *ResolverError
	if !isResolverError(err, &resolverErr) || resolverErr.Type != ErrTypeNotFound {
		t.Errorf("expected ErrTypeNotFound, got %v", err)
	}
}

func TestResolveCask_InvalidName(t *testing.T) {
	tests := []struct {
		name    string
		cask    string
		wantErr bool
	}{
		{"empty name", "", true},
		{"path traversal", "../etc/passwd", true},
		{"with slash", "foo/bar", true},
		{"with backslash", "foo\\bar", true},
		{"starts with hyphen", "-invalid", true},
		{"uppercase", "Firefox", true},
		{"valid name", "iterm2", false},
		{"with hyphen", "visual-studio-code", false},
		{"with underscore", "some_cask", false},
		{"with at sign", "cask@1.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := isValidCaskName(tt.cask)
			if valid == tt.wantErr {
				t.Errorf("isValidCaskName(%q) = %v, want %v", tt.cask, valid, !tt.wantErr)
			}
		})
	}
}

func TestResolveCask_MissingChecksum_NoCheck(t *testing.T) {
	server := createMockCaskServer(t)
	defer server.Close()

	resolver := New(WithCaskRegistry(server.URL))

	info, err := resolver.ResolveCask(context.Background(), "no-checksum-cask")
	if err != nil {
		t.Fatalf("ResolveCask() error = %v", err)
	}

	// Missing checksum should be empty string
	if info.Metadata["checksum"] != "" {
		t.Errorf("Checksum = %q, want empty string for :no_check", info.Metadata["checksum"])
	}
}

func TestResolveCask_MissingChecksum_Empty(t *testing.T) {
	server := createMockCaskServer(t)
	defer server.Close()

	resolver := New(WithCaskRegistry(server.URL))

	info, err := resolver.ResolveCask(context.Background(), "empty-checksum-cask")
	if err != nil {
		t.Fatalf("ResolveCask() error = %v", err)
	}

	// Empty checksum should be empty string
	if info.Metadata["checksum"] != "" {
		t.Errorf("Checksum = %q, want empty string", info.Metadata["checksum"])
	}
}

func TestResolveCask_NetworkError(t *testing.T) {
	// Create a server and immediately close it to simulate network error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	resolver := New(WithCaskRegistry(server.URL))

	_, err := resolver.ResolveCask(context.Background(), "iterm2")
	if err == nil {
		t.Fatal("expected network error")
	}
	// Just verify we got an error - the specific type depends on error classification
}

func TestResolveCask_UnexpectedStatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	resolver := New(WithCaskRegistry(server.URL))

	_, err := resolver.ResolveCask(context.Background(), "iterm2")
	if err == nil {
		t.Fatal("expected error for 500 status")
	}

	var resolverErr *ResolverError
	if !isResolverError(err, &resolverErr) || resolverErr.Type != ErrTypeNetwork {
		t.Errorf("expected ErrTypeNetwork, got %v", err)
	}
}

func TestResolveCask_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json{"))
	}))
	defer server.Close()

	resolver := New(WithCaskRegistry(server.URL))

	_, err := resolver.ResolveCask(context.Background(), "iterm2")
	if err == nil {
		t.Fatal("expected parse error for invalid JSON")
	}

	var resolverErr *ResolverError
	if !isResolverError(err, &resolverErr) || resolverErr.Type != ErrTypeParsing {
		t.Errorf("expected ErrTypeParsing, got %v", err)
	}
}

func TestSelectCaskVariation_NoVariations(t *testing.T) {
	info := &homebrewCaskInfo{
		Token:   "iterm2",
		Version: "3.6.6",
		SHA256:  "abc123",
		URL:     "https://example.com/app.zip",
	}

	url, checksum, version := selectCaskVariation(info)

	if url != info.URL {
		t.Errorf("URL = %q, want %q", url, info.URL)
	}
	if checksum != info.SHA256 {
		t.Errorf("Checksum = %q, want %q", checksum, info.SHA256)
	}
	if version != info.Version {
		t.Errorf("Version = %q, want %q", version, info.Version)
	}
}

func TestSelectCaskVariation_WithVariations(t *testing.T) {
	info := &homebrewCaskInfo{
		Token:   "vscode",
		Version: "1.0.0",
		SHA256:  "base-sha",
		URL:     "https://example.com/base.zip",
		Variations: map[string]caskVariation{
			"arm64_sonoma": {
				URL:    "https://example.com/arm64.zip",
				SHA256: "arm64-sha",
			},
			"sonoma": {
				URL:    "https://example.com/intel.zip",
				SHA256: "intel-sha",
			},
		},
	}

	url, checksum, _ := selectCaskVariation(info)

	// Test depends on runtime.GOARCH - just verify we got a valid result
	if url == "" {
		t.Error("URL should not be empty")
	}
	if checksum == "" {
		t.Error("Checksum should not be empty")
	}
}

func TestCaskProvider_ResolveLatest_MockServer(t *testing.T) {
	server := createMockCaskServer(t)
	defer server.Close()

	resolver := New(WithCaskRegistry(server.URL))
	provider := NewCaskProvider(resolver, "iterm2")

	info, err := provider.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest() error = %v", err)
	}

	if info.Version != "3.6.6" {
		t.Errorf("Version = %q, want %q", info.Version, "3.6.6")
	}

	if info.Metadata == nil {
		t.Fatal("Metadata should not be nil")
	}

	if _, ok := info.Metadata["url"]; !ok {
		t.Error("Metadata should contain 'url' field")
	}
	if _, ok := info.Metadata["checksum"]; !ok {
		t.Error("Metadata should contain 'checksum' field")
	}
}

func TestCaskProvider_ResolveVersion_MockServer(t *testing.T) {
	server := createMockCaskServer(t)
	defer server.Close()

	resolver := New(WithCaskRegistry(server.URL))
	provider := NewCaskProvider(resolver, "iterm2")

	// Test exact version match
	info, err := provider.ResolveVersion(context.Background(), "3.6.6")
	if err != nil {
		t.Fatalf("ResolveVersion() error = %v", err)
	}

	if info.Version != "3.6.6" {
		t.Errorf("Version = %q, want %q", info.Version, "3.6.6")
	}

	// Test non-existent version
	_, err = provider.ResolveVersion(context.Background(), "99.99.99")
	if err == nil {
		t.Error("expected error for non-existent version")
	}
}

func TestCaskProvider_ResolveLatest_UnknownCask_MockServer(t *testing.T) {
	server := createMockCaskServer(t)
	defer server.Close()

	resolver := New(WithCaskRegistry(server.URL))
	provider := NewCaskProvider(resolver, "unknown-cask-that-does-not-exist")

	_, err := provider.ResolveLatest(context.Background())
	if err == nil {
		t.Error("expected error for unknown cask")
	}
}

func TestCaskProvider_SourceDescription(t *testing.T) {
	resolver := New()
	provider := NewCaskProvider(resolver, "iterm2")

	desc := provider.SourceDescription()
	if desc != "Cask:iterm2" {
		t.Errorf("SourceDescription() = %q, want %q", desc, "Cask:iterm2")
	}
}

func TestCaskProvider_Interface(t *testing.T) {
	resolver := New()
	provider := NewCaskProvider(resolver, "iterm2")

	// Verify it implements VersionResolver
	var _ VersionResolver = provider
}

func TestCaskSourceStrategy_CanHandle(t *testing.T) {
	tests := []struct {
		name   string
		source string
		cask   string
		want   bool
	}{
		{"cask source with cask name", "cask", "iterm2", true},
		{"cask source without cask name", "cask", "", false},
		{"different source", "homebrew", "iterm2", false},
		{"empty source", "", "iterm2", false},
	}

	strategy := &CaskSourceStrategy{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recipe.Recipe{
				Version: recipe.VersionSection{
					Source: tt.source,
					Cask:   tt.cask,
				},
			}
			if got := strategy.CanHandle(r); got != tt.want {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want)
			}
		})
	}
}

// isResolverError is a helper to check if an error is a ResolverError
func isResolverError(err error, target **ResolverError) bool {
	if resolverErr, ok := err.(*ResolverError); ok {
		*target = resolverErr
		return true
	}
	return false
}

// Integration tests that use the real Homebrew Cask API
// These are skipped in short test mode

func TestResolveCask_RealAPI_iTerm2(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	resolver := New()

	info, err := resolver.ResolveCask(context.Background(), "iterm2")
	if err != nil {
		t.Fatalf("ResolveCask() error = %v", err)
	}

	// Verify we got a version
	if info.Version == "" {
		t.Error("Version should not be empty")
	}

	// Verify metadata contains expected fields
	if info.Metadata == nil {
		t.Fatal("Metadata should not be nil")
	}

	url, ok := info.Metadata["url"]
	if !ok || url == "" {
		t.Error("Metadata should contain 'url' field")
	}

	checksum, ok := info.Metadata["checksum"]
	if !ok || checksum == "" {
		t.Error("Metadata should contain 'checksum' field")
	}
	if !strings.HasPrefix(checksum, "sha256:") {
		t.Errorf("Checksum should start with 'sha256:', got %q", checksum)
	}
}

func TestResolveCask_RealAPI_VisualStudioCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	resolver := New()

	info, err := resolver.ResolveCask(context.Background(), "visual-studio-code")
	if err != nil {
		t.Fatalf("ResolveCask() error = %v", err)
	}

	// Verify we got a version
	if info.Version == "" {
		t.Error("Version should not be empty")
	}

	// Verify URL contains architecture-specific path
	url := info.Metadata["url"]
	if url == "" {
		t.Error("URL should not be empty")
	}

	// VS Code has architecture-specific URLs
	if !strings.Contains(url, "update.code.visualstudio.com") {
		t.Errorf("URL should be from VS Code CDN, got %q", url)
	}
}
