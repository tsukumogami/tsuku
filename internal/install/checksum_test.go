package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeFileChecksum(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create a test file with known content
	testFile := filepath.Join(tmpDir, "test.bin")
	content := []byte("hello world")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Expected SHA256 of "hello world"
	// echo -n "hello world" | sha256sum
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	checksum, err := ComputeFileChecksum(testFile)
	if err != nil {
		t.Fatalf("ComputeFileChecksum failed: %v", err)
	}

	if checksum != expected {
		t.Errorf("checksum mismatch: got %s, want %s", checksum, expected)
	}
}

func TestComputeFileChecksum_MissingFile(t *testing.T) {
	_, err := ComputeFileChecksum("/nonexistent/file")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestComputeBinaryChecksums(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create test binaries
	binary1 := filepath.Join(binDir, "tool1")
	binary2 := filepath.Join(binDir, "tool2")
	if err := os.WriteFile(binary1, []byte("binary1"), 0755); err != nil {
		t.Fatalf("failed to create binary1: %v", err)
	}
	if err := os.WriteFile(binary2, []byte("binary2"), 0755); err != nil {
		t.Fatalf("failed to create binary2: %v", err)
	}

	// Compute checksums
	binaries := []string{"bin/tool1", "bin/tool2"}
	checksums, err := ComputeBinaryChecksums(tmpDir, binaries)
	if err != nil {
		t.Fatalf("ComputeBinaryChecksums failed: %v", err)
	}

	// Verify we got checksums for both binaries
	if len(checksums) != 2 {
		t.Errorf("expected 2 checksums, got %d", len(checksums))
	}

	for _, binary := range binaries {
		if _, ok := checksums[binary]; !ok {
			t.Errorf("missing checksum for %s", binary)
		}
	}
}

func TestComputeBinaryChecksums_WithSymlink(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create actual binary
	actualBinary := filepath.Join(binDir, "actual")
	if err := os.WriteFile(actualBinary, []byte("actual content"), 0755); err != nil {
		t.Fatalf("failed to create actual binary: %v", err)
	}

	// Create symlink to actual binary
	symlink := filepath.Join(binDir, "link")
	if err := os.Symlink("actual", symlink); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Compute checksum via symlink
	checksums, err := ComputeBinaryChecksums(tmpDir, []string{"bin/link"})
	if err != nil {
		t.Fatalf("ComputeBinaryChecksums failed: %v", err)
	}

	// Should have checksum for the symlink path
	if _, ok := checksums["bin/link"]; !ok {
		t.Error("missing checksum for symlink")
	}

	// Checksum should match the actual file's checksum
	actualChecksum, err := ComputeFileChecksum(actualBinary)
	if err != nil {
		t.Fatalf("failed to compute actual checksum: %v", err)
	}

	if checksums["bin/link"] != actualChecksum {
		t.Errorf("symlink checksum doesn't match actual: got %s, want %s",
			checksums["bin/link"], actualChecksum)
	}
}

func TestComputeBinaryChecksums_MissingBinary(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := ComputeBinaryChecksums(tmpDir, []string{"bin/nonexistent"})
	if err == nil {
		t.Error("expected error for missing binary, got nil")
	}
}

func TestComputeBinaryChecksums_EmptyList(t *testing.T) {
	tmpDir := t.TempDir()

	checksums, err := ComputeBinaryChecksums(tmpDir, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checksums != nil {
		t.Error("expected nil for empty binary list")
	}

	checksums, err = ComputeBinaryChecksums(tmpDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checksums != nil {
		t.Error("expected nil for nil binary list")
	}
}

func TestVerifyBinaryChecksums_AllMatch(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create test binary
	binary := filepath.Join(binDir, "tool")
	content := []byte("test content")
	if err := os.WriteFile(binary, content, 0755); err != nil {
		t.Fatalf("failed to create binary: %v", err)
	}

	// Compute checksums
	checksums, err := ComputeBinaryChecksums(tmpDir, []string{"bin/tool"})
	if err != nil {
		t.Fatalf("ComputeBinaryChecksums failed: %v", err)
	}

	// Verify - should have no mismatches
	mismatches, err := VerifyBinaryChecksums(tmpDir, checksums)
	if err != nil {
		t.Fatalf("VerifyBinaryChecksums failed: %v", err)
	}
	if len(mismatches) != 0 {
		t.Errorf("expected no mismatches, got %d", len(mismatches))
	}
}

func TestVerifyBinaryChecksums_Mismatch(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create test binary
	binary := filepath.Join(binDir, "tool")
	if err := os.WriteFile(binary, []byte("original content"), 0755); err != nil {
		t.Fatalf("failed to create binary: %v", err)
	}

	// Compute checksums
	checksums, err := ComputeBinaryChecksums(tmpDir, []string{"bin/tool"})
	if err != nil {
		t.Fatalf("ComputeBinaryChecksums failed: %v", err)
	}

	// Modify the binary
	if err := os.WriteFile(binary, []byte("modified content"), 0755); err != nil {
		t.Fatalf("failed to modify binary: %v", err)
	}

	// Verify - should detect mismatch
	mismatches, err := VerifyBinaryChecksums(tmpDir, checksums)
	if err != nil {
		t.Fatalf("VerifyBinaryChecksums failed: %v", err)
	}
	if len(mismatches) != 1 {
		t.Fatalf("expected 1 mismatch, got %d", len(mismatches))
	}

	if mismatches[0].Path != "bin/tool" {
		t.Errorf("unexpected path: %s", mismatches[0].Path)
	}
	if mismatches[0].Error != nil {
		t.Errorf("unexpected error: %v", mismatches[0].Error)
	}
	if mismatches[0].Expected == "" {
		t.Error("expected checksum should not be empty")
	}
	if mismatches[0].Actual == "" {
		t.Error("actual checksum should not be empty")
	}
	if mismatches[0].Expected == mismatches[0].Actual {
		t.Error("expected and actual should be different")
	}
}

func TestVerifyBinaryChecksums_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stored checksums for a file that doesn't exist
	stored := map[string]string{
		"bin/missing": "abc123",
	}

	mismatches, err := VerifyBinaryChecksums(tmpDir, stored)
	if err != nil {
		t.Fatalf("VerifyBinaryChecksums failed: %v", err)
	}
	if len(mismatches) != 1 {
		t.Fatalf("expected 1 mismatch, got %d", len(mismatches))
	}

	if mismatches[0].Path != "bin/missing" {
		t.Errorf("unexpected path: %s", mismatches[0].Path)
	}
	if mismatches[0].Error == nil {
		t.Error("expected error for missing file")
	}
}

func TestVerifyBinaryChecksums_EmptyStored(t *testing.T) {
	tmpDir := t.TempDir()

	mismatches, err := VerifyBinaryChecksums(tmpDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mismatches != nil {
		t.Error("expected nil for empty stored checksums")
	}

	mismatches, err = VerifyBinaryChecksums(tmpDir, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mismatches != nil {
		t.Error("expected nil for empty stored checksums")
	}
}

func TestIsWithinDir(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		dir    string
		expect bool
	}{
		{
			name:   "path within dir",
			path:   "/home/user/tools/jq-1.7/bin/jq",
			dir:    "/home/user/tools/jq-1.7",
			expect: true,
		},
		{
			name:   "path equals dir",
			path:   "/home/user/tools/jq-1.7",
			dir:    "/home/user/tools/jq-1.7",
			expect: true,
		},
		{
			name:   "path outside dir",
			path:   "/home/user/other/file",
			dir:    "/home/user/tools/jq-1.7",
			expect: false,
		},
		{
			name:   "path in parent dir",
			path:   "/home/user/tools",
			dir:    "/home/user/tools/jq-1.7",
			expect: false,
		},
		{
			name:   "path with traversal attempt",
			path:   "/home/user/tools/jq-1.7/../other/file",
			dir:    "/home/user/tools/jq-1.7",
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isWithinDir(tt.path, tt.dir)
			if got != tt.expect {
				t.Errorf("isWithinDir(%q, %q) = %v, want %v", tt.path, tt.dir, got, tt.expect)
			}
		})
	}
}
