package addon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// VerifyChecksum verifies that the file at path has the expected SHA256 checksum.
// The expected checksum should be hex-encoded (64 characters).
// Returns nil if verification succeeds, error otherwise.
func VerifyChecksum(path, expectedSHA256 string) error {
	actual, err := ComputeChecksum(path)
	if err != nil {
		return err
	}

	expected := strings.ToLower(strings.TrimSpace(expectedSHA256))
	if actual != expected {
		return fmt.Errorf("checksum mismatch:\n  expected: %s\n  actual:   %s", expected, actual)
	}

	return nil
}

// ComputeChecksum computes the SHA256 checksum of the file at path.
// Returns the hex-encoded checksum string.
func ComputeChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to compute checksum: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
