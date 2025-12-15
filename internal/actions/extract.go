package actions

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	lzip "github.com/sorairolake/lzip-go"
	"github.com/ulikunitz/xz"
)

// isPathWithinDirectory checks if targetPath is safely contained within basePath
// SECURITY: Prevents path traversal attacks where malicious archives could write outside destPath
func isPathWithinDirectory(targetPath, basePath string) bool {
	// Get absolute paths to handle any relative path tricks
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return false
	}

	// Ensure the target starts with the base path
	// Add separator to prevent matching partial directory names (e.g., /tmp/foo matching /tmp/foobar)
	return absTarget == absBase || strings.HasPrefix(absTarget, absBase+string(os.PathSeparator))
}

// validateSymlinkTarget validates that a symlink target is safe
// SECURITY: Prevents symlink attacks where malicious archives point to sensitive locations
func validateSymlinkTarget(linkTarget, linkLocation, destPath string) error {
	// If the symlink target is absolute, it could point anywhere - reject it
	if filepath.IsAbs(linkTarget) {
		return fmt.Errorf("absolute symlink targets are not allowed: %s -> %s", linkLocation, linkTarget)
	}

	// Resolve where the symlink would actually point to
	resolvedTarget := filepath.Join(filepath.Dir(linkLocation), linkTarget)

	// Verify the resolved target is within the destination directory
	if !isPathWithinDirectory(resolvedTarget, destPath) {
		return fmt.Errorf("symlink target escapes destination directory: %s -> %s (resolves to %s)",
			linkLocation, linkTarget, resolvedTarget)
	}

	return nil
}

// ExtractAction implements archive extraction
type ExtractAction struct{ BaseAction }

// IsDeterministic returns true because extraction produces identical results.
func (ExtractAction) IsDeterministic() bool { return true }

// Name returns the action name
func (a *ExtractAction) Name() string {
	return "extract"
}

// Execute extracts an archive
//
// Parameters:
//   - archive (required): Archive filename to extract
//   - format (required): Archive format (tar.gz, tar.xz, tar.bz2, zip, auto)
//   - dest (optional): Destination directory (defaults to work_dir)
//   - strip_dirs (optional): Number of leading path components to strip (default: 0)
//   - files (optional): List of specific files to extract (default: all files)
func (a *ExtractAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get archive filename (required)
	archiveName, ok := GetString(params, "archive")
	if !ok {
		return fmt.Errorf("extract action requires 'archive' parameter")
	}

	// Build vars for variable substitution
	vars := GetStandardVars(ctx.Version, ctx.InstallDir, ctx.WorkDir)

	// Apply OS mapping if present
	if osMapping, ok := GetMapStringString(params, "os_mapping"); ok {
		if mappedOS, ok := osMapping[vars["os"]]; ok {
			vars["os"] = mappedOS
		}
	}

	// Apply arch mapping if present
	if archMapping, ok := GetMapStringString(params, "arch_mapping"); ok {
		if mappedArch, ok := archMapping[vars["arch"]]; ok {
			vars["arch"] = mappedArch
		}
	}

	archiveName = ExpandVars(archiveName, vars)
	archivePath := filepath.Join(ctx.WorkDir, archiveName)

	// Get format (required)
	format, ok := GetString(params, "format")
	if !ok {
		return fmt.Errorf("extract action requires 'format' parameter")
	}

	// Auto-detect format if needed
	if format == "auto" {
		format = a.detectFormat(archiveName)
	}

	// Get destination directory (defaults to work_dir)
	dest, _ := GetString(params, "dest")
	if dest == "" {
		dest = "."
	}
	dest = ExpandVars(dest, vars)
	destPath := filepath.Join(ctx.WorkDir, dest)

	// Get strip_dirs (defaults to 0)
	stripDirs, _ := GetInt(params, "strip_dirs")

	// Get files list (optional)
	files, _ := GetStringSlice(params, "files")

	// Log extraction details
	logger := ctx.Log()
	logger.Debug("extract action starting",
		"archive", archiveName,
		"format", format,
		"destPath", destPath,
		"stripDirs", stripDirs)

	fmt.Printf("   Extracting: %s\n", archiveName)
	fmt.Printf("   Format: %s\n", format)
	if stripDirs > 0 {
		fmt.Printf("   Strip dirs: %d\n", stripDirs)
	}

	// Extract based on format
	switch format {
	case "tar.gz", "tgz":
		return a.extractTarGz(archivePath, destPath, stripDirs, files)
	case "tar.xz", "txz":
		return a.extractTarXz(archivePath, destPath, stripDirs, files)
	case "tar.bz2", "tbz2", "tbz":
		return a.extractTarBz2(archivePath, destPath, stripDirs, files)
	case "tar.zst", "tzst":
		return a.extractTarZst(archivePath, destPath, stripDirs, files)
	case "tar.lz", "tlz":
		return a.extractTarLz(archivePath, destPath, stripDirs, files)
	case "tar":
		return a.extractTar(archivePath, destPath, stripDirs, files)
	case "zip":
		return a.extractZip(archivePath, destPath, stripDirs, files)
	default:
		return fmt.Errorf("unsupported archive format: %s", format)
	}
}

// detectFormat auto-detects archive format from filename
func (a *ExtractAction) detectFormat(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".txz"):
		return "tar.xz"
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"), strings.HasSuffix(lower, ".tbz"):
		return "tar.bz2"
	case strings.HasSuffix(lower, ".tar.zst"), strings.HasSuffix(lower, ".tzst"):
		return "tar.zst"
	case strings.HasSuffix(lower, ".tar.lz"), strings.HasSuffix(lower, ".tlz"):
		return "tar.lz"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	default:
		return "unknown"
	}
}

// extractTarGz extracts a tar.gz archive
func (a *ExtractAction) extractTarGz(archivePath, destPath string, stripDirs int, files []string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	return a.extractTarReader(tar.NewReader(gzr), destPath, stripDirs, files)
}

// extractTarXz extracts a tar.xz archive
func (a *ExtractAction) extractTarXz(archivePath, destPath string, stripDirs int, files []string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	xzr, err := xz.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create xz reader: %w", err)
	}

	return a.extractTarReader(tar.NewReader(xzr), destPath, stripDirs, files)
}

// extractTarBz2 extracts a tar.bz2 archive
func (a *ExtractAction) extractTarBz2(archivePath, destPath string, stripDirs int, files []string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	bzr := bzip2.NewReader(file)
	return a.extractTarReader(tar.NewReader(bzr), destPath, stripDirs, files)
}

// extractTarZst extracts a tar.zst archive
func (a *ExtractAction) extractTarZst(archivePath, destPath string, stripDirs int, files []string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	zr, err := zstd.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create zstd reader: %w", err)
	}
	defer zr.Close()

	return a.extractTarReader(tar.NewReader(zr), destPath, stripDirs, files)
}

// extractTarLz extracts a tar.lz archive
func (a *ExtractAction) extractTarLz(archivePath, destPath string, stripDirs int, files []string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	lr, err := lzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create lzip reader: %w", err)
	}

	return a.extractTarReader(tar.NewReader(lr), destPath, stripDirs, files)
}

// extractTar extracts a plain tar archive
func (a *ExtractAction) extractTar(archivePath, destPath string, stripDirs int, files []string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	return a.extractTarReader(tar.NewReader(file), destPath, stripDirs, files)
}

// extractTarReader extracts from a tar.Reader
func (a *ExtractAction) extractTarReader(tr *tar.Reader, destPath string, stripDirs int, files []string) error {
	// Build file filter map if files list provided
	fileFilter := make(map[string]bool)
	if len(files) > 0 {
		for _, f := range files {
			fileFilter[f] = true
		}
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Clean the path to remove leading "./"
		cleanPath := strings.TrimPrefix(header.Name, "./")

		// Apply strip_dirs
		parts := strings.Split(cleanPath, "/")
		if len(parts) <= stripDirs {
			continue
		}
		parts = parts[stripDirs:]
		relativePath := filepath.Join(parts...)

		// Apply file filter if provided
		if len(fileFilter) > 0 && !fileFilter[relativePath] {
			continue
		}

		target := filepath.Join(destPath, relativePath)

		// SECURITY: Validate that target path is within destPath (prevents path traversal)
		if !isPathWithinDirectory(target, destPath) {
			return fmt.Errorf("archive entry escapes destination directory: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("failed to write file: %w", err)
			}
			f.Close()

		case tar.TypeSymlink:
			// SECURITY: Validate symlink target to prevent escape attacks
			if err := validateSymlinkTarget(header.Linkname, target, destPath); err != nil {
				return err
			}

			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			// Use atomic symlink creation to prevent TOCTOU race conditions
			if err := atomicSymlink(header.Linkname, target); err != nil {
				return fmt.Errorf("failed to create symlink: %w", err)
			}
		}
	}

	return nil
}

// atomicSymlink creates a symlink atomically using rename
// SECURITY: Prevents TOCTOU race conditions where an attacker could replace
// the target between removal and symlink creation
func atomicSymlink(target, linkPath string) error {
	// Create temporary symlink with unique name
	tmpLink := linkPath + ".tmp"

	// Remove any existing temp symlink
	os.Remove(tmpLink)

	// Create the symlink at temporary location
	if err := os.Symlink(target, tmpLink); err != nil {
		return err
	}

	// Atomically rename to final location (POSIX guarantees atomic rename)
	if err := os.Rename(tmpLink, linkPath); err != nil {
		os.Remove(tmpLink) // Clean up temp file on failure
		return err
	}

	return nil
}

// extractZip extracts a zip archive
func (a *ExtractAction) extractZip(archivePath, destPath string, stripDirs int, files []string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	// Build file filter map if files list provided
	fileFilter := make(map[string]bool)
	if len(files) > 0 {
		for _, f := range files {
			fileFilter[f] = true
		}
	}

	for _, f := range r.File {
		// Clean the path
		cleanPath := strings.TrimPrefix(f.Name, "./")

		// Apply strip_dirs
		parts := strings.Split(cleanPath, "/")
		if len(parts) <= stripDirs {
			continue
		}
		parts = parts[stripDirs:]
		relativePath := filepath.Join(parts...)

		// Apply file filter if provided
		if len(fileFilter) > 0 && !fileFilter[relativePath] {
			continue
		}

		target := filepath.Join(destPath, relativePath)

		// SECURITY: Validate that target path is within destPath (prevents path traversal)
		if !isPathWithinDirectory(target, destPath) {
			return fmt.Errorf("zip entry escapes destination directory: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in zip: %w", err)
		}

		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create file: %w", err)
		}

		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			rc.Close()
			return fmt.Errorf("failed to write file: %w", err)
		}

		outFile.Close()
		rc.Close()
	}

	return nil
}
