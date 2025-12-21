package actions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandVars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		expected string
	}{
		{
			name:     "single variable",
			input:    "tool-{version}.tar.gz",
			vars:     map[string]string{"version": "1.2.3"},
			expected: "tool-1.2.3.tar.gz",
		},
		{
			name:     "multiple variables",
			input:    "{os}/{arch}/tool-{version}",
			vars:     map[string]string{"os": "linux", "arch": "amd64", "version": "2.0.0"},
			expected: "linux/amd64/tool-2.0.0",
		},
		{
			name:     "no variables",
			input:    "static-string",
			vars:     map[string]string{"version": "1.0.0"},
			expected: "static-string",
		},
		{
			name:     "empty vars map",
			input:    "tool-{version}.tar.gz",
			vars:     map[string]string{},
			expected: "tool-{version}.tar.gz",
		},
		{
			name:     "empty string",
			input:    "",
			vars:     map[string]string{"version": "1.0.0"},
			expected: "",
		},
		{
			name:     "variable used multiple times",
			input:    "{version}-{version}",
			vars:     map[string]string{"version": "1.0"},
			expected: "1.0-1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExpandVars(tt.input, tt.vars)
			if result != tt.expected {
				t.Errorf("ExpandVars(%q, %v) = %q, want %q", tt.input, tt.vars, result, tt.expected)
			}
		})
	}
}

func TestGetStandardVars(t *testing.T) {
	vars := GetStandardVars("1.2.3", "/install/dir", "/work/dir", "/libs/dir")

	if vars["version"] != "1.2.3" {
		t.Errorf("version = %q, want %q", vars["version"], "1.2.3")
	}
	if vars["install_dir"] != "/install/dir" {
		t.Errorf("install_dir = %q, want %q", vars["install_dir"], "/install/dir")
	}
	if vars["work_dir"] != "/work/dir" {
		t.Errorf("work_dir = %q, want %q", vars["work_dir"], "/work/dir")
	}
	if vars["libs_dir"] != "/libs/dir" {
		t.Errorf("libs_dir = %q, want %q", vars["libs_dir"], "/libs/dir")
	}
	// os and arch should be mapped from runtime values
	if vars["os"] == "" {
		t.Error("os should not be empty")
	}
	if vars["arch"] == "" {
		t.Error("arch should not be empty")
	}
}

func TestMapOS(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"darwin", "darwin"},
		{"linux", "linux"},
		{"windows", "windows"},
		{"freebsd", "freebsd"}, // unknown OS returns as-is
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := MapOS(tt.input)
			if result != tt.expected {
				t.Errorf("MapOS(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMapArch(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"amd64", "amd64"},
		{"arm64", "arm64"},
		{"386", "386"},
		{"mips", "mips"}, // unknown arch returns as-is
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := MapArch(tt.input)
			if result != tt.expected {
				t.Errorf("MapArch(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestApplyMapping(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		mapping  map[string]string
		expected string
	}{
		{
			name:     "value in mapping",
			value:    "darwin",
			mapping:  map[string]string{"darwin": "macos", "linux": "linux"},
			expected: "macos",
		},
		{
			name:     "value not in mapping",
			value:    "freebsd",
			mapping:  map[string]string{"darwin": "macos", "linux": "linux"},
			expected: "freebsd",
		},
		{
			name:     "empty mapping",
			value:    "darwin",
			mapping:  map[string]string{},
			expected: "darwin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyMapping(tt.value, tt.mapping)
			if result != tt.expected {
				t.Errorf("ApplyMapping(%q, %v) = %q, want %q", tt.value, tt.mapping, result, tt.expected)
			}
		})
	}
}

func TestGetString(t *testing.T) {
	params := map[string]interface{}{
		"name":   "test",
		"number": 42,
	}

	// Test existing string key
	val, ok := GetString(params, "name")
	if !ok || val != "test" {
		t.Errorf("GetString for 'name' = (%q, %v), want (%q, true)", val, ok, "test")
	}

	// Test non-existent key
	val, ok = GetString(params, "missing")
	if ok || val != "" {
		t.Errorf("GetString for 'missing' = (%q, %v), want (%q, false)", val, ok, "")
	}

	// Test non-string value
	val, ok = GetString(params, "number")
	if ok || val != "" {
		t.Errorf("GetString for 'number' = (%q, %v), want (%q, false)", val, ok, "")
	}
}

func TestGetInt(t *testing.T) {
	params := map[string]interface{}{
		"int_val":   42,
		"int64_val": int64(100),
		"string":    "not a number",
	}

	// Test int value
	val, ok := GetInt(params, "int_val")
	if !ok || val != 42 {
		t.Errorf("GetInt for 'int_val' = (%d, %v), want (42, true)", val, ok)
	}

	// Test int64 value
	val, ok = GetInt(params, "int64_val")
	if !ok || val != 100 {
		t.Errorf("GetInt for 'int64_val' = (%d, %v), want (100, true)", val, ok)
	}

	// Test non-existent key
	val, ok = GetInt(params, "missing")
	if ok || val != 0 {
		t.Errorf("GetInt for 'missing' = (%d, %v), want (0, false)", val, ok)
	}

	// Test non-int value
	val, ok = GetInt(params, "string")
	if ok || val != 0 {
		t.Errorf("GetInt for 'string' = (%d, %v), want (0, false)", val, ok)
	}
}

func TestGetBool(t *testing.T) {
	params := map[string]interface{}{
		"enabled":  true,
		"disabled": false,
		"string":   "not a bool",
	}

	// Test true value
	val, ok := GetBool(params, "enabled")
	if !ok || !val {
		t.Errorf("GetBool for 'enabled' = (%v, %v), want (true, true)", val, ok)
	}

	// Test false value
	val, ok = GetBool(params, "disabled")
	if !ok || val {
		t.Errorf("GetBool for 'disabled' = (%v, %v), want (false, true)", val, ok)
	}

	// Test non-existent key
	val, ok = GetBool(params, "missing")
	if ok || val {
		t.Errorf("GetBool for 'missing' = (%v, %v), want (false, false)", val, ok)
	}

	// Test non-bool value
	val, ok = GetBool(params, "string")
	if ok || val {
		t.Errorf("GetBool for 'string' = (%v, %v), want (false, false)", val, ok)
	}
}

func TestGetStringSlice(t *testing.T) {
	params := map[string]interface{}{
		"string_slice":    []string{"a", "b", "c"},
		"interface_slice": []interface{}{"x", "y", "z"},
		"mixed_slice":     []interface{}{"a", 1, "b"},
		"not_slice":       "string",
	}

	// Test []string
	val, ok := GetStringSlice(params, "string_slice")
	if !ok || len(val) != 3 || val[0] != "a" {
		t.Errorf("GetStringSlice for 'string_slice' = (%v, %v), want ([a b c], true)", val, ok)
	}

	// Test []interface{} with strings
	val, ok = GetStringSlice(params, "interface_slice")
	if !ok || len(val) != 3 || val[0] != "x" {
		t.Errorf("GetStringSlice for 'interface_slice' = (%v, %v), want ([x y z], true)", val, ok)
	}

	// Test []interface{} with mixed types
	val, ok = GetStringSlice(params, "mixed_slice")
	if ok || val != nil {
		t.Errorf("GetStringSlice for 'mixed_slice' = (%v, %v), want (nil, false)", val, ok)
	}

	// Test non-existent key
	val, ok = GetStringSlice(params, "missing")
	if ok || val != nil {
		t.Errorf("GetStringSlice for 'missing' = (%v, %v), want (nil, false)", val, ok)
	}

	// Test non-slice value
	val, ok = GetStringSlice(params, "not_slice")
	if ok || val != nil {
		t.Errorf("GetStringSlice for 'not_slice' = (%v, %v), want (nil, false)", val, ok)
	}
}

func TestGetMapStringString(t *testing.T) {
	params := map[string]interface{}{
		"string_map":    map[string]string{"a": "1", "b": "2"},
		"interface_map": map[string]interface{}{"x": "10", "y": "20"},
		"mixed_map":     map[string]interface{}{"a": "1", "b": 2},
		"not_map":       "string",
	}

	// Test map[string]string
	val, ok := GetMapStringString(params, "string_map")
	if !ok || len(val) != 2 || val["a"] != "1" {
		t.Errorf("GetMapStringString for 'string_map' = (%v, %v), want ({a:1 b:2}, true)", val, ok)
	}

	// Test map[string]interface{} with strings
	val, ok = GetMapStringString(params, "interface_map")
	if !ok || len(val) != 2 || val["x"] != "10" {
		t.Errorf("GetMapStringString for 'interface_map' = (%v, %v), want ({x:10 y:20}, true)", val, ok)
	}

	// Test map[string]interface{} with mixed types
	val, ok = GetMapStringString(params, "mixed_map")
	if ok || val != nil {
		t.Errorf("GetMapStringString for 'mixed_map' = (%v, %v), want (nil, false)", val, ok)
	}

	// Test non-existent key
	val, ok = GetMapStringString(params, "missing")
	if ok || val != nil {
		t.Errorf("GetMapStringString for 'missing' = (%v, %v), want (nil, false)", val, ok)
	}

	// Test non-map value
	val, ok = GetMapStringString(params, "not_map")
	if ok || val != nil {
		t.Errorf("GetMapStringString for 'not_map' = (%v, %v), want (nil, false)", val, ok)
	}
}

func TestVerifyChecksum_SHA256(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello world")

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	// SHA256 of "hello world"
	correctChecksum := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	// Test correct checksum
	if err := VerifyChecksum(testFile, correctChecksum, "sha256"); err != nil {
		t.Errorf("VerifyChecksum with correct checksum failed: %v", err)
	}

	// Test incorrect checksum
	if err := VerifyChecksum(testFile, "wrongchecksum", "sha256"); err == nil {
		t.Error("VerifyChecksum with incorrect checksum should fail")
	}

	// Test with uppercase checksum (should work)
	if err := VerifyChecksum(testFile, "B94D27B9934D3E08A52E52D7DA7DABFAC484EFE37A5380EE9088F7ACE2EFCDE9", "sha256"); err != nil {
		t.Errorf("VerifyChecksum with uppercase checksum failed: %v", err)
	}
}

func TestVerifyChecksum_SHA512(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello world")

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	// SHA512 of "hello world"
	correctChecksum := "309ecc489c12d6eb4cc40f50c902f2b4d0ed77ee511a7c7a9bcd3ca86d4cd86f989dd35bc5ff499670da34255b45b0cfd830e81f605dcf7dc5542e93ae9cd76f"

	// Test correct checksum
	if err := VerifyChecksum(testFile, correctChecksum, "sha512"); err != nil {
		t.Errorf("VerifyChecksum SHA512 with correct checksum failed: %v", err)
	}

	// Test incorrect checksum
	if err := VerifyChecksum(testFile, "wrongchecksum", "sha512"); err == nil {
		t.Error("VerifyChecksum SHA512 with incorrect checksum should fail")
	}
}

func TestVerifyChecksum_UnsupportedAlgo(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := VerifyChecksum(testFile, "checksum", "md5"); err == nil {
		t.Error("VerifyChecksum with unsupported algorithm should fail")
	}
}

func TestVerifyChecksum_FileNotFound(t *testing.T) {
	if err := VerifyChecksum("/nonexistent/file", "checksum", "sha256"); err == nil {
		t.Error("VerifyChecksum with non-existent file should fail")
	}
}

func TestReadChecksumFile(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "checksum only",
			content:  "abc123def456",
			expected: "abc123def456",
		},
		{
			name:     "checksum with filename",
			content:  "abc123def456 file.tar.gz",
			expected: "abc123def456",
		},
		{
			name:     "checksum with whitespace",
			content:  "  abc123def456  \n",
			expected: "abc123def456",
		},
		{
			name:     "BSD-style format",
			content:  "abc123def456  file.tar.gz",
			expected: "abc123def456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksumFile := filepath.Join(tmpDir, tt.name+".sha256")
			if err := os.WriteFile(checksumFile, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			result, err := ReadChecksumFile(checksumFile)
			if err != nil {
				t.Fatalf("ReadChecksumFile failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("ReadChecksumFile() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestReadChecksumFile_NotFound(t *testing.T) {
	_, err := ReadChecksumFile("/nonexistent/file.sha256")
	if err == nil {
		t.Error("ReadChecksumFile with non-existent file should fail")
	}
}

func TestReadChecksumFile_MultiLine(t *testing.T) {
	tmpDir := t.TempDir()

	// Multi-line checksum file like HashiCorp SHA256SUMS
	multiLineContent := `62fca69aa1fc3093a522182ab86ed0c5095fafc146b432cd52dca861c0a3545b  terraform_1.14.2_darwin_amd64.zip
c81719634fc5f325b3711e8b9c5444bd0d7b8590b0b9aa2ff8f00ff50a9d60c8  terraform_1.14.2_darwin_arm64.zip
8314673d57e9fb8e01bfc98d074f51f7efb6e55484cfb2b10baed686de2190da  terraform_1.14.2_linux_amd64.zip
01e5a239ad96bc40f37d6eca8cd8b6b0a72ffb227162574c0144a7d0e0741f86  terraform_1.14.2_linux_arm.zip
`

	tests := []struct {
		name           string
		targetFilename string
		expected       string
		wantErr        bool
	}{
		{
			name:           "find linux_amd64",
			targetFilename: "terraform_1.14.2_linux_amd64.zip",
			expected:       "8314673d57e9fb8e01bfc98d074f51f7efb6e55484cfb2b10baed686de2190da",
		},
		{
			name:           "find darwin_amd64",
			targetFilename: "terraform_1.14.2_darwin_amd64.zip",
			expected:       "62fca69aa1fc3093a522182ab86ed0c5095fafc146b432cd52dca861c0a3545b",
		},
		{
			name:           "find darwin_arm64",
			targetFilename: "terraform_1.14.2_darwin_arm64.zip",
			expected:       "c81719634fc5f325b3711e8b9c5444bd0d7b8590b0b9aa2ff8f00ff50a9d60c8",
		},
		{
			name:           "not found",
			targetFilename: "terraform_1.14.2_windows_amd64.zip",
			wantErr:        true,
		},
		{
			name:           "no target falls back to first line",
			targetFilename: "",
			expected:       "62fca69aa1fc3093a522182ab86ed0c5095fafc146b432cd52dca861c0a3545b",
		},
	}

	checksumFile := filepath.Join(tmpDir, "SHA256SUMS")
	if err := os.WriteFile(checksumFile, []byte(multiLineContent), 0644); err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result string
			var err error
			if tt.targetFilename == "" {
				result, err = ReadChecksumFile(checksumFile)
			} else {
				result, err = ReadChecksumFile(checksumFile, tt.targetFilename)
			}

			if tt.wantErr {
				if err == nil {
					t.Error("ReadChecksumFile should have failed")
				}
				return
			}

			if err != nil {
				t.Fatalf("ReadChecksumFile failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("ReadChecksumFile() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestResolveGo_IgnoresGoTools(t *testing.T) {
	// Create a temporary directory structure to simulate $TSUKU_HOME
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	toolsDir := filepath.Join(tmpHome, ".tsuku", "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create directories that should be matched (Go toolchain)
	goToolchainDir := filepath.Join(toolsDir, "go-1.23.4", "bin")
	if err := os.MkdirAll(goToolchainDir, 0755); err != nil {
		t.Fatal(err)
	}
	goExe := filepath.Join(goToolchainDir, "go")
	if err := os.WriteFile(goExe, []byte("#!/bin/sh\necho go"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create directories that should NOT be matched (Go tools)
	goMigrateDir := filepath.Join(toolsDir, "go-migrate-4.19.1", "bin")
	if err := os.MkdirAll(goMigrateDir, 0755); err != nil {
		t.Fatal(err)
	}
	goTaskDir := filepath.Join(toolsDir, "go-task-3.38.0", "bin")
	if err := os.MkdirAll(goTaskDir, 0755); err != nil {
		t.Fatal(err)
	}

	result := ResolveGo()

	// Should find the Go toolchain, not go-migrate or go-task
	if result == "" {
		t.Error("ResolveGo() returned empty string, expected to find go-1.23.4")
	}
	if filepath.Base(filepath.Dir(filepath.Dir(result))) != "go-1.23.4" {
		t.Errorf("ResolveGo() found wrong directory: %s, expected go-1.23.4", result)
	}
}

func TestResolveGo_PicksLatestVersion(t *testing.T) {
	// Create a temporary directory structure to simulate $TSUKU_HOME
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	toolsDir := filepath.Join(tmpHome, ".tsuku", "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create multiple Go toolchain versions
	for _, version := range []string{"go-1.21.0", "go-1.22.5", "go-1.23.4"} {
		dir := filepath.Join(toolsDir, version, "bin")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		goExe := filepath.Join(dir, "go")
		if err := os.WriteFile(goExe, []byte("#!/bin/sh\necho go"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	result := ResolveGo()

	// Should pick the latest version (lexicographically)
	if result == "" {
		t.Error("ResolveGo() returned empty string")
	}
	if filepath.Base(filepath.Dir(filepath.Dir(result))) != "go-1.23.4" {
		t.Errorf("ResolveGo() picked wrong version: %s, expected go-1.23.4", result)
	}
}

func TestResolveGoVersion(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	toolsDir := filepath.Join(tmpHome, ".tsuku", "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create specific Go version
	goDir := filepath.Join(toolsDir, "go-1.21.5", "bin")
	if err := os.MkdirAll(goDir, 0755); err != nil {
		t.Fatal(err)
	}
	goExe := filepath.Join(goDir, "go")
	if err := os.WriteFile(goExe, []byte("#!/bin/sh\necho go"), 0755); err != nil {
		t.Fatal(err)
	}

	// Should find the specific version
	result := ResolveGoVersion("1.21.5")
	if result == "" {
		t.Error("ResolveGoVersion(\"1.21.5\") returned empty string")
	}
	if result != goExe {
		t.Errorf("ResolveGoVersion(\"1.21.5\") = %q, want %q", result, goExe)
	}

	// Should NOT find a different version
	result = ResolveGoVersion("1.22.0")
	if result != "" {
		t.Errorf("ResolveGoVersion(\"1.22.0\") = %q, want empty string", result)
	}
}

func TestGetGoVersion(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock go that outputs version
	goExe := filepath.Join(tmpDir, "go")
	mockScript := `#!/bin/sh
echo "go version go1.23.4 linux/amd64"
`
	if err := os.WriteFile(goExe, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	version := GetGoVersion(goExe)
	if version != "1.23.4" {
		t.Errorf("GetGoVersion() = %q, want %q", version, "1.23.4")
	}
}

func TestGetGoVersion_InvalidOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mock go that outputs garbage
	goExe := filepath.Join(tmpDir, "go")
	mockScript := `#!/bin/sh
echo "not a valid version output"
`
	if err := os.WriteFile(goExe, []byte(mockScript), 0755); err != nil {
		t.Fatal(err)
	}

	version := GetGoVersion(goExe)
	if version != "" {
		t.Errorf("GetGoVersion() = %q, want empty string", version)
	}
}

func TestGetGoVersion_NonExistent(t *testing.T) {
	version := GetGoVersion("/nonexistent/go")
	if version != "" {
		t.Errorf("GetGoVersion() = %q, want empty string", version)
	}
}
