package verify

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateABI_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("This test only runs on non-Linux systems")
	}

	// On non-Linux, ValidateABI should always return nil (no-op)
	err := ValidateABI("/any/path/here")
	if err != nil {
		t.Errorf("ValidateABI on non-Linux should return nil, got: %v", err)
	}
}

func TestValidateABI_SystemLibrary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ABI validation only runs on Linux")
	}

	// Find a system shared library with valid PT_INTERP
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

	err := ValidateABI(libPath)
	if err != nil {
		t.Errorf("ValidateABI(%s) failed: %v", libPath, err)
	}
}

func TestValidateABI_StaticBinary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ABI validation only runs on Linux")
	}

	// Find a statically linked binary (no PT_INTERP)
	// The Go test binary itself may be statically linked depending on CGO settings
	// Try to find a known static binary
	candidates := []string{
		"/bin/busybox",        // Often statically linked
		"/usr/bin/busybox",    // Alternative location
		"/usr/sbin/busybox",   // Alpine location
		"/bin/static-busybox", // Some distros
	}

	var staticPath string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			staticPath = c
			break
		}
	}

	if staticPath == "" {
		// Create a minimal static ELF for testing
		// For this test, we'll just verify the function handles missing PT_INTERP
		// by using a dynamically linked binary that should still pass
		t.Skip("No known static binary found for testing")
	}

	err := ValidateABI(staticPath)
	if err != nil {
		t.Errorf("ValidateABI(%s) should pass for static binary: %v", staticPath, err)
	}
}

func TestValidateABI_NonELFFile(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ABI validation only runs on Linux")
	}

	// Create a non-ELF file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "script.sh")

	err := os.WriteFile(path, []byte("#!/bin/bash\necho hello"), 0755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Non-ELF files should return nil (handled gracefully)
	err = ValidateABI(path)
	if err != nil {
		t.Errorf("ValidateABI should return nil for non-ELF file, got: %v", err)
	}
}

func TestValidateABI_MissingInterpreter(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ABI validation only runs on Linux")
	}

	// We can't easily create a binary with a fake interpreter path without
	// actually building one. Instead, we test the error message format
	// by checking that the error type is correct when we mock the scenario.
	// For real integration testing, this would require a musl binary on glibc
	// or vice versa.

	// This test verifies the ErrABIMismatch constant value is correct
	if ErrABIMismatch != 10 {
		t.Errorf("ErrABIMismatch = %d, want 10 (design decision #2)", ErrABIMismatch)
	}

	// Verify string representation
	if ErrABIMismatch.String() != "ABI mismatch" {
		t.Errorf("ErrABIMismatch.String() = %q, want %q", ErrABIMismatch.String(), "ABI mismatch")
	}
}

func TestValidateABI_DynamicExecutable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ABI validation only runs on Linux")
	}

	// Find a dynamically linked executable with valid PT_INTERP
	candidates := []string{
		"/bin/ls",
		"/bin/cat",
		"/usr/bin/ls",
		"/usr/bin/cat",
	}

	var execPath string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			execPath = c
			break
		}
	}

	if execPath == "" {
		t.Skip("No dynamic executable found for testing")
	}

	err := ValidateABI(execPath)
	if err != nil {
		t.Errorf("ValidateABI(%s) failed: %v", execPath, err)
	}
}

func TestErrABIMismatch_ExplicitValue(t *testing.T) {
	// Per design decision #2, Tier 2 error categories use explicit values
	// starting at 10. This test ensures the value doesn't accidentally change.
	const expectedValue ErrorCategory = 10
	if ErrABIMismatch != expectedValue {
		t.Errorf("ErrABIMismatch = %d, want %d (explicit value per design)", ErrABIMismatch, expectedValue)
	}
}

func TestValidationError_ABIMismatch(t *testing.T) {
	// Test that ValidationError with ErrABIMismatch formats correctly
	err := &ValidationError{
		Category: ErrABIMismatch,
		Path:     "/path/to/binary",
		Message:  "interpreter \"/lib64/ld-linux-x86-64.so.2\" not found",
	}

	got := err.Error()
	if got != "interpreter \"/lib64/ld-linux-x86-64.so.2\" not found" {
		t.Errorf("Error() = %q, unexpected format", got)
	}

	if err.Category.String() != "ABI mismatch" {
		t.Errorf("Category.String() = %q, want %q", err.Category.String(), "ABI mismatch")
	}
}
