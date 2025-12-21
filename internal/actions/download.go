package actions

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/httputil"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/progress"
)

// DownloadAction implements file downloading with checksum verification.
// This is a composite action that decomposes to download_file primitive during
// plan generation.
type DownloadAction struct{ BaseAction }

// IsDeterministic returns true because downloads with checksums produce identical results.
func (DownloadAction) IsDeterministic() bool { return true }

// Name returns the action name
func (a *DownloadAction) Name() string {
	return "download"
}

// Decompose converts the download composite action to a download_file primitive.
// It downloads the file to compute checksum if not provided inline.
func (a *DownloadAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
	// Get URL (required)
	urlPattern, ok := GetString(params, "url")
	if !ok {
		return nil, fmt.Errorf("download action requires 'url' parameter")
	}

	// Apply custom mappings if provided
	osMapping, _ := GetMapStringString(params, "os_mapping")
	archMapping, _ := GetMapStringString(params, "arch_mapping")

	// Build vars for expansion
	vars := map[string]string{
		"version":     ctx.Version,
		"version_tag": ctx.VersionTag,
		"os":          ctx.OS,
		"arch":        ctx.Arch,
	}
	if len(osMapping) > 0 {
		vars["os"] = ApplyMapping(vars["os"], osMapping)
	}
	if len(archMapping) > 0 {
		vars["arch"] = ApplyMapping(vars["arch"], archMapping)
	}

	// Expand variables in URL
	downloadURL := ExpandVars(urlPattern, vars)

	// Get destination filename
	dest, _ := GetString(params, "dest")
	if dest == "" {
		// Default to basename of URL
		dest = filepath.Base(downloadURL)
		// Remove query parameters if present
		if idx := strings.Index(dest, "?"); idx != -1 {
			dest = dest[:idx]
		}
	} else {
		dest = ExpandVars(dest, vars)
	}

	// Get checksum algorithm (default to sha256)
	checksumAlgo, _ := GetString(params, "checksum_algo")
	if checksumAlgo == "" {
		checksumAlgo = "sha256"
	}

	// Resolve checksum
	var checksum string
	var size int64

	// First check inline checksum
	inlineChecksum, hasInline := GetString(params, "checksum")
	if hasInline && inlineChecksum != "" {
		checksum = inlineChecksum
	}

	// If no inline checksum, download file to compute checksum if Downloader is available
	if checksum == "" && ctx.Downloader != nil {
		result, err := ctx.Downloader.Download(ctx.Context, downloadURL)
		if err != nil {
			return nil, fmt.Errorf("failed to download for checksum computation: %w", err)
		}
		checksum = result.Checksum
		size = result.Size
		// Save to cache if configured, then cleanup temp file
		if ctx.DownloadCache != nil {
			_ = ctx.DownloadCache.Save(downloadURL, result.AssetPath, result.Checksum)
		}
		_ = result.Cleanup()
	}

	// Build download_file step
	downloadParams := map[string]interface{}{
		"url":  downloadURL,
		"dest": dest,
	}
	if checksum != "" {
		downloadParams["checksum"] = checksum
		downloadParams["checksum_algo"] = checksumAlgo
	}

	step := Step{
		Action:   "download_file",
		Params:   downloadParams,
		Checksum: checksum,
		Size:     size,
	}

	return []Step{step}, nil
}

// Execute downloads a file with optional checksum verification
//
// Parameters:
//   - url (required): URL to download from
//   - dest (optional): Destination filename (defaults to basename of URL)
//   - checksum_url (optional): URL to checksum file
//   - checksum (optional): Inline checksum value
//   - checksum_algo (optional): Hash algorithm (sha256, sha512), defaults to sha256
//   - os_mapping (optional): Map Go GOOS to URL patterns (e.g., {darwin: "macos"})
//   - arch_mapping (optional): Map Go GOARCH to URL patterns (e.g., {amd64: "x64"})
func (a *DownloadAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get URL (required)
	urlPattern, ok := GetString(params, "url")
	if !ok {
		return fmt.Errorf("download action requires 'url' parameter")
	}

	// Apply custom mappings if provided
	osMapping, _ := GetMapStringString(params, "os_mapping")
	archMapping, _ := GetMapStringString(params, "arch_mapping")

	// Build vars with custom mappings
	vars := GetStandardVars(ctx.Version, ctx.InstallDir, ctx.WorkDir, ctx.LibsDir)
	if len(osMapping) > 0 {
		vars["os"] = ApplyMapping(vars["os"], osMapping)
	}
	if len(archMapping) > 0 {
		vars["arch"] = ApplyMapping(vars["arch"], archMapping)
	}

	// Expand variables in URL
	url := ExpandVars(urlPattern, vars)

	// Get destination filename
	dest, ok := GetString(params, "dest")
	if !ok {
		// Default to basename of URL
		dest = filepath.Base(url)
		// Remove query parameters if present
		if idx := strings.Index(dest, "?"); idx != -1 {
			dest = dest[:idx]
		}
	} else {
		// Expand variables in dest
		dest = ExpandVars(dest, vars)
	}

	destPath := filepath.Join(ctx.WorkDir, dest)

	// Get logger for debug output
	logger := ctx.Log()
	logger.Debug("download action starting",
		"url", log.SanitizeURL(url),
		"dest", dest,
		"destPath", destPath)

	// Get checksum info for cache validation
	inlineChecksum, _ := GetString(params, "checksum")
	checksumAlgo, _ := GetString(params, "checksum_algo")
	if checksumAlgo == "" {
		checksumAlgo = "sha256"
	}

	// Check download cache if available
	var cache *DownloadCache
	if ctx.DownloadCacheDir != "" {
		cache = NewDownloadCache(ctx.DownloadCacheDir)
		logger.Debug("checking download cache", "cacheDir", ctx.DownloadCacheDir)
		found, err := cache.Check(url, destPath, inlineChecksum, checksumAlgo)
		if err != nil {
			// Log warning but continue with download
			logger.Warn("cache check failed", "error", err)
			fmt.Printf("   Warning: cache check failed: %v\n", err)
		} else if found {
			logger.Debug("cache hit", "dest", dest)
			fmt.Printf("   Using cached: %s\n", dest)
			// For cached files, still verify checksum if provided via URL
			// (inline checksum was already verified by cache.Check)
			checksumURL, hasChecksumURL := GetString(params, "checksum_url")
			if hasChecksumURL {
				if err := a.verifyChecksumFromURL(ctx.Context, ctx, checksumURL, destPath, checksumAlgo, vars); err != nil {
					// Cache may be stale, invalidate and re-download
					logger.Debug("cache checksum mismatch, will re-download")
					fmt.Printf("   Cache checksum mismatch, re-downloading...\n")
				} else {
					logger.Debug("restored from cache with valid checksum")
					fmt.Printf("   ✓ Restored from cache\n")
					return nil
				}
			} else {
				logger.Debug("restored from cache (no checksum URL)")
				fmt.Printf("   ✓ Restored from cache\n")
				return nil
			}
		} else {
			logger.Debug("cache miss")
		}
	}

	fmt.Printf("   Downloading: %s\n", url)
	fmt.Printf("   Destination: %s\n", dest)

	// Download file with context for cancellation support
	if err := a.downloadFile(ctx.Context, url, destPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Verify checksum if provided
	if err := a.verifyChecksum(ctx.Context, ctx, params, destPath, vars); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}
	logger.Debug("checksum verification passed", "algo", checksumAlgo)

	// Save to cache if available
	if cache != nil {
		if err := cache.Save(url, destPath, inlineChecksum); err != nil {
			// Log warning but don't fail the download
			logger.Warn("failed to cache download", "error", err)
			fmt.Printf("   Warning: failed to cache download: %v\n", err)
		} else {
			logger.Debug("saved to download cache")
		}
	}

	logger.Debug("download completed successfully", "dest", dest)
	fmt.Printf("   ✓ Downloaded successfully\n")
	return nil
}

// newDownloadHTTPClient creates a secure HTTP client for downloads using the
// shared httputil package for SSRF protection and security hardening.
func newDownloadHTTPClient() *http.Client {
	return httputil.NewSecureClient(httputil.ClientOptions{
		Timeout: config.GetAPITimeout(),
	})
}

// downloadFile performs the actual HTTP download with context for cancellation
// SECURITY: Enforces HTTPS for all downloads to prevent MITM attacks
func (a *DownloadAction) downloadFile(ctx context.Context, url, destPath string) error {
	// SECURITY: Enforce HTTPS for all downloads
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("download URL must use HTTPS for security, got: %s", url)
	}

	// Create secure HTTP client with decompression bomb and SSRF protection
	client := newDownloadHTTPClient()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Defense in depth: Explicitly request uncompressed response
	req.Header.Set("Accept-Encoding", "identity")

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

// verifyChecksum verifies the downloaded file's checksum
func (a *DownloadAction) verifyChecksum(ctx context.Context, execCtx *ExecutionContext, params map[string]interface{}, filePath string, vars map[string]string) error {
	// Check if checksum verification is requested
	checksumURL, hasChecksumURL := GetString(params, "checksum_url")
	inlineChecksum, hasInlineChecksum := GetString(params, "checksum")

	if !hasChecksumURL && !hasInlineChecksum {
		// No checksum verification requested
		return nil
	}

	// Get checksum algorithm (default to sha256)
	algo, _ := GetString(params, "checksum_algo")
	if algo == "" {
		algo = "sha256"
	}

	var expectedChecksum string

	if hasChecksumURL {
		// Download checksum file with context for cancellation
		checksumURL = ExpandVars(checksumURL, vars)
		checksumPath := filepath.Join(execCtx.WorkDir, "checksum.tmp")

		fmt.Printf("   Downloading checksum: %s\n", checksumURL)
		if err := a.downloadFile(ctx, checksumURL, checksumPath); err != nil {
			return fmt.Errorf("failed to download checksum: %w", err)
		}

		// Read checksum from file (pass target filename for multi-line checksum files)
		targetFilename := filepath.Base(filePath)
		checksum, err := ReadChecksumFile(checksumPath, targetFilename)
		if err != nil {
			return err
		}
		expectedChecksum = checksum

		// Clean up checksum file
		os.Remove(checksumPath)
	} else {
		// Use inline checksum
		expectedChecksum = inlineChecksum
	}

	// Verify checksum
	fmt.Printf("   Verifying %s checksum...\n", algo)
	if err := VerifyChecksum(filePath, expectedChecksum, algo); err != nil {
		return err
	}

	fmt.Printf("   ✓ Checksum verified\n")
	return nil
}

// verifyChecksumFromURL downloads a checksum file and verifies the file against it
func (a *DownloadAction) verifyChecksumFromURL(ctx context.Context, execCtx *ExecutionContext, checksumURL, filePath, algo string, vars map[string]string) error {
	// Download checksum file with context for cancellation
	checksumURL = ExpandVars(checksumURL, vars)
	checksumPath := filepath.Join(execCtx.WorkDir, "checksum.tmp")

	if err := a.downloadFile(ctx, checksumURL, checksumPath); err != nil {
		return fmt.Errorf("failed to download checksum: %w", err)
	}

	// Read checksum from file (pass target filename for multi-line checksum files)
	targetFilename := filepath.Base(filePath)
	checksum, err := ReadChecksumFile(checksumPath, targetFilename)
	if err != nil {
		return err
	}

	// Clean up checksum file
	os.Remove(checksumPath)

	// Verify checksum
	if err := VerifyChecksum(filePath, checksum, algo); err != nil {
		return err
	}

	return nil
}
