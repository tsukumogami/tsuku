package actions

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// AppBundleAction installs macOS .app bundles from ZIP archives.
// This is a walking skeleton implementation that supports ZIP extraction.
// Issue #864 will add DMG support.
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

// Execute downloads, extracts, and installs a macOS .app bundle.
//
// Parameters:
//   - url (required): Download URL for the ZIP archive
//   - checksum (required): SHA256 checksum in "sha256:..." format
//   - app_name (required): Name of .app bundle to install (e.g., "iTerm.app")
func (a *AppBundleAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Only run on macOS
	if ctx.OS != "darwin" {
		fmt.Printf("   Skipping app_bundle: macOS only\n")
		return nil
	}

	// Get parameters
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
		"appsDir", ctx.AppsDir)

	// Detect archive format (walking skeleton only supports ZIP)
	archiveFormat := detectArchiveFormatFromURL(url)
	if archiveFormat != "zip" {
		return fmt.Errorf("app_bundle only supports ZIP archives; got %s (DMG support coming in issue #864)", archiveFormat)
	}

	// Download the archive
	fmt.Printf("   Downloading: %s\n", url)
	archiveName := filepath.Base(url)
	archivePath := filepath.Join(ctx.WorkDir, archiveName)

	if err := downloadWithChecksum(ctx, url, archivePath, checksum); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Extract the ZIP
	fmt.Printf("   Extracting: %s\n", archiveName)
	extractDir := filepath.Join(ctx.WorkDir, "extracted")
	if err := extractZIP(archivePath, extractDir); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
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
