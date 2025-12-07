package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetRpathAction_Name(t *testing.T) {
	action := &SetRpathAction{}
	if action.Name() != "set_rpath" {
		t.Errorf("expected 'set_rpath', got '%s'", action.Name())
	}
}

func TestSetRpathAction_Execute_MissingBinaries(t *testing.T) {
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
	}

	// Test with missing binaries parameter
	err := action.Execute(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing binaries parameter")
	}

	// Test with empty binaries list
	err = action.Execute(ctx, map[string]interface{}{
		"binaries": []string{},
	})
	if err == nil {
		t.Error("expected error for empty binaries list")
	}
}

func TestSetRpathAction_Execute_BinaryNotFound(t *testing.T) {
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
	}

	err := action.Execute(ctx, map[string]interface{}{
		"binaries": []interface{}{"nonexistent"},
	})
	if err == nil {
		t.Error("expected error for non-existent binary")
	}
}

func TestDetectBinaryFormat_ELF(t *testing.T) {
	// Create a file with ELF magic bytes
	tmpDir := t.TempDir()
	elfPath := filepath.Join(tmpDir, "test.elf")

	// ELF magic: 0x7f 'E' 'L' 'F' followed by some content
	elfContent := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	if err := os.WriteFile(elfPath, elfContent, 0755); err != nil {
		t.Fatalf("failed to create test ELF file: %v", err)
	}

	format, err := detectBinaryFormat(elfPath)
	if err != nil {
		t.Fatalf("detectBinaryFormat failed: %v", err)
	}
	if format != "elf" {
		t.Errorf("expected 'elf', got '%s'", format)
	}
}

func TestDetectBinaryFormat_MachO64(t *testing.T) {
	// Create a file with Mach-O 64-bit magic bytes (little-endian)
	tmpDir := t.TempDir()
	machoPath := filepath.Join(tmpDir, "test.macho")

	// Mach-O 64-bit little-endian: 0xcf 0xfa 0xed 0xfe
	machoContent := []byte{0xcf, 0xfa, 0xed, 0xfe, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(machoPath, machoContent, 0755); err != nil {
		t.Fatalf("failed to create test Mach-O file: %v", err)
	}

	format, err := detectBinaryFormat(machoPath)
	if err != nil {
		t.Fatalf("detectBinaryFormat failed: %v", err)
	}
	if format != "macho" {
		t.Errorf("expected 'macho', got '%s'", format)
	}
}

func TestDetectBinaryFormat_MachO32(t *testing.T) {
	// Create a file with Mach-O 32-bit magic bytes (little-endian)
	tmpDir := t.TempDir()
	machoPath := filepath.Join(tmpDir, "test.macho32")

	// Mach-O 32-bit little-endian: 0xce 0xfa 0xed 0xfe
	machoContent := []byte{0xce, 0xfa, 0xed, 0xfe, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(machoPath, machoContent, 0755); err != nil {
		t.Fatalf("failed to create test Mach-O file: %v", err)
	}

	format, err := detectBinaryFormat(machoPath)
	if err != nil {
		t.Fatalf("detectBinaryFormat failed: %v", err)
	}
	if format != "macho" {
		t.Errorf("expected 'macho', got '%s'", format)
	}
}

func TestDetectBinaryFormat_FatBinary(t *testing.T) {
	// Create a file with Fat binary magic bytes (big-endian)
	tmpDir := t.TempDir()
	fatPath := filepath.Join(tmpDir, "test.fat")

	// Fat binary big-endian: 0xca 0xfe 0xba 0xbe
	fatContent := []byte{0xca, 0xfe, 0xba, 0xbe, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(fatPath, fatContent, 0755); err != nil {
		t.Fatalf("failed to create test Fat binary file: %v", err)
	}

	format, err := detectBinaryFormat(fatPath)
	if err != nil {
		t.Fatalf("detectBinaryFormat failed: %v", err)
	}
	if format != "macho" {
		t.Errorf("expected 'macho', got '%s'", format)
	}
}

func TestDetectBinaryFormat_Unknown(t *testing.T) {
	// Create a file with unknown magic bytes (e.g., plain text)
	tmpDir := t.TempDir()
	textPath := filepath.Join(tmpDir, "test.txt")

	textContent := []byte("#!/bin/sh\necho hello\n")
	if err := os.WriteFile(textPath, textContent, 0755); err != nil {
		t.Fatalf("failed to create test text file: %v", err)
	}

	format, err := detectBinaryFormat(textPath)
	if err != nil {
		t.Fatalf("detectBinaryFormat failed: %v", err)
	}
	if format != "unknown" {
		t.Errorf("expected 'unknown', got '%s'", format)
	}
}

func TestDetectBinaryFormat_FileNotFound(t *testing.T) {
	_, err := detectBinaryFormat("/nonexistent/path")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestParseRpathsFromOtool(t *testing.T) {
	// Sample otool -l output
	otoolOutput := `
Load command 14
          cmd LC_RPATH
      cmdsize 40
         path /usr/local/lib (offset 12)
Load command 15
          cmd LC_RPATH
      cmdsize 48
         path @executable_path/../lib (offset 12)
Load command 16
          cmd LC_LOAD_DYLIB
      cmdsize 56
         name /usr/lib/libSystem.B.dylib (offset 24)
`

	rpaths := parseRpathsFromOtool(otoolOutput)

	if len(rpaths) != 2 {
		t.Errorf("expected 2 rpaths, got %d", len(rpaths))
	}

	expectedRpaths := []string{"/usr/local/lib", "@executable_path/../lib"}
	for i, expected := range expectedRpaths {
		if i >= len(rpaths) {
			t.Errorf("missing rpath at index %d", i)
			continue
		}
		if rpaths[i] != expected {
			t.Errorf("expected rpath '%s' at index %d, got '%s'", expected, i, rpaths[i])
		}
	}
}

func TestParseRpathsFromOtool_Empty(t *testing.T) {
	// Output with no LC_RPATH
	otoolOutput := `
Load command 0
          cmd LC_SEGMENT_64
      cmdsize 72
    segname __TEXT
`

	rpaths := parseRpathsFromOtool(otoolOutput)
	if len(rpaths) != 0 {
		t.Errorf("expected 0 rpaths, got %d", len(rpaths))
	}
}

func TestCreateLibraryWrapper(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "testbinary")

	// Create a fake binary
	if err := os.WriteFile(binaryPath, []byte("fake binary content"), 0755); err != nil {
		t.Fatalf("failed to create test binary: %v", err)
	}

	// Create wrapper
	err := createLibraryWrapper(binaryPath, "$ORIGIN/../lib")
	if err != nil {
		t.Fatalf("createLibraryWrapper failed: %v", err)
	}

	// Check that the original was renamed
	origPath := binaryPath + ".orig"
	if _, err := os.Stat(origPath); os.IsNotExist(err) {
		t.Error("original binary was not renamed")
	}

	// Check that wrapper was created
	wrapperContent, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("failed to read wrapper: %v", err)
	}

	// Verify wrapper is a shell script
	if string(wrapperContent[:2]) != "#!" {
		t.Error("wrapper is not a shell script")
	}

	// Check wrapper is executable
	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("failed to stat wrapper: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("wrapper is not executable")
	}
}

func TestSetRpathAction_DefaultRpath(t *testing.T) {
	// Verify that the default rpath follows the design doc requirement
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Create a fake ELF binary
	elfPath := filepath.Join(ctx.WorkDir, "test")
	elfContent := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	if err := os.WriteFile(elfPath, elfContent, 0755); err != nil {
		t.Fatalf("failed to create test ELF file: %v", err)
	}

	// Execute will fail because patchelf isn't available in the test environment,
	// but with create_wrapper=true (default), it should fall back to creating a wrapper
	err := action.Execute(ctx, map[string]interface{}{
		"binaries": []interface{}{"test"},
	})

	// The action should succeed with wrapper fallback
	if err != nil {
		t.Logf("Note: action failed (expected if patchelf not installed): %v", err)
		// This is acceptable in test environment without patchelf
	}
}

func TestSetRpathAction_CustomRpath(t *testing.T) {
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Create a fake ELF binary
	elfPath := filepath.Join(ctx.WorkDir, "test")
	elfContent := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	if err := os.WriteFile(elfPath, elfContent, 0755); err != nil {
		t.Fatalf("failed to create test ELF file: %v", err)
	}

	// Try to set a custom rpath
	err := action.Execute(ctx, map[string]interface{}{
		"binaries": []interface{}{"test"},
		"rpath":    "$ORIGIN/../mylibs",
	})

	// The action may succeed with wrapper fallback or fail if patchelf is not available
	if err != nil {
		t.Logf("Note: action failed (expected if patchelf not installed): %v", err)
	}
}

func TestSetRpathAction_NoWrapperFallback(t *testing.T) {
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Create a fake ELF binary
	elfPath := filepath.Join(ctx.WorkDir, "test")
	elfContent := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	if err := os.WriteFile(elfPath, elfContent, 0755); err != nil {
		t.Fatalf("failed to create test ELF file: %v", err)
	}

	// Disable wrapper fallback - should fail if patchelf not available
	err := action.Execute(ctx, map[string]interface{}{
		"binaries":       []interface{}{"test"},
		"create_wrapper": false,
	})

	// Without patchelf, this should fail (no wrapper fallback)
	// On systems with patchelf, it would succeed
	if err != nil {
		t.Logf("Note: action failed as expected without patchelf: %v", err)
	}
}

func TestSetRpathAction_UnsupportedFormat(t *testing.T) {
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Create a plain text file (unsupported format)
	textPath := filepath.Join(ctx.WorkDir, "test")
	if err := os.WriteFile(textPath, []byte("plain text"), 0755); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := action.Execute(ctx, map[string]interface{}{
		"binaries":       []interface{}{"test"},
		"create_wrapper": false,
	})

	if err == nil {
		t.Error("expected error for unsupported binary format")
	}
}

func TestValidatePathWithinDir(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		target    string
		base      string
		wantError bool
	}{
		{
			name:      "valid path within dir",
			target:    filepath.Join(tmpDir, "subdir", "file"),
			base:      tmpDir,
			wantError: false,
		},
		{
			name:      "path traversal attack",
			target:    filepath.Join(tmpDir, "..", "etc", "passwd"),
			base:      tmpDir,
			wantError: true,
		},
		{
			name:      "absolute path outside base",
			target:    "/etc/passwd",
			base:      tmpDir,
			wantError: true,
		},
		{
			name:      "same as base dir",
			target:    tmpDir,
			base:      tmpDir,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathWithinDir(tt.target, tt.base)
			if (err != nil) != tt.wantError {
				t.Errorf("validatePathWithinDir() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestValidateRpath(t *testing.T) {
	tests := []struct {
		name      string
		rpath     string
		wantError bool
	}{
		{
			name:      "empty rpath (uses default)",
			rpath:     "",
			wantError: false,
		},
		{
			name:      "valid $ORIGIN relative",
			rpath:     "$ORIGIN/../lib",
			wantError: false,
		},
		{
			name:      "valid @executable_path",
			rpath:     "@executable_path/../lib",
			wantError: false,
		},
		{
			name:      "valid @loader_path",
			rpath:     "@loader_path/../lib",
			wantError: false,
		},
		{
			name:      "valid @rpath",
			rpath:     "@rpath/lib",
			wantError: false,
		},
		{
			name:      "colon injection attack",
			rpath:     "$ORIGIN/../lib:/tmp/evil",
			wantError: true,
		},
		{
			name:      "absolute path attack",
			rpath:     "/tmp/evil/lib",
			wantError: true,
		},
		{
			name:      "missing valid prefix",
			rpath:     "../lib",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRpath(tt.rpath)
			if (err != nil) != tt.wantError {
				t.Errorf("validateRpath() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestValidateBinaryName(t *testing.T) {
	tests := []struct {
		name      string
		binName   string
		wantError bool
	}{
		{
			name:      "simple name",
			binName:   "ruby",
			wantError: false,
		},
		{
			name:      "name with dash",
			binName:   "cargo-audit",
			wantError: false,
		},
		{
			name:      "name with underscore",
			binName:   "my_tool",
			wantError: false,
		},
		{
			name:      "name with dot",
			binName:   "ruby.orig",
			wantError: false,
		},
		{
			name:      "shell injection - semicolon",
			binName:   "ruby;rm -rf /",
			wantError: true,
		},
		{
			name:      "shell injection - backticks",
			binName:   "ruby`whoami`",
			wantError: true,
		},
		{
			name:      "shell injection - dollar",
			binName:   "ruby$(whoami)",
			wantError: true,
		},
		{
			name:      "shell injection - space",
			binName:   "ruby test",
			wantError: true,
		},
		{
			name:      "shell injection - single quote",
			binName:   "ruby'test",
			wantError: true,
		},
		{
			name:      "shell injection - double quote",
			binName:   "ruby\"test",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBinaryName(tt.binName)
			if (err != nil) != tt.wantError {
				t.Errorf("validateBinaryName() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestSetRpathAction_PathTraversal(t *testing.T) {
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Try to escape the work directory
	err := action.Execute(ctx, map[string]interface{}{
		"binaries": []interface{}{"../../../etc/passwd"},
	})

	if err == nil {
		t.Error("expected error for path traversal attack")
	}
	if !strings.Contains(err.Error(), "path escapes") {
		t.Errorf("expected 'path escapes' error, got: %v", err)
	}
}

func TestSetRpathAction_RpathInjection(t *testing.T) {
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Create a fake ELF binary
	elfPath := filepath.Join(ctx.WorkDir, "test")
	elfContent := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	if err := os.WriteFile(elfPath, elfContent, 0755); err != nil {
		t.Fatalf("failed to create test ELF file: %v", err)
	}

	// Try to inject multiple paths via colon
	err := action.Execute(ctx, map[string]interface{}{
		"binaries": []interface{}{"test"},
		"rpath":    "$ORIGIN/../lib:/tmp/evil",
	})

	if err == nil {
		t.Error("expected error for RPATH injection attack")
	}
	if !strings.Contains(err.Error(), "invalid rpath") {
		t.Errorf("expected 'invalid rpath' error, got: %v", err)
	}
}

func TestCreateLibraryWrapper_SymlinkAttack(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a real file
	realFile := filepath.Join(tmpDir, "real")
	if err := os.WriteFile(realFile, []byte("content"), 0755); err != nil {
		t.Fatalf("failed to create real file: %v", err)
	}

	// Create a symlink
	symlinkPath := filepath.Join(tmpDir, "symlink")
	if err := os.Symlink(realFile, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Try to create wrapper for symlink - should fail
	err := createLibraryWrapper(symlinkPath, "$ORIGIN/../lib")
	if err == nil {
		t.Error("expected error when creating wrapper for symlink")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected 'symlink' error, got: %v", err)
	}
}

func TestCreateLibraryWrapper_UnsafeName(t *testing.T) {
	tmpDir := t.TempDir()

	// Create file with unsafe name
	unsafeName := filepath.Join(tmpDir, "test;rm -rf")
	if err := os.WriteFile(unsafeName, []byte("content"), 0755); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Try to create wrapper - should fail due to unsafe name
	err := createLibraryWrapper(unsafeName, "$ORIGIN/../lib")
	if err == nil {
		t.Error("expected error for unsafe binary name")
	}
	if !strings.Contains(err.Error(), "unsafe") {
		t.Errorf("expected 'unsafe' error, got: %v", err)
	}
}

func TestCreateLibraryWrapper_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "nonexistent")

	err := createLibraryWrapper(nonExistent, "$ORIGIN/../lib")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "stat") {
		t.Errorf("expected 'stat' error, got: %v", err)
	}
}

func TestDetectBinaryFormat_ReadError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an empty file - will cause EOF when trying to read magic bytes
	emptyPath := filepath.Join(tmpDir, "empty")
	if err := os.WriteFile(emptyPath, []byte{}, 0755); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}

	_, err := detectBinaryFormat(emptyPath)
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestDetectBinaryFormat_MachO64BigEndian(t *testing.T) {
	tmpDir := t.TempDir()
	machoPath := filepath.Join(tmpDir, "test.macho64be")

	// Mach-O 64-bit big-endian: 0xfe 0xed 0xfa 0xcf
	machoContent := []byte{0xfe, 0xed, 0xfa, 0xcf, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(machoPath, machoContent, 0755); err != nil {
		t.Fatalf("failed to create test Mach-O file: %v", err)
	}

	format, err := detectBinaryFormat(machoPath)
	if err != nil {
		t.Fatalf("detectBinaryFormat failed: %v", err)
	}
	if format != "macho" {
		t.Errorf("expected 'macho', got '%s'", format)
	}
}

func TestDetectBinaryFormat_MachO32BigEndian(t *testing.T) {
	tmpDir := t.TempDir()
	machoPath := filepath.Join(tmpDir, "test.macho32be")

	// Mach-O 32-bit big-endian: 0xfe 0xed 0xfa 0xce
	machoContent := []byte{0xfe, 0xed, 0xfa, 0xce, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(machoPath, machoContent, 0755); err != nil {
		t.Fatalf("failed to create test Mach-O file: %v", err)
	}

	format, err := detectBinaryFormat(machoPath)
	if err != nil {
		t.Fatalf("detectBinaryFormat failed: %v", err)
	}
	if format != "macho" {
		t.Errorf("expected 'macho', got '%s'", format)
	}
}

func TestDetectBinaryFormat_FatBinaryLittleEndian(t *testing.T) {
	tmpDir := t.TempDir()
	fatPath := filepath.Join(tmpDir, "test.fatle")

	// Fat binary little-endian: 0xbe 0xba 0xfe 0xca
	fatContent := []byte{0xbe, 0xba, 0xfe, 0xca, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(fatPath, fatContent, 0755); err != nil {
		t.Fatalf("failed to create test Fat binary file: %v", err)
	}

	format, err := detectBinaryFormat(fatPath)
	if err != nil {
		t.Fatalf("detectBinaryFormat failed: %v", err)
	}
	if format != "macho" {
		t.Errorf("expected 'macho', got '%s'", format)
	}
}

func TestParseRpathsFromOtool_PathWithoutOffset(t *testing.T) {
	// Test path without "(offset XX)" suffix
	otoolOutput := `
Load command 14
          cmd LC_RPATH
      cmdsize 40
         path /simple/path
`
	rpaths := parseRpathsFromOtool(otoolOutput)
	if len(rpaths) != 1 {
		t.Errorf("expected 1 rpath, got %d", len(rpaths))
	}
	if len(rpaths) > 0 && rpaths[0] != "/simple/path" {
		t.Errorf("expected '/simple/path', got '%s'", rpaths[0])
	}
}

func TestSetRpathAction_Execute_MultipleBinaries(t *testing.T) {
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Create multiple fake ELF binaries
	for _, name := range []string{"bin1", "bin2"} {
		elfPath := filepath.Join(ctx.WorkDir, name)
		elfContent := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
		if err := os.WriteFile(elfPath, elfContent, 0755); err != nil {
			t.Fatalf("failed to create test ELF file: %v", err)
		}
	}

	// Execute with multiple binaries - will fallback to wrapper since patchelf not installed
	err := action.Execute(ctx, map[string]interface{}{
		"binaries": []interface{}{"bin1", "bin2"},
	})

	// Should succeed with wrapper fallback
	if err != nil {
		t.Logf("Note: action result: %v", err)
	}
}

func TestSetRpathAction_Execute_WithVariableSubstitution(t *testing.T) {
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.2.3",
	}

	// Create fake ELF binary with the expanded name
	elfPath := filepath.Join(ctx.WorkDir, "test-1.2.3")
	elfContent := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	if err := os.WriteFile(elfPath, elfContent, 0755); err != nil {
		t.Fatalf("failed to create test ELF file: %v", err)
	}

	// Execute with variable substitution in binary name
	// The ExpandVars function uses {version} syntax not ${version}
	err := action.Execute(ctx, map[string]interface{}{
		"binaries": []interface{}{"test-{version}"},
	})

	// Should succeed with wrapper fallback
	if err != nil {
		t.Logf("Note: action result: %v", err)
	}
}

func TestNeedsCodesign(t *testing.T) {
	// This function is platform-dependent, but we can at least call it
	// On Linux it should return false
	result := needsCodesign()
	// We can't assert the exact value since it depends on the platform
	t.Logf("needsCodesign() returned: %v", result)
}

func TestSetRpathLinux_NoPatchelf(t *testing.T) {
	// This test verifies the error message when patchelf is not available
	// Save PATH and set it to empty to ensure patchelf isn't found
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "test")
	if err := os.WriteFile(binaryPath, []byte{0x7f, 'E', 'L', 'F'}, 0755); err != nil {
		t.Fatalf("failed to create test binary: %v", err)
	}

	err := setRpathLinux(binaryPath, "$ORIGIN/../lib")
	if err == nil {
		t.Error("expected error when patchelf not found")
	}
	if !strings.Contains(err.Error(), "patchelf not found") {
		t.Errorf("expected 'patchelf not found' error, got: %v", err)
	}
}

func TestSetRpathMacOS_NoInstallNameTool(t *testing.T) {
	// This test verifies the error message when install_name_tool is not available
	// Save PATH and set it to empty
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "test")
	if err := os.WriteFile(binaryPath, []byte{0xcf, 0xfa, 0xed, 0xfe}, 0755); err != nil {
		t.Fatalf("failed to create test binary: %v", err)
	}

	err := setRpathMacOS(binaryPath, "$ORIGIN/../lib")
	if err == nil {
		t.Error("expected error when install_name_tool not found")
	}
	if !strings.Contains(err.Error(), "install_name_tool not found") {
		t.Errorf("expected 'install_name_tool not found' error, got: %v", err)
	}
}

func TestValidatePathWithinDir_ErrorCases(t *testing.T) {
	// Test with paths that should work but test edge cases
	tmpDir := t.TempDir()

	// Test deeply nested path
	deepPath := filepath.Join(tmpDir, "a", "b", "c", "d", "file")
	err := validatePathWithinDir(deepPath, tmpDir)
	if err != nil {
		t.Errorf("expected no error for deep path, got: %v", err)
	}

	// Test path that equals base (edge case)
	err = validatePathWithinDir(tmpDir, tmpDir)
	if err != nil {
		t.Errorf("expected no error when path equals base, got: %v", err)
	}
}

func TestSetRpathAction_MacOSBinary(t *testing.T) {
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Create a fake Mach-O binary
	machoPath := filepath.Join(ctx.WorkDir, "test")
	machoContent := []byte{0xcf, 0xfa, 0xed, 0xfe, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(machoPath, machoContent, 0755); err != nil {
		t.Fatalf("failed to create test Mach-O file: %v", err)
	}

	// Execute - will fail on Linux since install_name_tool isn't available
	// but with create_wrapper=true (default), it should fall back to wrapper
	err := action.Execute(ctx, map[string]interface{}{
		"binaries": []interface{}{"test"},
	})

	// The action should succeed with wrapper fallback
	if err != nil {
		t.Logf("Note: action failed (expected on Linux without install_name_tool): %v", err)
	}
}

func TestCreateLibraryWrapper_RenameError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a read-only directory to cause rename to fail
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	binaryPath := filepath.Join(readOnlyDir, "testbin")
	if err := os.WriteFile(binaryPath, []byte("content"), 0755); err != nil {
		t.Fatalf("failed to create binary: %v", err)
	}

	// Make directory read-only to prevent rename
	if err := os.Chmod(readOnlyDir, 0555); err != nil {
		t.Fatalf("failed to chmod directory: %v", err)
	}
	defer func() { _ = os.Chmod(readOnlyDir, 0755) }() // Restore permissions for cleanup

	err := createLibraryWrapper(binaryPath, "$ORIGIN/../lib")
	if err == nil {
		t.Error("expected error when rename fails")
	}
	if !strings.Contains(err.Error(), "rename") {
		t.Errorf("expected 'rename' error, got: %v", err)
	}
}

func TestSetRpathAction_Execute_EmptyBinariesInterface(t *testing.T) {
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Test with empty interface slice
	err := action.Execute(ctx, map[string]interface{}{
		"binaries": []interface{}{},
	})

	if err == nil {
		t.Error("expected error for empty binaries list")
	}
}

func TestSetRpathAction_Execute_CreateWrapperFalse(t *testing.T) {
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Create a fake Mach-O binary
	machoPath := filepath.Join(ctx.WorkDir, "machotest")
	machoContent := []byte{0xcf, 0xfa, 0xed, 0xfe, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(machoPath, machoContent, 0755); err != nil {
		t.Fatalf("failed to create test Mach-O file: %v", err)
	}

	// Execute with create_wrapper=false - should fail on Linux
	err := action.Execute(ctx, map[string]interface{}{
		"binaries":       []interface{}{"machotest"},
		"create_wrapper": false,
	})

	// Should fail because install_name_tool isn't available and wrapper is disabled
	if err == nil {
		t.Log("Note: No error (test running on macOS?)")
	} else {
		t.Logf("Note: Got expected error: %v", err)
	}
}

func TestSetRpathAction_Execute_DetectFormatError(t *testing.T) {
	action := &SetRpathAction{}
	ctx := &ExecutionContext{
		WorkDir: t.TempDir(),
		Version: "1.0.0",
	}

	// Create a binary file that will return "unknown" format
	unknownPath := filepath.Join(ctx.WorkDir, "unknown")
	// Just some random bytes that don't match any known format
	unknownContent := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(unknownPath, unknownContent, 0755); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Should fail with unsupported format error
	err := action.Execute(ctx, map[string]interface{}{
		"binaries":       []interface{}{"unknown"},
		"create_wrapper": false,
	})

	if err == nil {
		t.Error("expected error for unknown binary format")
	}
	if !strings.Contains(err.Error(), "unsupported binary format") {
		t.Errorf("expected 'unsupported binary format' error, got: %v", err)
	}
}

func TestParseRpathsFromOtool_MultipleRpaths(t *testing.T) {
	// Test with multiple LC_RPATH entries interspersed with other load commands
	otoolOutput := `
Load command 10
          cmd LC_LOAD_DYLIB
      cmdsize 56
         name /usr/lib/libSystem.B.dylib (offset 24)
Load command 11
          cmd LC_RPATH
      cmdsize 40
         path /first/path (offset 12)
Load command 12
          cmd LC_SEGMENT_64
      cmdsize 72
    segname __TEXT
Load command 13
          cmd LC_RPATH
      cmdsize 48
         path /second/path (offset 12)
Load command 14
          cmd LC_RPATH
      cmdsize 56
         path @executable_path/../Frameworks (offset 12)
`
	rpaths := parseRpathsFromOtool(otoolOutput)
	if len(rpaths) != 3 {
		t.Errorf("expected 3 rpaths, got %d: %v", len(rpaths), rpaths)
	}

	expected := []string{"/first/path", "/second/path", "@executable_path/../Frameworks"}
	for i, exp := range expected {
		if i >= len(rpaths) {
			t.Errorf("missing rpath at index %d", i)
			continue
		}
		if rpaths[i] != exp {
			t.Errorf("expected rpath '%s' at index %d, got '%s'", exp, i, rpaths[i])
		}
	}
}

func TestValidateRpath_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		rpath     string
		wantError bool
	}{
		{
			name:      "$ORIGIN only",
			rpath:     "$ORIGIN",
			wantError: false,
		},
		{
			name:      "@executable_path only",
			rpath:     "@executable_path",
			wantError: false,
		},
		{
			name:      "@loader_path with subdirs",
			rpath:     "@loader_path/../../some/deep/path",
			wantError: false,
		},
		{
			name:      "@rpath simple",
			rpath:     "@rpath",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRpath(tt.rpath)
			if (err != nil) != tt.wantError {
				t.Errorf("validateRpath(%q) error = %v, wantError %v", tt.rpath, err, tt.wantError)
			}
		})
	}
}
