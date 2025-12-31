package main

import (
	"context"
	"testing"

	"github.com/tsukumogami/tsuku/internal/actions"
)

func TestVerifyCommand_Exists(t *testing.T) {
	t.Parallel()

	// Test with a command that should exist on any Unix-like system
	params := map[string]interface{}{
		"command": "sh",
	}

	check := verifyCommand(context.Background(), params)

	if check.Status != "pass" {
		t.Errorf("verifyCommand(sh) status = %q, want %q", check.Status, "pass")
	}
	if check.Path == "" {
		t.Error("verifyCommand(sh) path should not be empty")
	}
	if check.Command != "sh" {
		t.Errorf("verifyCommand(sh) command = %q, want %q", check.Command, "sh")
	}
}

func TestVerifyCommand_NotExists(t *testing.T) {
	t.Parallel()

	params := map[string]interface{}{
		"command": "nonexistent_command_12345",
	}

	check := verifyCommand(context.Background(), params)

	if check.Status != "fail" {
		t.Errorf("verifyCommand(nonexistent) status = %q, want %q", check.Status, "fail")
	}
	if check.Error == "" {
		t.Error("verifyCommand(nonexistent) error should not be empty")
	}
}

func TestVerifyCommand_EmptyCommand(t *testing.T) {
	t.Parallel()

	params := map[string]interface{}{
		"command": "",
	}

	check := verifyCommand(context.Background(), params)

	if check.Status != "fail" {
		t.Errorf("verifyCommand(empty) status = %q, want %q", check.Status, "fail")
	}
}

func TestVerifyCommand_MissingCommand(t *testing.T) {
	t.Parallel()

	params := map[string]interface{}{}

	check := verifyCommand(context.Background(), params)

	if check.Status != "fail" {
		t.Errorf("verifyCommand(missing) status = %q, want %q", check.Status, "fail")
	}
}

func TestVerifyCommand_WithVersion(t *testing.T) {
	t.Parallel()

	// Test version checking with bash which should exist and have a version
	params := map[string]interface{}{
		"command":       "bash",
		"version_flag":  "--version",
		"version_regex": `([0-9]+\.[0-9]+)`,
		"min_version":   "1.0",
	}

	check := verifyCommand(context.Background(), params)

	if check.Status != "pass" {
		t.Errorf("verifyCommand(bash with version) status = %q, want %q; error: %s", check.Status, "pass", check.Error)
	}
	if check.Version == "" {
		t.Error("verifyCommand(bash with version) should have detected version")
	}
}

func TestVerifyCommand_VersionMismatch(t *testing.T) {
	t.Parallel()

	// Request an impossibly high version
	params := map[string]interface{}{
		"command":       "bash",
		"version_flag":  "--version",
		"version_regex": `([0-9]+\.[0-9]+)`,
		"min_version":   "999.0",
	}

	check := verifyCommand(context.Background(), params)

	if check.Status != "version_mismatch" {
		t.Errorf("verifyCommand(high version) status = %q, want %q", check.Status, "version_mismatch")
	}
}

func TestVersionSatisfiesMinimum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		detected string
		minimum  string
		want     bool
	}{
		{"equal versions", "1.2.3", "1.2.3", true},
		{"higher major", "2.0.0", "1.0.0", true},
		{"higher minor", "1.3.0", "1.2.0", true},
		{"higher patch", "1.2.4", "1.2.3", true},
		{"lower major", "1.0.0", "2.0.0", false},
		{"lower minor", "1.1.0", "1.2.0", false},
		{"lower patch", "1.2.2", "1.2.3", false},
		{"with v prefix detected", "v1.2.3", "1.2.3", true},
		{"with v prefix minimum", "1.2.3", "v1.2.3", true},
		{"both v prefix", "v1.2.3", "v1.2.3", true},
		{"fewer parts in detected", "1.2", "1.2.0", false},
		{"fewer parts in minimum", "1.2.3", "1.2", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := versionSatisfiesMinimum(tt.detected, tt.minimum)
			if got != tt.want {
				t.Errorf("versionSatisfiesMinimum(%q, %q) = %v, want %v", tt.detected, tt.minimum, got, tt.want)
			}
		})
	}
}

func TestGetString_Helper(t *testing.T) {
	t.Parallel()

	params := map[string]interface{}{
		"command": "test",
	}

	val, ok := actions.GetString(params, "command")
	if !ok || val != "test" {
		t.Errorf("GetString(command) = %q, %v; want %q, true", val, ok, "test")
	}

	val, ok = actions.GetString(params, "missing")
	if ok {
		t.Errorf("GetString(missing) = %q, %v; want \"\", false", val, ok)
	}
}
