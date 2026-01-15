package actions

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// AppBundleAction installs macOS .app bundles from ZIP or DMG archives.
// It supports downloading and extracting .app bundles from:
// - ZIP archives (cross-platform extraction)
// - DMG disk images (macOS only, uses hdiutil)
type AppBundleAction struct{ BaseAction }

// IsDeterministic returns true because app_bundle produces identical results
// given identical inputs (download URL + checksum).
func (AppBundleAction) IsDeterministic() bool { return true }

// Name returns the action name
func (a *AppBundleAction) Name() string {
	return "app_bundle"
}

// Preflight validates parameters without side effects.
func (a *AppBundleAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	// Required parameters
	if _, ok := GetString(params, "url"); !ok {
		result.AddError("app_bundle action requires 'url' parameter")
	}
	if _, ok := GetString(params, "checksum"); !ok {
		result.AddError("app_bundle action requires 'checksum' parameter")
	}
	if _, ok := GetString(params, "app_name"); !ok {
		result.AddError("app_bundle action requires 'app_name' parameter")
	}

	// macOS-only action
	if runtime.GOOS != "darwin" {
		result.AddWarning("app_bundle action only works on macOS; step will be skipped on other platforms")
	}

	return result
}

// AppBundleResult contains the result of an app_bundle installation.
// This is stored in ExecutionContext.AppResult for state tracking.
type AppBundleResult struct {
	AppPath            string   // Installed .app path
	ApplicationSymlink string   // ~/Applications symlink path (if created)
	Binaries           []string // Symlinked binary names
}

// Execute downloads, extracts, and installs a macOS .app bundle.
//
// Parameters:
//   - url (required): Download URL for the ZIP or DMG archive
//   - checksum (required): SHA256 checksum in "sha256:..." format
//   - app_name (required): Name of .app bundle to install (e.g., "iTerm.app")
//   - binaries (optional): Paths to CLI tools within .app to symlink to $TSUKU_HOME/tools/current
//   - symlink_applications (optional): Create ~/Applications symlink (default: true)
func (a *AppBundleAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Only run on macOS
	if ctx.OS != "darwin" {
		fmt.Printf("   Skipping app_bundle: macOS only\n")
		return nil
	}

	// Get required parameters
	url, ok := GetString(params, "url")
	if !ok {
		return fmt.Errorf("app_bundle action requires 'url' parameter")
	}

	checksum, ok := GetString(params, "checksum")
	if !ok {
		return fmt.Errorf("app_bundle action requires 'checksum' parameter")
	}

	appName, ok := GetString(params, "app_name")
	if !ok {
		return fmt.Errorf("app_bundle action requires 'app_name' parameter")
	}

	// Get optional parameters
	binaries, _ := GetStringSlice(params, "binaries")
	symlinkApplications := GetBoolDefault(params, "symlink_applications", true)

	// Ensure AppsDir exists
	if ctx.AppsDir == "" {
		return fmt.Errorf("AppsDir not configured in execution context")
	}
	if err := os.MkdirAll(ctx.AppsDir, 0755); err != nil {
		return fmt.Errorf("failed to create apps directory: %w", err)
	}

	logger := ctx.Log()
	logger.Debug("app_bundle action starting",
		"url", url,
		"checksum", checksum,
		"appName", appName,
		"appsDir", ctx.AppsDir,
		"binaries", binaries,
		"symlinkApplications", symlinkApplications)

	// Detect archive format
	archiveFormat := detectArchiveFormatFromURL(url)
	if archiveFormat != "zip" && archiveFormat != "dmg" {
		return fmt.Errorf("app_bundle supports ZIP and DMG archives; got %s", archiveFormat)
	}

	// Download the archive
	fmt.Printf("   Downloading: %s\n", url)
	archiveName := filepath.Base(url)
	archivePath := filepath.Join(ctx.WorkDir, archiveName)

	if err := downloadWithChecksum(ctx, url, archivePath, checksum); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Extract based on archive format
	fmt.Printf("   Extracting: %s\n", archiveName)
	extractDir := filepath.Join(ctx.WorkDir, "extracted")

	var extractErr error
	switch archiveFormat {
	case "zip":
		extractErr = extractZIP(archivePath, extractDir)
	case "dmg":
		extractErr = extractDMG(archivePath, extractDir)
	}
	if extractErr != nil {
		return fmt.Errorf("extraction failed: %w", extractErr)
	}

	// Find the .app bundle
	appPath, err := findAppBundle(extractDir, appName)
	if err != nil {
		return err
	}

	// Determine destination path
	// Format: $TSUKU_HOME/apps/<recipe-name>-<version>.app
	recipeName := ""
	if ctx.Recipe != nil {
		recipeName = ctx.Recipe.Metadata.Name
	}
	if recipeName == "" {
		recipeName = strings.TrimSuffix(appName, ".app")
	}
	destName := fmt.Sprintf("%s-%s.app", recipeName, ctx.Version)
	destPath := filepath.Join(ctx.AppsDir, destName)

	// Copy .app bundle to apps directory
	fmt.Printf("   Installing: %s -> %s\n", appName, destPath)
	if err := copyDir(appPath, destPath); err != nil {
		return fmt.Errorf("failed to copy .app bundle: %w", err)
	}

	fmt.Printf("   Installed: %s\n", destPath)

	// Initialize result for state tracking
	result := &AppBundleResult{
		AppPath:  destPath,
		Binaries: []string{},
	}

	// Create binary symlinks
	if len(binaries) > 0 {
		// Ensure CurrentDir exists (for binary symlinks)
		if ctx.CurrentDir != "" {
			if err := os.MkdirAll(ctx.CurrentDir, 0755); err != nil {
				return fmt.Errorf("failed to create current directory: %w", err)
			}

			for _, binaryPath := range binaries {
				binaryName := filepath.Base(binaryPath)
				targetPath := filepath.Join(destPath, binaryPath)
				symlinkPath := filepath.Join(ctx.CurrentDir, binaryName)

				// Verify the binary exists in the .app bundle
				if _, err := os.Stat(targetPath); os.IsNotExist(err) {
					fmt.Printf("   Warning: binary not found in app bundle: %s\n", binaryPath)
					continue
				}

				// Create symlink atomically
				if err := atomicSymlink(targetPath, symlinkPath); err != nil {
					return fmt.Errorf("failed to create symlink for %s: %w", binaryName, err)
				}

				fmt.Printf("   Symlinked: %s -> %s\n", symlinkPath, targetPath)
				result.Binaries = append(result.Binaries, binaryName)
			}
		}
	}

	// Create ~/Applications symlink for Launchpad/Spotlight integration
	if symlinkApplications {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			applicationsDir := filepath.Join(homeDir, "Applications")
			// Ensure ~/Applications exists
			if err := os.MkdirAll(applicationsDir, 0755); err != nil {
				fmt.Printf("   Warning: could not create ~/Applications: %v\n", err)
			} else {
				// Use app_name for the symlink (e.g., "Visual Studio Code.app")
				applicationSymlink := filepath.Join(applicationsDir, appName)

				// Create symlink atomically
				if err := atomicSymlink(destPath, applicationSymlink); err != nil {
					fmt.Printf("   Warning: could not create ~/Applications symlink: %v\n", err)
				} else {
					fmt.Printf("   Symlinked: %s -> %s\n", applicationSymlink, destPath)
					result.ApplicationSymlink = applicationSymlink
				}
			}
		}
	}

	// Store result for state tracking
	ctx.AppResult = result

	return nil
}

// detectArchiveFormatFromURL detects archive format from URL extension.
func detectArchiveFormatFromURL(url string) string {
	lower := strings.ToLower(url)
	if strings.HasSuffix(lower, ".zip") {
		return "zip"
	}
	if strings.HasSuffix(lower, ".dmg") {
		return "dmg"
	}
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return "tar.gz"
	}
	return "unknown"
}

// downloadWithChecksum downloads a file and verifies its checksum.
func downloadWithChecksum(ctx *ExecutionContext, url, destPath, checksum string) error {
	// Use the download_file action for the actual download
	downloadAction := &DownloadFileAction{}
	params := map[string]interface{}{
		"url":      url,
		"dest":     filepath.Base(destPath),
		"checksum": checksum,
	}
	return downloadAction.Execute(ctx, params)
}

// extractZIP extracts a ZIP archive to the destination directory.
func extractZIP(archivePath, destDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open ZIP: %w", err)
	}
	defer reader.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	for _, file := range reader.File {
		targetPath := filepath.Join(destDir, file.Name)

		// Security: validate path is within destination
		if !isPathWithinDirectory(targetPath, destDir) {
			return fmt.Errorf("ZIP contains path traversal: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, file.Mode()); err != nil {
				return err
			}
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		// Handle symlinks
		if file.Mode()&os.ModeSymlink != 0 {
			rc, err := file.Open()
			if err != nil {
				return err
			}
			linkTarget, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return err
			}

			// Security: validate symlink target
			if err := validateSymlinkTarget(string(linkTarget), targetPath, destDir); err != nil {
				return err
			}

			if err := os.Symlink(string(linkTarget), targetPath); err != nil {
				return err
			}
			continue
		}

		// Extract regular file
		rc, err := file.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

// extractDMG extracts a DMG disk image to the destination directory.
// It uses hdiutil to mount the DMG, copies the contents, and unmounts.
// This function only works on macOS.
func extractDMG(dmgPath, destDir string) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Create a temporary mount point
	mountPoint, err := os.MkdirTemp("", "tsuku-dmg-*")
	if err != nil {
		return fmt.Errorf("failed to create mount point: %w", err)
	}
	// Ensure cleanup of mount point directory
	defer os.RemoveAll(mountPoint)

	// Mount the DMG read-only without Finder interference
	// -nobrowse: Don't show in Finder
	// -readonly: Mount read-only
	// -mountpoint: Specify where to mount
	mountCmd := exec.Command("hdiutil", "attach", dmgPath,
		"-nobrowse", "-readonly", "-mountpoint", mountPoint)
	if output, err := mountCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to mount DMG: %w\nOutput: %s", err, string(output))
	}

	// Ensure we unmount even if copy fails
	defer func() {
		detachCmd := exec.Command("hdiutil", "detach", mountPoint, "-quiet", "-force")
		_ = detachCmd.Run() // Best effort; ignore errors on cleanup
	}()

	// Copy all contents from mount point to destination
	// We search for .app bundles and copy them
	entries, err := os.ReadDir(mountPoint)
	if err != nil {
		return fmt.Errorf("failed to read mount point: %w", err)
	}

	// Copy all entries (directories and files) from mount to dest
	for _, entry := range entries {
		srcPath := filepath.Join(mountPoint, entry.Name())
		dstPath := filepath.Join(destDir, entry.Name())

		// Skip hidden system files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Get file info to handle symlinks properly
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("failed to get file info for %s: %w", entry.Name(), err)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			// Handle symlinks (some DMGs use aliases/symlinks)
			linkTarget, err := os.Readlink(srcPath)
			if err != nil {
				return fmt.Errorf("failed to read symlink %s: %w", srcPath, err)
			}
			if err := os.Symlink(linkTarget, dstPath); err != nil {
				return fmt.Errorf("failed to create symlink %s: %w", dstPath, err)
			}
		} else if entry.IsDir() {
			// Copy directories (including .app bundles)
			if err := copyDir(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy %s: %w", entry.Name(), err)
			}
		} else {
			// Copy regular files
			if err := copyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to copy file %s: %w", entry.Name(), err)
			}
		}
	}

	return nil
}

// findAppBundle searches for a .app bundle in the extracted directory.
func findAppBundle(extractDir, appName string) (string, error) {
	// First, try exact match at root level
	directPath := filepath.Join(extractDir, appName)
	if info, err := os.Stat(directPath); err == nil && info.IsDir() {
		return directPath, nil
	}

	// Search recursively (up to 2 levels deep)
	var found string
	err := filepath.Walk(extractDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == appName {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return "", fmt.Errorf("error searching for .app bundle: %w", err)
	}

	if found == "" {
		return "", fmt.Errorf(".app bundle not found: %s in %s", appName, extractDir)
	}

	return found, nil
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	// Remove existing destination if present
	if _, err := os.Stat(dst); err == nil {
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("failed to remove existing destination: %w", err)
		}
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate destination path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		// Handle directories
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, dstPath)
		}

		// Copy regular files
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}
