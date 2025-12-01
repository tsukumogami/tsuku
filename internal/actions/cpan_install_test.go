package actions

import (
	"strings"
	"testing"
)

func TestIsValidDistribution(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid distribution names
		{"simple name", "App", true},
		{"with hyphen", "App-Ack", true},
		{"with underscore", "App_Test", true},
		{"mixed case", "MyDistribution", true},
		{"with numbers", "App5", true},
		{"complex", "File-Slurp-Tiny", true},
		{"with underscore and hyphen", "My_App-Test", true},

		// Invalid distribution names
		{"empty", "", false},
		{"starts with number", "1App", false},
		{"starts with hyphen", "-App", false},
		{"starts with underscore", "_App", false},
		{"contains dot", "App.Ack", false},
		{"contains space", "App Ack", false},
		{"contains slash", "App/Ack", false},
		{"contains at", "@scope/pkg", false},
		{"module name with colons", "App::Ack", false},
		{"too long", strings.Repeat("a", 129), false},

		// Security test cases
		{"injection semicolon", "App;echo", false},
		{"injection backtick", "App`pwd`", false},
		{"injection dollar", "App$()", false},
		{"path traversal", "../../etc/passwd", false},
		{"command substitution", "$(whoami)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidDistribution(tt.input)
			if result != tt.expected {
				t.Errorf("isValidDistribution(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidCpanVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid versions
		{"empty (latest)", "", true},
		{"simple", "1.0.0", true},
		{"two parts", "1.2", true},
		{"four parts", "1.2.3.4", true},
		{"with underscore", "1.2.3_01", true},
		{"with TRIAL", "1.2.3-TRIAL", true},
		{"v prefix", "v1.2.3", true},
		{"v with multiple parts", "v1.2.3.4", true},
		{"single digit", "5", true},

		// Invalid versions
		{"starts with letter (not v)", "a1.0.0", false},
		{"contains space", "1.0 .0", false},
		{"too long", strings.Repeat("1", 51), false},

		// Security test cases
		{"injection semicolon", "1.0.0;echo", false},
		{"injection backtick", "1.0.0`pwd`", false},
		{"injection dollar", "1.0.0$()", false},
		{"injection pipe", "1.0.0|cat", false},
		{"injection ampersand", "1.0.0&rm", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidCpanVersion(tt.input)
			if result != tt.expected {
				t.Errorf("isValidCpanVersion(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCpanInstallAction_Execute_Validation(t *testing.T) {
	action := &CpanInstallAction{}

	tests := []struct {
		name        string
		params      map[string]interface{}
		version     string
		expectError string
	}{
		{
			name:        "missing distribution parameter",
			params:      map[string]interface{}{},
			version:     "1.0.0",
			expectError: "requires 'distribution' parameter",
		},
		{
			name: "invalid distribution name",
			params: map[string]interface{}{
				"distribution": "invalid;dist",
				"executables":  []interface{}{"exe"},
			},
			version:     "1.0.0",
			expectError: "invalid distribution name",
		},
		{
			name: "module name instead of distribution",
			params: map[string]interface{}{
				"distribution": "App::Ack",
				"executables":  []interface{}{"ack"},
			},
			version:     "1.0.0",
			expectError: "invalid distribution name",
		},
		{
			name: "missing executables",
			params: map[string]interface{}{
				"distribution": "App-Ack",
			},
			version:     "1.0.0",
			expectError: "requires 'executables' parameter",
		},
		{
			name: "empty executables",
			params: map[string]interface{}{
				"distribution": "App-Ack",
				"executables":  []interface{}{},
			},
			version:     "1.0.0",
			expectError: "requires 'executables' parameter",
		},
		{
			name: "invalid executable with path",
			params: map[string]interface{}{
				"distribution": "App-Ack",
				"executables":  []interface{}{"../bin/exe"},
			},
			version:     "1.0.0",
			expectError: "must not contain path separators",
		},
		{
			name: "invalid executable with shell metacharacter",
			params: map[string]interface{}{
				"distribution": "App-Ack",
				"executables":  []interface{}{"exe;rm"},
			},
			version:     "1.0.0",
			expectError: "contains shell metacharacters",
		},
		{
			name: "invalid version",
			params: map[string]interface{}{
				"distribution": "App-Ack",
				"executables":  []interface{}{"ack"},
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

			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestCpanInstallAction_Name(t *testing.T) {
	action := &CpanInstallAction{}
	if action.Name() != "cpan_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "cpan_install")
	}
}

func TestCpanInstallAction_Registration(t *testing.T) {
	// Verify action is registered in the global registry
	action := Get("cpan_install")
	if action == nil {
		t.Error("cpan_install action not found in registry")
		return
	}
	if action.Name() != "cpan_install" {
		t.Errorf("registered action name = %q, want %q", action.Name(), "cpan_install")
	}
}
