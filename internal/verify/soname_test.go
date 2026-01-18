package verify

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExtractELFSoname(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ELF tests only run on Linux")
	}

	// Find a system shared library with a known soname
	candidates := []string{
		"/lib/x86_64-linux-gnu/libc.so.6",
		"/lib64/libc.so.6",
		"/usr/lib/libc.so.6",
		"/lib/aarch64-linux-gnu/libc.so.6",
	}

	var libPath string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			libPath = c
			break
		}
	}

	if libPath == "" {
		t.Skip("No system libc found for testing")
	}

	soname, err := ExtractELFSoname(libPath)
	if err != nil {
		t.Fatalf("ExtractELFSoname(%s) failed: %v", libPath, err)
	}

	// libc.so.6 should have soname "libc.so.6"
	if soname != "libc.so.6" {
		t.Errorf("ExtractELFSoname(%s) = %q, want %q", libPath, soname, "libc.so.6")
	}
}

func TestExtractELFSoname_NoSoname(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ELF tests only run on Linux")
	}

	// Test with a statically linked executable (no DT_SONAME)
	// The go binary itself is typically statically linked
	goPath, err := os.Executable()
	if err != nil {
		t.Skip("Cannot find current executable")
	}

	// This should return empty string, not error, since executables don't have DT_SONAME
	soname, err := ExtractELFSoname(goPath)
	if err != nil {
		t.Fatalf("ExtractELFSoname(%s) failed: %v", goPath, err)
	}

	if soname != "" {
		t.Errorf("ExtractELFSoname(%s) = %q, want empty string (executables have no soname)", goPath, soname)
	}
}

func TestExtractMachOInstallName(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}

	// On macOS Big Sur+, most system libraries are in the dyld cache
	// Try to find a library that exists on disk
	candidates := []string{
		"/usr/lib/libobjc.A.dylib",
		"/usr/lib/libSystem.B.dylib",
	}

	var libPath string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			libPath = c
			break
		}
	}

	if libPath == "" {
		t.Skip("No system dylib found on disk for testing (may be in dyld cache)")
	}

	installName, err := ExtractMachOInstallName(libPath)
	if err != nil {
		t.Fatalf("ExtractMachOInstallName(%s) failed: %v", libPath, err)
	}

	// Install name should be a path-like string
	if installName == "" {
		t.Errorf("ExtractMachOInstallName(%s) returned empty string, expected install name", libPath)
	}
}

func TestExtractSoname_FormatDetection(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Need Linux or macOS for binary format testing")
	}

	var libPath string

	if runtime.GOOS == "linux" {
		candidates := []string{
			"/lib/x86_64-linux-gnu/libc.so.6",
			"/lib64/libc.so.6",
			"/usr/lib/libc.so.6",
			"/lib/aarch64-linux-gnu/libc.so.6",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				libPath = c
				break
			}
		}
	} else if runtime.GOOS == "darwin" {
		candidates := []string{
			"/usr/lib/libobjc.A.dylib",
			"/usr/lib/libSystem.B.dylib",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				libPath = c
				break
			}
		}
	}

	if libPath == "" {
		t.Skip("No system library found for testing")
	}

	soname, err := ExtractSoname(libPath)
	if err != nil {
		t.Fatalf("ExtractSoname(%s) failed: %v", libPath, err)
	}

	// Should return non-empty soname for system libraries
	if soname == "" {
		t.Errorf("ExtractSoname(%s) returned empty string, expected soname/install name", libPath)
	}
}

func TestExtractSoname_InvalidFile(t *testing.T) {
	// Create a file with invalid content
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.so")

	err := os.WriteFile(path, []byte("not a binary file content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err = ExtractSoname(path)
	if err == nil {
		t.Errorf("ExtractSoname(%s) should have failed for non-binary file", path)
	}
}

func TestExtractSoname_NonexistentFile(t *testing.T) {
	_, err := ExtractSoname("/nonexistent/path/to/file.so")
	if err == nil {
		t.Error("ExtractSoname should fail for nonexistent file")
	}
}

func TestExtractSoname_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.so")

	err := os.WriteFile(path, []byte{}, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err = ExtractSoname(path)
	if err == nil {
		t.Errorf("ExtractSoname(%s) should have failed for empty file", path)
	}
}

func TestExtractSoname_Directory(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := ExtractSoname(tmpDir)
	if err == nil {
		t.Errorf("ExtractSoname(%s) should have failed for directory", tmpDir)
	}
}

func TestExtractSonames_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	sonames, err := ExtractSonames(tmpDir)
	if err != nil {
		t.Fatalf("ExtractSonames(%s) failed: %v", tmpDir, err)
	}

	if len(sonames) != 0 {
		t.Errorf("ExtractSonames(%s) = %v, want empty slice", tmpDir, sonames)
	}
}

func TestExtractSonames_MixedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some non-binary files that should be skipped
	err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("readme"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err = os.WriteFile(filepath.Join(tmpDir, "invalid.so"), []byte("not binary"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Should complete without error, returning empty list
	sonames, err := ExtractSonames(tmpDir)
	if err != nil {
		t.Fatalf("ExtractSonames(%s) failed: %v", tmpDir, err)
	}

	// All files are non-binary, so no sonames should be found
	if len(sonames) != 0 {
		t.Errorf("ExtractSonames(%s) = %v, want empty slice (all files are non-binary)", tmpDir, sonames)
	}
}

func TestExtractSonames_NonexistentDirectory(t *testing.T) {
	_, err := ExtractSonames("/nonexistent/directory/path")
	if err == nil {
		t.Error("ExtractSonames should fail for nonexistent directory")
	}
}

func TestExtractSonames_SystemLibDirectory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("System library directory test only runs on Linux")
	}

	// Test with a system library directory
	// Note: On minimal systems this directory might not exist or be empty
	candidates := []string{
		"/lib/x86_64-linux-gnu",
		"/lib64",
		"/usr/lib",
		"/lib/aarch64-linux-gnu",
	}

	var libDir string
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			libDir = c
			break
		}
	}

	if libDir == "" {
		t.Skip("No system library directory found")
	}

	sonames, err := ExtractSonames(libDir)
	if err != nil {
		t.Fatalf("ExtractSonames(%s) failed: %v", libDir, err)
	}

	// System lib directory should have at least one library with a soname
	if len(sonames) == 0 {
		t.Logf("No sonames found in %s (may be expected on minimal systems)", libDir)
	}

	// Check that all returned sonames are non-empty
	for _, soname := range sonames {
		if soname == "" {
			t.Errorf("ExtractSonames returned empty string in results")
		}
	}
}
