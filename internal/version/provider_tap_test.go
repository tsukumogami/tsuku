package version

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// Sample formula content for testing
const sampleTerraformFormula = `
class Terraform < Formula
  desc "Tool to build, change, and version infrastructure"
  homepage "https://www.terraform.io/"
  version "1.7.0"
  license "MPL-2.0"

  bottle do
    root_url "https://github.com/hashicorp/homebrew-tap/releases/download/terraform-1.7.0"
    sha256 "abc123def456abc123def456abc123def456abc123def456abc123def456abc1" => :arm64_sonoma
    sha256 "def456abc123def456abc123def456abc123def456abc123def456abc123def4" => :arm64_ventura
    sha256 "aabbccddee11223344556677889900aabbccddee11223344556677889900aabb" => :sonoma
    sha256 "ffeeddccbb99887766554433221100aabbccddee11223344556677889900ffee" => :ventura
    sha256 "fedcba987654321fedcba987654321fedcba987654321fedcba987654321fedc" => :x86_64_linux
    sha256 "abcdef123456789abcdef123456789abcdef123456789abcdef123456789abcd" => :arm64_linux
  end
end
`

// Formula with alternative sha256 syntax: sha256 platform: "hash"
const sampleAlternateSyntaxFormula = `
class Vault < Formula
  version "1.15.0"

  bottle do
    root_url "https://github.com/hashicorp/homebrew-tap/releases/download/vault-1.15.0"
    sha256 arm64_sonoma: "1111111111111111111111111111111111111111111111111111111111111111"
    sha256 sonoma: "2222222222222222222222222222222222222222222222222222222222222222"
    sha256 x86_64_linux: "3333333333333333333333333333333333333333333333333333333333333333"
  end
end
`

// Formula without bottle block (source-only)
const sampleSourceOnlyFormula = `
class SomeFormula < Formula
  version "2.0.0"
  url "https://example.com/source.tar.gz"
end
`

// Formula without version
const sampleNoVersionFormula = `
class SomeFormula < Formula
  bottle do
    sha256 "abc123def456abc123def456abc123def456abc123def456abc123def456abc1" => :sonoma
  end
end
`

// Formula with bottle block but no checksums
const sampleNoChecksumsFormula = `
class SomeFormula < Formula
  version "1.0.0"
  bottle do
    root_url "https://example.com/bottles"
  end
end
`

func TestParseFormulaFile_Success(t *testing.T) {
	info, err := parseFormulaFile(sampleTerraformFormula)
	if err != nil {
		t.Fatalf("parseFormulaFile() error = %v", err)
	}

	if info.Version != "1.7.0" {
		t.Errorf("Version = %q, want %q", info.Version, "1.7.0")
	}

	if info.RootURL != "https://github.com/hashicorp/homebrew-tap/releases/download/terraform-1.7.0" {
		t.Errorf("RootURL = %q, want correct URL", info.RootURL)
	}

	expectedChecksums := map[string]string{
		"arm64_sonoma":  "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
		"arm64_ventura": "def456abc123def456abc123def456abc123def456abc123def456abc123def4",
		"sonoma":        "aabbccddee11223344556677889900aabbccddee11223344556677889900aabb",
		"ventura":       "ffeeddccbb99887766554433221100aabbccddee11223344556677889900ffee",
		"x86_64_linux":  "fedcba987654321fedcba987654321fedcba987654321fedcba987654321fedc",
		"arm64_linux":   "abcdef123456789abcdef123456789abcdef123456789abcdef123456789abcd",
	}

	if len(info.Checksums) != len(expectedChecksums) {
		t.Errorf("Checksums count = %d, want %d", len(info.Checksums), len(expectedChecksums))
	}

	for platform, expected := range expectedChecksums {
		if got := info.Checksums[platform]; got != expected {
			t.Errorf("Checksums[%s] = %q, want %q", platform, got, expected)
		}
	}
}

func TestParseFormulaFile_AlternateSyntax(t *testing.T) {
	info, err := parseFormulaFile(sampleAlternateSyntaxFormula)
	if err != nil {
		t.Fatalf("parseFormulaFile() error = %v", err)
	}

	if info.Version != "1.15.0" {
		t.Errorf("Version = %q, want %q", info.Version, "1.15.0")
	}

	expectedChecksums := map[string]string{
		"arm64_sonoma": "1111111111111111111111111111111111111111111111111111111111111111",
		"sonoma":       "2222222222222222222222222222222222222222222222222222222222222222",
		"x86_64_linux": "3333333333333333333333333333333333333333333333333333333333333333",
	}

	for platform, expected := range expectedChecksums {
		if got := info.Checksums[platform]; got != expected {
			t.Errorf("Checksums[%s] = %q, want %q", platform, got, expected)
		}
	}
}

func TestParseFormulaFile_NoBottleBlock(t *testing.T) {
	_, err := parseFormulaFile(sampleSourceOnlyFormula)
	if err == nil {
		t.Error("parseFormulaFile() expected error for source-only formula")
	}
	if !strings.Contains(err.Error(), "no bottle block found") {
		t.Errorf("Error message = %q, want to contain 'no bottle block found'", err.Error())
	}
}

func TestParseFormulaFile_NoVersion(t *testing.T) {
	_, err := parseFormulaFile(sampleNoVersionFormula)
	if err == nil {
		t.Error("parseFormulaFile() expected error for formula without version")
	}
	if !strings.Contains(err.Error(), "no version found") {
		t.Errorf("Error message = %q, want to contain 'no version found'", err.Error())
	}
}

func TestParseFormulaFile_NoChecksums(t *testing.T) {
	_, err := parseFormulaFile(sampleNoChecksumsFormula)
	if err == nil {
		t.Error("parseFormulaFile() expected error for formula without checksums")
	}
	if !strings.Contains(err.Error(), "no bottle checksums found") {
		t.Errorf("Error message = %q, want to contain 'no bottle checksums found'", err.Error())
	}
}

func TestGetPlatformTags(t *testing.T) {
	tests := []struct {
		name         string
		goos         string
		goarch       string
		macOSVersion int
		wantFirst    string
		wantLen      int
	}{
		{"darwin arm64 sonoma", "darwin", "arm64", 14, "arm64_sonoma", 4},
		{"darwin amd64 sonoma", "darwin", "amd64", 14, "sonoma", 4},
		{"darwin arm64 ventura", "darwin", "arm64", 13, "arm64_ventura", 3},
		{"darwin amd64 ventura", "darwin", "amd64", 13, "ventura", 3},
		{"darwin arm64 default", "darwin", "arm64", 0, "arm64_sonoma", 4}, // defaults to sonoma
		{"linux amd64", "linux", "amd64", 0, "x86_64_linux", 1},
		{"linux arm64", "linux", "arm64", 0, "arm64_linux", 1},
		{"unknown os", "windows", "amd64", 0, "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := getPlatformTags(tt.goos, tt.goarch, tt.macOSVersion)
			if len(tags) != tt.wantLen {
				t.Errorf("getPlatformTags() len = %d, want %d (got %v)", len(tags), tt.wantLen, tags)
			}
			if tt.wantLen > 0 && (len(tags) == 0 || tags[0] != tt.wantFirst) {
				t.Errorf("getPlatformTags() first = %q, want %q", tags[0], tt.wantFirst)
			}
		})
	}
}

func TestBuildBottleURL(t *testing.T) {
	tests := []struct {
		name     string
		rootURL  string
		formula  string
		version  string
		platform string
		want     string
	}{
		{
			name:     "standard URL",
			rootURL:  "https://github.com/hashicorp/homebrew-tap/releases/download/terraform-1.7.0",
			formula:  "terraform",
			version:  "1.7.0",
			platform: "arm64_sonoma",
			want:     "https://github.com/hashicorp/homebrew-tap/releases/download/terraform-1.7.0/terraform--1.7.0.arm64_sonoma.bottle.tar.gz",
		},
		{
			name:     "trailing slash in root_url",
			rootURL:  "https://example.com/bottles/",
			formula:  "myformula",
			version:  "2.0.0",
			platform: "x86_64_linux",
			want:     "https://example.com/bottles/myformula--2.0.0.x86_64_linux.bottle.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildBottleURL(tt.rootURL, tt.formula, tt.version, tt.platform)
			if got != tt.want {
				t.Errorf("buildBottleURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseTap(t *testing.T) {
	tests := []struct {
		name      string
		tap       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"valid tap", "hashicorp/tap", "hashicorp", "homebrew-tap", false},
		{"github tap", "github/gh", "github", "homebrew-gh", false},
		{"invalid single part", "hashicorp", "", "", true},
		{"invalid three parts", "a/b/c", "", "", true},
		{"empty", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := parseTap(tt.tap)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if owner != tt.wantOwner {
				t.Errorf("parseTap() owner = %q, want %q", owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("parseTap() repo = %q, want %q", repo, tt.wantRepo)
			}
		})
	}
}

func TestTapProvider_ResolveLatest(t *testing.T) {
	// Create a test server that serves formula content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check the request path
		if strings.Contains(r.URL.Path, "Formula/terraform.rb") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(sampleTerraformFormula))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create a resolver with a custom HTTP client that redirects to our test server
	resolver := New()
	resolver.httpClient = &http.Client{
		Transport: &testTransport{
			server: server,
		},
	}

	provider := NewTapProvider(resolver, "hashicorp/tap", "terraform")

	info, err := provider.ResolveLatest(context.Background())
	if err != nil {
		t.Fatalf("ResolveLatest() error = %v", err)
	}

	if info.Version != "1.7.0" {
		t.Errorf("Version = %q, want %q", info.Version, "1.7.0")
	}

	// Verify metadata
	if info.Metadata == nil {
		t.Fatal("Metadata is nil")
	}

	if info.Metadata["formula"] != "terraform" {
		t.Errorf("Metadata[formula] = %q, want %q", info.Metadata["formula"], "terraform")
	}

	if info.Metadata["tap"] != "hashicorp/tap" {
		t.Errorf("Metadata[tap] = %q, want %q", info.Metadata["tap"], "hashicorp/tap")
	}

	// Checksum should have sha256: prefix
	if !strings.HasPrefix(info.Metadata["checksum"], "sha256:") {
		t.Errorf("Metadata[checksum] should start with 'sha256:', got %q", info.Metadata["checksum"])
	}

	// Bottle URL should be properly constructed
	if info.Metadata["bottle_url"] == "" {
		t.Error("Metadata[bottle_url] is empty")
	}
}

func TestTapProvider_ResolveVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "Formula/terraform.rb") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(sampleTerraformFormula))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	resolver := New()
	resolver.httpClient = &http.Client{
		Transport: &testTransport{server: server},
	}

	provider := NewTapProvider(resolver, "hashicorp/tap", "terraform")

	// Test exact version match
	info, err := provider.ResolveVersion(context.Background(), "1.7.0")
	if err != nil {
		t.Fatalf("ResolveVersion() error = %v", err)
	}
	if info.Version != "1.7.0" {
		t.Errorf("Version = %q, want %q", info.Version, "1.7.0")
	}

	// Test non-existent version
	_, err = provider.ResolveVersion(context.Background(), "99.99.99")
	if err == nil {
		t.Error("ResolveVersion() expected error for non-existent version")
	}
}

func TestTapProvider_ResolveLatest_FormulaNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	resolver := New()
	resolver.httpClient = &http.Client{
		Transport: &testTransport{server: server},
	}

	provider := NewTapProvider(resolver, "hashicorp/tap", "nonexistent")

	_, err := provider.ResolveLatest(context.Background())
	if err == nil {
		t.Error("ResolveLatest() expected error for non-existent formula")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error message = %q, want to contain 'not found'", err.Error())
	}
}

func TestTapProvider_SourceDescription(t *testing.T) {
	resolver := New()
	provider := NewTapProvider(resolver, "hashicorp/tap", "terraform")

	desc := provider.SourceDescription()
	if desc != "Tap:hashicorp/tap/terraform" {
		t.Errorf("SourceDescription() = %q, want %q", desc, "Tap:hashicorp/tap/terraform")
	}
}

func TestTapProvider_Interface(t *testing.T) {
	resolver := New()
	provider := NewTapProvider(resolver, "hashicorp/tap", "terraform")

	// Verify it implements VersionResolver
	var _ VersionResolver = provider
}

func TestTapSourceStrategy_CanHandle(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		tap     string
		formula string
		want    bool
	}{
		{"tap source with tap and formula", "tap", "hashicorp/tap", "terraform", true},
		{"tap source without tap", "tap", "", "terraform", false},
		{"tap source without formula", "tap", "hashicorp/tap", "", false},
		{"tap source without both", "tap", "", "", false},
		{"different source", "homebrew", "hashicorp/tap", "terraform", false},
		{"empty source", "", "hashicorp/tap", "terraform", false},
	}

	strategy := &TapSourceStrategy{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recipe.Recipe{
				Version: recipe.VersionSection{
					Source:  tt.source,
					Tap:     tt.tap,
					Formula: tt.formula,
				},
			}
			if got := strategy.CanHandle(r); got != tt.want {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want)
			}
		})
	}
}

// testTransport is a custom transport that redirects requests to the test server
type testTransport struct {
	server *httptest.Server
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite the URL to point to our test server
	newURL := t.server.URL + req.URL.Path
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	return http.DefaultTransport.RoundTrip(newReq)
}
