package actions

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// Preflight validates parameters without side effects.
func (a *DownloadAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	// Validate required URL parameter
	url, hasURL := GetString(params, "url")
	if !hasURL {
		result.AddError("download action requires 'url' parameter")
	}

	// ERROR: Static checksum not supported - use checksum_url or download_file
	if _, hasChecksum := GetString(params, "checksum"); hasChecksum {
		result.AddError("download action does not support static 'checksum'; use 'checksum_url' for dynamic verification or 'download_file' for static URLs")
	}

	// Check for signature verification parameters
	_, hasSigURL := GetString(params, "signature_url")
	_, hasSigKeyURL := GetString(params, "signature_key_url")
	sigFingerprint, hasSigFingerprint := GetString(params, "signature_key_fingerprint")
	hasAnySigParam := hasSigURL || hasSigKeyURL || hasSigFingerprint
	hasAllSigParams := hasSigURL && hasSigKeyURL && hasSigFingerprint

	// ERROR: Partial signature params - must provide all three
	if hasAnySigParam && !hasAllSigParams {
		var missing []string
		if !hasSigURL {
			missing = append(missing, "signature_url")
		}
		if !hasSigKeyURL {
			missing = append(missing, "signature_key_url")
		}
		if !hasSigFingerprint {
			missing = append(missing, "signature_key_fingerprint")
		}
		result.AddError("incomplete signature verification: missing " + strings.Join(missing, ", "))
	}

	// ERROR: signature_url and checksum_url are mutually exclusive
	_, hasChecksumURL := GetString(params, "checksum_url")
	if hasSigURL && hasChecksumURL {
		result.AddError("signature_url and checksum_url are mutually exclusive; use one verification method")
	}

	// ERROR: Invalid fingerprint format
	if hasSigFingerprint {
		if err := ValidateFingerprint(sigFingerprint); err != nil {
			result.AddError(err.Error())
		}
	}

	// WARNING: Missing verification (checksum_url or signature_url)
	if !hasChecksumURL && !hasSigURL {
		if _, hasSkipReason := GetString(params, "skip_verification_reason"); !hasSkipReason {
			result.AddWarning("no upstream verification (checksum_url or signature_url); integrity relies on plan-time computation")
		}
	}

	// ERROR: URL without variables - should use download_file instead
	if hasURL && url != "" && !strings.Contains(url, "{") {
		result.AddError("download URL contains no variables; use 'download_file' action for static URLs")
	}

	// WARNING: Signature params but no placeholders in URL
	if hasSigURL && hasURL && url != "" && !strings.Contains(url, "{") {
		result.AddWarning("signature_url provided but URL has no placeholders; consider using download_file instead")
	}

	// WARNING: Unused os_mapping
	if _, hasOSMapping := GetMapStringString(params, "os_mapping"); hasOSMapping {
		if !containsPlaceholder(url, "os") {
			result.AddWarning("os_mapping provided but URL does not contain {os} placeholder; mapping will have no effect")
		}
	}

	// WARNING: Unused arch_mapping
	if _, hasArchMapping := GetMapStringString(params, "arch_mapping"); hasArchMapping {
		if !containsPlaceholder(url, "arch") {
			result.AddWarning("arch_mapping provided but URL does not contain {arch} placeholder; mapping will have no effect")
		}
	}

	return result
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

	// Resolve checksum by downloading file if Downloader is available
	var checksum string
	var size int64

	if ctx.Downloader != nil {
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

// Execute downloads a file with optional checksum verification via URL
//
// Parameters:
//   - url (required): URL to download from (must contain variables like {version})
//   - dest (optional): Destination filename (defaults to basename of URL)
//   - checksum_url (optional): URL to checksum file for verification
//   - checksum_algo (optional): Hash algorithm (sha256, sha512), defaults to sha256
//   - os_mapping (optional): Map Go GOOS to URL patterns (e.g., {darwin: "macos"})
//   - arch_mapping (optional): Map Go GOARCH to URL patterns (e.g., {amd64: "x64"})
//
// Note: This action does not support inline checksum parameter. Use download_file
// action for static URLs with inline checksums.
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

	// Get checksum algorithm for cache validation
	checksumAlgo, _ := GetString(params, "checksum_algo")
	if checksumAlgo == "" {
		checksumAlgo = "sha256"
	}

	// Check download cache if available
	var cache *DownloadCache
	if ctx.DownloadCacheDir != "" {
		cache = NewDownloadCache(ctx.DownloadCacheDir)
		logger.Debug("checking download cache", "cacheDir", ctx.DownloadCacheDir)
		// download action does not support inline checksum, pass empty string
		found, err := cache.Check(url, destPath, "", checksumAlgo)
		if err != nil {
			// Log warning but continue with download
			logger.Warn("cache check failed", "error", err)
			fmt.Printf("   Warning: cache check failed: %v\n", err)
		} else if found {
			logger.Debug("cache hit", "dest", dest)
			fmt.Printf("   Using cached: %s\n", dest)
			// For cached files, verify checksum via URL if available
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

	// Verify PGP signature if provided
	if err := a.verifySignature(ctx.Context, ctx, params, destPath, vars); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	// Save to cache if available (no inline checksum for download action)
	if cache != nil {
		if err := cache.Save(url, destPath, ""); err != nil {
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

// downloadFile performs the actual HTTP download with context for cancellation.
// Implements retry logic with exponential backoff for transient errors (403, 429, 5xx).
// SECURITY: Enforces HTTPS for all downloads to prevent MITM attacks
func (a *DownloadAction) downloadFile(ctx context.Context, url, destPath string) error {
	// SECURITY: Enforce HTTPS for all downloads
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("download URL must use HTTPS for security, got: %s", url)
	}

	const maxRetries = 3
	baseDelay := time.Second

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := baseDelay * time.Duration(1<<(attempt-1))
			fmt.Printf("   Retry %d/%d after %v...\n", attempt, maxRetries, delay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := a.doDownloadFile(ctx, url, destPath)
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
		// unless context is cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return fmt.Errorf("download failed after %d retries: %w", maxRetries, lastErr)
}

// doDownloadFile performs a single HTTP download attempt
func (a *DownloadAction) doDownloadFile(ctx context.Context, url, destPath string) error {
	// Create secure HTTP client with decompression bomb and SSRF protection
	client := newDownloadHTTPClient()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent to avoid 403 from servers that block default Go HTTP client
	req.Header.Set("User-Agent", httputil.DefaultUserAgent)

	// Defense in depth: Explicitly request uncompressed response
	req.Header.Set("Accept-Encoding", "identity")

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

// verifyChecksum verifies the downloaded file's checksum using checksum_url
func (a *DownloadAction) verifyChecksum(ctx context.Context, execCtx *ExecutionContext, params map[string]interface{}, filePath string, vars map[string]string) error {
	// Check if checksum verification is requested via URL
	checksumURL, hasChecksumURL := GetString(params, "checksum_url")

	if !hasChecksumURL {
		// No checksum verification requested
		return nil
	}

	// Get checksum algorithm (default to sha256)
	algo, _ := GetString(params, "checksum_algo")
	if algo == "" {
		algo = "sha256"
	}

	// Download checksum file with context for cancellation
	checksumURL = ExpandVars(checksumURL, vars)
	checksumPath := filepath.Join(execCtx.WorkDir, "checksum.tmp")

	fmt.Printf("   Downloading checksum: %s\n", checksumURL)
	if err := a.downloadFile(ctx, checksumURL, checksumPath); err != nil {
		return fmt.Errorf("failed to download checksum: %w", err)
	}

	// Read checksum from file (pass target filename for multi-line checksum files)
	targetFilename := filepath.Base(filePath)
	expectedChecksum, err := ReadChecksumFile(checksumPath, targetFilename)
	if err != nil {
		return err
	}

	// Clean up checksum file
	os.Remove(checksumPath)

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

// verifySignature verifies the downloaded file using PGP signature verification.
func (a *DownloadAction) verifySignature(ctx context.Context, execCtx *ExecutionContext, params map[string]interface{}, filePath string, vars map[string]string) error {
	// Check if signature verification is requested
	signatureURLPattern, hasSigURL := GetString(params, "signature_url")
	if !hasSigURL {
		return nil
	}

	// Get required signature parameters
	keyURLPattern, hasKeyURL := GetString(params, "signature_key_url")
	fingerprint, hasFingerprint := GetString(params, "signature_key_fingerprint")

	if !hasKeyURL || !hasFingerprint {
		return fmt.Errorf("signature verification requires signature_key_url and signature_key_fingerprint")
	}

	// Normalize fingerprint
	fingerprint, err := ParseFingerprint(fingerprint)
	if err != nil {
		return err
	}

	// Expand variables in URLs
	signatureURL := ExpandVars(signatureURLPattern, vars)
	keyURL := ExpandVars(keyURLPattern, vars)

	fmt.Printf("   Verifying PGP signature...\n")
	fmt.Printf("   Signature: %s\n", signatureURL)
	fmt.Printf("   Key: %s\n", keyURL)
	fmt.Printf("   Fingerprint: %s\n", FormatFingerprint(fingerprint))

	// Fetch signature
	signatureData, err := FetchSignature(ctx, signatureURL)
	if err != nil {
		return fmt.Errorf("failed to fetch signature: %w", err)
	}

	// Get key from cache or fetch
	keyCacheDir := execCtx.KeyCacheDir
	if keyCacheDir == "" {
		// Fall back to a subdirectory in the work dir for testing
		keyCacheDir = filepath.Join(execCtx.WorkDir, ".keys")
	}

	keyCache := NewPGPKeyCache(keyCacheDir)
	key, err := keyCache.Get(ctx, fingerprint, keyURL)
	if err != nil {
		return fmt.Errorf("failed to get signing key: %w", err)
	}

	// Verify signature
	if err := VerifyPGPSignature(ctx, filePath, signatureData, key); err != nil {
		return err
	}

	fmt.Printf("   ✓ PGP signature verified\n")
	return nil
}
