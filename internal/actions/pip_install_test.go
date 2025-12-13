package actions

import (
	"testing"
)

func TestPipInstallAction_Name(t *testing.T) {
	action := &PipInstallAction{}
	if action.Name() != "pip_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "pip_install")
	}
}

func TestPipInstallAction_IsPrimitive(t *testing.T) {
	if !IsPrimitive("pip_install") {
		t.Error("pip_install should be registered as a primitive")
	}
}

func TestPipInstallAction_IsRegistered(t *testing.T) {
	action := Get("pip_install")
	if action == nil {
		t.Error("pip_install should be registered in the action registry")
	}
	if action.Name() != "pip_install" {
		t.Errorf("registered action has wrong name: got %q, want %q", action.Name(), "pip_install")
	}
}

func TestBuildPipInstallArgs(t *testing.T) {
	tests := []struct {
		name            string
		sourceDir       string
		requirements    string
		constraints     string
		useHashes       bool
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:         "requirements only without hashes",
			requirements: "/path/to/requirements.txt",
			wantContains: []string{
				"install",
				"-r", "/path/to/requirements.txt",
				"--disable-pip-version-check",
			},
			wantNotContains: []string{
				"--require-hashes",
				"--no-deps",
				"--only-binary",
			},
		},
		{
			name:         "requirements with hashes",
			requirements: "/path/to/requirements.txt",
			useHashes:    true,
			wantContains: []string{
				"install",
				"--require-hashes",
				"--no-deps",
				"--only-binary", ":all:",
				"-r", "/path/to/requirements.txt",
			},
		},
		{
			name:      "source directory",
			sourceDir: "/path/to/source",
			wantContains: []string{
				"install",
				"/path/to/source",
			},
		},
		{
			name:         "with constraints",
			requirements: "/path/to/requirements.txt",
			constraints:  "/path/to/constraints.txt",
			wantContains: []string{
				"-c", "/path/to/constraints.txt",
				"-r", "/path/to/requirements.txt",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildPipInstallArgs(tt.sourceDir, tt.requirements, tt.constraints, tt.useHashes)

			// Check that expected args are present
			for _, want := range tt.wantContains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("args should contain %q, got %v", want, args)
				}
			}

			// Check that unwanted args are absent
			for _, notWant := range tt.wantNotContains {
				for _, arg := range args {
					if arg == notWant {
						t.Errorf("args should not contain %q, got %v", notWant, args)
					}
				}
			}
		})
	}
}

func TestGetSourceDateEpoch(t *testing.T) {
	// Test that it returns a consistent value
	epoch1 := getSourceDateEpoch()
	epoch2 := getSourceDateEpoch()

	if epoch1 != epoch2 {
		t.Errorf("getSourceDateEpoch() should return consistent value, got %d and %d", epoch1, epoch2)
	}

	// Should be a reasonable timestamp (after year 2000, before year 2100)
	if epoch1 < 946684800 || epoch1 > 4102444800 {
		t.Errorf("getSourceDateEpoch() returned unreasonable value: %d", epoch1)
	}
}

func TestPipInstallAction_Execute_MissingParams(t *testing.T) {
	action := &PipInstallAction{}
	ctx := &ExecutionContext{
		WorkDir:    "/tmp",
		InstallDir: "/tmp/install",
	}

	tests := []struct {
		name       string
		params     map[string]interface{}
		wantErrMsg string
	}{
		{
			name:       "missing python_version",
			params:     map[string]interface{}{},
			wantErrMsg: "python_version",
		},
		{
			name: "missing source_dir and requirements",
			params: map[string]interface{}{
				"python_version": "3.11",
			},
			wantErrMsg: "source_dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Error("Execute() should return error for missing params")
			}
			if tt.wantErrMsg != "" && !pipTestContains(err.Error(), tt.wantErrMsg) {
				t.Errorf("error should mention %q, got %q", tt.wantErrMsg, err.Error())
			}
		})
	}
}

func pipTestContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
