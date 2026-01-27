package verify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/install"
)

func TestVerifyIntegrity_AllMatch(t *testing.T) {
	// Create temp directory with test files
	tmpDir := t.TempDir()

	// Create test files
	file1 := filepath.Join(tmpDir, "lib", "libtest.so")
	if err := os.MkdirAll(filepath.Dir(file1), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := os.WriteFile(file1, []byte("test content 1"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	file2 := filepath.Join(tmpDir, "lib", "libother.so")
	if err := os.WriteFile(file2, []byte("test content 2"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Compute actual checksums
	checksum1, err := install.ComputeFileChecksum(file1)
	if err != nil {
		t.Fatalf("Failed to compute checksum: %v", err)
	}
	checksum2, err := install.ComputeFileChecksum(file2)
	if err != nil {
		t.Fatalf("Failed to compute checksum: %v", err)
	}

	// Create stored checksums map
	stored := map[string]string{
		"lib/libtest.so":  checksum1,
		"lib/libother.so": checksum2,
	}

	// Run verification
	result, err := VerifyIntegrity(tmpDir, stored)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}

	// Check result
	if result.Skipped {
		t.Error("Expected Skipped to be false")
	}
	if result.Verified != 2 {
		t.Errorf("Expected Verified=2, got %d", result.Verified)
	}
	if len(result.Mismatches) != 0 {
		t.Errorf("Expected no mismatches, got %d", len(result.Mismatches))
	}
	if len(result.Missing) != 0 {
		t.Errorf("Expected no missing files, got %d", len(result.Missing))
	}
}

func TestVerifyIntegrity_Mismatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	file1 := filepath.Join(tmpDir, "lib", "libtest.so")
	if err := os.MkdirAll(filepath.Dir(file1), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := os.WriteFile(file1, []byte("modified content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Use a different checksum than what the file actually has
	stored := map[string]string{
		"lib/libtest.so": "0000000000000000000000000000000000000000000000000000000000000000",
	}

	result, err := VerifyIntegrity(tmpDir, stored)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}

	if result.Verified != 0 {
		t.Errorf("Expected Verified=0, got %d", result.Verified)
	}
	if len(result.Mismatches) != 1 {
		t.Fatalf("Expected 1 mismatch, got %d", len(result.Mismatches))
	}

	mismatch := result.Mismatches[0]
	if mismatch.Path != "lib/libtest.so" {
		t.Errorf("Expected mismatch path 'lib/libtest.so', got '%s'", mismatch.Path)
	}
	if mismatch.Expected != "0000000000000000000000000000000000000000000000000000000000000000" {
		t.Errorf("Unexpected expected checksum: %s", mismatch.Expected)
	}
	if mismatch.Actual == "" {
		t.Error("Actual checksum should not be empty")
	}
}

func TestVerifyIntegrity_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Don't create any files, but reference one in stored checksums
	stored := map[string]string{
		"lib/libmissing.so": "0000000000000000000000000000000000000000000000000000000000000000",
	}

	result, err := VerifyIntegrity(tmpDir, stored)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}

	if result.Verified != 0 {
		t.Errorf("Expected Verified=0, got %d", result.Verified)
	}
	if len(result.Mismatches) != 0 {
		t.Errorf("Expected no mismatches, got %d", len(result.Mismatches))
	}
	if len(result.Missing) != 1 {
		t.Fatalf("Expected 1 missing file, got %d", len(result.Missing))
	}
	if result.Missing[0] != "lib/libmissing.so" {
		t.Errorf("Expected missing file 'lib/libmissing.so', got '%s'", result.Missing[0])
	}
}

func TestVerifyIntegrity_EmptyChecksums(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty map
	stored := map[string]string{}

	result, err := VerifyIntegrity(tmpDir, stored)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}

	if !result.Skipped {
		t.Error("Expected Skipped to be true")
	}
	if result.Reason == "" {
		t.Error("Expected Reason to be set")
	}
	if result.Verified != 0 {
		t.Errorf("Expected Verified=0, got %d", result.Verified)
	}
}

func TestVerifyIntegrity_NilChecksums(t *testing.T) {
	tmpDir := t.TempDir()

	result, err := VerifyIntegrity(tmpDir, nil)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}

	if !result.Skipped {
		t.Error("Expected Skipped to be true")
	}
	if result.Reason == "" {
		t.Error("Expected Reason to be set")
	}
}

func TestVerifyIntegrity_Symlink(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a real file
	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	realFile := filepath.Join(libDir, "libtest.so.1.0")
	if err := os.WriteFile(realFile, []byte("library content"), 0644); err != nil {
		t.Fatalf("Failed to create real file: %v", err)
	}

	// Create a symlink
	symlinkFile := filepath.Join(libDir, "libtest.so")
	if err := os.Symlink("libtest.so.1.0", symlinkFile); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Compute checksum of the real file
	checksum, err := install.ComputeFileChecksum(realFile)
	if err != nil {
		t.Fatalf("Failed to compute checksum: %v", err)
	}

	// Store checksum for the symlink path (but it should resolve to real file)
	stored := map[string]string{
		"lib/libtest.so": checksum,
	}

	result, err := VerifyIntegrity(tmpDir, stored)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}

	if result.Verified != 1 {
		t.Errorf("Expected Verified=1, got %d", result.Verified)
	}
	if len(result.Mismatches) != 0 {
		t.Errorf("Expected no mismatches, got %d", len(result.Mismatches))
	}
}

func TestVerifyIntegrity_Mixed(t *testing.T) {
	tmpDir := t.TempDir()

	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Create a file that will match
	goodFile := filepath.Join(libDir, "libgood.so")
	if err := os.WriteFile(goodFile, []byte("good content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	goodChecksum, _ := install.ComputeFileChecksum(goodFile)

	// Create a file that will be modified
	badFile := filepath.Join(libDir, "libbad.so")
	if err := os.WriteFile(badFile, []byte("bad content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	stored := map[string]string{
		"lib/libgood.so":    goodChecksum,
		"lib/libbad.so":     "0000000000000000000000000000000000000000000000000000000000000000", // wrong
		"lib/libmissing.so": "0000000000000000000000000000000000000000000000000000000000000000", // doesn't exist
	}

	result, err := VerifyIntegrity(tmpDir, stored)
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}

	if result.Verified != 1 {
		t.Errorf("Expected Verified=1, got %d", result.Verified)
	}
	if len(result.Mismatches) != 1 {
		t.Errorf("Expected 1 mismatch, got %d", len(result.Mismatches))
	}
	if len(result.Missing) != 1 {
		t.Errorf("Expected 1 missing, got %d", len(result.Missing))
	}
}
