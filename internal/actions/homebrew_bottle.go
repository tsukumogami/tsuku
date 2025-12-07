package actions

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// HomebrewBottleAction downloads and extracts Homebrew bottles from GHCR
type HomebrewBottleAction struct{}

// Name returns the action name
func (a *HomebrewBottleAction) Name() string { return "homebrew_bottle" }

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
func (a *HomebrewBottleAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get formula name (required)
	formula, ok := GetString(params, "formula")
	if !ok {
		return fmt.Errorf("homebrew_bottle action requires 'formula' parameter")
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

	// Step 5: Relocate placeholders
	// Determine install path for placeholder replacement
	installPath := ctx.ToolInstallDir
	if installPath == "" {
		installPath = ctx.InstallDir
	}

	// Relocate placeholders in files
	// - Text files: Direct replacement (no length limit)
	// - Binary files: Use patchelf/install_name_tool to reset RPATH
	if err := a.relocatePlaceholders(ctx.WorkDir, installPath); err != nil {
		return fmt.Errorf("failed to relocate placeholders: %w", err)
	}

	fmt.Printf("   Extracted and relocated: %s\n", formula)

	return nil
}

// validateFormulaName ensures the formula name is safe
func (a *HomebrewBottleAction) validateFormulaName(name string) error {
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
func (a *HomebrewBottleAction) getPlatformTag(os, arch string) (string, error) {
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

// getGHCRToken obtains an anonymous token for GHCR access
func (a *HomebrewBottleAction) getGHCRToken(formula string) (string, error) {
	url := fmt.Sprintf("https://ghcr.io/token?service=ghcr.io&scope=repository:homebrew/core/%s:pull", formula)

	resp, err := http.Get(url)
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
func (a *HomebrewBottleAction) getBlobSHA(formula, version, platformTag, token string) (string, error) {
	// Query the manifest index
	url := fmt.Sprintf("https://ghcr.io/v2/homebrew/core/%s/manifests/%s", formula, version)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json")

	resp, err := http.DefaultClient.Do(req)
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
func (a *HomebrewBottleAction) downloadBottle(formula, blobSHA, token, destPath string) error {
	url := fmt.Sprintf("https://ghcr.io/v2/homebrew/core/%s/blobs/sha256:%s", formula, blobSHA)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
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
func (a *HomebrewBottleAction) verifySHA256(filePath, expectedSHA string) error {
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

// relocatePlaceholders replaces Homebrew placeholders in all files
// For text files: direct replacement with install path
// For binary files: use patchelf/install_name_tool to reset RPATH
func (a *HomebrewBottleAction) relocatePlaceholders(dir, installPath string) error {
	replacement := []byte(installPath)

	// Collect binaries that need RPATH fixup
	var binariesToFix []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and symlinks
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		// Check if file contains any placeholder
		hasPlaceholder := false
		for _, placeholder := range homebrewPlaceholders {
			if bytes.Contains(content, placeholder) {
				hasPlaceholder = true
				break
			}
		}

		if !hasPlaceholder {
			return nil
		}

		// Determine if binary or text file
		isBinary := a.isBinaryFile(content)

		if isBinary {
			// Binary files: collect for RPATH fixup using patchelf/install_name_tool
			binariesToFix = append(binariesToFix, path)
		} else {
			// Text files: simple replacement with install path
			newContent := content
			for _, placeholder := range homebrewPlaceholders {
				newContent = bytes.ReplaceAll(newContent, placeholder, replacement)
			}

			// Homebrew bottles often have read-only files; make writable before writing
			originalMode := info.Mode()
			if originalMode&0200 == 0 {
				if err := os.Chmod(path, originalMode|0200); err != nil {
					return fmt.Errorf("failed to make %s writable: %w", path, err)
				}
			}

			if err := os.WriteFile(path, newContent, originalMode); err != nil {
				return fmt.Errorf("failed to write %s: %w", path, err)
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Fix RPATH on binary files using patchelf/install_name_tool
	for _, binaryPath := range binariesToFix {
		if err := a.fixBinaryRpath(binaryPath, installPath); err != nil {
			return fmt.Errorf("failed to fix RPATH for %s: %w", binaryPath, err)
		}
	}

	return nil
}

// fixBinaryRpath uses patchelf or install_name_tool to set a proper RPATH
// This replaces the Homebrew placeholder RPATH with a working path
func (a *HomebrewBottleAction) fixBinaryRpath(binaryPath, installPath string) error {
	// Detect binary format
	f, err := os.Open(binaryPath)
	if err != nil {
		return err
	}

	magic := make([]byte, 4)
	_, err = f.Read(magic)
	f.Close()
	if err != nil {
		return err
	}

	// Check if it's an ELF binary
	if bytes.Equal(magic, []byte{0x7f, 'E', 'L', 'F'}) {
		return a.fixElfRpath(binaryPath, installPath)
	}

	// Check if it's a Mach-O binary
	if bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xce}) || // 32-bit big-endian
		bytes.Equal(magic, []byte{0xce, 0xfa, 0xed, 0xfe}) || // 32-bit little-endian
		bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xcf}) || // 64-bit big-endian
		bytes.Equal(magic, []byte{0xcf, 0xfa, 0xed, 0xfe}) || // 64-bit little-endian
		bytes.Equal(magic, []byte{0xca, 0xfe, 0xba, 0xbe}) || // Fat binary big-endian
		bytes.Equal(magic, []byte{0xbe, 0xba, 0xfe, 0xca}) { // Fat binary little-endian
		return a.fixMachoRpath(binaryPath, installPath)
	}

	// Not a recognized binary format, skip silently
	return nil
}

// fixElfRpath uses patchelf to set RPATH on Linux ELF binaries
func (a *HomebrewBottleAction) fixElfRpath(binaryPath, installPath string) error {
	patchelf, err := exec.LookPath("patchelf")
	if err != nil {
		// patchelf not available - try to proceed without it
		// The binary may still work if its dependencies are system libraries
		fmt.Printf("   Warning: patchelf not found, skipping RPATH fix for %s\n", filepath.Base(binaryPath))
		return nil
	}

	// Homebrew bottles often have read-only files; make writable before patching
	info, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to stat binary: %w", err)
	}
	originalMode := info.Mode()
	if originalMode&0200 == 0 {
		if err := os.Chmod(binaryPath, originalMode|0200); err != nil {
			return fmt.Errorf("failed to make binary writable: %w", err)
		}
		// Restore original mode after patching (best-effort cleanup)
		defer func() { _ = os.Chmod(binaryPath, originalMode) }()
	}

	// Remove existing RPATH first (contains placeholders)
	removeCmd := exec.Command(patchelf, "--remove-rpath", binaryPath)
	if output, err := removeCmd.CombinedOutput(); err != nil {
		// Some binaries might not have RPATH, which is fine
		if !strings.Contains(string(output), "cannot find") {
			// Log but continue
			fmt.Printf("   Note: Could not remove existing RPATH from %s\n", filepath.Base(binaryPath))
		}
	}

	// For shared libraries, set RPATH to $ORIGIN so they can find sibling libraries
	// For executables, RPATH would typically be $ORIGIN/../lib
	// Since Homebrew bottles are libraries, use $ORIGIN
	newRpath := "$ORIGIN"

	// Check if there's a lib subdirectory (common pattern)
	libDir := filepath.Join(filepath.Dir(binaryPath), "lib")
	if _, err := os.Stat(libDir); err == nil {
		// Binary is not in lib/, might need to point to lib/
		relPath, _ := filepath.Rel(filepath.Dir(binaryPath), libDir)
		if relPath != "" && relPath != "." {
			newRpath = "$ORIGIN/" + relPath
		}
	}

	setCmd := exec.Command(patchelf, "--force-rpath", "--set-rpath", newRpath, binaryPath)
	if output, err := setCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("patchelf --set-rpath failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return nil
}

// fixMachoRpath uses install_name_tool to fix RPATH on macOS Mach-O binaries
func (a *HomebrewBottleAction) fixMachoRpath(binaryPath, installPath string) error {
	installNameTool, err := exec.LookPath("install_name_tool")
	if err != nil {
		fmt.Printf("   Warning: install_name_tool not found, skipping RPATH fix for %s\n", filepath.Base(binaryPath))
		return nil
	}

	otool, err := exec.LookPath("otool")
	if err != nil {
		fmt.Printf("   Warning: otool not found, skipping RPATH fix for %s\n", filepath.Base(binaryPath))
		return nil
	}

	// Homebrew bottles often have read-only files; make writable before patching
	info, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to stat binary: %w", err)
	}
	originalMode := info.Mode()
	if originalMode&0200 == 0 {
		if err := os.Chmod(binaryPath, originalMode|0200); err != nil {
			return fmt.Errorf("failed to make binary writable: %w", err)
		}
		// Restore original mode after patching (best-effort cleanup)
		defer func() { _ = os.Chmod(binaryPath, originalMode) }()
	}

	// Get existing rpaths that contain placeholders
	otoolCmd := exec.Command(otool, "-l", binaryPath)
	output, err := otoolCmd.Output()
	if err != nil {
		return fmt.Errorf("otool failed: %w", err)
	}

	// Parse and delete rpaths containing HOMEBREW placeholders
	lines := strings.Split(string(output), "\n")
	inRpathSection := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "cmd LC_RPATH" {
			inRpathSection = true
			continue
		}
		if inRpathSection && strings.HasPrefix(line, "path ") {
			pathLine := strings.TrimPrefix(line, "path ")
			if idx := strings.Index(pathLine, " (offset"); idx != -1 {
				pathLine = pathLine[:idx]
			}
			// Delete if it contains placeholder
			if strings.Contains(pathLine, "HOMEBREW") {
				deleteCmd := exec.Command(installNameTool, "-delete_rpath", pathLine, binaryPath)
				_ = deleteCmd.Run() // Ignore errors
			}
			inRpathSection = false
		}
	}

	// Add new RPATH
	newRpath := "@loader_path"
	addCmd := exec.Command(installNameTool, "-add_rpath", newRpath, binaryPath)
	if output, err := addCmd.CombinedOutput(); err != nil {
		// Ignore "would duplicate" errors
		if !strings.Contains(string(output), "would duplicate") {
			return fmt.Errorf("install_name_tool -add_rpath failed: %s: %w", strings.TrimSpace(string(output)), err)
		}
	}

	// Re-sign the binary (required on Apple Silicon)
	if runtime.GOARCH == "arm64" {
		codesign, err := exec.LookPath("codesign")
		if err == nil {
			signCmd := exec.Command(codesign, "-f", "-s", "-", binaryPath)
			_ = signCmd.Run() // Best effort
		}
	}

	return nil
}

// isBinaryFile detects if content is binary (contains null bytes in first 8KB)
func (a *HomebrewBottleAction) isBinaryFile(content []byte) bool {
	// Check first 8KB for null bytes
	checkLen := 8192
	if len(content) < checkLen {
		checkLen = len(content)
	}

	for i := 0; i < checkLen; i++ {
		if content[i] == 0 {
			return true
		}
	}

	return false
}

// GetCurrentPlatformTag returns the platform tag for the current runtime
// This is useful for testing and standalone usage
func GetCurrentPlatformTag() (string, error) {
	action := &HomebrewBottleAction{}
	return action.getPlatformTag(runtime.GOOS, runtime.GOARCH)
}
