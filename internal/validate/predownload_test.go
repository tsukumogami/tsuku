package validate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/httputil"
)

func TestPreDownloader_Download_Success(t *testing.T) {
	content := "hello world test content"
	expectedChecksum := sha256sum(content)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	p := NewPreDownloader().WithHTTPClient(server.Client())

	ctx := context.Background()
	result, err := p.Download(ctx, server.URL+"/test.txt")
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	defer func() { _ = result.Cleanup() }()

	// Verify checksum
	if result.Checksum != expectedChecksum {
		t.Errorf("Checksum = %q, want %q", result.Checksum, expectedChecksum)
	}

	// Verify size
	if result.Size != int64(len(content)) {
		t.Errorf("Size = %d, want %d", result.Size, len(content))
	}

	// Verify file exists and has correct content
	data, err := os.ReadFile(result.AssetPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}
	if string(data) != content {
		t.Errorf("File content = %q, want %q", string(data), content)
	}

	// Verify filename is extracted from URL
	if filepath.Base(result.AssetPath) != "test.txt" {
		t.Errorf("Filename = %q, want %q", filepath.Base(result.AssetPath), "test.txt")
	}
}

func TestPreDownloader_Download_FilenameWithQuery(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer server.Close()

	p := NewPreDownloader().WithHTTPClient(server.Client())

	ctx := context.Background()
	result, err := p.Download(ctx, server.URL+"/archive.tar.gz?token=abc123")
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	defer func() { _ = result.Cleanup() }()

	// Query parameters should be stripped from filename
	if filepath.Base(result.AssetPath) != "archive.tar.gz" {
		t.Errorf("Filename = %q, want %q", filepath.Base(result.AssetPath), "archive.tar.gz")
	}
}

func TestPreDownloader_Download_HTTPError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    string
	}{
		{
			name:       "404 Not Found",
			statusCode: http.StatusNotFound,
			wantErr:    "bad status: 404",
		},
		{
			name:       "500 Internal Server Error",
			statusCode: http.StatusInternalServerError,
			wantErr:    "bad status: 500",
		},
		{
			name:       "403 Forbidden",
			statusCode: http.StatusForbidden,
			wantErr:    "bad status: 403",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			p := NewPreDownloader().WithHTTPClient(server.Client())

			ctx := context.Background()
			result, err := p.Download(ctx, server.URL+"/file.txt")

			if err == nil {
				_ = result.Cleanup()
				t.Fatal("Download() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Download() error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestPreDownloader_Download_NonHTTPS(t *testing.T) {
	p := NewPreDownloader()

	ctx := context.Background()
	_, err := p.Download(ctx, "http://example.com/file.txt")

	if err == nil {
		t.Fatal("Download() expected error for non-HTTPS URL, got nil")
	}
	if !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("Download() error = %q, want to contain 'HTTPS'", err.Error())
	}
}

func TestPreDownloader_Download_CleanupOnError(t *testing.T) {
	// Server that returns partial content then closes connection
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		// Simulate connection close mid-download by not writing Content-Length
		// and then closing (the test client will handle this)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	p := NewPreDownloader().
		WithHTTPClient(server.Client()).
		WithTempDir(tempDir)

	ctx := context.Background()
	result, err := p.Download(ctx, server.URL+"/file.txt")
	if err != nil {
		// Expected if connection issues occur
		// Verify no temp directories remain
		entries, _ := os.ReadDir(tempDir)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "tsuku-validate-") {
				t.Errorf("Temp directory should be cleaned up on error: %s", e.Name())
			}
		}
		return
	}
	defer func() { _ = result.Cleanup() }()
}

func TestPreDownloader_Download_CompressedResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("compressed data"))
	}))
	defer server.Close()

	p := NewPreDownloader().WithHTTPClient(server.Client())

	ctx := context.Background()
	_, err := p.Download(ctx, server.URL+"/file.txt")

	if err == nil {
		t.Fatal("Download() expected error for compressed response, got nil")
	}
	if !strings.Contains(err.Error(), "compressed") {
		t.Errorf("Download() error = %q, want to contain 'compressed'", err.Error())
	}
}

func TestPreDownloader_Download_ContextCancellation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		// Simulate slow response - wait for context to be done
		<-r.Context().Done()
	}))
	defer server.Close()

	p := NewPreDownloader().WithHTTPClient(server.Client())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := p.Download(ctx, server.URL+"/file.txt")

	if err == nil {
		t.Fatal("Download() expected error for canceled context, got nil")
	}
}

func TestDownloadResult_Cleanup(t *testing.T) {
	// Create a temp directory with a file
	tempDir := t.TempDir()
	downloadDir := filepath.Join(tempDir, "tsuku-validate-test")
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	filePath := filepath.Join(downloadDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result := &DownloadResult{
		AssetPath: filePath,
		Checksum:  "abc123",
		Size:      4,
	}

	if err := result.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v, want nil", err)
	}

	// Verify directory is removed
	if _, err := os.Stat(downloadDir); !os.IsNotExist(err) {
		t.Error("Cleanup() should remove parent directory")
	}
}

func TestDownloadResult_Cleanup_EmptyPath(t *testing.T) {
	result := &DownloadResult{}

	if err := result.Cleanup(); err != nil {
		t.Errorf("Cleanup() with empty path error = %v, want nil", err)
	}
}

func TestPreDownloader_WithTempDir(t *testing.T) {
	content := "test content"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	customTempDir := t.TempDir()
	p := NewPreDownloader().
		WithHTTPClient(server.Client()).
		WithTempDir(customTempDir)

	ctx := context.Background()
	result, err := p.Download(ctx, server.URL+"/file.txt")
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	defer func() { _ = result.Cleanup() }()

	// Verify file is in custom temp dir
	if !strings.HasPrefix(result.AssetPath, customTempDir) {
		t.Errorf("AssetPath = %q, want to start with %q", result.AssetPath, customTempDir)
	}
}

func TestValidatePreDownloadIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"public IPv4", "8.8.8.8", false},
		{"public IPv6", "2001:4860:4860::8888", false},
		{"private 10.x.x.x", "10.0.0.1", true},
		{"private 172.16.x.x", "172.16.0.1", true},
		{"private 192.168.x.x", "192.168.1.1", true},
		{"loopback IPv4", "127.0.0.1", true},
		{"loopback IPv6", "::1", true},
		{"link-local", "169.254.1.1", true},
		{"multicast", "224.0.0.1", true},
		{"unspecified", "0.0.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			err := httputil.ValidateIP(ip, "test-host")

			if (err != nil) != tt.wantErr {
				t.Errorf("validatePreDownloadIP(%q) error = %v, wantErr = %v", tt.ip, err, tt.wantErr)
			}
		})
	}
}

func TestPreDownloader_Download_LargeFile(t *testing.T) {
	// Test with a moderately large file to ensure streaming works
	size := 1024 * 1024 // 1MB
	content := strings.Repeat("x", size)
	expectedChecksum := sha256sum(content)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	p := NewPreDownloader().WithHTTPClient(server.Client())

	ctx := context.Background()
	result, err := p.Download(ctx, server.URL+"/large.bin")
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	defer func() { _ = result.Cleanup() }()

	if result.Checksum != expectedChecksum {
		t.Errorf("Checksum mismatch for large file")
	}
	if result.Size != int64(size) {
		t.Errorf("Size = %d, want %d", result.Size, size)
	}
}

// Helper functions

func sha256sum(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
