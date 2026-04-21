package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/httputil"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/progress"
)

// DownloadFileAction implements deterministic file downloading with required checksum.
// This is a primitive action used in plans where URLs are fully resolved.
// For recipe-level downloads with placeholders, use the download action instead.
type DownloadFileAction struct{ BaseAction }

// IsDeterministic returns true because download_file requires checksums.
func (DownloadFileAction) IsDeterministic() bool { return true }

// RequiresNetwork returns true because download_file fetches files from URLs.
func (DownloadFileAction) RequiresNetwork() bool { return true }

// Name returns the action name
func (a *DownloadFileAction) Name() string {
	return "download_file"
}

// Execute downloads a file with required checksum verification
//
// Parameters:
//   - url (required): Fully resolved URL to download from (no placeholders)
//   - dest (optional): Destination filename (defaults to basename of URL)
//   - checksum (required): SHA256 checksum in hex format
//   - checksum_algo (optional): Hash algorithm (sha256, sha512), defaults to sha256
//   - size (optional): Expected file size in bytes
func (a *DownloadFileAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get URL (required)
	url, ok := GetString(params, "url")
	if !ok {
		return fmt.Errorf("download_file action requires 'url' parameter")
	}

	// Get checksum (required for download_file)
	checksum, ok := GetString(params, "checksum")
	if !ok || checksum == "" {
		return fmt.Errorf("download_file action requires 'checksum' parameter")
	}

	// Get destination filename
	dest, ok := GetString(params, "dest")
	if !ok {
		// Default to basename of URL
		dest = filepath.Base(url)
		// Remove query parameters if present
		if idx := strings.Index(dest, "?"); idx != -1 {
			dest = dest[:idx]
		}
	}

	destPath := filepath.Join(ctx.WorkDir, dest)

	// Get logger for debug output
	logger := ctx.Log()
	logger.Debug("download_file action starting",
		"url", log.SanitizeURL(url),
		"dest", dest,
		"destPath", destPath)

	// Get checksum algorithm
	checksumAlgo, _ := GetString(params, "checksum_algo")
	if checksumAlgo == "" {
		checksumAlgo = "sha256"
	}

	reporter := ctx.GetReporter()

	// Check download cache if available
	var cache *DownloadCache
	if ctx.DownloadCacheDir != "" {
		cache = NewDownloadCache(ctx.DownloadCacheDir)
		cache.SetSkipSecurityChecks(ctx.SkipCacheSecurityChecks)
		logger.Debug("checking download cache", "cacheDir", ctx.DownloadCacheDir)
		found, err := cache.Check(url, destPath, checksum, checksumAlgo)
		if err != nil {
			// Log warning but continue with download
			logger.Warn("cache check failed", "error", err)
			reporter.Warn("cache check failed: %v", err)
		} else if found {
			logger.Debug("cache hit", "dest", dest)
			reporter.Log("Using cached: %s", dest)
			return nil
		} else {
			logger.Debug("cache miss")
		}
	}

	// Download file with context for cancellation support
	if err := downloadFileHTTP(ctx.Context, url, destPath, reporter); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Verify checksum (required)
	if err := VerifyChecksum(destPath, checksum, checksumAlgo); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}
	logger.Debug("checksum verification passed", "algo", checksumAlgo)

	// Save to cache if available
	if cache != nil {
		if err := cache.Save(url, destPath, checksum); err != nil {
			// Log warning but don't fail the download
			logger.Warn("failed to cache download", "error", err)
			reporter.Warn("failed to cache download: %v", err)
		} else {
			logger.Debug("saved to download cache")
		}
	}

	logger.Debug("download_file completed successfully", "dest", dest)
	return nil
}

// isRetryableStatusCode returns true if the HTTP status code is transient and
// the request should be retried. This includes:
// - 403 Forbidden: Often caused by rate limiting or bot detection
// - 429 Too Many Requests: Explicit rate limiting
// - 5xx Server Errors: Temporary server issues
func isRetryableStatusCode(statusCode int) bool {
	return statusCode == http.StatusForbidden ||
		statusCode == http.StatusTooManyRequests ||
		statusCode >= 500
}

// makeDownloadCallback returns a progress callback that formats and emits
// transient status messages via reporter.Status. The name parameter is the
// display name (already sanitized). The callback is only invoked by
// ProgressWriter when the file is large enough to warrant reporting.
func makeDownloadCallback(reporter progress.Reporter, name string) func(written, total int64) {
	return func(written, total int64) {
		var msg string
		if total > 0 {
			pct := float64(written) / float64(total) * 100
			if pct > 100 {
				pct = 100
			}
			transferred := formatBytes(written)
			totalStr := formatBytes(total)
			msg = fmt.Sprintf("Downloading %s (%s / %s, %.0f%%)", name, transferred, totalStr, pct)
		} else {
			transferred := formatBytes(written)
			msg = fmt.Sprintf("Downloading %s (%s...)", name, transferred)
		}
		reporter.Status(msg)
	}
}

// downloadDisplayName derives a safe display name from a download URL.
// It strips query parameters and sanitizes the result against ANSI injection.
func downloadDisplayName(downloadURL string) string {
	base := filepath.Base(downloadURL)
	if idx := strings.Index(base, "?"); idx != -1 {
		base = base[:idx]
	}
	return progress.SanitizeDisplayString(base)
}

// downloadFileHTTP performs the actual HTTP download with context for cancellation.
// Implements retry logic with exponential backoff for transient errors (403, 429, 5xx).
// SECURITY: Enforces HTTPS for all downloads to prevent MITM attacks
func downloadFileHTTP(ctx context.Context, downloadURL, destPath string, reporter progress.Reporter) error {
	// SECURITY: Enforce HTTPS for all downloads
	if !strings.HasPrefix(downloadURL, "https://") {
		return fmt.Errorf("download URL must use HTTPS for security, got: %s", downloadURL)
	}

	const maxRetries = 3
	baseDelay := time.Second

	name := downloadDisplayName(downloadURL)
	callback := makeDownloadCallback(reporter, name)

	var pw *progress.ProgressWriter
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := baseDelay * time.Duration(1<<(attempt-1))
			reporter.Log("Retry %d/%d after %v...", attempt, maxRetries, delay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			if pw != nil {
				pw.Reset()
			}
		} else {
			// Non-TTY: log once at the start of the first attempt
			if !progress.ShouldShowProgress() {
				reporter.Log("Downloading %s", name)
			}
		}

		err := doDownloadFileHTTP(ctx, downloadURL, destPath, callback, &pw)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable (contains status code info)
		// Non-retryable errors (400, 404, etc.) fail immediately
		if httpErr, ok := err.(*httpStatusError); ok {
			if !isRetryableStatusCode(httpErr.StatusCode) {
				return err
			}
			// Retryable error, continue to next attempt
			continue
		}

		// For non-HTTP errors (network issues, etc.), also retry
		// unless context is canceled
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return fmt.Errorf("download failed after %d retries: %w", maxRetries, lastErr)
}

// httpStatusError wraps an HTTP error with status code for retry logic
type httpStatusError struct {
	StatusCode int
	Status     string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("bad status: %s", e.Status)
}

// doDownloadFileHTTP performs a single HTTP download attempt. It creates a
// ProgressWriter backed by the supplied callback and writes the address of that
// writer to pwOut so the retry loop can call Reset() before the next attempt.
func doDownloadFileHTTP(ctx context.Context, downloadURL, destPath string, callback func(written, total int64), pwOut **progress.ProgressWriter) error {
	// Create secure HTTP client with decompression bomb and SSRF protection
	client := newDownloadHTTPClient()
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent to avoid 403 from servers that block default Go HTTP client
	req.Header.Set("User-Agent", httputil.DefaultUserAgent)

	// Defense in depth: Explicitly request uncompressed response
	req.Header.Set("Accept-Encoding", "identity")

	// Add authorization for ghcr.io URLs (Homebrew bottles)
	if token, err := getGHCRTokenForURL(downloadURL); err == nil && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Perform request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &httpStatusError{StatusCode: resp.StatusCode, Status: resp.Status}
	}

	// Defense in depth: Reject compressed responses
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return fmt.Errorf("compressed responses not supported (got %s)", encoding)
	}

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Wrap the destination file with a ProgressWriter for byte-level progress.
	// The ProgressWriter suppresses the callback for small files automatically.
	pw := progress.NewProgressWriter(out, resp.ContentLength, callback)
	*pwOut = pw
	if _, err := io.Copy(pw, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// getGHCRTokenForURL checks if a URL is a ghcr.io Homebrew bottle URL and fetches
// an anonymous access token if so. Returns empty string for non-ghcr.io URLs.
func getGHCRTokenForURL(downloadURL string) (string, error) {
	// Only handle ghcr.io/v2/homebrew/core/... URLs
	if !strings.HasPrefix(downloadURL, "https://ghcr.io/v2/homebrew/core/") {
		return "", nil
	}

	// Parse URL to extract formula name
	// URL format: https://ghcr.io/v2/homebrew/core/{formula}/blobs/sha256:{hash}
	// For versioned formulas: https://ghcr.io/v2/homebrew/core/{formula}/{version}/blobs/sha256:{hash}
	// e.g., openssl@3 becomes openssl/3 in the path
	parsed, err := url.Parse(downloadURL)
	if err != nil {
		return "", nil
	}

	// Extract formula from path: /v2/homebrew/core/{formula}[/{version}]/blobs/...
	// We need everything between "core/" and "/blobs/"
	path := parsed.Path
	corePrefix := "/v2/homebrew/core/"
	if !strings.HasPrefix(path, corePrefix) {
		return "", nil
	}
	afterCore := strings.TrimPrefix(path, corePrefix)

	// Find "/blobs/" to know where the formula path ends
	blobsIdx := strings.Index(afterCore, "/blobs/")
	if blobsIdx == -1 {
		return "", nil
	}
	formula := afterCore[:blobsIdx]

	// Fetch anonymous token from ghcr.io
	// Note: formula may contain slashes for versioned formulas (e.g., "openssl/3")
	// The token endpoint accepts unencoded slashes in the scope parameter
	tokenURL := fmt.Sprintf("https://ghcr.io/token?service=ghcr.io&scope=repository:homebrew/core/%s:pull", formula)

	resp, err := newDownloadHTTPClient().Get(tokenURL)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request returned %d", resp.StatusCode)
	}

	var tokenResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	return tokenResp.Token, nil
}
