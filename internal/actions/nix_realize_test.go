package actions

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestNixRealizeAction_Name(t *testing.T) {
	t.Parallel()
	action := &NixRealizeAction{}
	if action.Name() != "nix_realize" {
		t.Errorf("Name() = %q, want %q", action.Name(), "nix_realize")
	}
}

func TestIsValidFlakeRef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ref      string
		expected bool
	}{
		// Valid flake references
		{"nixpkgs package", "nixpkgs#hello", true},
		{"github flake", "github:user/repo#package", true},
		{"github with rev", "github:user/repo/abc123#pkg", true},
		{"path flake", "path:/some/path#attr", true},
		{"nested attribute", "nixpkgs#python3Packages.pytorch", true},
		{"with hyphen", "nixpkgs#cargo-audit", true},
		{"with underscore", "nixpkgs#my_package", true},
		{"with at sign", "github:user/repo@v1.0.0#pkg", true},

		// Invalid - no hash separator
		{"no hash", "nixpkgs", false},
		{"no hash github", "github:user/repo", false},

		// Invalid - empty or too long
		{"empty", "", false},
		{"too long", string(make([]byte, 513)), false},

		// Invalid - shell metacharacters
		{"semicolon", "nixpkgs#hello;rm -rf /", false},
		{"pipe", "nixpkgs#hello|evil", false},
		{"ampersand", "nixpkgs#pkg&&evil", false},
		{"dollar", "nixpkgs#$(evil)", false},
		{"backtick", "nixpkgs#`evil`", false},
		{"redirect", "nixpkgs#pkg>file", false},
		{"redirect in", "nixpkgs#pkg<file", false},
		{"parentheses", "nixpkgs#pkg()", false},
		{"brackets", "nixpkgs#pkg[]", false},
		{"braces", "nixpkgs#pkg{}", false},
		{"space", "nixpkgs#hello world", false},

		// Invalid - path traversal
		{"path traversal", "nixpkgs#../etc/passwd", false},
		{"dot dot", "nixpkgs#foo..bar", false},

		// Invalid - special characters
		{"single quote", "nixpkgs#pkg'foo", false},
		{"double quote", "nixpkgs#pkg\"foo", false},
		{"newline", "nixpkgs#pkg\nfoo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidFlakeRef(tt.ref)
			if result != tt.expected {
				t.Errorf("isValidFlakeRef(%q) = %v, expected %v", tt.ref, result, tt.expected)
			}
		})
	}
}

func TestIsValidNixStorePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		// Valid store paths
		{"derivation", "/nix/store/abc123-hello-1.0.0.drv", true},
		{"output", "/nix/store/xyz789-hello-1.0.0", true},
		{"with plus", "/nix/store/abc123-gcc++-12.0", true},
		{"long hash", "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-pkg", true},

		// Invalid - not starting with /nix/store/
		{"no prefix", "abc123-hello", false},
		{"wrong prefix", "/usr/store/abc123", false},
		{"partial prefix", "/nix/abc123", false},

		// Invalid - empty or too long
		{"empty", "", false},
		{"too long", "/nix/store/" + string(make([]byte, 300)), false},

		// Invalid - shell metacharacters
		{"semicolon", "/nix/store/abc;evil", false},
		{"pipe", "/nix/store/abc|evil", false},
		{"dollar", "/nix/store/$(evil)", false},
		{"backtick", "/nix/store/abc`evil`", false},
		{"space", "/nix/store/abc def", false},

		// Invalid - path traversal
		{"path traversal", "/nix/store/../etc/passwd", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidNixStorePath(tt.path)
			if result != tt.expected {
				t.Errorf("isValidNixStorePath(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGetLocksMap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		params     map[string]interface{}
		expectOK   bool
		expectKeys []string
	}{
		{
			name:     "no locks",
			params:   map[string]interface{}{},
			expectOK: false,
		},
		{
			name: "interface map",
			params: map[string]interface{}{
				"locks": map[string]interface{}{
					"locked_ref": "github:NixOS/nixpkgs/abc123",
					"system":     "x86_64-linux",
				},
			},
			expectOK:   true,
			expectKeys: []string{"locked_ref", "system"},
		},
		{
			name: "string map",
			params: map[string]interface{}{
				"locks": map[string]string{
					"locked_ref": "test",
				},
			},
			expectOK:   true,
			expectKeys: []string{"locked_ref"},
		},
		{
			name: "json string",
			params: map[string]interface{}{
				"locks": `{"locked_ref": "test", "system": "linux"}`,
			},
			expectOK:   true,
			expectKeys: []string{"locked_ref", "system"},
		},
		{
			name: "invalid type",
			params: map[string]interface{}{
				"locks": 123,
			},
			expectOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := getLocksMap(tt.params)
			if ok != tt.expectOK {
				t.Errorf("getLocksMap() ok = %v, expected %v", ok, tt.expectOK)
			}
			if tt.expectOK && result != nil {
				for _, key := range tt.expectKeys {
					if _, exists := result[key]; !exists {
						t.Errorf("getLocksMap() missing expected key %q", key)
					}
				}
			}
		})
	}
}

func TestNixRealizeAction_Execute_PlatformCheck(t *testing.T) {
	t.Parallel()
	// Skip on Linux - the platform check passes there
	if runtime.GOOS == "linux" {
		t.Skip("Skipping platform check test on Linux")
	}

	action := &NixRealizeAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
	}
	params := map[string]interface{}{
		"flake_ref":   "nixpkgs#hello",
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

func TestNixRealizeAction_Execute_MissingParams(t *testing.T) {
	t.Parallel()
	// Skip on non-Linux - will fail at platform check
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixRealizeAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
	}

	tests := []struct {
		name           string
		params         map[string]interface{}
		expectedErrMsg string
	}{
		{
			name:           "missing flake_ref and package",
			params:         map[string]interface{}{},
			expectedErrMsg: "requires 'flake_ref' or 'package'",
		},
		{
			name: "missing executables",
			params: map[string]interface{}{
				"flake_ref": "nixpkgs#hello",
			},
			expectedErrMsg: "requires 'executables'",
		},
		{
			name: "empty executables",
			params: map[string]interface{}{
				"flake_ref":   "nixpkgs#hello",
				"executables": []string{},
			},
			expectedErrMsg: "requires 'executables'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Error("Expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("Expected error containing %q, got: %v", tt.expectedErrMsg, err)
			}
		})
	}
}

func TestNixRealizeAction_Execute_InvalidInputs(t *testing.T) {
	t.Parallel()
	// Skip on non-Linux - will fail at platform check
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixRealizeAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
	}

	tests := []struct {
		name           string
		params         map[string]interface{}
		expectedErrMsg string
	}{
		{
			name: "invalid flake ref",
			params: map[string]interface{}{
				"flake_ref":   "invalid;injection",
				"executables": []string{"hello"},
			},
			expectedErrMsg: "invalid flake reference",
		},
		{
			name: "invalid package name",
			params: map[string]interface{}{
				"package":     "pkg;rm -rf /",
				"executables": []string{"hello"},
			},
			expectedErrMsg: "invalid nixpkgs package name",
		},
		{
			name: "invalid executable name",
			params: map[string]interface{}{
				"flake_ref":   "nixpkgs#hello",
				"executables": []string{"../evil"},
			},
			expectedErrMsg: "invalid executable name",
		},
		{
			name: "invalid derivation path",
			params: map[string]interface{}{
				"flake_ref":       "nixpkgs#hello",
				"executables":     []string{"hello"},
				"derivation_path": "/tmp/evil.drv",
			},
			expectedErrMsg: "invalid derivation path",
		},
		{
			name: "invalid output path",
			params: map[string]interface{}{
				"flake_ref":   "nixpkgs#hello",
				"executables": []string{"hello"},
				"output_path": "/home/user/evil",
			},
			expectedErrMsg: "invalid output path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Error("Expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("Expected error containing %q, got: %v", tt.expectedErrMsg, err)
			}
		})
	}
}

func TestNixRealizeIsPrimitive(t *testing.T) {
	t.Parallel()
	if !IsPrimitive("nix_realize") {
		t.Error("nix_realize should be registered as a primitive")
	}
}

func TestCreateNixRealizeWrapper(t *testing.T) {
	t.Parallel()
	// Create a temporary directory for the test
	tmpDir := t.TempDir()
	binDir := tmpDir

	// Test creating a wrapper
	err := createNixRealizeWrapper("hello", binDir, "/home/user/.tsuku/.nix-internal", "nixpkgs#hello")
	if err != nil {
		t.Fatalf("createNixRealizeWrapper() error = %v", err)
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

	// Verify it references the flake
	if !strings.Contains(contentStr, "nixpkgs#hello") {
		t.Error("wrapper should contain flake reference")
	}

	// Verify it uses nix shell with --no-update-lock-file
	if !strings.Contains(contentStr, "--no-update-lock-file") {
		t.Error("wrapper should use --no-update-lock-file flag")
	}

	// Verify it mentions nix_realize in comments
	if !strings.Contains(contentStr, "nix_realize") {
		t.Error("wrapper should mention nix_realize in comments")
	}
}

func TestCreateNixRealizeWrapper_MultipleExecutables(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	executables := []string{"foo", "bar", "baz"}
	for _, exe := range executables {
		err := createNixRealizeWrapper(exe, tmpDir, "/home/user/.tsuku/.nix-internal", "nixpkgs#mypackage")
		if err != nil {
			t.Fatalf("createNixRealizeWrapper(%q) error = %v", exe, err)
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

func TestIsValidFlakeRef_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ref      string
		expected bool
	}{
		// More edge cases for coverage
		// Note: "#" contains "#" so it passes the Contains check, but is technically valid
		{"just hash", "#", true},                     // Contains # so passes validation
		{"multiple hashes", "nixpkgs#foo#bar", true}, // Valid - multiple # are allowed
		{"github with long path", "github:org/repo/a/b/c#pkg", true},
		{"with numbers in attr", "nixpkgs#python310", true},
		{"uppercase in attr", "nixpkgs#GCC", true},
		{"equals sign", "nixpkgs#foo=bar", false}, // Invalid character
		{"percent", "nixpkgs#foo%20bar", false},   // Invalid character
		{"hash sign", "nixpkgs#foo#", true},       // Trailing # is technically valid
		{"tilde", "nixpkgs#foo~bar", false},       // Invalid character
		{"star", "nixpkgs#foo*", false},           // Invalid character
		{"question mark", "nixpkgs#foo?", false},  // Invalid character
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidFlakeRef(tt.ref)
			if result != tt.expected {
				t.Errorf("isValidFlakeRef(%q) = %v, expected %v", tt.ref, result, tt.expected)
			}
		})
	}
}

func TestIsValidNixStorePath_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		// More edge cases
		// /nix/store/ is 11 chars, so 245 chars after = 256 total (at limit)
		{"exactly at limit", "/nix/store/" + strings.Repeat("a", 245), true}, // 11 + 245 = 256 = limit
		{"just over limit", "/nix/store/" + strings.Repeat("a", 246), false}, // 11 + 246 = 257 > limit
		{"with multiple dots", "/nix/store/abc-1.2.3.4.5", true},
		{"with double hyphen", "/nix/store/abc--def", true},
		{"store only", "/nix/store/", true}, // Just the prefix is valid technically
		{"valid drv extension", "/nix/store/abc.drv", true},
		{"nested path", "/nix/store/abc/def/ghi", true},
		{"ampersand", "/nix/store/abc&def", false},
		{"hash in path", "/nix/store/abc#def", false},
		{"at sign", "/nix/store/abc@def", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidNixStorePath(tt.path)
			if result != tt.expected {
				t.Errorf("isValidNixStorePath(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGetLocksMap_InvalidJSON(t *testing.T) {
	t.Parallel()
	params := map[string]interface{}{
		"locks": "{invalid json}",
	}

	result, ok := getLocksMap(params)
	if ok {
		t.Error("getLocksMap() should return false for invalid JSON")
	}
	if result != nil {
		t.Error("getLocksMap() should return nil for invalid JSON")
	}
}

func TestGetLocksMap_EmptyStringMap(t *testing.T) {
	t.Parallel()
	params := map[string]interface{}{
		"locks": map[string]string{},
	}

	result, ok := getLocksMap(params)
	if !ok {
		t.Error("getLocksMap() should return true for empty string map")
	}
	if len(result) != 0 {
		t.Errorf("getLocksMap() should return empty map, got %v", result)
	}
}

func TestNixRealizeAction_Execute_PackageFallback(t *testing.T) {
	t.Parallel()
	// Skip on non-Linux - will fail at platform check
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixRealizeAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
	}

	// Test that package parameter works as fallback when flake_ref is missing
	params := map[string]interface{}{
		"package":     "hello",
		"executables": []string{"hello"},
	}

	// This will fail later (no nix-portable or nix itself) but should pass validation
	err := action.Execute(ctx, params)
	// Should NOT fail with "requires 'flake_ref' or 'package'" error
	if err != nil && strings.Contains(err.Error(), "requires 'flake_ref' or 'package'") {
		t.Errorf("package parameter should be accepted as alternative to flake_ref")
	}
}

func TestNixRealizeAction_Execute_BothFlakeRefAndPackage(t *testing.T) {
	t.Parallel()
	// Skip on non-Linux - will fail at platform check
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixRealizeAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
	}

	// Test that flake_ref takes precedence when both are provided
	params := map[string]interface{}{
		"flake_ref":   "nixpkgs#hello",
		"package":     "world",
		"executables": []string{"hello"},
	}

	// This will fail later (no nix-portable) but should pass validation
	err := action.Execute(ctx, params)
	// Should NOT fail at parameter validation
	if err != nil && strings.Contains(err.Error(), "requires 'flake_ref' or 'package'") {
		t.Errorf("should accept when both flake_ref and package are provided")
	}
}

func TestNixRealizeAction_Execute_WithLocks(t *testing.T) {
	t.Parallel()
	// Skip on non-Linux - will fail at platform check
	if runtime.GOOS != "linux" {
		t.Skip("Skipping on non-Linux")
	}

	action := &NixRealizeAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		InstallDir: t.TempDir(),
	}

	// Test with locks parameter
	params := map[string]interface{}{
		"flake_ref":   "nixpkgs#hello",
		"executables": []string{"hello"},
		"locks": map[string]interface{}{
			"locked_ref":  "github:NixOS/nixpkgs/abc123",
			"system":      "x86_64-linux",
			"nix_version": "2.18.1",
		},
	}

	// This will fail later (no nix-portable) but should pass validation
	err := action.Execute(ctx, params)
	// Should NOT fail at parameter validation or locks parsing
	if err != nil && strings.Contains(err.Error(), "locks") {
		t.Errorf("should accept valid locks parameter: %v", err)
	}
}
