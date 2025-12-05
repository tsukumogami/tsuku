package actions

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/tsukumogami/tsuku/internal/progress"
)

// nix-portable version and checksums
// Pin to known-good version - updating requires code change for supply chain security
const (
	nixPortableVersion = "v012"
)

// nixPortableChecksums maps architecture to SHA256 checksum
var nixPortableChecksums = map[string]string{
	"amd64": "b409c55904c909ac3aeda3fb1253319f86a89ddd1ba31a5dec33d4a06414c72a",
	"arm64": "af41d8defdb9fa17ee361220ee05a0c758d3e6231384a3f969a314f9133744ea",
}

// nixPortableArchNames maps Go GOARCH to nix-portable release names
var nixPortableArchNames = map[string]string{
	"amd64": "x86_64",
	"arm64": "aarch64",
}

// getNixInternalDir returns the path to tsuku's internal nix directory
func getNixInternalDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".tsuku", ".nix-internal"), nil
}

// ResolveNixPortable finds tsuku's internal nix-portable binary.
// Returns empty string if not bootstrapped yet.
func ResolveNixPortable() string {
	internalDir, err := getNixInternalDir()
	if err != nil {
		return ""
	}

	nixPath := filepath.Join(internalDir, "nix-portable")
	if info, err := os.Stat(nixPath); err == nil && info.Mode()&0111 != 0 {
		return nixPath
	}
	return ""
}

// EnsureNixPortable ensures nix-portable is available in the internal directory.
// It downloads and installs nix-portable if not present.
// Uses file locking to prevent race conditions from concurrent tsuku processes.
// Returns the path to the nix-portable binary.
//
// Security:
//   - Only supports Linux (nix-portable limitation)
//   - Uses hardcoded SHA256 checksums for supply chain security
//   - Uses file locking to prevent TOCTOU race conditions
//
// Deprecated: Use EnsureNixPortableWithContext for cancellation support.
func EnsureNixPortable() (string, error) {
	return EnsureNixPortableWithContext(context.Background())
}

// EnsureNixPortableWithContext ensures nix-portable is available in the internal directory.
// It downloads and installs nix-portable if not present.
// Uses file locking to prevent race conditions from concurrent tsuku processes.
// Returns the path to the nix-portable binary.
// The context parameter enables cancellation of the download operation.
//
// Security:
//   - Only supports Linux (nix-portable limitation)
//   - Uses hardcoded SHA256 checksums for supply chain security
//   - Uses file locking to prevent TOCTOU race conditions
func EnsureNixPortableWithContext(ctx context.Context) (string, error) {
	// Check platform - nix-portable only supports Linux
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("nix_install action only supports Linux (nix-portable does not support %s)", runtime.GOOS)
	}

	// Check architecture
	archName, ok := nixPortableArchNames[runtime.GOARCH]
	if !ok {
		return "", fmt.Errorf("nix_install action does not support architecture: %s", runtime.GOARCH)
	}

	expectedChecksum, ok := nixPortableChecksums[runtime.GOARCH]
	if !ok {
		return "", fmt.Errorf("no checksum available for architecture: %s", runtime.GOARCH)
	}

	internalDir, err := getNixInternalDir()
	if err != nil {
		return "", err
	}

	// Create internal directory if it doesn't exist
	if err := os.MkdirAll(internalDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create internal nix directory: %w", err)
	}

	// Acquire exclusive file lock to prevent race conditions
	lockPath := filepath.Join(internalDir, ".lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to open lock file: %w", err)
	}
	defer lockFile.Close()

	// Acquire exclusive lock (blocking)
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return "", fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) // Unlock error is non-critical during cleanup
	}()

	// Check if nix-portable already exists (now that we have the lock)
	nixPortablePath := filepath.Join(internalDir, "nix-portable")
	if path := ResolveNixPortable(); path != "" {
		// Verify version matches
		versionPath := filepath.Join(internalDir, "version")
		if versionData, err := os.ReadFile(versionPath); err == nil {
			if string(versionData) == nixPortableVersion {
				return path, nil
			}
			// Version mismatch - need to re-download
			fmt.Printf("   Upgrading nix-portable from %s to %s\n", string(versionData), nixPortableVersion)
		}
	}

	// Check for existing /nix/store (potential conflicts with system Nix)
	if _, err := os.Stat("/nix/store"); err == nil {
		fmt.Println("   Warning: /nix/store exists. nix-portable may have conflicts with system Nix.")
	}

	// Download nix-portable
	url := fmt.Sprintf("https://github.com/DavHau/nix-portable/releases/download/%s/nix-portable-%s",
		nixPortableVersion, archName)

	fmt.Printf("   Downloading nix-portable %s for %s...\n", nixPortableVersion, archName)
	fmt.Printf("   URL: %s\n", url)

	// Download to temporary file first with context for cancellation
	tmpPath := nixPortablePath + ".tmp"
	if err := downloadFileWithContext(ctx, url, tmpPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to download nix-portable: %w", err)
	}

	// Verify checksum
	fmt.Printf("   Verifying checksum...\n")
	if err := VerifyChecksum(tmpPath, expectedChecksum, "sha256"); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("nix-portable checksum verification failed: %w", err)
	}

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to make nix-portable executable: %w", err)
	}

	// Atomic rename to final location
	if err := os.Rename(tmpPath, nixPortablePath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to install nix-portable: %w", err)
	}

	// Write version file
	versionPath := filepath.Join(internalDir, "version")
	if err := os.WriteFile(versionPath, []byte(nixPortableVersion), 0644); err != nil {
		// Non-fatal - version file is for upgrade detection
		fmt.Printf("   Warning: failed to write version file: %v\n", err)
	}

	fmt.Printf("   nix-portable %s installed successfully\n", nixPortableVersion)
	return nixPortablePath, nil
}

// downloadFileWithContext downloads a file from URL to the specified path with context for cancellation
func downloadFileWithContext(ctx context.Context, url, destPath string) error {
	// Create the file
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Make HTTP request with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Copy response body to file with progress display
	if progress.ShouldShowProgress() && resp.ContentLength > 0 {
		pw := progress.NewWriter(out, resp.ContentLength, os.Stdout)
		defer pw.Finish()
		_, err = io.Copy(pw, resp.Body)
	} else {
		_, err = io.Copy(out, resp.Body)
	}
	return err
}

// GetNixInternalDir returns the path to tsuku's internal nix directory.
// This is exported for use by nix_install action.
func GetNixInternalDir() (string, error) {
	return getNixInternalDir()
}
