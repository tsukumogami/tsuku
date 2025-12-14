package actions

import (
	"context"
	"testing"
)

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
