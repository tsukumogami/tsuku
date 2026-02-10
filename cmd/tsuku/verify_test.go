package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/verify"
)

func TestIsSharedLibrary(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		// Linux shared objects
		{"simple .so", "libfoo.so", true},
		{"versioned .so.1", "libfoo.so.1", true},
		{"versioned .so.1.2", "libfoo.so.1.2", true},
		{"versioned .so.1.2.3", "libfoo.so.1.2.3", true},
		{"long version", "libz.so.1.3.1", true},

		// macOS dynamic libraries
		{"simple .dylib", "libfoo.dylib", true},
		{"versioned dylib", "libfoo.1.dylib", true},

		// Non-libraries
		{"static library .a", "libfoo.a", false},
		{"object file .o", "foo.o", false},
		{"header file", "foo.h", false},
		{"text file", "README.txt", false},
		{"no extension", "libfoo", false},
		{".so in middle", "libfoo.source.c", false},
		{".so suffix with letters", "libfoo.something", false},

		// Edge cases
		{"just .so", ".so", true},
		{"just .dylib", ".dylib", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSharedLibrary(tt.filename)
			if got != tt.want {
				t.Errorf("isSharedLibrary(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestFindLibraryFiles(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Create lib/ subdirectory (common library layout)
	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	// Test files to create:
	// libfoo.so:       should be found (real file)
	// libfoo.so.1:     symlink to libfoo.so.1.2.3 (should be skipped)
	// libfoo.so.1.2:   symlink to libfoo.so.1.2.3 (should be skipped)
	// libfoo.so.1.2.3: should be found (real file)
	// libbar.a:        static library (should not be found)
	// foo.h:           header file (should not be found)

	// Create the real file first
	realFile := filepath.Join(libDir, "libfoo.so.1.2.3")
	if err := os.WriteFile(realFile, []byte("dummy"), 0644); err != nil {
		t.Fatalf("failed to create real file: %v", err)
	}

	// Create symlinks
	for _, link := range []string{"libfoo.so.1.2", "libfoo.so.1"} {
		linkPath := filepath.Join(libDir, link)
		// Link to the next version (e.g., .1 -> .1.2, .1.2 -> .1.2.3)
		target := "libfoo.so.1.2.3"
		if link == "libfoo.so.1.2" {
			target = "libfoo.so.1.2.3"
		}
		if err := os.Symlink(target, linkPath); err != nil {
			t.Fatalf("failed to create symlink %s: %v", link, err)
		}
	}

	// Create another standalone .so file
	if err := os.WriteFile(filepath.Join(libDir, "libfoo.so"), []byte("dummy"), 0644); err != nil {
		t.Fatalf("failed to create libfoo.so: %v", err)
	}

	// Create non-library files
	if err := os.WriteFile(filepath.Join(libDir, "libbar.a"), []byte("dummy"), 0644); err != nil {
		t.Fatalf("failed to create libbar.a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "foo.h"), []byte("dummy"), 0644); err != nil {
		t.Fatalf("failed to create foo.h: %v", err)
	}

	// Run findLibraryFiles
	found, err := findLibraryFiles(tmpDir)
	if err != nil {
		t.Fatalf("findLibraryFiles failed: %v", err)
	}

	// Should find exactly 2 files: libfoo.so and libfoo.so.1.2.3
	// (symlinks are resolved and deduplicated)
	if len(found) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(found), found)
	}

	// Verify the found files
	foundMap := make(map[string]bool)
	for _, f := range found {
		foundMap[filepath.Base(f)] = true
	}

	if !foundMap["libfoo.so"] {
		t.Error("expected to find libfoo.so")
	}
	if !foundMap["libfoo.so.1.2.3"] {
		t.Error("expected to find libfoo.so.1.2.3")
	}
	if foundMap["libbar.a"] {
		t.Error("should not have found libbar.a")
	}
	if foundMap["foo.h"] {
		t.Error("should not have found foo.h")
	}
}

func TestFindLibraryFiles_Empty(t *testing.T) {
	tmpDir := t.TempDir()

	found, err := findLibraryFiles(tmpDir)
	if err != nil {
		t.Fatalf("findLibraryFiles failed: %v", err)
	}

	if len(found) != 0 {
		t.Errorf("expected 0 files in empty dir, got %d", len(found))
	}
}

func TestFindLibraryFiles_BrokenSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a broken symlink
	if err := os.Symlink("nonexistent.so", filepath.Join(tmpDir, "broken.so")); err != nil {
		t.Fatalf("failed to create broken symlink: %v", err)
	}

	// Should not error, just skip the broken symlink
	found, err := findLibraryFiles(tmpDir)
	if err != nil {
		t.Fatalf("findLibraryFiles failed: %v", err)
	}

	if len(found) != 0 {
		t.Errorf("expected 0 files (broken symlink skipped), got %d", len(found))
	}
}

func TestCheckDependencyResults_Empty(t *testing.T) {
	// Empty results should pass (statically linked)
	result := checkDependencyResults(nil, false)
	if !result {
		t.Error("expected empty results to pass")
	}
}

func TestCheckDependencyResults_AllPass(t *testing.T) {
	results := []verify.DepResult{
		{
			Soname:   "libfoo.so.1",
			Category: verify.DepPureSystem,
			Status:   verify.ValidationPass,
		},
		{
			Soname:   "libbar.so.2",
			Category: verify.DepTsukuManaged,
			Status:   verify.ValidationPass,
			Recipe:   "bar",
			Version:  "1.0.0",
		},
	}

	result := checkDependencyResults(results, false)
	if !result {
		t.Error("expected all passing results to return true")
	}
}

func TestCheckDependencyResults_OneFail(t *testing.T) {
	results := []verify.DepResult{
		{
			Soname:   "libfoo.so.1",
			Category: verify.DepPureSystem,
			Status:   verify.ValidationPass,
		},
		{
			Soname:   "libmissing.so.1",
			Category: verify.DepPureSystem,
			Status:   verify.ValidationFail,
			Error:    "not found",
		},
	}

	result := checkDependencyResults(results, false)
	if result {
		t.Error("expected failing dependency to return false")
	}
}

func TestCheckDependencyResults_TransitiveFail(t *testing.T) {
	results := []verify.DepResult{
		{
			Soname:   "libfoo.so.1",
			Category: verify.DepTsukuManaged,
			Status:   verify.ValidationPass,
			Recipe:   "foo",
			Version:  "1.0.0",
			Transitive: []verify.DepResult{
				{
					Soname:   "libbaz.so.1",
					Category: verify.DepPureSystem,
					Status:   verify.ValidationFail,
					Error:    "not found",
				},
			},
		},
	}

	result := checkDependencyResults(results, false)
	if result {
		t.Error("expected failing transitive dependency to return false")
	}
}

func TestVerifyBinaryIntegrityInternal_NoChecksums(t *testing.T) {
	// When no checksums are stored (pre-feature installation), should pass
	versionState := &install.VersionState{
		BinaryChecksums: nil,
	}

	err := verifyBinaryIntegrityInternal("/nonexistent", versionState, false)
	if err != nil {
		t.Errorf("expected nil error for no checksums, got %v", err)
	}
}

func TestVerifyBinaryIntegrityInternal_AllMatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test binary
	binPath := filepath.Join(tmpDir, "bin", "test")
	if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		t.Fatal(err)
	}
	content := []byte("test binary content")
	if err := os.WriteFile(binPath, content, 0755); err != nil {
		t.Fatal(err)
	}

	// Compute checksum
	checksums, err := install.ComputeBinaryChecksums(tmpDir, []string{"bin/test"})
	if err != nil {
		t.Fatal(err)
	}

	versionState := &install.VersionState{
		BinaryChecksums: checksums,
	}

	err = verifyBinaryIntegrityInternal(tmpDir, versionState, false)
	if err != nil {
		t.Errorf("expected nil error for matching checksums, got %v", err)
	}
}

func TestVerifyBinaryIntegrityInternal_Mismatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test binary
	binPath := filepath.Join(tmpDir, "bin", "test")
	if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binPath, []byte("original"), 0755); err != nil {
		t.Fatal(err)
	}

	// Compute checksum of original
	checksums, err := install.ComputeBinaryChecksums(tmpDir, []string{"bin/test"})
	if err != nil {
		t.Fatal(err)
	}

	// Modify the binary
	if err := os.WriteFile(binPath, []byte("modified"), 0755); err != nil {
		t.Fatal(err)
	}

	versionState := &install.VersionState{
		BinaryChecksums: checksums,
	}

	err = verifyBinaryIntegrityInternal(tmpDir, versionState, false)
	if err == nil {
		t.Error("expected error for mismatched checksums")
	}
}
