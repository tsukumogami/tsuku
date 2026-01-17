package verify

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateHeader_ELF_SharedObject(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ELF tests only run on Linux")
	}

	// Find a system shared library to test with
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

	info, err := ValidateHeader(libPath)
	if err != nil {
		t.Fatalf("ValidateHeader(%s) failed: %v", libPath, err)
	}

	if info.Format != "ELF" {
		t.Errorf("Format = %q, want %q", info.Format, "ELF")
	}
	if info.Type != "shared object" {
		t.Errorf("Type = %q, want %q", info.Type, "shared object")
	}
	if info.Architecture == "" {
		t.Error("Architecture is empty")
	}
	// libc should have dependencies (like ld-linux)
	// But we don't require it since some static-pie builds may not
}

func TestValidateHeader_MachO_Dylib(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O tests only run on macOS")
	}

	// On macOS Big Sur+, system libraries are in the dyld cache
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

	info, err := ValidateHeader(libPath)
	if err != nil {
		t.Fatalf("ValidateHeader(%s) failed: %v", libPath, err)
	}

	if info.Format != "Mach-O" {
		t.Errorf("Format = %q, want %q", info.Format, "Mach-O")
	}
	if info.Type != "dynamic library" && info.Type != "bundle" {
		t.Errorf("Type = %q, want dynamic library or bundle", info.Type)
	}
}

func TestValidateHeader_InvalidFormat(t *testing.T) {
	// Create a file with invalid magic bytes
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.so")

	err := os.WriteFile(path, []byte("not a binary file content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err = ValidateHeader(path)
	if err == nil {
		t.Fatal("Expected error for invalid format")
	}

	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Expected ValidationError, got %T", err)
	}
	if verr.Category != ErrInvalidFormat {
		t.Errorf("Category = %v, want %v", verr.Category, ErrInvalidFormat)
	}
}

func TestValidateHeader_Truncated(t *testing.T) {
	// Create a file with valid ELF magic but truncated
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "truncated.so")

	// ELF magic followed by truncated data
	data := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	err := os.WriteFile(path, data, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err = ValidateHeader(path)
	if err == nil {
		t.Fatal("Expected error for truncated file")
	}

	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Expected ValidationError, got %T", err)
	}
	// Could be truncated or corrupted depending on how parser handles it
	if verr.Category != ErrTruncated && verr.Category != ErrCorrupted {
		t.Errorf("Category = %v, want ErrTruncated or ErrCorrupted", verr.Category)
	}
}

func TestValidateHeader_StaticLibrary(t *testing.T) {
	// Create a file with ar archive magic (static library)
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "libfoo.a")

	// ar archive magic
	data := []byte("!<arch>\ntest content here")
	err := os.WriteFile(path, data, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err = ValidateHeader(path)
	if err == nil {
		t.Fatal("Expected error for static library")
	}

	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Expected ValidationError, got %T", err)
	}
	if verr.Category != ErrNotSharedLib {
		t.Errorf("Category = %v, want %v", verr.Category, ErrNotSharedLib)
	}
	if verr.Message == "" || verr.Message == verr.Category.String() {
		t.Error("Expected descriptive message for static library")
	}
}

func TestValidateHeader_NotFound(t *testing.T) {
	_, err := ValidateHeader("/nonexistent/path/to/library.so")
	if err == nil {
		t.Fatal("Expected error for nonexistent file")
	}

	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Expected ValidationError, got %T", err)
	}
	if verr.Category != ErrUnreadable {
		t.Errorf("Category = %v, want %v", verr.Category, ErrUnreadable)
	}
}

func TestValidateHeader_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.so")

	err := os.WriteFile(path, []byte{}, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err = ValidateHeader(path)
	if err == nil {
		t.Fatal("Expected error for empty file")
	}

	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Expected ValidationError, got %T", err)
	}
	if verr.Category != ErrInvalidFormat {
		t.Errorf("Category = %v, want %v", verr.Category, ErrInvalidFormat)
	}
}

func TestValidateHeader_Executable_NotSharedLib(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Executable test only runs on Linux")
	}

	// /bin/ls should be an executable, not a shared library
	// Note: On some systems, /bin/ls might be a PIE (position independent executable)
	// which is technically ET_DYN. We test with a statically linked binary if available.
	candidates := []string{
		"/bin/true",  // Often statically linked
		"/bin/false", // Often statically linked
	}

	var execPath string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			execPath = c
			break
		}
	}

	if execPath == "" {
		t.Skip("No suitable executable found for testing")
	}

	_, err := ValidateHeader(execPath)
	// If it fails with ErrNotSharedLib, that's correct
	// If it succeeds (PIE executable), that's also acceptable since PIE is ET_DYN
	if err != nil {
		verr, ok := err.(*ValidationError)
		if !ok {
			t.Fatalf("Expected ValidationError, got %T", err)
		}
		if verr.Category != ErrNotSharedLib {
			// Could be wrong arch on cross-compilation environment
			if verr.Category != ErrWrongArch {
				t.Errorf("Category = %v, want ErrNotSharedLib or ErrWrongArch", verr.Category)
			}
		}
	}
	// If no error, the executable is a PIE which is technically valid as ET_DYN
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name   string
		magic  []byte
		expect string
	}{
		{"ELF", []byte{0x7f, 'E', 'L', 'F', 0x00, 0x00, 0x00, 0x00}, "elf"},
		{"Mach-O 64", []byte{0xfe, 0xed, 0xfa, 0xcf, 0x00, 0x00, 0x00, 0x00}, "macho"},
		{"Mach-O 32", []byte{0xfe, 0xed, 0xfa, 0xce, 0x00, 0x00, 0x00, 0x00}, "macho"},
		{"Mach-O 64 rev", []byte{0xcf, 0xfa, 0xed, 0xfe, 0x00, 0x00, 0x00, 0x00}, "macho"},
		{"Mach-O 32 rev", []byte{0xce, 0xfa, 0xed, 0xfe, 0x00, 0x00, 0x00, 0x00}, "macho"},
		{"Fat binary", []byte{0xca, 0xfe, 0xba, 0xbe, 0x00, 0x00, 0x00, 0x00}, "fat"},
		{"Ar archive", []byte{'!', '<', 'a', 'r', 'c', 'h', '>', '\n'}, "ar"},
		{"Unknown", []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, ""},
		{"Text file", []byte("hello wo"), ""},
		{"Short", []byte{0x7f, 'E'}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectFormat(tt.magic)
			if got != tt.expect {
				t.Errorf("detectFormat() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestErrorCategory_String(t *testing.T) {
	tests := []struct {
		cat    ErrorCategory
		expect string
	}{
		{ErrUnreadable, "unreadable"},
		{ErrInvalidFormat, "invalid format"},
		{ErrNotSharedLib, "not a shared library"},
		{ErrWrongArch, "wrong architecture"},
		{ErrTruncated, "truncated"},
		{ErrCorrupted, "corrupted"},
		{ErrorCategory(99), "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			got := tt.cat.String()
			if got != tt.expect {
				t.Errorf("String() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name   string
		err    *ValidationError
		expect string
	}{
		{
			name:   "with message",
			err:    &ValidationError{Category: ErrNotSharedLib, Message: "file is executable"},
			expect: "file is executable",
		},
		{
			name:   "with underlying error",
			err:    &ValidationError{Category: ErrUnreadable, Err: os.ErrNotExist},
			expect: "unreadable: file does not exist",
		},
		{
			name:   "category only",
			err:    &ValidationError{Category: ErrCorrupted},
			expect: "corrupted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expect {
				t.Errorf("Error() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestMapArchitectures(t *testing.T) {
	// Test ELF architecture mapping
	t.Run("ELF", func(t *testing.T) {
		if mapELFMachine(mapGoArchToELF("amd64")) != "x86_64" {
			t.Error("amd64 should map to x86_64")
		}
		if mapELFMachine(mapGoArchToELF("arm64")) != "arm64" {
			t.Error("arm64 should map to arm64")
		}
	})

	// Test Mach-O architecture mapping
	t.Run("MachO", func(t *testing.T) {
		if mapMachOCpu(mapGoArchToMachO("amd64")) != "x86_64" {
			t.Error("amd64 should map to x86_64")
		}
		if mapMachOCpu(mapGoArchToMachO("arm64")) != "arm64" {
			t.Error("arm64 should map to arm64")
		}
	})
}

// Benchmarks

func BenchmarkValidateHeader_ELF(b *testing.B) {
	if runtime.GOOS != "linux" {
		b.Skip("ELF benchmarks only run on Linux")
	}

	candidates := []string{
		"/lib/x86_64-linux-gnu/libc.so.6",
		"/lib64/libc.so.6",
		"/usr/lib/libc.so.6",
	}

	var libPath string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			libPath = c
			break
		}
	}

	if libPath == "" {
		b.Skip("No system libc found for benchmarking")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ValidateHeader(libPath)
	}
}

func BenchmarkDetectFormat(b *testing.B) {
	magic := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = detectFormat(magic)
	}
}
