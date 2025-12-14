package actions

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNpmExecAction_Name(t *testing.T) {
	t.Parallel()
	action := &NpmExecAction{}
	if action.Name() != "npm_exec" {
		t.Errorf("Name() = %q, want %q", action.Name(), "npm_exec")
	}
}

func TestNpmExecAction_Registration(t *testing.T) {
	t.Parallel()
	action := Get("npm_exec")
	if action == nil {
		t.Fatal("npm_exec action not registered")
	}
	if action.Name() != "npm_exec" {
		t.Errorf("registered action Name() = %q, want %q", action.Name(), "npm_exec")
	}
}

func TestNpmExecAction_IsPrimitive(t *testing.T) {
	t.Parallel()
	if !IsPrimitive("npm_exec") {
		t.Error("npm_exec should be registered as a primitive")
	}
}

func TestNpmExecAction_Execute_MissingParams(t *testing.T) {
	t.Parallel()
	action := &NpmExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
	}

	tests := []struct {
		name   string
		params map[string]interface{}
		errMsg string
	}{
		{
			name:   "missing source_dir",
			params: map[string]interface{}{},
			errMsg: "npm_exec action requires 'source_dir' parameter",
		},
		{
			name: "missing command",
			params: map[string]interface{}{
				"source_dir": ".",
			},
			errMsg: "npm_exec action requires 'command' parameter",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a package.json for source_dir validation to pass
			if _, ok := tc.params["source_dir"]; ok {
				sourceDir := filepath.Join(ctx.WorkDir, tc.params["source_dir"].(string))
				if err := os.MkdirAll(sourceDir, 0755); err != nil {
					t.Fatalf("failed to create source dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(sourceDir, "package.json"), []byte("{}"), 0644); err != nil {
					t.Fatalf("failed to create package.json: %v", err)
				}
			}

			err := action.Execute(ctx, tc.params)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tc.errMsg {
				t.Errorf("error = %q, want %q", err.Error(), tc.errMsg)
			}
		})
	}
}

func TestNpmExecAction_Execute_MissingPackageJSON(t *testing.T) {
	t.Parallel()
	action := &NpmExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
	}

	// Create empty source directory (no package.json)
	sourceDir := filepath.Join(ctx.WorkDir, "project")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	params := map[string]interface{}{
		"source_dir": "project",
		"command":    "build",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error for missing package.json")
	}
	if err.Error() != "source_dir does not contain package.json: "+sourceDir {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNpmExecAction_Execute_MissingLockfile(t *testing.T) {
	t.Parallel()
	action := &NpmExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
	}

	// Create source directory with package.json but no lockfile
	sourceDir := filepath.Join(ctx.WorkDir, "project")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "package.json"), []byte(`{"name": "test"}`), 0644); err != nil {
		t.Fatalf("failed to create package.json: %v", err)
	}

	params := map[string]interface{}{
		"source_dir":   "project",
		"command":      "build",
		"use_lockfile": true,
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error for missing lockfile")
	}
	expectedErr := "use_lockfile is true but package-lock.json not found in " + sourceDir
	if err.Error() != expectedErr {
		t.Errorf("error = %q, want %q", err.Error(), expectedErr)
	}
}

func TestParseVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input   string
		major   int
		minor   int
		patch   int
		wantErr bool
	}{
		{"20.10.0", 20, 10, 0, false},
		{"18.0.0", 18, 0, 0, false},
		{"22.1.2", 22, 1, 2, false},
		{"18.0", 18, 0, 0, false},
		{"18", 18, 0, 0, false},
		{"invalid", 0, 0, 0, true},
		{"", 0, 0, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			major, minor, patch, err := parseVersion(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if major != tc.major || minor != tc.minor || patch != tc.patch {
				t.Errorf("parseVersion(%q) = %d.%d.%d, want %d.%d.%d",
					tc.input, major, minor, patch, tc.major, tc.minor, tc.patch)
			}
		})
	}
}

func TestVersionGTE(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		iMajor, iMinor, iPatch int
		rMajor, rMinor, rPatch int
		want                   bool
	}{
		{"equal", 20, 10, 0, 20, 10, 0, true},
		{"greater_major", 21, 0, 0, 20, 10, 0, true},
		{"greater_minor", 20, 11, 0, 20, 10, 0, true},
		{"greater_patch", 20, 10, 1, 20, 10, 0, true},
		{"less_major", 19, 0, 0, 20, 10, 0, false},
		{"less_minor", 20, 9, 0, 20, 10, 0, false},
		{"less_patch", 20, 10, 0, 20, 10, 1, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := versionGTE(tc.iMajor, tc.iMinor, tc.iPatch, tc.rMajor, tc.rMinor, tc.rPatch)
			if got != tc.want {
				t.Errorf("versionGTE(%d.%d.%d >= %d.%d.%d) = %v, want %v",
					tc.iMajor, tc.iMinor, tc.iPatch,
					tc.rMajor, tc.rMinor, tc.rPatch,
					got, tc.want)
			}
		})
	}
}

func TestVersionGT(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		iMajor, iMinor, iPatch int
		rMajor, rMinor, rPatch int
		want                   bool
	}{
		{"equal", 20, 10, 0, 20, 10, 0, false},
		{"greater_major", 21, 0, 0, 20, 10, 0, true},
		{"greater_minor", 20, 11, 0, 20, 10, 0, true},
		{"greater_patch", 20, 10, 1, 20, 10, 0, true},
		{"less_major", 19, 0, 0, 20, 10, 0, false},
		{"less_minor", 20, 9, 0, 20, 10, 0, false},
		{"less_patch", 20, 10, 0, 20, 10, 1, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := versionGT(tc.iMajor, tc.iMinor, tc.iPatch, tc.rMajor, tc.rMinor, tc.rPatch)
			if got != tc.want {
				t.Errorf("versionGT(%d.%d.%d > %d.%d.%d) = %v, want %v",
					tc.iMajor, tc.iMinor, tc.iPatch,
					tc.rMajor, tc.rMinor, tc.rPatch,
					got, tc.want)
			}
		})
	}
}

func TestNpmExecAction_DefaultParameters(t *testing.T) {
	t.Parallel()
	// Test that default parameters are applied correctly
	// This is a parameter extraction test, not an execution test

	action := &NpmExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
	}

	// Create source directory with package.json
	sourceDir := filepath.Join(ctx.WorkDir, "project")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "package.json"), []byte(`{"name": "test"}`), 0644); err != nil {
		t.Fatalf("failed to create package.json: %v", err)
	}

	// Minimal params - should use defaults for use_lockfile and ignore_scripts
	params := map[string]interface{}{
		"source_dir": "project",
		"command":    "build",
	}

	// This will fail because use_lockfile defaults to true and no lockfile exists
	// But the error message confirms the default was applied
	err := action.Execute(ctx, params)
	if err == nil {
		t.Fatal("expected error")
	}
	expectedErr := "use_lockfile is true but package-lock.json not found in " + sourceDir
	if err.Error() != expectedErr {
		t.Errorf("error = %q, want %q (confirms use_lockfile defaults to true)", err.Error(), expectedErr)
	}
}

func TestNpmExecAction_UseLockfileFalse(t *testing.T) {
	t.Parallel()
	action := &NpmExecAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
	}

	// Create source directory with package.json but no lockfile
	sourceDir := filepath.Join(ctx.WorkDir, "project")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "package.json"), []byte(`{"name": "test"}`), 0644); err != nil {
		t.Fatalf("failed to create package.json: %v", err)
	}

	params := map[string]interface{}{
		"source_dir":   "project",
		"command":      "build",
		"use_lockfile": false, // Should not require lockfile
	}

	// This will fail at npm install (npm not in PATH in test), but should NOT fail on missing lockfile
	err := action.Execute(ctx, params)
	if err == nil {
		t.Skip("npm is available, test passed")
	}
	// Error should be about npm install failing, not missing lockfile
	if err.Error() == "use_lockfile is true but package-lock.json not found in "+sourceDir {
		t.Error("use_lockfile=false should not require lockfile")
	}
}
