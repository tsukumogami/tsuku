package actions

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestIsPathWithinDirectory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		targetPath string
		basePath   string
		expected   bool
	}{
		{
			name:       "path within directory",
			targetPath: "/tmp/extract/file.txt",
			basePath:   "/tmp/extract",
			expected:   true,
		},
		{
			name:       "path is directory itself",
			targetPath: "/tmp/extract",
			basePath:   "/tmp/extract",
			expected:   true,
		},
		{
			name:       "path outside directory",
			targetPath: "/tmp/other/file.txt",
			basePath:   "/tmp/extract",
			expected:   false,
		},
		{
			name:       "path traversal attempt",
			targetPath: "/tmp/extract/../other/file.txt",
			basePath:   "/tmp/extract",
			expected:   false,
		},
		{
			name:       "nested path within",
			targetPath: "/tmp/extract/sub/dir/file.txt",
			basePath:   "/tmp/extract",
			expected:   true,
		},
		{
			name:       "similar prefix but different dir",
			targetPath: "/tmp/extract-other/file.txt",
			basePath:   "/tmp/extract",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPathWithinDirectory(tt.targetPath, tt.basePath)
			if result != tt.expected {
				t.Errorf("isPathWithinDirectory(%q, %q) = %v, want %v",
					tt.targetPath, tt.basePath, result, tt.expected)
			}
		})
	}
}

func TestValidateSymlinkTarget(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	tests := []struct {
		name         string
		linkTarget   string
		linkLocation string
		destPath     string
		shouldError  bool
	}{
		{
			name:         "relative symlink within directory",
			linkTarget:   "../lib/libfoo.so",
			linkLocation: filepath.Join(tmpDir, "bin", "foo"),
			destPath:     tmpDir,
			shouldError:  false,
		},
		{
			name:         "absolute symlink - rejected",
			linkTarget:   "/etc/passwd",
			linkLocation: filepath.Join(tmpDir, "link"),
			destPath:     tmpDir,
			shouldError:  true,
		},
		{
			name:         "relative symlink escaping directory",
			linkTarget:   "../../../../../../etc/passwd",
			linkLocation: filepath.Join(tmpDir, "bin", "foo"),
			destPath:     tmpDir,
			shouldError:  true,
		},
		{
			name:         "same directory symlink",
			linkTarget:   "other-file",
			linkLocation: filepath.Join(tmpDir, "link"),
			destPath:     tmpDir,
			shouldError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSymlinkTarget(tt.linkTarget, tt.linkLocation, tt.destPath)
			if tt.shouldError && err == nil {
				t.Error("validateSymlinkTarget should have returned error")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("validateSymlinkTarget returned unexpected error: %v", err)
			}
		})
	}
}

func TestExtractAction_DetectFormat(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}

	tests := []struct {
		filename string
		expected string
	}{
		{"file.tar.gz", "tar.gz"},
		{"file.tgz", "tar.gz"},
		{"file.tar.xz", "tar.xz"},
		{"file.txz", "tar.xz"},
		{"file.tar.bz2", "tar.bz2"},
		{"file.tbz2", "tar.bz2"},
		{"file.tbz", "tar.bz2"},
		{"file.tar", "tar"},
		{"file.zip", "zip"},
		{"file.unknown", "unknown"},
		{"FILE.TAR.GZ", "tar.gz"}, // case insensitive
		{"FILE.ZIP", "zip"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := action.detectFormat(tt.filename)
			if result != tt.expected {
				t.Errorf("detectFormat(%q) = %q, want %q", tt.filename, result, tt.expected)
			}
		})
	}
}

func TestExtractAction_Execute_MissingArchive(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	// Missing 'archive' parameter
	err := action.Execute(ctx, map[string]interface{}{
		"format": "tar.gz",
	})
	if err == nil {
		t.Error("Execute should fail when 'archive' parameter is missing")
	}
}

func TestExtractAction_Execute_MissingFormat(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	// Missing 'format' parameter
	err := action.Execute(ctx, map[string]interface{}{
		"archive": "test.tar.gz",
	})
	if err == nil {
		t.Error("Execute should fail when 'format' parameter is missing")
	}
}

func TestExtractAction_Execute_UnsupportedFormat(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}
	tmpDir := t.TempDir()

	// Create a dummy file
	dummyFile := filepath.Join(tmpDir, "test.rar")
	if err := os.WriteFile(dummyFile, []byte("dummy"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"archive": "test.rar",
		"format":  "rar",
	})
	if err == nil {
		t.Error("Execute should fail for unsupported format")
	}
}

func TestExtractAction_ExtractTarGz(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}
	tmpDir := t.TempDir()

	// Create a tar.gz archive in memory
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	// Add a file to the archive
	content := []byte("test content")
	hdr := &tar.Header{
		Name: "testdir/testfile.txt",
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}

	_ = tw.Close()
	_ = gzw.Close()

	// Write archive to file
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	// Extract
	destPath := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(destPath, 0755); err != nil {
		t.Fatal(err)
	}

	if err := action.extractTarGz(archivePath, destPath, 0, nil); err != nil {
		t.Fatalf("extractTarGz failed: %v", err)
	}

	// Verify extraction
	extractedFile := filepath.Join(destPath, "testdir", "testfile.txt")
	extractedContent, err := os.ReadFile(extractedFile)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}
	if string(extractedContent) != "test content" {
		t.Errorf("Extracted content = %q, want %q", string(extractedContent), "test content")
	}
}

func TestExtractAction_ExtractTarGz_StripDirs(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}
	tmpDir := t.TempDir()

	// Create a tar.gz archive with nested directory
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	content := []byte("stripped content")
	hdr := &tar.Header{
		Name: "top/middle/file.txt",
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}

	_ = tw.Close()
	_ = gzw.Close()

	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	destPath := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(destPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Extract with strip_dirs=1
	if err := action.extractTarGz(archivePath, destPath, 1, nil); err != nil {
		t.Fatalf("extractTarGz with strip_dirs failed: %v", err)
	}

	// Should be at middle/file.txt (stripped "top")
	extractedFile := filepath.Join(destPath, "middle", "file.txt")
	if _, err := os.Stat(extractedFile); os.IsNotExist(err) {
		t.Errorf("Expected file at %s after stripping 1 dir", extractedFile)
	}
}

func TestExtractAction_ExtractZip(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}
	tmpDir := t.TempDir()

	// Create a zip archive
	archivePath := filepath.Join(tmpDir, "test.zip")
	zipFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	zw := zip.NewWriter(zipFile)
	content := []byte("zip content")

	fw, err := zw.Create("zipdir/zipfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}

	_ = zw.Close()
	_ = zipFile.Close()

	// Extract
	destPath := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(destPath, 0755); err != nil {
		t.Fatal(err)
	}

	if err := action.extractZip(archivePath, destPath, 0, nil); err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}

	// Verify extraction
	extractedFile := filepath.Join(destPath, "zipdir", "zipfile.txt")
	extractedContent, err := os.ReadFile(extractedFile)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}
	if string(extractedContent) != "zip content" {
		t.Errorf("Extracted content = %q, want %q", string(extractedContent), "zip content")
	}
}

func TestAtomicSymlink(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a target file
	targetFile := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("target"), 0644); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(tmpDir, "link.txt")

	// Create symlink atomically
	if err := atomicSymlink("target.txt", linkPath); err != nil {
		t.Fatalf("atomicSymlink failed: %v", err)
	}

	// Verify symlink exists and points to correct target
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}
	if target != "target.txt" {
		t.Errorf("Symlink target = %q, want %q", target, "target.txt")
	}

	// Test replacing existing symlink
	if err := atomicSymlink("other.txt", linkPath); err != nil {
		t.Fatalf("atomicSymlink replace failed: %v", err)
	}

	target, err = os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Failed to read replaced symlink: %v", err)
	}
	if target != "other.txt" {
		t.Errorf("Replaced symlink target = %q, want %q", target, "other.txt")
	}
}

// TestExtractTar_PathTraversal_SecurityEdgeCases tests comprehensive path traversal attack vectors
func TestExtractTar_PathTraversal_SecurityEdgeCases(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}

	tests := []struct {
		name      string
		filename  string
		shouldErr bool
	}{
		// Classic path traversal patterns
		{"basic traversal", "../../../etc/passwd", true},
		{"deeply nested traversal", "../../../../../../../../../../tmp/evil", true},

		// Traversal with encoded sequences (after cleaning)
		{"traversal in middle", "foo/../../../bar", true},
		{"traversal at end", "foo/bar/../../../..", true},

		// Traversal with current directory
		{"dot current with traversal", "./../../etc/passwd", true},

		// Note: Absolute paths like "/etc/passwd" become "etc/passwd" after Join
		// This is actually safe behavior - they end up within the dest dir
		{"absolute path becomes relative", "/etc/passwd", false},

		// Valid relative paths (should work)
		{"simple file", "file.txt", false},
		{"nested file", "dir/subdir/file.txt", false},
		{"deep nesting", "a/b/c/d/e/f/g.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create a tar.gz archive with the malicious path
			var buf bytes.Buffer
			gzw := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gzw)

			content := []byte("malicious content")
			hdr := &tar.Header{
				Name: tt.filename,
				Mode: 0644,
				Size: int64(len(content)),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatalf("Failed to write tar header: %v", err)
			}
			if _, err := tw.Write(content); err != nil {
				t.Fatalf("Failed to write tar content: %v", err)
			}

			_ = tw.Close()
			_ = gzw.Close()

			archivePath := filepath.Join(tmpDir, "test.tar.gz")
			if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
				t.Fatal(err)
			}

			destPath := filepath.Join(tmpDir, "extracted")
			if err := os.MkdirAll(destPath, 0755); err != nil {
				t.Fatal(err)
			}

			err := action.extractTarGz(archivePath, destPath, 0, nil)

			if tt.shouldErr {
				if err == nil {
					t.Error("Expected error for path traversal attempt, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for valid path: %v", err)
				}
			}
		})
	}
}

// TestExtractZip_PathTraversal_SecurityEdgeCases tests zip-specific path traversal attacks
func TestExtractZip_PathTraversal_SecurityEdgeCases(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}

	tests := []struct {
		name      string
		filename  string
		shouldErr bool
	}{
		// Classic path traversal patterns
		{"basic traversal", "../../../etc/passwd", true},
		{"deeply nested traversal", "../../../../../../../../../../tmp/evil", true},

		// Traversal with current directory
		{"dot current with traversal", "./../../etc/passwd", true},

		// Note: Absolute paths in zip become relative after Join - safe behavior
		{"absolute path becomes relative", "/etc/passwd", false},

		// Valid relative paths (should work)
		{"simple file", "file.txt", false},
		{"nested file", "dir/subdir/file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create a zip archive with the potentially malicious path
			archivePath := filepath.Join(tmpDir, "test.zip")
			zipFile, err := os.Create(archivePath)
			if err != nil {
				t.Fatal(err)
			}

			zw := zip.NewWriter(zipFile)
			content := []byte("zip content")

			fw, err := zw.Create(tt.filename)
			if err != nil {
				// Some filenames might fail at create time
				_ = zw.Close()
				_ = zipFile.Close()
				// This is also a valid security behavior - failing early
				return
			}
			if _, err := fw.Write(content); err != nil {
				t.Fatal(err)
			}

			_ = zw.Close()
			_ = zipFile.Close()

			destPath := filepath.Join(tmpDir, "extracted")
			if err := os.MkdirAll(destPath, 0755); err != nil {
				t.Fatal(err)
			}

			err = action.extractZip(archivePath, destPath, 0, nil)

			if tt.shouldErr {
				if err == nil {
					t.Error("Expected error for path traversal attempt, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for valid path: %v", err)
				}
			}
		})
	}
}

// TestExtractTar_SymlinkAttacks_SecurityEdgeCases tests extended symlink attack scenarios
func TestExtractTar_SymlinkAttacks_SecurityEdgeCases(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}

	tests := []struct {
		name       string
		linkName   string
		linkTarget string
		shouldErr  bool
	}{
		// Absolute symlink targets (should be blocked)
		{"absolute symlink to root", "link", "/", true},
		{"absolute symlink to etc", "link", "/etc/passwd", true},
		{"absolute symlink to tmp", "link", "/tmp/evil", true},

		// Relative symlink escapes (should be blocked)
		{"escape via parent", "link", "../../../etc/passwd", true},
		{"deep escape", "nested/dir/link", "../../../../../../../../tmp/evil", true},

		// Self-referential symlinks (could cause loops)
		{"self-reference", "link", "link", false}, // Not an escape, just a loop
		{"cyclic a->b", "a", "b", false},          // Would need b->a for cycle

		// Valid relative symlinks within archive (should work)
		{"same dir symlink", "link", "target.txt", false},
		{"sibling dir symlink", "bin/link", "../lib/libfoo.so", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create a tar.gz archive with symlink
			var buf bytes.Buffer
			gzw := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gzw)

			// First add a regular file to reference
			content := []byte("target content")
			regHdr := &tar.Header{
				Name: "target.txt",
				Mode: 0644,
				Size: int64(len(content)),
			}
			if err := tw.WriteHeader(regHdr); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write(content); err != nil {
				t.Fatal(err)
			}

			// Add the symlink
			linkHdr := &tar.Header{
				Name:     tt.linkName,
				Mode:     0777,
				Typeflag: tar.TypeSymlink,
				Linkname: tt.linkTarget,
			}
			if err := tw.WriteHeader(linkHdr); err != nil {
				t.Fatal(err)
			}

			_ = tw.Close()
			_ = gzw.Close()

			archivePath := filepath.Join(tmpDir, "test.tar.gz")
			if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
				t.Fatal(err)
			}

			destPath := filepath.Join(tmpDir, "extracted")
			if err := os.MkdirAll(destPath, 0755); err != nil {
				t.Fatal(err)
			}

			err := action.extractTarGz(archivePath, destPath, 0, nil)

			if tt.shouldErr {
				if err == nil {
					t.Error("Expected error for symlink attack, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for valid symlink: %v", err)
				}
			}
		})
	}
}
