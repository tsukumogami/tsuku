package install

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ChecksumMismatch represents a binary that failed integrity verification.
type ChecksumMismatch struct {
	Path     string // Relative path within tool directory
	Expected string // Expected SHA256 hex hash
	Actual   string // Actual SHA256 hex hash (empty if Error is set)
	Error    error  // Non-nil if file is missing or unreadable
}

// ComputeFileChecksum computes the SHA256 checksum of a file.
// Returns the hex-encoded checksum or an error.
func ComputeFileChecksum(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// ComputeBinaryChecksums computes SHA256 checksums for installed binaries.
// toolDir is the absolute path to the tool installation directory.
// binaries is the list of relative binary paths (e.g., "bin/jq", "cargo/bin/cargo").
// Returns a map of relative path to hex-encoded checksum.
func ComputeBinaryChecksums(toolDir string, binaries []string) (map[string]string, error) {
	if len(binaries) == 0 {
		return nil, nil
	}

	// Canonicalize toolDir to handle symlinks (e.g., macOS /var -> /private/var)
	canonicalToolDir, err := filepath.EvalSymlinks(toolDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve tool directory: %w", err)
	}

	checksums := make(map[string]string, len(binaries))

	for _, binaryPath := range binaries {
		absPath := filepath.Join(toolDir, binaryPath)

		// Resolve symlinks to get the actual file
		realPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve binary path %s: %w", binaryPath, err)
		}

		// Verify the resolved path is still within the tool directory
		// (prevent symlink attacks pointing outside)
		if !isWithinDir(realPath, canonicalToolDir) {
			return nil, fmt.Errorf("binary %s resolves outside tool directory: %s", binaryPath, realPath)
		}

		checksum, err := ComputeFileChecksum(realPath)
		if err != nil {
			return nil, fmt.Errorf("failed to compute checksum for %s: %w", binaryPath, err)
		}

		checksums[binaryPath] = checksum
	}

	return checksums, nil
}

// VerifyBinaryChecksums verifies stored checksums against current binary state.
// toolDir is the absolute path to the tool installation directory.
// stored is a map of relative path to expected hex-encoded checksum.
// Returns a slice of mismatches (empty if all verified), or an error for unexpected failures.
func VerifyBinaryChecksums(toolDir string, stored map[string]string) ([]ChecksumMismatch, error) {
	if len(stored) == 0 {
		return nil, nil
	}

	var mismatches []ChecksumMismatch

	for binaryPath, expectedChecksum := range stored {
		absPath := filepath.Join(toolDir, binaryPath)

		// Resolve symlinks to get the actual file
		realPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			mismatches = append(mismatches, ChecksumMismatch{
				Path:     binaryPath,
				Expected: expectedChecksum,
				Error:    fmt.Errorf("failed to resolve path: %w", err),
			})
			continue
		}

		// Compute current checksum
		actualChecksum, err := ComputeFileChecksum(realPath)
		if err != nil {
			mismatches = append(mismatches, ChecksumMismatch{
				Path:     binaryPath,
				Expected: expectedChecksum,
				Error:    err,
			})
			continue
		}

		// Compare
		if actualChecksum != expectedChecksum {
			mismatches = append(mismatches, ChecksumMismatch{
				Path:     binaryPath,
				Expected: expectedChecksum,
				Actual:   actualChecksum,
			})
		}
	}

	return mismatches, nil
}

// ComputeLibraryChecksums computes SHA256 checksums for all regular files in a library directory.
// libDir is the absolute path to the library installation directory.
// Returns a map of relative path to hex-encoded checksum.
// Symlinks are skipped (only real files are checksummed).
func ComputeLibraryChecksums(libDir string) (map[string]string, error) {
	checksums := make(map[string]string)

	err := filepath.Walk(libDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Use Lstat to detect symlinks (Walk uses Stat which follows symlinks)
		linfo, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("failed to lstat %s: %w", path, err)
		}

		// Skip symlinks - only checksum real files
		if linfo.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		// Compute relative path from library directory root
		relPath, err := filepath.Rel(libDir, path)
		if err != nil {
			return fmt.Errorf("failed to compute relative path for %s: %w", path, err)
		}

		// Compute checksum
		checksum, err := ComputeFileChecksum(path)
		if err != nil {
			return fmt.Errorf("failed to compute checksum for %s: %w", relPath, err)
		}

		checksums[relPath] = checksum
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk library directory: %w", err)
	}

	return checksums, nil
}

// VerifyLibraryChecksums verifies stored checksums against current library file state.
// libDir is the absolute path to the library installation directory.
// stored is a map of relative path to expected hex-encoded checksum.
// Returns a slice of mismatches (empty if all verified), or an error for unexpected failures.
//
// Note: This is a basic implementation for CI validation. Production-grade verification
// with detailed reporting will be implemented in issue #950.
func VerifyLibraryChecksums(libDir string, stored map[string]string) ([]ChecksumMismatch, error) {
	if len(stored) == 0 {
		return nil, nil
	}

	var mismatches []ChecksumMismatch

	for relPath, expectedChecksum := range stored {
		absPath := filepath.Join(libDir, relPath)

		// Compute current checksum
		actualChecksum, err := ComputeFileChecksum(absPath)
		if err != nil {
			mismatches = append(mismatches, ChecksumMismatch{
				Path:     relPath,
				Expected: expectedChecksum,
				Error:    err,
			})
			continue
		}

		// Compare
		if actualChecksum != expectedChecksum {
			mismatches = append(mismatches, ChecksumMismatch{
				Path:     relPath,
				Expected: expectedChecksum,
				Actual:   actualChecksum,
			})
		}
	}

	return mismatches, nil
}

// isWithinDir checks if path is within the specified directory.
// Both paths should be absolute and cleaned.
func isWithinDir(path, dir string) bool {
	// Clean and resolve to absolute paths
	path = filepath.Clean(path)
	dir = filepath.Clean(dir)

	// Check if path starts with dir
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}

	// If the relative path starts with "..", the path is outside dir
	return !filepath.IsAbs(rel) && (rel == "." || (len(rel) >= 2 && rel[:2] != ".."))
}
