package actions

import "testing"

func TestIsValidPyPIVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		version  string
		expected bool
	}{
		// Valid versions
		{"simple version", "1.0.0", true},
		{"two digit version", "24.10.0", true},
		{"single number", "1", true},
		{"two numbers", "1.0", true},
		{"release candidate", "1.2.3rc1", true},
		{"alpha release", "2.0.0a1", true},
		{"beta release", "3.0.0b2", true},
		{"dev release", "1.0.0dev1", true},
		{"post release", "1.0.0post1", true},

		// Invalid versions - empty
		{"empty string", "", false},

		// Invalid versions - too long
		{"too long", string(make([]byte, 51)), false},

		// Invalid versions - invalid characters
		{"command injection semicolon", "1.0.0; rm -rf /", false},
		{"path traversal", "../etc/passwd", false},
		{"subshell injection", "$(evil)", false},
		{"backtick injection", "`evil`", false},
		{"uppercase letters", "1.0.0RC1", false},
		{"space in version", "1.0 0", false},
		{"pipe in version", "1.0.0|cat", false},
		{"ampersand in version", "1.0.0&cmd", false},
		{"double ampersand", "1.0.0&&cmd", false},

		// Invalid versions - wrong structure
		{"starts with letter", "v1.0.0", false},
		{"starts with dot", ".1.0.0", false},
		{"dot in release tag", "1.0.0rc.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For too long test, generate proper string
			version := tt.version
			if tt.name == "too long" {
				version = "1"
				for i := 0; i < 50; i++ {
					version += "0"
				}
			}

			result := isValidPyPIVersion(version)
			if result != tt.expected {
				t.Errorf("isValidPyPIVersion(%q) = %v, want %v", tt.version, result, tt.expected)
			}
		})
	}
}

func TestPipxInstallAction_Name(t *testing.T) {
	t.Parallel()
	action := &PipxInstallAction{}
	if action.Name() != "pipx_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "pipx_install")
	}
}
