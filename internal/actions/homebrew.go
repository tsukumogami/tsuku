package actions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ghcrHTTPClient returns an HTTP client with appropriate timeouts for GHCR requests.
func ghcrHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// HomebrewAction downloads and extracts Homebrew bottles from GHCR
type HomebrewAction struct{ BaseAction }

// IsDeterministic returns true because homebrew downloads with checksums.
func (HomebrewAction) IsDeterministic() bool { return true }

// RequiresNetwork returns true because homebrew decomposes into download_file
// which fetches bottles from GHCR.
func (HomebrewAction) RequiresNetwork() bool { return true }

// Dependencies returns patchelf as a Linux-only install-time dependency.
// The homebrew action decomposes to homebrew_relocate which requires patchelf for RPATH fixup on Linux.
// macOS uses install_name_tool (system-provided) instead.
// TODO(#644): Remove this method once composite actions automatically aggregate primitive dependencies.
// This is a workaround because dependency resolution happens before decomposition.
func (HomebrewAction) Dependencies() ActionDeps {
	return ActionDeps{
		LinuxInstallTime: []string{"patchelf"},
	}
}

// Name returns the action name
func (a *HomebrewAction) Name() string { return "homebrew" }

// Preflight validates parameters without side effects.
func (a *HomebrewAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}
	if _, ok := GetString(params, "formula"); !ok {
		result.AddError("homebrew action requires 'formula' parameter")
	}
	return result
}

// Execute downloads a Homebrew bottle and extracts it to the install directory
//
// Parameters:
//   - formula (required): Homebrew formula name (e.g., "libyaml")
//
// The action:
// 1. Obtains anonymous GHCR token
// 2. Queries GHCR manifest for platform-specific blob SHA
// 3. Downloads and verifies bottle SHA256
// 4. Extracts tarball to install directory
// 5. Relocates @@HOMEBREW_PREFIX@@ placeholders
func (a *HomebrewAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get formula name (required)
	formula, ok := GetString(params, "formula")
	if !ok {
		return fmt.Errorf("homebrew action requires 'formula' parameter")
	}

	// Validate formula name for security
	if err := a.validateFormulaName(formula); err != nil {
		return err
	}

	// Determine platform tag for bottle selection
	platformTag, err := a.getPlatformTag(ctx.OS, ctx.Arch)
	if err != nil {
		return fmt.Errorf("unsupported platform: %w", err)
	}

	fmt.Printf("   Fetching Homebrew bottle: %s (%s)\n", formula, platformTag)

	// Step 1: Get anonymous GHCR token
	token, err := a.getGHCRToken(formula)
	if err != nil {
		return fmt.Errorf("failed to get GHCR token: %w", err)
	}

	// Step 2: Get manifest and find platform-specific blob SHA
	blobSHA, err := a.getBlobSHA(formula, ctx.VersionTag, platformTag, token)
	if err != nil {
		return fmt.Errorf("failed to get blob SHA: %w", err)
	}

	// Step 3: Download bottle
	bottlePath := filepath.Join(ctx.WorkDir, fmt.Sprintf("%s.tar.gz", formula))
	if err := a.downloadBottle(formula, blobSHA, token, bottlePath); err != nil {
		return fmt.Errorf("failed to download bottle: %w", err)
	}

	// Verify SHA256
	if err := a.verifySHA256(bottlePath, blobSHA); err != nil {
		return fmt.Errorf("SHA256 verification failed: %w", err)
	}

	fmt.Printf("   SHA256 verified: %s\n", blobSHA[:16]+"...")

	// Step 4: Extract bottle
	extractAction := &ExtractAction{}
	extractParams := map[string]interface{}{
		"archive":    filepath.Base(bottlePath),
		"format":     "tar.gz",
		"strip_dirs": 2, // Homebrew bottles have formula/version/ prefix
	}

	if err := extractAction.Execute(ctx, extractParams); err != nil {
		return fmt.Errorf("failed to extract bottle: %w", err)
	}

	// Step 5: Relocate placeholders using homebrew_relocate action
	relocateAction := &HomebrewRelocateAction{}
	relocateParams := map[string]interface{}{
		"formula": formula,
	}

	if err := relocateAction.Execute(ctx, relocateParams); err != nil {
		return fmt.Errorf("failed to relocate placeholders: %w", err)
	}

	fmt.Printf("   Extracted and relocated: %s\n", formula)

	return nil
}

// validateFormulaName ensures the formula name is safe
func (a *HomebrewAction) validateFormulaName(name string) error {
	if name == "" {
		return fmt.Errorf("formula name cannot be empty")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("formula name cannot contain '..': %s", name)
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("formula name cannot contain path separators: %s", name)
	}
	// Only allow alphanumeric, hyphen, underscore, @, and . (for versioned formulas like python@3.12)
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '@' || c == '.') {
			return fmt.Errorf("formula name contains invalid character '%c': %s", c, name)
		}
	}
	return nil
}

// getPlatformTag returns the Homebrew platform tag for the current OS/arch
func (a *HomebrewAction) getPlatformTag(os, arch string) (string, error) {
	// Homebrew uses specific platform tags in manifests
	// Format: {os}.{codename/version}
	switch {
	case os == "darwin" && arch == "arm64":
		return "arm64_sonoma", nil
	case os == "darwin" && arch == "amd64":
		return "sonoma", nil
	case os == "linux" && arch == "arm64":
		return "arm64_linux", nil
	case os == "linux" && arch == "amd64":
		return "x86_64_linux", nil
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", os, arch)
	}
}

// ghcrTokenResponse represents the GHCR token API response
type ghcrTokenResponse struct {
	Token string `json:"token"`
}

// formulaToGHCRPath converts a formula name to the GHCR repository path.
// Homebrew uses @ for versioned formulas (e.g., openssl@3) but GHCR uses / (e.g., openssl/3).
func formulaToGHCRPath(formula string) string {
	return strings.ReplaceAll(formula, "@", "/")
}

// getGHCRToken obtains an anonymous token for GHCR access
func (a *HomebrewAction) getGHCRToken(formula string) (string, error) {
	ghcrPath := formulaToGHCRPath(formula)
	url := fmt.Sprintf("https://ghcr.io/token?service=ghcr.io&scope=repository:homebrew/core/%s:pull", ghcrPath)

	resp, err := ghcrHTTPClient().Get(url)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token request returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp ghcrTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.Token == "" {
		return "", fmt.Errorf("empty token in response")
	}

	return tokenResp.Token, nil
}

// ghcrManifest represents the GHCR manifest structure
type ghcrManifest struct {
	Manifests []ghcrManifestEntry `json:"manifests"`
}

// ghcrManifestEntry represents a single manifest entry
type ghcrManifestEntry struct {
	Digest      string            `json:"digest"`
	Platform    ghcrPlatform      `json:"platform"`
	Annotations map[string]string `json:"annotations"`
}

// ghcrPlatform represents platform info in manifest
type ghcrPlatform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

// getBlobSHA queries the GHCR manifest to find the platform-specific blob SHA
func (a *HomebrewAction) getBlobSHA(formula, version, platformTag, token string) (string, error) {
	// Query the manifest index
	ghcrPath := formulaToGHCRPath(formula)
	url := fmt.Sprintf("https://ghcr.io/v2/homebrew/core/%s/manifests/%s", ghcrPath, version)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json")

	resp, err := ghcrHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("manifest request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("manifest request returned %d: %s", resp.StatusCode, string(body))
	}

	var manifest ghcrManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return "", fmt.Errorf("failed to parse manifest: %w", err)
	}

	// The expected ref name format is "{version}.{platform_tag}"
	// e.g., "0.2.5.arm64_sonoma" or "0.2.5.x86_64_linux"
	expectedRefName := fmt.Sprintf("%s.%s", version, platformTag)

	// Find the entry matching our platform tag
	for _, entry := range manifest.Manifests {
		// Check org.opencontainers.image.ref.name annotation for the bottle tag
		// Format: "{version}.{platform_tag}" e.g., "0.2.5.arm64_sonoma"
		if refName, ok := entry.Annotations["org.opencontainers.image.ref.name"]; ok {
			if refName == expectedRefName {
				// Return the blob digest from sh.brew.bottle.digest annotation
				if digest, ok := entry.Annotations["sh.brew.bottle.digest"]; ok {
					// Digest format: sha256:xxx or just the hash
					if strings.HasPrefix(digest, "sha256:") {
						return strings.TrimPrefix(digest, "sha256:"), nil
					}
					return digest, nil
				}
				// Fall back to manifest digest if no specific bottle digest
				if strings.HasPrefix(entry.Digest, "sha256:") {
					return strings.TrimPrefix(entry.Digest, "sha256:"), nil
				}
				return entry.Digest, nil
			}
		}
	}

	return "", fmt.Errorf("no bottle found for platform tag: %s (expected ref: %s)", platformTag, expectedRefName)
}

// downloadBottle downloads a bottle blob from GHCR
func (a *HomebrewAction) downloadBottle(formula, blobSHA, token, destPath string) error {
	ghcrPath := formulaToGHCRPath(formula)
	url := fmt.Sprintf("https://ghcr.io/v2/homebrew/core/%s/blobs/sha256:%s", ghcrPath, blobSHA)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := ghcrHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download request returned %d: %s", resp.StatusCode, string(body))
	}

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Copy response to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// verifySHA256 verifies the SHA256 checksum of a file
func (a *HomebrewAction) verifySHA256(filePath, expectedSHA string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("failed to hash file: %w", err)
	}

	actualSHA := hex.EncodeToString(hasher.Sum(nil))
	if actualSHA != expectedSHA {
		return fmt.Errorf("SHA256 mismatch: expected %s, got %s", expectedSHA, actualSHA)
	}

	return nil
}

// homebrewPlaceholders contains all Homebrew placeholders that need relocation
var homebrewPlaceholders = [][]byte{
	[]byte("@@HOMEBREW_PREFIX@@"),
	[]byte("@@HOMEBREW_CELLAR@@"),
}

// GetCurrentPlatformTag returns the platform tag for the current runtime
// This is useful for testing and standalone usage
func GetCurrentPlatformTag() (string, error) {
	action := &HomebrewAction{}
	return action.getPlatformTag(runtime.GOOS, runtime.GOARCH)
}

// Decompose resolves the Homebrew bottle metadata and returns primitive steps.
// This enables deterministic plan generation by querying GHCR at evaluation time
// and computing checksums before execution.
func (a *HomebrewAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
	// Get formula name (required)
	formula, ok := GetString(params, "formula")
	if !ok {
		return nil, fmt.Errorf("homebrew action requires 'formula' parameter")
	}

	// Validate formula name for security
	if err := a.validateFormulaName(formula); err != nil {
		return nil, err
	}

	// Determine platform tag for bottle selection
	platformTag, err := a.getPlatformTag(ctx.OS, ctx.Arch)
	if err != nil {
		return nil, fmt.Errorf("unsupported platform: %w", err)
	}

	// Get anonymous GHCR token
	token, err := a.getGHCRToken(formula)
	if err != nil {
		return nil, fmt.Errorf("failed to get GHCR token: %w", err)
	}

	// Get manifest and find platform-specific blob SHA
	blobSHA, err := a.getBlobSHA(formula, ctx.VersionTag, platformTag, token)
	if err != nil {
		return nil, fmt.Errorf("failed to get blob SHA: %w", err)
	}

	// Construct the download URL
	ghcrPath := formulaToGHCRPath(formula)
	url := fmt.Sprintf("https://ghcr.io/v2/homebrew/core/%s/blobs/sha256:%s", ghcrPath, blobSHA)
	destFile := fmt.Sprintf("%s.tar.gz", formula)

	// Download the file to verify accessibility and cache it
	var size int64
	if ctx.Downloader != nil {
		apiCtx, cancel := context.WithTimeout(ctx.Context, 60*time.Second)
		defer cancel()

		// Download with authorization header (GHCR requires auth even for public images)
		result, err := a.downloadWithAuth(apiCtx, url, token)
		if err != nil {
			return nil, fmt.Errorf("failed to download bottle for checksum verification: %w", err)
		}

		// Verify the checksum matches what GHCR reported
		if result.Checksum != blobSHA {
			_ = result.Cleanup()
			return nil, fmt.Errorf("checksum mismatch: GHCR reported %s, downloaded file has %s", blobSHA, result.Checksum)
		}

		size = result.Size

		// Save to cache if configured
		if ctx.DownloadCache != nil {
			_ = ctx.DownloadCache.Save(url, result.AssetPath, result.Checksum)
		}
		_ = result.Cleanup()
	}

	// Return primitive steps
	return []Step{
		{
			Action: "download_file",
			Params: map[string]interface{}{
				"url":      url,
				"dest":     destFile,
				"checksum": blobSHA,
			},
			Checksum: blobSHA,
			Size:     size,
		},
		{
			Action: "extract",
			Params: map[string]interface{}{
				"archive":    destFile,
				"format":     "tar.gz",
				"strip_dirs": 2, // Homebrew bottles have formula/version/ prefix
			},
		},
		{
			Action: "homebrew_relocate",
			Params: map[string]interface{}{
				"formula": formula,
			},
		},
	}, nil
}

// downloadWithAuth downloads a file with authorization header.
// This is needed because GHCR requires Bearer token even for public images.
func (a *HomebrewAction) downloadWithAuth(ctx context.Context, url, token string) (*DownloadResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := ghcrHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download request returned %d: %s", resp.StatusCode, string(body))
	}

	// Create temp file
	tmpDir, err := os.MkdirTemp("", "homebrew-bottle-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	tmpFile := filepath.Join(tmpDir, "bottle.tar.gz")
	out, err := os.Create(tmpFile)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	// Hash while downloading
	hasher := sha256.New()
	writer := io.MultiWriter(out, hasher)

	size, err := io.Copy(writer, resp.Body)
	out.Close()
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to download: %w", err)
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))

	return &DownloadResult{
		AssetPath: tmpFile,
		Checksum:  checksum,
		Size:      size,
	}, nil
}
