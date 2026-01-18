package verify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExtractRpaths_ELF(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ELF RPATH tests only run on Linux")
	}

	// Test with system binary that might have RPATH
	// Most system binaries don't have RPATH, so we mainly test that
	// the function doesn't error on valid binaries
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
		t.Skip("No system library found for testing")
	}

	rpaths, err := ExtractRpaths(libPath)
	if err != nil {
		t.Fatalf("ExtractRpaths(%s) failed: %v", libPath, err)
	}

	// System libraries typically don't have RPATH
	// This test mainly verifies the function doesn't error
	t.Logf("RPATHs from %s: %v", libPath, rpaths)
}

func TestExtractRpaths_NonBinaryFile(t *testing.T) {
	// Create a non-binary file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "script.sh")

	err := os.WriteFile(path, []byte("#!/bin/bash\necho hello"), 0755)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	rpaths, err := ExtractRpaths(path)
	if err != nil {
		t.Errorf("ExtractRpaths should return nil for non-binary, got error: %v", err)
	}
	if len(rpaths) != 0 {
		t.Errorf("ExtractRpaths should return empty slice for non-binary, got: %v", rpaths)
	}
}

func TestExtractRpaths_NonExistent(t *testing.T) {
	_, err := ExtractRpaths("/nonexistent/path/to/binary")
	if err == nil {
		t.Error("ExtractRpaths should error for non-existent file")
	}
}

func TestExpandPathVariables_Origin(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "bin", "myapp")

	// Create the bin directory
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	tests := []struct {
		name       string
		dep        string
		expected   string
		wantPrefix string
	}{
		{
			name:       "$ORIGIN with path",
			dep:        "$ORIGIN/../lib/libfoo.so",
			expected:   filepath.Join(tmpDir, "lib/libfoo.so"),
			wantPrefix: "",
		},
		{
			name:       "${ORIGIN} with path",
			dep:        "${ORIGIN}/../lib/libbar.so",
			expected:   filepath.Join(tmpDir, "lib/libbar.so"),
			wantPrefix: "",
		},
		{
			name:       "$ORIGIN alone",
			dep:        "$ORIGIN",
			expected:   filepath.Join(tmpDir, "bin"),
			wantPrefix: "",
		},
		{
			name:       "${ORIGIN} alone",
			dep:        "${ORIGIN}",
			expected:   filepath.Join(tmpDir, "bin"),
			wantPrefix: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expanded, err := ExpandPathVariables(tt.dep, binaryPath, nil, tt.wantPrefix)
			if err != nil {
				t.Fatalf("ExpandPathVariables failed: %v", err)
			}
			// Clean expected for comparison
			expected := filepath.Clean(tt.expected)
			if expanded != expected {
				t.Errorf("got %q, want %q", expanded, expected)
			}
		})
	}
}

func TestExpandPathVariables_LoaderPath(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "lib", "libmain.dylib")

	// Create the lib directory
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	tests := []struct {
		name     string
		dep      string
		expected string
	}{
		{
			name:     "@loader_path with path",
			dep:      "@loader_path/../Frameworks/libdep.dylib",
			expected: filepath.Join(tmpDir, "Frameworks/libdep.dylib"),
		},
		{
			name:     "@loader_path alone",
			dep:      "@loader_path",
			expected: filepath.Join(tmpDir, "lib"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expanded, err := ExpandPathVariables(tt.dep, binaryPath, nil, "")
			if err != nil {
				t.Fatalf("ExpandPathVariables failed: %v", err)
			}
			expected := filepath.Clean(tt.expected)
			if expanded != expected {
				t.Errorf("got %q, want %q", expanded, expected)
			}
		})
	}
}

func TestExpandPathVariables_ExecutablePath(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "Contents", "MacOS", "myapp")

	// Create the directory
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	tests := []struct {
		name     string
		dep      string
		expected string
	}{
		{
			name:     "@executable_path with path",
			dep:      "@executable_path/../Frameworks/libdep.dylib",
			expected: filepath.Join(tmpDir, "Contents/Frameworks/libdep.dylib"),
		},
		{
			name:     "@executable_path alone",
			dep:      "@executable_path",
			expected: filepath.Join(tmpDir, "Contents/MacOS"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expanded, err := ExpandPathVariables(tt.dep, binaryPath, nil, "")
			if err != nil {
				t.Fatalf("ExpandPathVariables failed: %v", err)
			}
			expected := filepath.Clean(tt.expected)
			if expanded != expected {
				t.Errorf("got %q, want %q", expanded, expected)
			}
		})
	}
}

func TestExpandPathVariables_Rpath(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "bin", "myapp")
	libDir := filepath.Join(tmpDir, "lib")

	// Create directories and a library file
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		t.Fatalf("Failed to create bin directory: %v", err)
	}
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("Failed to create lib directory: %v", err)
	}

	// Create a dummy library file
	libPath := filepath.Join(libDir, "libfoo.dylib")
	if err := os.WriteFile(libPath, []byte("dummy"), 0644); err != nil {
		t.Fatalf("Failed to create library file: %v", err)
	}

	rpaths := []string{
		filepath.Join(tmpDir, "lib"),
	}

	expanded, err := ExpandPathVariables("@rpath/libfoo.dylib", binaryPath, rpaths, "")
	if err != nil {
		t.Fatalf("ExpandPathVariables failed: %v", err)
	}

	expected := filepath.Join(tmpDir, "lib/libfoo.dylib")
	if expanded != expected {
		t.Errorf("got %q, want %q", expanded, expected)
	}
}

func TestExpandPathVariables_RpathWithLoaderPath(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "bin", "myapp")
	libDir := filepath.Join(tmpDir, "lib")

	// Create directories
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		t.Fatalf("Failed to create bin directory: %v", err)
	}
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("Failed to create lib directory: %v", err)
	}

	// Create a dummy library file
	libPath := filepath.Join(libDir, "libfoo.dylib")
	if err := os.WriteFile(libPath, []byte("dummy"), 0644); err != nil {
		t.Fatalf("Failed to create library file: %v", err)
	}

	// RPATH contains @loader_path
	rpaths := []string{
		"@loader_path/../lib",
	}

	expanded, err := ExpandPathVariables("@rpath/libfoo.dylib", binaryPath, rpaths, "")
	if err != nil {
		t.Fatalf("ExpandPathVariables failed: %v", err)
	}

	expected := filepath.Join(tmpDir, "lib/libfoo.dylib")
	if expanded != expected {
		t.Errorf("got %q, want %q", expanded, expected)
	}
}

func TestExpandPathVariables_RpathNoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "bin", "myapp")

	// Create the bin directory
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// RPATH that doesn't match any existing file
	rpaths := []string{
		filepath.Join(tmpDir, "nonexistent"),
	}

	// Should return the first candidate even if it doesn't exist
	expanded, err := ExpandPathVariables("@rpath/libfoo.dylib", binaryPath, rpaths, "")
	if err != nil {
		t.Fatalf("ExpandPathVariables failed: %v", err)
	}

	expected := filepath.Join(tmpDir, "nonexistent/libfoo.dylib")
	if expanded != expected {
		t.Errorf("got %q, want %q", expanded, expected)
	}
}

func TestExpandPathVariables_RpathEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "bin", "myapp")

	// Create the bin directory
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// No RPATHs
	_, err := ExpandPathVariables("@rpath/libfoo.dylib", binaryPath, nil, "")
	if err == nil {
		t.Fatal("Expected error for @rpath with no RPATH entries")
	}

	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Expected ValidationError, got %T", err)
	}
	if verr.Category != ErrUnexpandedVariable {
		t.Errorf("Category = %v, want %v", verr.Category, ErrUnexpandedVariable)
	}
}

func TestExpandPathVariables_PlainPath(t *testing.T) {
	expanded, err := ExpandPathVariables("/usr/lib/libfoo.so", "/bin/app", nil, "")
	if err != nil {
		t.Fatalf("ExpandPathVariables failed: %v", err)
	}

	if expanded != "/usr/lib/libfoo.so" {
		t.Errorf("got %q, want %q", expanded, "/usr/lib/libfoo.so")
	}
}

func TestExpandPathVariables_PathLengthLimit(t *testing.T) {
	// Create a path longer than MaxPathLength
	longPath := "/" + strings.Repeat("a", MaxPathLength)

	_, err := ExpandPathVariables(longPath, "/bin/app", nil, "")
	if err == nil {
		t.Fatal("Expected error for path exceeding length limit")
	}

	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Expected ValidationError, got %T", err)
	}
	if verr.Category != ErrPathLengthExceeded {
		t.Errorf("Category = %v, want %v", verr.Category, ErrPathLengthExceeded)
	}
}

func TestExpandPathVariables_AllowedPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "tools", "myapp", "bin", "app")
	allowedDir := filepath.Join(tmpDir, "tools")

	// Create directories
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	tests := []struct {
		name      string
		dep       string
		wantError bool
	}{
		{
			name:      "path within allowed directory",
			dep:       "$ORIGIN/../lib/libfoo.so",
			wantError: false,
		},
		{
			name:      "path outside allowed directory",
			dep:       "$ORIGIN/../../../../etc/passwd",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExpandPathVariables(tt.dep, binaryPath, nil, allowedDir)
			if tt.wantError {
				if err == nil {
					t.Fatal("Expected error for path outside allowed directory")
				}
				verr, ok := err.(*ValidationError)
				if !ok {
					t.Fatalf("Expected ValidationError, got %T", err)
				}
				if verr.Category != ErrPathOutsideAllowed {
					t.Errorf("Category = %v, want %v", verr.Category, ErrPathOutsideAllowed)
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestExpandPathVariables_UnexpandedVariable(t *testing.T) {
	// Test detection of unexpanded variables
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"plain path", "/usr/lib/libfoo.so", false},
		{"$ORIGIN", "$ORIGIN/lib", true},
		{"${ORIGIN}", "${ORIGIN}/lib", true},
		{"@rpath", "@rpath/libfoo.dylib", true},
		{"@loader_path", "@loader_path/lib", true},
		{"@executable_path", "@executable_path/lib", true},
		{"$HOME (shell variable)", "$HOME/lib", true},
		// Note: The function is intentionally conservative about detecting potential
		// variables. These edge cases would not appear in actual library paths.
		{"@ in email-like", "foo@bar.com", true}, // Conservative: could be @bar
		{"$ before letter", "100$file", true},    // Conservative: could be $file variable
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsPathVariable(tt.path)
			if got != tt.want {
				t.Errorf("containsPathVariable(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseRpathString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLen   int
		wantError bool
	}{
		{
			name:      "single path",
			input:     "/usr/lib",
			wantLen:   1,
			wantError: false,
		},
		{
			name:      "multiple paths",
			input:     "/usr/lib:/usr/local/lib:$ORIGIN/../lib",
			wantLen:   3,
			wantError: false,
		},
		{
			name:      "empty string",
			input:     "",
			wantLen:   0,
			wantError: false,
		},
		{
			name:      "path with spaces trimmed",
			input:     " /usr/lib : /usr/local/lib ",
			wantLen:   2,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rpaths, err := parseRpathString(tt.input, "/bin/test")
			if tt.wantError {
				if err == nil {
					t.Fatal("Expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if len(rpaths) != tt.wantLen {
				t.Errorf("got %d rpaths, want %d", len(rpaths), tt.wantLen)
			}
		})
	}
}

func TestParseRpathString_RpathLimit(t *testing.T) {
	// Create a string with more than MaxRpathEntries
	var parts []string
	for i := 0; i <= MaxRpathEntries; i++ {
		parts = append(parts, "/usr/lib")
	}
	input := strings.Join(parts, ":")

	_, err := parseRpathString(input, "/bin/test")
	if err == nil {
		t.Fatal("Expected error for exceeding RPATH limit")
	}

	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Expected ValidationError, got %T", err)
	}
	if verr.Category != ErrRpathLimitExceeded {
		t.Errorf("Category = %v, want %v", verr.Category, ErrRpathLimitExceeded)
	}
}

func TestParseRpathString_PathLengthLimit(t *testing.T) {
	// Create a path longer than MaxPathLength
	longPath := "/" + strings.Repeat("a", MaxPathLength)
	input := "/usr/lib:" + longPath

	_, err := parseRpathString(input, "/bin/test")
	if err == nil {
		t.Fatal("Expected error for path exceeding length limit")
	}

	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Expected ValidationError, got %T", err)
	}
	if verr.Category != ErrPathLengthExceeded {
		t.Errorf("Category = %v, want %v", verr.Category, ErrPathLengthExceeded)
	}
}

func TestErrorCategory_RpathErrors(t *testing.T) {
	tests := []struct {
		cat    ErrorCategory
		expect string
	}{
		{ErrRpathLimitExceeded, "RPATH limit exceeded"},
		{ErrPathLengthExceeded, "path length exceeded"},
		{ErrUnexpandedVariable, "unexpanded path variable"},
		{ErrPathOutsideAllowed, "path outside allowed directories"},
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

func TestErrorCategory_ExplicitValues(t *testing.T) {
	// Verify error categories have explicit values per design decision #2
	tests := []struct {
		name     string
		cat      ErrorCategory
		expected ErrorCategory
	}{
		{"ErrRpathLimitExceeded", ErrRpathLimitExceeded, 12},
		{"ErrPathLengthExceeded", ErrPathLengthExceeded, 13},
		{"ErrUnexpandedVariable", ErrUnexpandedVariable, 14},
		{"ErrPathOutsideAllowed", ErrPathOutsideAllowed, 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cat != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.cat, tt.expected)
			}
		})
	}
}

func BenchmarkExpandPathVariables(b *testing.B) {
	tmpDir := b.TempDir()
	binaryPath := filepath.Join(tmpDir, "bin", "myapp")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		b.Fatalf("Failed to create directory: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExpandPathVariables("$ORIGIN/../lib/libfoo.so", binaryPath, nil, "")
	}
}
