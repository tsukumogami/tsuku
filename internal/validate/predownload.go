package validate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/httputil"
)

// DownloadResult contains the result of a pre-download operation.
type DownloadResult struct {
	AssetPath string // Path to the downloaded file
	Checksum  string // SHA256 checksum (hex encoded)
	Size      int64  // File size in bytes
}

// PreDownloader downloads assets before container execution and computes checksums.
type PreDownloader struct {
	httpClient *http.Client
	tempDir    string
}

// NewPreDownloader creates a new PreDownloader with sensible defaults.
func NewPreDownloader() *PreDownloader {
	return &PreDownloader{
		httpClient: newPreDownloadHTTPClient(),
		tempDir:    os.TempDir(),
	}
}

// WithTempDir sets a custom temp directory for downloads.
func (p *PreDownloader) WithTempDir(dir string) *PreDownloader {
	p.tempDir = dir
	return p
}

// WithHTTPClient sets a custom HTTP client.
func (p *PreDownloader) WithHTTPClient(client *http.Client) *PreDownloader {
	p.httpClient = client
	return p
}

// Download downloads a file from the given URL and computes its SHA256 checksum.
// The file is downloaded to a temporary directory within the configured temp dir.
// On success, returns the download result with path, checksum, and size.
// On failure, any partial download is cleaned up.
func (p *PreDownloader) Download(ctx context.Context, url string) (*DownloadResult, error) {
	// SECURITY: Enforce HTTPS for all downloads
	if !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("download URL must use HTTPS for security, got: %s", url)
	}

	// Create temp directory for this download
	downloadDir, err := os.MkdirTemp(p.tempDir, "tsuku-validate-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Determine filename from URL
	filename := filepath.Base(url)
	if idx := strings.Index(filename, "?"); idx != -1 {
		filename = filename[:idx]
	}
	if filename == "" || filename == "." {
		filename = "download"
	}

	destPath := filepath.Join(downloadDir, filename)

	// Download with cleanup on failure
	result, err := p.downloadWithChecksum(ctx, url, destPath)
	if err != nil {
		// Clean up temp directory on failure
		os.RemoveAll(downloadDir)
		return nil, err
	}

	return result, nil
}

// downloadWithChecksum performs the actual download and computes SHA256 during transfer.
func (p *PreDownloader) downloadWithChecksum(ctx context.Context, url, destPath string) (*DownloadResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Defense in depth: Explicitly request uncompressed response
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	// Defense in depth: Reject compressed responses
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return nil, fmt.Errorf("compressed responses not supported (got %s)", encoding)
	}

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Use TeeReader to compute hash while writing
	hash := sha256.New()
	reader := io.TeeReader(resp.Body, hash)

	// Copy response body to file
	size, err := io.Copy(out, reader)
	if err != nil {
		// Clean up partial file
		out.Close()
		os.Remove(destPath)
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Compute final checksum
	checksum := hex.EncodeToString(hash.Sum(nil))

	return &DownloadResult{
		AssetPath: destPath,
		Checksum:  checksum,
		Size:      size,
	}, nil
}

// Cleanup removes the downloaded file and its parent directory.
// This should be called after the download is no longer needed.
func (r *DownloadResult) Cleanup() error {
	if r.AssetPath == "" {
		return nil
	}
	// Remove the parent directory (created by PreDownloader.Download)
	dir := filepath.Dir(r.AssetPath)
	return os.RemoveAll(dir)
}

// newPreDownloadHTTPClient creates a secure HTTP client for downloads using the
// shared httputil package for SSRF protection and security hardening.
// Uses a longer timeout (10 minutes) to allow for large asset downloads.
func newPreDownloadHTTPClient() *http.Client {
	return httputil.NewSecureClient(httputil.ClientOptions{
		Timeout:               10 * time.Minute, // Allow longer downloads for large assets
		ResponseHeaderTimeout: 30 * time.Second,
	})
}

// PreDownloaderAdapter wraps PreDownloader to implement actions.Downloader interface.
// This adapter converts between validate.DownloadResult and actions.DownloadResult.
type PreDownloaderAdapter struct {
	inner *PreDownloader
}

// NewPreDownloaderAdapter creates a new adapter wrapping the given PreDownloader.
func NewPreDownloaderAdapter(p *PreDownloader) *PreDownloaderAdapter {
	return &PreDownloaderAdapter{inner: p}
}

// Download implements actions.Downloader by delegating to PreDownloader
// and converting the result type.
func (a *PreDownloaderAdapter) Download(ctx context.Context, url string) (*actions.DownloadResult, error) {
	result, err := a.inner.Download(ctx, url)
	if err != nil {
		return nil, err
	}
	return &actions.DownloadResult{
		AssetPath: result.AssetPath,
		Checksum:  result.Checksum,
		Size:      result.Size,
	}, nil
}
