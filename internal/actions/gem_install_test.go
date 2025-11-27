package actions

import "testing"

func TestIsValidGemName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid gem names
		{"simple name", "bundler", true},
		{"with hyphen", "factory-bot", true},
		{"with underscore", "rspec_support", true},
		{"mixed case", "MyGem", true},
		{"with numbers", "rails5", true},
		{"hyphen and underscore", "my-gem_name", true},

		// Invalid gem names
		{"empty", "", false},
		{"starts with number", "1gem", false},
		{"starts with hyphen", "-gem", false},
		{"starts with underscore", "_gem", false},
		{"contains dot", "my.gem", false},
		{"contains space", "my gem", false},
		{"contains slash", "my/gem", false},
		{"contains at", "@scope/gem", false},
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},

		// Security test cases
		{"injection semicolon", "gem;echo", false},
		{"injection backtick", "gem`pwd`", false},
		{"injection dollar", "gem$()", false},
		{"path traversal", "../../etc/passwd", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidGemName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidGemName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidGemVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid versions
		{"simple", "1.0.0", true},
		{"two parts", "1.2", true},
		{"four parts", "1.2.3.4", true},
		{"with pre", "1.0.0.pre", true},
		{"with rc", "1.0.0.rc1", true},
		{"with beta", "1.0.0.beta.2", true},
		{"with hyphen pre", "1.0.0-pre.1", true},
		{"with alpha", "1.0.0alpha", true},

		// Invalid versions
		{"empty", "", false},
		{"starts with letter", "v1.0.0", false},
		{"contains plus", "1.0.0+build", false},
		{"contains space", "1.0 .0", false},
		{"too long", "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0", false},

		// Security test cases
		{"injection semicolon", "1.0.0;echo", false},
		{"injection backtick", "1.0.0`pwd`", false},
		{"injection dollar", "1.0.0$()", false},
		{"injection pipe", "1.0.0|cat", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidGemVersion(tt.input)
			if result != tt.expected {
				t.Errorf("isValidGemVersion(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGemInstallAction_Execute_Validation(t *testing.T) {
	action := &GemInstallAction{}

	tests := []struct {
		name        string
		params      map[string]interface{}
		version     string
		expectError string
	}{
		{
			name:        "missing gem parameter",
			params:      map[string]interface{}{},
			version:     "1.0.0",
			expectError: "requires 'gem' parameter",
		},
		{
			name: "invalid gem name",
			params: map[string]interface{}{
				"gem":         "invalid;gem",
				"executables": []interface{}{"exe"},
			},
			version:     "1.0.0",
			expectError: "invalid gem name",
		},
		{
			name: "missing executables",
			params: map[string]interface{}{
				"gem": "bundler",
			},
			version:     "1.0.0",
			expectError: "requires 'executables' parameter",
		},
		{
			name: "empty executables",
			params: map[string]interface{}{
				"gem":         "bundler",
				"executables": []interface{}{},
			},
			version:     "1.0.0",
			expectError: "requires 'executables' parameter",
		},
		{
			name: "invalid executable with path",
			params: map[string]interface{}{
				"gem":         "bundler",
				"executables": []interface{}{"../bin/exe"},
			},
			version:     "1.0.0",
			expectError: "must not contain path separators",
		},
		{
			name: "invalid executable with shell metacharacter",
			params: map[string]interface{}{
				"gem":         "bundler",
				"executables": []interface{}{"exe;rm"},
			},
			version:     "1.0.0",
			expectError: "contains shell metacharacters",
		},
		{
			name: "invalid version",
			params: map[string]interface{}{
				"gem":         "bundler",
				"executables": []interface{}{"bundle"},
			},
			version:     ";echo hack",
			expectError: "invalid version format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &ExecutionContext{
				Version:    tt.version,
				InstallDir: "/tmp/test",
			}

			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectError)
				return
			}

			if !containsString(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestGemInstallAction_Name(t *testing.T) {
	action := &GemInstallAction{}
	if action.Name() != "gem_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "gem_install")
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
