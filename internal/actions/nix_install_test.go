package actions

import (
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
