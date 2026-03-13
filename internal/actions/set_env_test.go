package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetEnvAction_Name(t *testing.T) {
	t.Parallel()
	action := &SetEnvAction{}
	if action.Name() != "set_env" {
		t.Errorf("Name() = %q, want %q", action.Name(), "set_env")
	}
}

func TestSetEnvAction_Execute(t *testing.T) {
	t.Parallel()
	action := &SetEnvAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "JAVA_HOME", "value": "{install_dir}"},
			map[string]interface{}{"name": "VERSION", "value": "{version}"},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify env.sh was created
	envFile := filepath.Join(tmpDir, "env.sh")
	content, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("Failed to read env.sh: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "export JAVA_HOME="+tmpDir) {
		t.Errorf("env.sh should contain JAVA_HOME=%s, got: %s", tmpDir, contentStr)
	}
	if !strings.Contains(contentStr, "export VERSION=1.0.0") {
		t.Errorf("env.sh should contain VERSION=1.0.0, got: %s", contentStr)
	}
}

func TestSetEnvAction_Execute_MissingVars(t *testing.T) {
	t.Parallel()
	action := &SetEnvAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("Execute() should fail when 'vars' parameter is missing")
	}
}

func TestSetEnvAction_parseVars(t *testing.T) {
	t.Parallel()
	action := &SetEnvAction{}

	// Valid input
	vars := []interface{}{
		map[string]interface{}{"name": "FOO", "value": "bar"},
		map[string]interface{}{"name": "BAZ", "value": "qux"},
	}

	result, err := action.parseVars(vars)
	if err != nil {
		t.Fatalf("parseVars() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("parseVars() returned %d vars, want 2", len(result))
	}
	if result[0].Name != "FOO" || result[0].Value != "bar" {
		t.Errorf("parseVars()[0] = {%q, %q}, want {FOO, bar}", result[0].Name, result[0].Value)
	}
}

func TestSetEnvAction_parseVars_InvalidFormat(t *testing.T) {
	t.Parallel()
	action := &SetEnvAction{}

	tests := []struct {
		name  string
		input interface{}
	}{
		{
			name:  "not an array",
			input: "string",
		},
		{
			name:  "array item not a map",
			input: []interface{}{"string"},
		},
		{
			name:  "missing name",
			input: []interface{}{map[string]interface{}{"value": "bar"}},
		},
		{
			name:  "missing value",
			input: []interface{}{map[string]interface{}{"name": "FOO"}},
		},
		{
			name:  "name not string",
			input: []interface{}{map[string]interface{}{"name": 123, "value": "bar"}},
		},
		{
			name:  "value not string",
			input: []interface{}{map[string]interface{}{"name": "FOO", "value": 123}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := action.parseVars(tt.input)
			if err == nil {
				t.Errorf("parseVars(%v) should fail", tt.input)
			}
		})
	}
}

// TestSetEnvAction_ParseVars_Errors tests parseVars error paths via Execute
func TestSetEnvAction_ParseVars_Errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		vars        any
		errContains string
	}{
		{
			name:        "missing name",
			vars:        []any{map[string]any{"value": "bar"}},
			errContains: "name",
		},
		{
			name:        "missing value",
			vars:        []any{map[string]any{"name": "FOO"}},
			errContains: "value",
		},
		{
			name:        "non-array",
			vars:        "not an array",
			errContains: "array",
		},
		{
			name:        "non-map entry",
			vars:        []any{"not a map"},
			errContains: "map",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			action := &SetEnvAction{}
			ctx := &ExecutionContext{
				Context:    context.Background(),
				WorkDir:    t.TempDir(),
				InstallDir: t.TempDir(),
				Version:    "1.0.0",
			}
			err := action.Execute(ctx, map[string]any{"vars": tt.vars})
			if err == nil || !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Expected error containing %q, got %v", tt.errContains, err)
			}
		})
	}
}
