package actions

import (
	"context"
	"testing"
)

func TestNpmInstallAction_IsDecomposable(t *testing.T) {
	t.Parallel()
	if !IsDecomposable("npm_install") {
		t.Error("npm_install should be decomposable")
	}
}

func TestIsValidNpmPackage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		valid bool
	}{
		{"serve", true},
		{"netlify-cli", true},
		{"@types/node", true},
		{"@scope/package", true},
		{"package123", true},
		{"my.package", true},
		{"my_package", true},
		{"", false},
		{"package;rm -rf", false},
		{"package`id`", false},
		{"package$HOME", false},
		{"package && ls", false},
		{"package\nls", false},
		{"package with space", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidNpmPackage(tc.name)
			if got != tc.valid {
				t.Errorf("isValidNpmPackage(%q) = %v, want %v", tc.name, got, tc.valid)
			}
		})
	}
}

func TestDetectNativeAddons(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		lockfile string
		expected bool
	}{
		{
			name: "no native addons",
			lockfile: `{
				"packages": {
					"node_modules/lodash": {
						"version": "4.17.21"
					}
				}
			}`,
			expected: false,
		},
		{
			name: "has gypfile",
			lockfile: `{
				"packages": {
					"node_modules/native-pkg": {
						"version": "1.0.0",
						"gypfile": true
					}
				}
			}`,
			expected: true,
		},
		{
			name: "has install script",
			lockfile: `{
				"packages": {
					"node_modules/pkg-with-script": {
						"version": "1.0.0",
						"hasInstallScript": true
					}
				}
			}`,
			expected: true,
		},
		{
			name:     "invalid json",
			lockfile: "not json",
			expected: false,
		},
		{
			name:     "empty packages",
			lockfile: `{"packages": {}}`,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectNativeAddons(tc.lockfile)
			if got != tc.expected {
				t.Errorf("detectNativeAddons() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestNpmInstallAction_Decompose_MissingParams(t *testing.T) {
	t.Parallel()
	action := &NpmInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "1.0.0",
	}

	tests := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name:   "missing package",
			params: map[string]interface{}{},
		},
		{
			name: "missing executables",
			params: map[string]interface{}{
				"package": "serve",
			},
		},
		{
			name: "empty executables",
			params: map[string]interface{}{
				"package":     "serve",
				"executables": []interface{}{},
			},
		},
		{
			name: "invalid package name",
			params: map[string]interface{}{
				"package":     "serve; rm -rf /",
				"executables": []interface{}{"serve"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := action.Decompose(ctx, tc.params)
			if err == nil {
				t.Error("Decompose() should fail with missing/invalid params")
			}
		})
	}
}

func TestNpmInstallAction_Decompose_MissingVersion(t *testing.T) {
	t.Parallel()
	action := &NpmInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "", // Missing version
	}

	params := map[string]interface{}{
		"package":     "serve",
		"executables": []interface{}{"serve"},
	}

	_, err := action.Decompose(ctx, params)
	if err == nil {
		t.Error("Decompose() should fail with missing version")
	}
}

func TestNpmInstallAction_Name(t *testing.T) {
	t.Parallel()
	action := &NpmInstallAction{}
	if action.Name() != "npm_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "npm_install")
	}
}

func TestNpmInstallAction_Execute_MissingParams(t *testing.T) {
	t.Parallel()
	action := &NpmInstallAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	tests := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name:   "missing package",
			params: map[string]interface{}{},
		},
		{
			name: "missing executables",
			params: map[string]interface{}{
				"package": "some-package",
			},
		},
		{
			name: "empty executables",
			params: map[string]interface{}{
				"package":     "some-package",
				"executables": []interface{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Error("Execute() should fail with missing required params")
			}
		})
	}
}
