package verify

import (
	"debug/elf"
	"debug/macho"
	"errors"
	"io"
	"os"
	"os/exec"
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

func TestValidationError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	verr := &ValidationError{
		Category: ErrCorrupted,
		Path:     "/some/path",
		Err:      underlying,
	}

	if verr.Unwrap() != underlying {
		t.Errorf("Unwrap() returned wrong error")
	}

	// Test with nil Err
	verr2 := &ValidationError{Category: ErrCorrupted}
	if verr2.Unwrap() != nil {
		t.Error("Unwrap() should return nil when Err is nil")
	}
}

func TestValidationStatus_String_Unknown(t *testing.T) {
	s := ValidationStatus(99)
	got := s.String()
	want := "unknown(99)"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestErrorCategory_String_Unknown(t *testing.T) {
	c := ErrorCategory(999)
	got := c.String()
	want := "unknown(999)"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestValidateHeader_RealELF(t *testing.T) {
	// Runs on all Linux systems where libc is available

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
		t.Errorf("Format = %q, want ELF", info.Format)
	}
	if info.Architecture == "" {
		t.Error("Architecture should not be empty")
	}
}

func TestValidateHeader_BadInput(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content []byte
		wantCat *ErrorCategory // if non-nil, assert this category
	}{
		{
			name:    "EmptyFile",
			file:    "empty.so",
			content: []byte{},
		},
		{
			name:    "UnknownMagic",
			file:    "unknown.so",
			content: []byte{0xDE, 0xAD, 0xBE, 0xEF},
			wantCat: func() *ErrorCategory { c := ErrInvalidFormat; return &c }(),
		},
		{
			name:    "FakeMachOMagic",
			file:    "fake.dylib",
			content: []byte{0xfe, 0xed, 0xfa, 0xcf, 0, 0, 0, 0},
		},
		{
			name:    "FakeFatMagic",
			file:    "fake.fat",
			content: []byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 0},
		},
		{
			name:    "TruncatedELF",
			file:    "truncated.so",
			content: []byte{0x7f, 'E', 'L', 'F'},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, tt.file)
			if err := os.WriteFile(path, tt.content, 0644); err != nil {
				t.Fatal(err)
			}

			_, err := ValidateHeader(path)
			if err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
			if tt.wantCat != nil {
				verr, ok := err.(*ValidationError)
				if !ok {
					t.Fatalf("expected ValidationError, got %T", err)
				}
				if verr.Category != *tt.wantCat {
					t.Errorf("Category = %v, want %v", verr.Category, *tt.wantCat)
				}
			}
		})
	}
}

func TestValidateHeader_ELFExecutable(t *testing.T) {
	candidates := []string{
		"/bin/true",
		"/usr/bin/true",
		"/bin/false",
		"/usr/bin/false",
	}

	var binPath string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			binPath = c
			break
		}
	}

	if binPath == "" {
		t.Skip("no suitable binary found")
	}

	info, err := ValidateHeader(binPath)
	if err != nil {
		// Expected for executables (ErrNotSharedLib)
		verr, ok := err.(*ValidationError)
		if ok && verr.Category == ErrNotSharedLib {
			// This is the expected behavior
			return
		}
		t.Logf("ValidateHeader error: %v", err)
	}
	if info != nil {
		t.Logf("ValidateHeader returned info for executable: %+v", info)
	}
}

func TestValidateHeader_RelocatableObject(t *testing.T) {
	// Check if gcc is available
	gccPath, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc not available")
	}

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "test.c")
	objPath := filepath.Join(tmpDir, "test.o")

	// Write minimal C source
	if err := os.WriteFile(srcPath, []byte("int x;\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Compile to .o (relocatable object, ET_REL)
	cmd := exec.Command(gccPath, "-c", "-o", objPath, srcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("gcc compilation failed: %v: %s", err, out)
	}

	_, err = ValidateHeader(objPath)
	if err == nil {
		t.Fatal("expected error for relocatable object file")
	}

	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if verr.Category != ErrNotSharedLib {
		t.Errorf("Category = %v, want ErrNotSharedLib", verr.Category)
	}
}

func TestMapGoArchToELF_AllCases(t *testing.T) {
	tests := []struct {
		goarch string
		want   elf.Machine
	}{
		{"amd64", elf.EM_X86_64},
		{"arm64", elf.EM_AARCH64},
		{"386", elf.EM_386},
		{"arm", elf.EM_ARM},
		{"mips", elf.EM_NONE},
		{"unknown", elf.EM_NONE},
	}

	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			got := mapGoArchToELF(tt.goarch)
			if got != tt.want {
				t.Errorf("mapGoArchToELF(%q) = %v, want %v", tt.goarch, got, tt.want)
			}
		})
	}
}

func TestMapGoArchToMachO_AllCases(t *testing.T) {
	tests := []struct {
		goarch string
		want   macho.Cpu
	}{
		{"amd64", macho.CpuAmd64},
		{"arm64", macho.CpuArm64},
		{"386", macho.Cpu386},
		{"mips", macho.Cpu(0)},
		{"unknown", macho.Cpu(0)},
	}

	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			got := mapGoArchToMachO(tt.goarch)
			if got != tt.want {
				t.Errorf("mapGoArchToMachO(%q) = %v, want %v", tt.goarch, got, tt.want)
			}
		})
	}
}

func TestMapELFMachine_AllCases(t *testing.T) {
	tests := []struct {
		machine elf.Machine
		want    string
	}{
		{elf.EM_X86_64, "x86_64"},
		{elf.EM_AARCH64, "arm64"},
		{elf.EM_386, "i386"},
		{elf.EM_ARM, "arm"},
		{elf.EM_MIPS, "unknown(8)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := mapELFMachine(tt.machine)
			if got != tt.want {
				t.Errorf("mapELFMachine(%v) = %q, want %q", tt.machine, got, tt.want)
			}
		})
	}
}

func TestMapMachOCpu_AllCases(t *testing.T) {
	tests := []struct {
		cpu  macho.Cpu
		want string
	}{
		{macho.CpuAmd64, "x86_64"},
		{macho.CpuArm64, "arm64"},
		{macho.Cpu386, "i386"},
		{macho.Cpu(99), "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := mapMachOCpu(tt.cpu)
			if got != tt.want {
				t.Errorf("mapMachOCpu(%v) = %q, want %q", tt.cpu, got, tt.want)
			}
		})
	}
}

func TestElfTypeName(t *testing.T) {
	tests := []struct {
		typ  elf.Type
		want string
	}{
		{elf.ET_NONE, "unknown"},
		{elf.ET_REL, "relocatable object"},
		{elf.ET_EXEC, "executable"},
		{elf.ET_DYN, "shared object"},
		{elf.ET_CORE, "core dump"},
		{elf.Type(99), "unknown type (99)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := elfTypeName(tt.typ)
			if got != tt.want {
				t.Errorf("elfTypeName(%v) = %q, want %q", tt.typ, got, tt.want)
			}
		})
	}
}

func TestMachoTypeName(t *testing.T) {
	tests := []struct {
		typ  macho.Type
		want string
	}{
		{macho.TypeObj, "object file"},
		{macho.TypeExec, "executable"},
		{macho.TypeDylib, "dynamic library"},
		{macho.TypeBundle, "bundle"},
		{macho.Type(99), "unknown type (99)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := machoTypeName(tt.typ)
			if got != tt.want {
				t.Errorf("machoTypeName(%v) = %q, want %q", tt.typ, got, tt.want)
			}
		})
	}
}

func TestCategorizeELFError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantCat ErrorCategory
		wantMsg string
	}{
		{
			name:    "bad magic",
			err:     errors.New("bad magic number"),
			wantCat: ErrInvalidFormat,
			wantMsg: "invalid ELF magic number",
		},
		{
			name:    "EOF",
			err:     io.EOF,
			wantCat: ErrTruncated,
			wantMsg: "file is truncated",
		},
		{
			name:    "unexpected EOF",
			err:     io.ErrUnexpectedEOF,
			wantCat: ErrTruncated,
			wantMsg: "file is truncated",
		},
		{
			name:    "not exist",
			err:     os.ErrNotExist,
			wantCat: ErrUnreadable,
			wantMsg: "file not found",
		},
		{
			name:    "permission denied",
			err:     os.ErrPermission,
			wantCat: ErrUnreadable,
			wantMsg: "permission denied",
		},
		{
			name:    "offset error",
			err:     errors.New("invalid offset in section header"),
			wantCat: ErrCorrupted,
		},
		{
			name:    "section error",
			err:     errors.New("invalid section data"),
			wantCat: ErrCorrupted,
		},
		{
			name:    "generic error",
			err:     errors.New("something unexpected"),
			wantCat: ErrUnreadable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verr := categorizeELFError("/path/to/file", tt.err)
			if verr.Category != tt.wantCat {
				t.Errorf("Category = %v, want %v", verr.Category, tt.wantCat)
			}
			if tt.wantMsg != "" && verr.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", verr.Message, tt.wantMsg)
			}
			if verr.Path != "/path/to/file" {
				t.Errorf("Path = %q, want %q", verr.Path, "/path/to/file")
			}
		})
	}
}

func TestCategorizeMachOError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantCat ErrorCategory
	}{
		{"invalid magic", errors.New("invalid magic number"), ErrInvalidFormat},
		{"EOF", io.EOF, ErrTruncated},
		{"not exist", os.ErrNotExist, ErrUnreadable},
		{"permission", os.ErrPermission, ErrUnreadable},
		{"command error", errors.New("invalid command in load"), ErrCorrupted},
		{"load error", errors.New("load section failed"), ErrCorrupted},
		{"generic error", errors.New("something else"), ErrUnreadable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verr := categorizeMachOError("/path/to/file", tt.err)
			if verr.Category != tt.wantCat {
				t.Errorf("Category = %v, want %v", verr.Category, tt.wantCat)
			}
		})
	}
}

func TestCategorizeFatError(t *testing.T) {
	t.Run("not fat", func(t *testing.T) {
		verr := categorizeFatError("/path/to/file", macho.ErrNotFat)
		if verr.Category != ErrInvalidFormat {
			t.Errorf("Category = %v, want %v", verr.Category, ErrInvalidFormat)
		}
	})

	t.Run("other error falls through to macho", func(t *testing.T) {
		verr := categorizeFatError("/path/to/file", os.ErrPermission)
		if verr.Category != ErrUnreadable {
			t.Errorf("Category = %v, want %v", verr.Category, ErrUnreadable)
		}
	})
}

func TestReadMagic_ShortFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := tmpDir + "/short.bin"
	if err := os.WriteFile(path, []byte{0x7f}, 0644); err != nil {
		t.Fatal(err)
	}

	magic, err := readMagic(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(magic) != 1 {
		t.Errorf("expected 1 byte, got %d", len(magic))
	}
}

func TestValidateHeader_PermissionDenied(t *testing.T) {
	tmpDir := t.TempDir()
	path := tmpDir + "/noperm.so"
	if err := os.WriteFile(path, []byte{0x7f, 'E', 'L', 'F'}, 0000); err != nil {
		t.Fatal(err)
	}

	_, err := ValidateHeader(path)
	if err == nil {
		// On some systems (root), this may succeed - skip
		t.Skip("permission test not applicable (possibly running as root)")
	}

	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if verr.Category != ErrUnreadable && verr.Category != ErrTruncated && verr.Category != ErrCorrupted {
		t.Errorf("Category = %v, want ErrUnreadable/ErrTruncated/ErrCorrupted", verr.Category)
	}
}

func TestErrorCategory_String_AllTier2(t *testing.T) {
	tests := []struct {
		cat    ErrorCategory
		expect string
	}{
		{ErrABIMismatch, "ABI mismatch"},
		{ErrUnknownDependency, "unknown dependency"},
		{ErrRpathLimitExceeded, "RPATH limit exceeded"},
		{ErrPathLengthExceeded, "path length exceeded"},
		{ErrUnexpandedVariable, "unexpanded path variable"},
		{ErrPathOutsideAllowed, "path outside allowed directories"},
		{ErrMaxDepthExceeded, "max depth exceeded"},
		{ErrMissingSoname, "missing soname"},
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
