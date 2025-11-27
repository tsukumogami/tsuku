package actions

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DownloadAction implements file downloading with checksum verification
type DownloadAction struct{}

// Name returns the action name
func (a *DownloadAction) Name() string {
	return "download"
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
	vars := GetStandardVars(ctx.Version, ctx.InstallDir, ctx.WorkDir)
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

	fmt.Printf("   Downloading: %s\n", url)
	fmt.Printf("   Destination: %s\n", dest)

	// Download file
	if err := a.downloadFile(url, destPath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Verify checksum if provided
	if err := a.verifyChecksum(ctx, params, destPath, vars); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	fmt.Printf("   ✓ Downloaded successfully\n")
	return nil
}

// downloadFile performs the actual HTTP download
// SECURITY: Enforces HTTPS for all downloads to prevent MITM attacks
func (a *DownloadAction) downloadFile(url, destPath string) error {
	// SECURITY: Enforce HTTPS for all downloads
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("download URL must use HTTPS for security, got: %s", url)
	}

	// Create HTTP client with redirect security check
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// SECURITY: Prevent redirect downgrade attacks (HTTPS -> HTTP)
			if req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to non-HTTPS URL is not allowed: %s", req.URL)
			}
			// Limit redirect depth
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
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

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Copy response body to file
	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// verifyChecksum verifies the downloaded file's checksum
func (a *DownloadAction) verifyChecksum(ctx *ExecutionContext, params map[string]interface{}, filePath string, vars map[string]string) error {
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
		// Download checksum file
		checksumURL = ExpandVars(checksumURL, vars)
		checksumPath := filepath.Join(ctx.WorkDir, "checksum.tmp")

		fmt.Printf("   Downloading checksum: %s\n", checksumURL)
		if err := a.downloadFile(checksumURL, checksumPath); err != nil {
			return fmt.Errorf("failed to download checksum: %w", err)
		}

		// Read checksum from file
		checksum, err := ReadChecksumFile(checksumPath)
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
