package verify

import (
	"os"
	"path/filepath"

	"github.com/tsukumogami/tsuku/internal/install"
)

// IntegrityResult represents the outcome of integrity verification.
type IntegrityResult struct {
	// Verified is the count of files that passed verification.
	Verified int

	// Mismatches contains files where checksums didn't match.
	Mismatches []IntegrityMismatch

	// Missing contains files that no longer exist.
	Missing []string

	// Skipped is true if verification was skipped (no stored checksums).
	Skipped bool

	// Reason explains why verification was skipped.
	Reason string
}

// IntegrityMismatch records a checksum mismatch.
type IntegrityMismatch struct {
	Path     string // Relative path from library directory
	Expected string // Stored checksum
	Actual   string // Computed checksum
}

// VerifyIntegrity compares current file checksums against stored values.
//
// Parameters:
//   - libDir: Absolute path to library directory
//   - stored: Map of relative path -> expected SHA256 hex
//
// Returns IntegrityResult with verification outcome.
func VerifyIntegrity(libDir string, stored map[string]string) (*IntegrityResult, error) {
	// No checksums stored - skip with informative reason
	if len(stored) == 0 {
		return &IntegrityResult{
			Skipped: true,
			Reason:  "no stored checksums (pre-checksum installation)",
		}, nil
	}

	result := &IntegrityResult{}

	for relPath, expected := range stored {
		fullPath := filepath.Join(libDir, relPath)

		// Check if file exists (resolve symlinks)
		realPath, err := filepath.EvalSymlinks(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				result.Missing = append(result.Missing, relPath)
				continue
			}
			// For broken symlinks, check if the original path exists as a symlink
			if _, statErr := os.Lstat(fullPath); os.IsNotExist(statErr) {
				result.Missing = append(result.Missing, relPath)
				continue
			}
			// Other errors (e.g., permission denied) - treat as missing
			result.Missing = append(result.Missing, relPath)
			continue
		}

		// Compute current checksum using existing helper
		actual, err := install.ComputeFileChecksum(realPath)
		if err != nil {
			// File exists but can't be read - treat as missing
			result.Missing = append(result.Missing, relPath)
			continue
		}

		// Compare checksums
		if actual != expected {
			result.Mismatches = append(result.Mismatches, IntegrityMismatch{
				Path:     relPath,
				Expected: expected,
				Actual:   actual,
			})
		} else {
			result.Verified++
		}
	}

	return result, nil
}
