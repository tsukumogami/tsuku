package actions

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestIsValidNixPackage(t *testing.T) {
	tests := []struct {
		name     string
		pkg      string
		expected bool
	}{
		// Valid package names
		{"simple package", "hello", true},
		{"with hyphen", "cargo-audit", true},
		{"with underscore", "my_package", true},
		{"nested attribute", "python3Packages.pytorch", true},
		{"deeply nested", "nodePackages.some-tool", true},
		{"numbers", "gcc12", true},
		{"starts with number", "3proxy", true},

		// Invalid - empty or too long
		{"empty", "", false},
		{"too long", string(make([]byte, 257)), false},

		// Invalid - shell metacharacters (command injection)
		{"semicolon injection", "hello; rm -rf /", false},
		{"pipe injection", "hello|cat /etc/passwd", false},
		{"ampersand injection", "hello && evil", false},
		{"dollar injection", "$(evil)", false},
		{"backtick injection", "pkg`whoami`", false},
		{"redirect injection", "hello > /etc/passwd", false},
		{"redirect in injection", "hello < /etc/passwd", false},
		{"parentheses", "hello()", false},
		{"brackets", "hello[]", false},
		{"braces", "hello{}", false},
		{"space injection", "hello world", false},

		// Invalid - path traversal
		{"path traversal", "../../../etc/passwd", false},
		{"dot dot in middle", "foo..bar", false},
		{"slash", "foo/bar", false},
		{"backslash", "foo\\bar", false},

		// Invalid - special characters
		{"single quote", "hello'world", false},
		{"double quote", "hello\"world", false},
		{"newline", "hello\nworld", false},
		{"tab", "hello\tworld", false},
		{"null byte", "hello\x00world", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidNixPackage(tt.pkg)
			if result != tt.expected {
				t.Errorf("isValidNixPackage(%q) = %v, expected %v", tt.pkg, result, tt.expected)
			}
		})
	}
}

func TestValidateExecutableName(t *testing.T) {
	tests := []struct {
		name        string
		exe         string
		expectError bool
	}{
		// Valid executable names
		{"simple", "hello", false},
		{"with hyphen", "cargo-audit", false},
		{"with underscore", "my_tool", false},
		{"with numbers", "python3", false},
		{"with dot", "file.sh", false},

		// Invalid - path separators
		{"forward slash", "bin/hello", true},
		{"backslash", "bin\\hello", true},
		{"path traversal", "../hello", true},
		{"dot dot", "..", true},
		{"single dot", ".", true},

		// Invalid - shell metacharacters
		{"dollar", "$PATH", true},
		{"backtick", "`cmd`", true},
		{"pipe", "cmd|evil", true},
		{"semicolon", "cmd;evil", true},
		{"ampersand", "cmd&evil", true},
		{"redirect out", "cmd>file", true},
		{"redirect in", "cmd<file", true},
		{"parentheses open", "cmd(", true},
		{"parentheses close", "cmd)", true},
		{"bracket open", "cmd[", true},
		{"bracket close", "cmd]", true},
		{"brace open", "cmd{", true},
		{"brace close", "cmd}", true},

		// Invalid - control characters
		{"empty", "", true},
		{"newline", "hello\nworld", true},
		{"tab", "hello\tworld", true},
		{"null", "hello\x00world", true},
		{"bell", "hello\x07world", true},

		// Invalid - too long
		{"too long", string(make([]byte, 257)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExecutableName(tt.exe)
			if tt.expectError && err == nil {
				t.Errorf("validateExecutableName(%q) expected error, got nil", tt.exe)
			}
			if !tt.expectError && err != nil {
				t.Errorf("validateExecutableName(%q) unexpected error: %v", tt.exe, err)
			}
		})
	}
}

func TestResolveNixPortable(t *testing.T) {
	// ResolveNixPortable should return empty string when not installed
	// This is a quick sanity check - integration tests will verify actual installation
	result := ResolveNixPortable()

	// Can't assert specific value since it depends on whether nix-portable is installed
	// Just verify it doesn't panic and returns a string
	_ = result
}

func TestGetNixInternalDir(t *testing.T) {
	// Should return a path ending with .nix-internal
	dir, err := GetNixInternalDir()
	if err != nil {
		t.Fatalf("GetNixInternalDir() error: %v", err)
	}

	if dir == "" {
		t.Error("GetNixInternalDir() returned empty string")
	}

	// Should contain .nix-internal
	if !strings.Contains(dir, ".nix-internal") {
		t.Errorf("GetNixInternalDir() = %q, expected to contain '.nix-internal'", dir)
	}
}

func TestNixInstallAction_Decompose_PlatformCheck(t *testing.T) {
	// Skip on Linux - the platform check passes there
	if runtime.GOOS == "linux" {
		t.Skip("Skipping platform check test on Linux")
	}

	action := &NixInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
	}
	params := map[string]interface{}{
		"package":     "hello",
		"executables": []string{"hello"},
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Expected platform error on non-Linux")
	}
	if err != nil && !strings.Contains(err.Error(), "only supports Linux") {
		t.Errorf("Expected 'only supports Linux' error, got: %v", err)
	}
}

func TestNixInstallAction_Decompose_MissingPackage(t *testing.T) {
	// Skip on non-Linux
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
	}
	params := map[string]interface{}{
		"executables": []string{"hello"},
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Expected error for missing package")
	}
	if err != nil && !strings.Contains(err.Error(), "requires 'package'") {
		t.Errorf("Expected 'requires package' error, got: %v", err)
	}
}

func TestNixInstallAction_Decompose_InvalidPackage(t *testing.T) {
	// Skip on non-Linux
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
	}
	params := map[string]interface{}{
		"package":     "hello;rm -rf /",
		"executables": []string{"hello"},
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Expected error for invalid package")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid nixpkgs package name") {
		t.Errorf("Expected 'invalid nixpkgs package name' error, got: %v", err)
	}
}

func TestNixInstallAction_Decompose_MissingExecutables(t *testing.T) {
	// Skip on non-Linux
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
	}
	params := map[string]interface{}{
		"package": "hello",
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Expected error for missing executables")
	}
	if err != nil && !strings.Contains(err.Error(), "requires 'executables'") {
		t.Errorf("Expected 'requires executables' error, got: %v", err)
	}
}

func TestNixInstallAction_Decompose_InvalidExecutable(t *testing.T) {
	// Skip on non-Linux
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
	}
	params := map[string]interface{}{
		"package":     "hello",
		"executables": []string{"../evil"},
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Expected error for invalid executable")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid executable name") {
		t.Errorf("Expected 'invalid executable name' error, got: %v", err)
	}
}

func TestNixInstallAction_ImplementsDecomposable(t *testing.T) {
	// Verify that NixInstallAction implements Decomposable interface
	var _ Decomposable = (*NixInstallAction)(nil)
}

func TestNixInstallAction_Name(t *testing.T) {
	action := &NixInstallAction{}
	if action.Name() != "nix_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "nix_install")
	}
}

func TestNixInstallAction_Execute_PlatformCheck(t *testing.T) {
	// Skip on Linux - the platform check passes there
	if runtime.GOOS == "linux" {
		t.Skip("Skipping platform check test on Linux")
	}

	action := &NixInstallAction{}
	ctx := &ExecutionContext{}
	params := map[string]interface{}{
		"package":     "hello",
		"executables": []string{"hello"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Expected platform error on non-Linux")
	}
	if err != nil && !strings.Contains(err.Error(), "only supports Linux") {
		t.Errorf("Expected 'only supports Linux' error, got: %v", err)
	}
}

func TestNixInstallAction_Execute_MissingPackage(t *testing.T) {
	// Skip on non-Linux - will fail at platform check
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixInstallAction{}
	ctx := &ExecutionContext{}
	params := map[string]interface{}{
		"executables": []string{"hello"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Expected error for missing package")
	}
	if err != nil && !strings.Contains(err.Error(), "requires 'package'") {
		t.Errorf("Expected 'requires package' error, got: %v", err)
	}
}

func TestNixInstallAction_Execute_InvalidPackage(t *testing.T) {
	// Skip on non-Linux - will fail at platform check
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixInstallAction{}
	ctx := &ExecutionContext{}
	params := map[string]interface{}{
		"package":     "hello;rm -rf /",
		"executables": []string{"hello"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Expected error for invalid package")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid nixpkgs package name") {
		t.Errorf("Expected 'invalid nixpkgs package name' error, got: %v", err)
	}
}

func TestNixInstallAction_Execute_MissingExecutables(t *testing.T) {
	// Skip on non-Linux - will fail at platform check
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixInstallAction{}
	ctx := &ExecutionContext{}
	params := map[string]interface{}{
		"package": "hello",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Expected error for missing executables")
	}
	if err != nil && !strings.Contains(err.Error(), "requires 'executables'") {
		t.Errorf("Expected 'requires executables' error, got: %v", err)
	}
}

func TestNixInstallAction_Execute_EmptyExecutables(t *testing.T) {
	// Skip on non-Linux - will fail at platform check
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixInstallAction{}
	ctx := &ExecutionContext{}
	params := map[string]interface{}{
		"package":     "hello",
		"executables": []string{},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Expected error for empty executables")
	}
	if err != nil && !strings.Contains(err.Error(), "requires 'executables'") {
		t.Errorf("Expected 'requires executables' error, got: %v", err)
	}
}

func TestNixInstallAction_Execute_InvalidExecutable(t *testing.T) {
	// Skip on non-Linux - will fail at platform check
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixInstallAction{}
	ctx := &ExecutionContext{}
	params := map[string]interface{}{
		"package":     "hello",
		"executables": []string{"../evil"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Expected error for invalid executable")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid executable name") {
		t.Errorf("Expected 'invalid executable name' error, got: %v", err)
	}
}

func TestNixInstallAction_Decompose_EmptyExecutables(t *testing.T) {
	// Skip on non-Linux
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
	}
	params := map[string]interface{}{
		"package":     "hello",
		"executables": []string{},
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Expected error for empty executables")
	}
	if err != nil && !strings.Contains(err.Error(), "requires 'executables'") {
		t.Errorf("Expected 'requires executables' error, got: %v", err)
	}
}

func TestCreateNixWrapper(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()
	binDir := tmpDir

	// Test creating a wrapper
	err := createNixWrapper("hello", binDir, "/home/user/.tsuku/.nix-internal", "hello")
	if err != nil {
		t.Fatalf("createNixWrapper() error = %v", err)
	}

	// Verify the wrapper file was created
	wrapperPath := tmpDir + "/hello"
	info, err := os.Stat(wrapperPath)
	if err != nil {
		t.Fatalf("wrapper file not found: %v", err)
	}

	// Verify it's executable (mode 0755)
	if info.Mode().Perm() != 0755 {
		t.Errorf("wrapper mode = %v, want 0755", info.Mode().Perm())
	}

	// Read and verify content
	content, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatalf("failed to read wrapper: %v", err)
	}

	contentStr := string(content)

	// Verify it starts with shebang
	if !strings.HasPrefix(contentStr, "#!/bin/bash") {
		t.Error("wrapper should start with #!/bin/bash")
	}

	// Verify it contains NP_LOCATION
	if !strings.Contains(contentStr, "NP_LOCATION") {
		t.Error("wrapper should contain NP_LOCATION")
	}

	// Verify it references the package
	if !strings.Contains(contentStr, "nixpkgs#hello") {
		t.Error("wrapper should contain nixpkgs#package reference")
	}

	// Verify it uses nix shell
	if !strings.Contains(contentStr, "nix shell") {
		t.Error("wrapper should use nix shell")
	}

	// Verify it mentions nix_install in comments
	if !strings.Contains(contentStr, "nix_install") {
		t.Error("wrapper should mention nix_install in comments")
	}
}

func TestCreateNixWrapper_MultipleExecutables(t *testing.T) {
	tmpDir := t.TempDir()

	executables := []string{"foo", "bar", "baz"}
	for _, exe := range executables {
		err := createNixWrapper(exe, tmpDir, "/home/user/.tsuku/.nix-internal", "mypackage")
		if err != nil {
			t.Fatalf("createNixWrapper(%q) error = %v", exe, err)
		}
	}

	// Verify all wrappers were created
	for _, exe := range executables {
		wrapperPath := tmpDir + "/" + exe
		if _, err := os.Stat(wrapperPath); err != nil {
			t.Errorf("wrapper for %q not found: %v", exe, err)
		}
	}
}

func TestIsValidNixPackage_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		pkg      string
		expected bool
	}{
		// More edge cases
		{"single char", "a", true},
		{"single digit", "1", true},
		{"uppercase only", "GCC", true},
		{"mixed case", "GnuPG", true},
		{"multiple dots", "a.b.c.d", true},
		{"exactly at limit", strings.Repeat("a", 256), true},
		{"just over limit", strings.Repeat("a", 257), false},
		{"equals sign", "foo=bar", false},
		{"colon", "foo:bar", false},
		{"hash", "foo#bar", false},
		{"at sign", "foo@bar", false},
		{"tilde", "foo~bar", false},
		{"caret", "foo^bar", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidNixPackage(tt.pkg)
			if result != tt.expected {
				t.Errorf("isValidNixPackage(%q) = %v, expected %v", tt.pkg, result, tt.expected)
			}
		})
	}
}

func TestValidateExecutableName_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		exe         string
		expectError bool
	}{
		// More edge cases
		{"single char", "a", false},
		{"exactly at limit", strings.Repeat("a", 256), false},
		{"just over limit", strings.Repeat("a", 257), true},
		{"multiple dots", "a.b.c", false},
		{"starts with dot", ".hidden", false},
		{"ends with dot", "file.", false},
		{"multiple hyphens", "a--b--c", false},
		{"multiple underscores", "a__b__c", false},
		// Note: space is NOT in the shell metacharacter check, so these are valid
		{"space in middle", "foo bar", false},
		{"space at start", " foo", false},
		{"space at end", "foo ", false},
		{"carriage return", "foo\rbar", true},
		{"escape char", "foo\x1bbar", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExecutableName(tt.exe)
			if tt.expectError && err == nil {
				t.Errorf("validateExecutableName(%q) expected error, got nil", tt.exe)
			}
			if !tt.expectError && err != nil {
				t.Errorf("validateExecutableName(%q) unexpected error: %v", tt.exe, err)
			}
		})
	}
}

func TestDetectProotFallback(t *testing.T) {
	// This function checks for proot indicators in nix-portable output
	// We can test it with a fake path that doesn't exist - it should return false
	result := detectProotFallback("/nonexistent/path", "/tmp/test")

	// Should return false since the command will fail
	if result {
		t.Error("detectProotFallback() should return false for nonexistent path")
	}
}
