// Package addon provides download and verification for the tsuku-llm addon binary.
// The addon is downloaded on demand with SHA256 verification at download time
// and before each execution to prevent post-download tampering.
package addon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
)

// AddonManager handles downloading, verifying, and locating the tsuku-llm addon.
type AddonManager struct {
	mu sync.Mutex

	// homeDir is the tsuku home directory ($TSUKU_HOME or ~/.tsuku)
	homeDir string

	// cachedPath is the verified addon path (set after successful EnsureAddon)
	cachedPath string
}

// NewAddonManager creates a new addon manager using the default home directory.
func NewAddonManager() *AddonManager {
	return NewAddonManagerWithHome("")
}

// NewAddonManagerWithHome creates a new addon manager with a custom home directory.
// If homeDir is empty, it uses TSUKU_HOME env var or defaults to ~/.tsuku.
func NewAddonManagerWithHome(homeDir string) *AddonManager {
	if homeDir == "" {
		homeDir = os.Getenv("TSUKU_HOME")
	}
	if homeDir == "" {
		if h, err := os.UserHomeDir(); err == nil {
			homeDir = filepath.Join(h, ".tsuku")
		}
	}

	return &AddonManager{
		homeDir: homeDir,
	}
}

// HomeDir returns the tsuku home directory.
func (m *AddonManager) HomeDir() string {
	return m.homeDir
}

// AddonDir returns the versioned directory for the addon.
// Format: $TSUKU_HOME/tools/tsuku-llm/<version>/
func (m *AddonManager) AddonDir() (string, error) {
	manifest, err := GetManifest()
	if err != nil {
		return "", err
	}
	return filepath.Join(m.homeDir, "tools", "tsuku-llm", manifest.Version), nil
}

// BinaryPath returns the full path to the addon binary.
// Format: $TSUKU_HOME/tools/tsuku-llm/<version>/tsuku-llm
func (m *AddonManager) BinaryPath() (string, error) {
	dir, err := m.AddonDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, BinaryName()), nil
}

// IsInstalled checks if the addon binary exists at the expected path.
// Note: This does not verify the checksum.
func (m *AddonManager) IsInstalled() bool {
	path, err := m.BinaryPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// EnsureAddon ensures the addon is downloaded and verified.
// It returns the path to the verified binary.
//
// This method:
// 1. Gets platform info from the embedded manifest
// 2. If the binary exists, verifies its checksum
// 3. If verification fails or binary is missing, downloads it
// 4. Verifies the downloaded binary
// 5. Returns the path to the verified binary
//
// The method is safe for concurrent calls via mutex protection.
func (m *AddonManager) EnsureAddon(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return cached path if already verified this session
	if m.cachedPath != "" {
		// Re-verify to catch tampering
		if err := m.verifyBinary(m.cachedPath); err == nil {
			return m.cachedPath, nil
		}
		// Verification failed - clear cache and re-download
		m.cachedPath = ""
	}

	// Get platform info
	platformInfo, err := GetCurrentPlatformInfo()
	if err != nil {
		return "", err
	}

	binaryPath, err := m.BinaryPath()
	if err != nil {
		return "", err
	}

	// Check if binary exists
	if _, err := os.Stat(binaryPath); err == nil {
		// Verify existing binary
		if err := m.verifyBinary(binaryPath); err == nil {
			m.cachedPath = binaryPath
			return binaryPath, nil
		}
		// Verification failed - need to re-download
		fmt.Println("   Existing addon failed verification, re-downloading...")
	}

	// Download addon
	if err := m.downloadAddon(ctx, platformInfo, binaryPath); err != nil {
		return "", err
	}

	// Verify downloaded binary
	if err := m.verifyBinary(binaryPath); err != nil {
		// Clean up invalid download
		_ = os.Remove(binaryPath)
		return "", fmt.Errorf("downloaded addon failed verification: %w", err)
	}

	m.cachedPath = binaryPath
	return binaryPath, nil
}

// VerifyBeforeExecution verifies the addon binary checksum before execution.
// This catches post-download tampering. Call this in ServerLifecycle.EnsureRunning().
func (m *AddonManager) VerifyBeforeExecution(binaryPath string) error {
	return m.verifyBinary(binaryPath)
}

// verifyBinary verifies the binary at path against the expected checksum.
func (m *AddonManager) verifyBinary(path string) error {
	platformInfo, err := GetCurrentPlatformInfo()
	if err != nil {
		return err
	}

	return VerifyChecksum(path, platformInfo.SHA256)
}

// downloadAddon downloads the addon binary with proper setup.
func (m *AddonManager) downloadAddon(ctx context.Context, info *PlatformInfo, destPath string) error {
	// Create addon directory
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create addon directory: %w", err)
	}

	// Acquire lock to prevent concurrent downloads
	lockPath := filepath.Join(dir, ".download.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("failed to open download lock: %w", err)
	}
	defer lockFile.Close()

	// Acquire exclusive lock (blocking)
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire download lock: %w", err)
	}
	defer func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	}()

	// Check again after acquiring lock (another process may have downloaded)
	if _, err := os.Stat(destPath); err == nil {
		if err := m.verifyBinary(destPath); err == nil {
			return nil // Already downloaded and verified
		}
	}

	fmt.Printf("   Downloading tsuku-llm addon...\n")
	fmt.Printf("   URL: %s\n", info.URL)

	// Download to temp file
	tmpPath := destPath + ".tmp"
	if err := Download(ctx, info.URL, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	// Verify checksum before moving to final location
	fmt.Println("   Verifying checksum...")
	if err := VerifyChecksum(tmpPath, info.SHA256); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to make addon executable: %w", err)
	}

	// Atomic rename to final location
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to install addon: %w", err)
	}

	fmt.Println("   tsuku-llm addon installed successfully")
	return nil
}

// Legacy compatibility functions

// AddonPath returns the path to the tsuku-llm binary.
// Deprecated: Use NewAddonManager().BinaryPath() instead.
func AddonPath() string {
	home := os.Getenv("TSUKU_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		home = filepath.Join(userHome, ".tsuku")
	}

	binName := "tsuku-llm"
	if runtime.GOOS == "windows" {
		binName = "tsuku-llm.exe"
	}

	// Return versioned path if manifest available
	manifest, err := GetManifest()
	if err == nil {
		return filepath.Join(home, "tools", "tsuku-llm", manifest.Version, binName)
	}

	// Fallback to legacy path for compatibility
	return filepath.Join(home, "tools", "tsuku-llm", binName)
}

// IsInstalled checks if the addon is installed.
// Deprecated: Use NewAddonManager().IsInstalled() instead.
func IsInstalled() bool {
	_, err := os.Stat(AddonPath())
	return err == nil
}
