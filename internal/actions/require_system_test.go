package actions

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRequireSystemAction_Name(t *testing.T) {
	action := &RequireSystemAction{}
	if got := action.Name(); got != "require_system" {
		t.Errorf("Name() = %q, want %q", got, "require_system")
	}
}

func TestRequireSystemAction_IsDeterministic(t *testing.T) {
	action := &RequireSystemAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestValidateCommandName(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr bool
	}{
		{"valid simple", "docker", false},
		{"valid with hyphen", "docker-compose", false},
		{"valid with underscore", "foo_bar", false},
		{"valid with dot", "foo.bar", false},
		{"empty", "", true},
		{"path separator slash", "bin/docker", true},
		{"path separator backslash", "bin\\docker", true},
		{"path traversal", "../docker", true},
		{"shell metachar pipe", "docker|bash", true},
		{"shell metachar semicolon", "docker;bash", true},
		{"shell metachar ampersand", "docker&bash", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCommandName(tt.command)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCommandName(%q) error = %v, wantErr %v", tt.command, err, tt.wantErr)
			}
		})
	}
}

func TestDetectVersion(t *testing.T) {
	// Create a temporary directory for test scripts
	tmpDir := t.TempDir()

	// Create a mock version command
	mockCmd := filepath.Join(tmpDir, "mockver")
	script := `#!/bin/sh
echo "mockver version 1.2.3 (build 456)"`
	if err := os.WriteFile(mockCmd, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	// Add tmpDir to PATH for this test
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
	defer os.Setenv("PATH", oldPath)

	tests := []struct {
		name         string
		command      string
		versionFlag  string
		versionRegex string
		want         string
		wantErr      bool
	}{
		{
			name:         "extract version from output",
			command:      "mockver",
			versionFlag:  "--version",
			versionRegex: `version ([0-9.]+)`,
			want:         "1.2.3",
			wantErr:      false,
		},
		{
			name:         "invalid regex",
			command:      "mockver",
			versionFlag:  "--version",
			versionRegex: `[invalid`,
			want:         "",
			wantErr:      true,
		},
		{
			name:         "regex does not match",
			command:      "mockver",
			versionFlag:  "--version",
			versionRegex: `notfound ([0-9.]+)`,
			want:         "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectVersion(tt.command, tt.versionFlag, tt.versionRegex)
			if (err != nil) != tt.wantErr {
				t.Errorf("detectVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("detectVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersionSatisfied(t *testing.T) {
	tests := []struct {
		found    string
		required string
		want     bool
	}{
		{"1.2.3", "1.2.3", true},  // equal
		{"1.2.4", "1.2.3", true},  // greater patch
		{"1.3.0", "1.2.3", true},  // greater minor
		{"2.0.0", "1.2.3", true},  // greater major
		{"1.2.2", "1.2.3", false}, // less patch
		{"1.1.9", "1.2.3", false}, // less minor
		{"0.9.9", "1.2.3", false}, // less major
	}

	for _, tt := range tests {
		name := tt.found + "_vs_" + tt.required
		t.Run(name, func(t *testing.T) {
			got := versionSatisfied(tt.found, tt.required)
			if got != tt.want {
				t.Errorf("versionSatisfied(%q, %q) = %v, want %v", tt.found, tt.required, got, tt.want)
			}
		})
	}
}

func TestGetPlatformGuide(t *testing.T) {
	tests := []struct {
		name         string
		installGuide map[string]string
		platform     string
		want         string
	}{
		{
			name: "platform-specific guide",
			installGuide: map[string]string{
				"darwin": "brew install docker",
				"linux":  "apt install docker",
			},
			platform: "darwin",
			want:     "brew install docker",
		},
		{
			name: "fallback guide",
			installGuide: map[string]string{
				"darwin":   "brew install docker",
				"fallback": "see https://example.com",
			},
			platform: "windows",
			want:     "see https://example.com",
		},
		{
			name: "no matching guide",
			installGuide: map[string]string{
				"darwin": "brew install docker",
			},
			platform: "windows",
			want:     "",
		},
		{
			name:         "nil install guide",
			installGuide: nil,
			platform:     "linux",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPlatformGuide(tt.installGuide, tt.platform)
			if got != tt.want {
				t.Errorf("getPlatformGuide() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRequireSystemAction_Execute_MissingCommand(t *testing.T) {
	action := &RequireSystemAction{}
	ctx := &ExecutionContext{}

	params := map[string]interface{}{
		"command": "nonexistent-command-12345",
		"install_guide": map[string]interface{}{
			"darwin": "brew install nonexistent",
			"linux":  "apt install nonexistent",
		},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Fatal("Execute() expected error for missing command, got nil")
	}

	// Check error is SystemDepMissingError
	missingErr, ok := err.(*SystemDepMissingError)
	if !ok {
		t.Fatalf("Execute() error type = %T, want *SystemDepMissingError", err)
	}

	if missingErr.Command != "nonexistent-command-12345" {
		t.Errorf("SystemDepMissingError.Command = %q, want %q", missingErr.Command, "nonexistent-command-12345")
	}

	// Check platform-specific guide is included
	expectedGuide := map[string]string{
		"darwin": "brew install nonexistent",
		"linux":  "apt install nonexistent",
	}[runtime.GOOS]

	if missingErr.InstallGuide != expectedGuide {
		t.Errorf("SystemDepMissingError.InstallGuide = %q, want %q", missingErr.InstallGuide, expectedGuide)
	}
}

func TestRequireSystemAction_Execute_CommandFound(t *testing.T) {
	// Use 'sh' as a command that's guaranteed to exist
	action := &RequireSystemAction{}
	ctx := &ExecutionContext{}

	params := map[string]interface{}{
		"command": "sh",
	}

	err := action.Execute(ctx, params)
	if err != nil {
		t.Errorf("Execute() unexpected error for existing command 'sh': %v", err)
	}
}

func TestRequireSystemAction_Execute_VersionCheck(t *testing.T) {
	// Create a temporary directory for test scripts
	tmpDir := t.TempDir()

	// Create a mock version command
	mockCmd := filepath.Join(tmpDir, "mocktool")
	script := `#!/bin/sh
echo "mocktool version 2.0.0"`
	if err := os.WriteFile(mockCmd, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	// Add tmpDir to PATH for this test
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
	defer os.Setenv("PATH", oldPath)

	action := &RequireSystemAction{}
	ctx := &ExecutionContext{}

	tests := []struct {
		name    string
		params  map[string]interface{}
		wantErr bool
		errType string
	}{
		{
			name: "version sufficient",
			params: map[string]interface{}{
				"command":       "mocktool",
				"version_flag":  "--version",
				"version_regex": `version ([0-9.]+)`,
				"min_version":   "1.5.0",
			},
			wantErr: false,
		},
		{
			name: "version insufficient",
			params: map[string]interface{}{
				"command":       "mocktool",
				"version_flag":  "--version",
				"version_regex": `version ([0-9.]+)`,
				"min_version":   "3.0.0",
				"install_guide": map[string]interface{}{
					"fallback": "upgrade to 3.0.0+",
				},
			},
			wantErr: true,
			errType: "SystemDepVersionError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Execute(ctx, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errType == "SystemDepVersionError" {
				if _, ok := err.(*SystemDepVersionError); !ok {
					t.Errorf("Execute() error type = %T, want *SystemDepVersionError", err)
				}
			}
		})
	}
}

func TestRequireSystemAction_Execute_InvalidParams(t *testing.T) {
	action := &RequireSystemAction{}
	ctx := &ExecutionContext{}

	tests := []struct {
		name   string
		params map[string]interface{}
	}{
		{"missing command", map[string]interface{}{}},
		{"empty command", map[string]interface{}{"command": ""}},
		{"invalid command", map[string]interface{}{"command": "../bin/docker"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Errorf("Execute() expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestSystemDepMissingError_Error(t *testing.T) {
	err := &SystemDepMissingError{
		Command:      "docker",
		InstallGuide: "brew install docker",
	}

	msg := err.Error()
	if !strings.Contains(msg, "docker") {
		t.Errorf("Error message missing command name: %s", msg)
	}
	if !strings.Contains(msg, "brew install docker") {
		t.Errorf("Error message missing install guide: %s", msg)
	}
}

func TestSystemDepVersionError_Error(t *testing.T) {
	err := &SystemDepVersionError{
		Command:      "docker",
		Found:        "19.0.0",
		Required:     "20.10.0",
		InstallGuide: "brew upgrade docker",
	}

	msg := err.Error()
	if !strings.Contains(msg, "docker") {
		t.Errorf("Error message missing command name: %s", msg)
	}
	if !strings.Contains(msg, "19.0.0") {
		t.Errorf("Error message missing found version: %s", msg)
	}
	if !strings.Contains(msg, "20.10.0") {
		t.Errorf("Error message missing required version: %s", msg)
	}
	if !strings.Contains(msg, "brew upgrade docker") {
		t.Errorf("Error message missing install guide: %s", msg)
	}
}
