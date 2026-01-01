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

	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/progress"
)

// DownloadFileAction implements deterministic file downloading with required checksum.
// This is a primitive action used in plans where URLs are fully resolved.
// For recipe-level downloads with placeholders, use the download action instead.
type DownloadFileAction struct{ BaseAction }

// IsDeterministic returns true because download_file requires checksums.
func (DownloadFileAction) IsDeterministic() bool { return true }

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

	// Check download cache if available
	var cache *DownloadCache
	if ctx.DownloadCacheDir != "" {
		cache = NewDownloadCache(ctx.DownloadCacheDir)
		logger.Debug("checking download cache", "cacheDir", ctx.DownloadCacheDir)
		found, err := cache.Check(url, destPath, checksum, checksumAlgo)
		if err != nil {
			// Log warning but continue with download
			logger.Warn("cache check failed", "error", err)
			fmt.Printf("   Warning: cache check failed: %v\n", err)
		} else if found {
			logger.Debug("cache hit", "dest", dest)
			fmt.Printf("   Using cached: %s\n", dest)
			fmt.Printf("   ✓ Restored from cache\n")
			return nil
		} else {
			logger.Debug("cache miss")
		}
	}

	fmt.Printf("   Downloading: %s\n", url)
	fmt.Printf("   Destination: %s\n", dest)

	// Download file with context for cancellation support
	if err := downloadFileHTTP(ctx.Context, url, destPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Verify checksum (required)
	fmt.Printf("   Verifying %s checksum...\n", checksumAlgo)
	if err := VerifyChecksum(destPath, checksum, checksumAlgo); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}
	logger.Debug("checksum verification passed", "algo", checksumAlgo)

	// Save to cache if available
	if cache != nil {
		if err := cache.Save(url, destPath, checksum); err != nil {
			// Log warning but don't fail the download
			logger.Warn("failed to cache download", "error", err)
			fmt.Printf("   Warning: failed to cache download: %v\n", err)
		} else {
			logger.Debug("saved to download cache")
		}
	}

	logger.Debug("download_file completed successfully", "dest", dest)
	fmt.Printf("   ✓ Downloaded successfully\n")
	return nil
}

// downloadFileHTTP performs the actual HTTP download with context for cancellation
// SECURITY: Enforces HTTPS for all downloads to prevent MITM attacks
func downloadFileHTTP(ctx context.Context, downloadURL, destPath string) error {
	// SECURITY: Enforce HTTPS for all downloads
	if !strings.HasPrefix(downloadURL, "https://") {
		return fmt.Errorf("download URL must use HTTPS for security, got: %s", downloadURL)
	}

	// Create secure HTTP client with decompression bomb and SSRF protection
	client := newDownloadHTTPClient()
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

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
		return fmt.Errorf("bad status: %s", resp.Status)
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

	// Copy response body to file with progress display
	if progress.ShouldShowProgress() && resp.ContentLength > 0 {
		pw := progress.NewWriter(out, resp.ContentLength, os.Stdout)
		defer pw.Finish()
		if _, err := io.Copy(pw, resp.Body); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
	} else {
		if _, err := io.Copy(out, resp.Body); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
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
