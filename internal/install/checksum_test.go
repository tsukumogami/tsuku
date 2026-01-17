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

func TestComputeLibraryChecksums(t *testing.T) {
	// Create temp directory with test files
	tmpDir := t.TempDir()

	// Create nested directory structure like a real library
	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	// Create test library files
	file1 := filepath.Join(libDir, "libtest.so.1")
	file2 := filepath.Join(libDir, "libtest.so.1.0.0")
	if err := os.WriteFile(file1, []byte("library content 1"), 0644); err != nil {
		t.Fatalf("failed to create file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("library content 2"), 0644); err != nil {
		t.Fatalf("failed to create file2: %v", err)
	}

	// Compute checksums
	checksums, err := ComputeLibraryChecksums(tmpDir)
	if err != nil {
		t.Fatalf("ComputeLibraryChecksums failed: %v", err)
	}

	// Verify we got checksums for both files
	if len(checksums) != 2 {
		t.Errorf("expected 2 checksums, got %d", len(checksums))
	}

	// Verify relative paths are used as keys
	expectedKeys := []string{
		filepath.Join("lib", "libtest.so.1"),
		filepath.Join("lib", "libtest.so.1.0.0"),
	}
	for _, key := range expectedKeys {
		if _, ok := checksums[key]; !ok {
			t.Errorf("missing checksum for %s", key)
		}
	}

	// Verify checksums are valid SHA256 hex strings (64 characters)
	for path, checksum := range checksums {
		if len(checksum) != 64 {
			t.Errorf("checksum for %s has invalid length: got %d, want 64", path, len(checksum))
		}
	}
}

func TestComputeLibraryChecksums_WithSymlinks(t *testing.T) {
	// Create temp directory with test files and symlinks
	tmpDir := t.TempDir()

	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	// Create real file
	realFile := filepath.Join(libDir, "libtest.so.1.0.0")
	if err := os.WriteFile(realFile, []byte("real library content"), 0644); err != nil {
		t.Fatalf("failed to create real file: %v", err)
	}

	// Create symlink pointing to real file
	symlink := filepath.Join(libDir, "libtest.so.1")
	if err := os.Symlink("libtest.so.1.0.0", symlink); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Create another symlink (version-less)
	symlink2 := filepath.Join(libDir, "libtest.so")
	if err := os.Symlink("libtest.so.1", symlink2); err != nil {
		t.Fatalf("failed to create symlink2: %v", err)
	}

	// Compute checksums
	checksums, err := ComputeLibraryChecksums(tmpDir)
	if err != nil {
		t.Fatalf("ComputeLibraryChecksums failed: %v", err)
	}

	// Should only have checksum for the real file, symlinks should be skipped
	if len(checksums) != 1 {
		t.Errorf("expected 1 checksum (symlinks should be skipped), got %d", len(checksums))
	}

	// Verify only the real file has a checksum
	realFileKey := filepath.Join("lib", "libtest.so.1.0.0")
	if _, ok := checksums[realFileKey]; !ok {
		t.Errorf("missing checksum for real file %s", realFileKey)
	}

	// Verify symlinks are not in the map
	symlinkKey := filepath.Join("lib", "libtest.so.1")
	if _, ok := checksums[symlinkKey]; ok {
		t.Errorf("symlink %s should not have a checksum", symlinkKey)
	}
}

func TestComputeLibraryChecksums_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	checksums, err := ComputeLibraryChecksums(tmpDir)
	if err != nil {
		t.Fatalf("ComputeLibraryChecksums failed: %v", err)
	}

	if len(checksums) != 0 {
		t.Errorf("expected 0 checksums for empty directory, got %d", len(checksums))
	}
}

func TestComputeLibraryChecksums_NestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested structure: lib/subdir/file
	nestedDir := filepath.Join(tmpDir, "lib", "pkgconfig")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	// Create files at different levels
	topFile := filepath.Join(tmpDir, "lib", "libtest.so")
	nestedFile := filepath.Join(nestedDir, "test.pc")
	if err := os.WriteFile(topFile, []byte("top level"), 0644); err != nil {
		t.Fatalf("failed to create top file: %v", err)
	}
	if err := os.WriteFile(nestedFile, []byte("nested file"), 0644); err != nil {
		t.Fatalf("failed to create nested file: %v", err)
	}

	// Compute checksums
	checksums, err := ComputeLibraryChecksums(tmpDir)
	if err != nil {
		t.Fatalf("ComputeLibraryChecksums failed: %v", err)
	}

	// Verify we got checksums for both files
	if len(checksums) != 2 {
		t.Errorf("expected 2 checksums, got %d", len(checksums))
	}

	// Verify relative paths include nested structure
	expectedKeys := []string{
		filepath.Join("lib", "libtest.so"),
		filepath.Join("lib", "pkgconfig", "test.pc"),
	}
	for _, key := range expectedKeys {
		if _, ok := checksums[key]; !ok {
			t.Errorf("missing checksum for %s", key)
		}
	}
}

func TestComputeLibraryChecksums_NonexistentDirectory(t *testing.T) {
	_, err := ComputeLibraryChecksums("/nonexistent/directory")
	if err == nil {
		t.Error("expected error for nonexistent directory, got nil")
	}
}

func TestVerifyLibraryChecksums_AllMatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	file1 := filepath.Join(libDir, "libtest.so.1")
	if err := os.WriteFile(file1, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Compute checksums
	checksums, err := ComputeLibraryChecksums(tmpDir)
	if err != nil {
		t.Fatalf("ComputeLibraryChecksums failed: %v", err)
	}

	// Verify - should have no mismatches
	mismatches, err := VerifyLibraryChecksums(tmpDir, checksums)
	if err != nil {
		t.Fatalf("VerifyLibraryChecksums failed: %v", err)
	}
	if len(mismatches) != 0 {
		t.Errorf("expected no mismatches, got %d", len(mismatches))
	}
}

func TestVerifyLibraryChecksums_Mismatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	file1 := filepath.Join(libDir, "libtest.so.1")
	if err := os.WriteFile(file1, []byte("original content"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Compute checksums
	checksums, err := ComputeLibraryChecksums(tmpDir)
	if err != nil {
		t.Fatalf("ComputeLibraryChecksums failed: %v", err)
	}

	// Modify the file
	if err := os.WriteFile(file1, []byte("modified content"), 0644); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	// Verify - should detect mismatch
	mismatches, err := VerifyLibraryChecksums(tmpDir, checksums)
	if err != nil {
		t.Fatalf("VerifyLibraryChecksums failed: %v", err)
	}
	if len(mismatches) != 1 {
		t.Fatalf("expected 1 mismatch, got %d", len(mismatches))
	}

	expectedPath := filepath.Join("lib", "libtest.so.1")
	if mismatches[0].Path != expectedPath {
		t.Errorf("unexpected path: got %s, want %s", mismatches[0].Path, expectedPath)
	}
	if mismatches[0].Error != nil {
		t.Errorf("unexpected error: %v", mismatches[0].Error)
	}
	if mismatches[0].Expected == mismatches[0].Actual {
		t.Error("expected and actual checksums should be different")
	}
}

func TestVerifyLibraryChecksums_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stored checksums for a file that doesn't exist
	stored := map[string]string{
		"lib/missing.so": "abc123",
	}

	mismatches, err := VerifyLibraryChecksums(tmpDir, stored)
	if err != nil {
		t.Fatalf("VerifyLibraryChecksums failed: %v", err)
	}
	if len(mismatches) != 1 {
		t.Fatalf("expected 1 mismatch, got %d", len(mismatches))
	}

	if mismatches[0].Path != "lib/missing.so" {
		t.Errorf("unexpected path: %s", mismatches[0].Path)
	}
	if mismatches[0].Error == nil {
		t.Error("expected error for missing file")
	}
}

func TestVerifyLibraryChecksums_EmptyStored(t *testing.T) {
	tmpDir := t.TempDir()

	mismatches, err := VerifyLibraryChecksums(tmpDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mismatches != nil {
		t.Error("expected nil for empty stored checksums")
	}

	mismatches, err = VerifyLibraryChecksums(tmpDir, map[string]string{})
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
