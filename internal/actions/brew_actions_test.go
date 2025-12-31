package actions

import "testing"

func TestBrewInstallAction_Name(t *testing.T) {
	t.Parallel()
	action := &BrewInstallAction{}
	if action.Name() != "brew_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "brew_install")
	}
}

func TestBrewInstallAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &BrewInstallAction{}
	constraint := action.ImplicitConstraint()

	if constraint == nil {
		t.Fatal("ImplicitConstraint() returned nil")
	}
	if constraint.OS != "darwin" {
		t.Errorf("OS = %q, want %q", constraint.OS, "darwin")
	}
	if constraint.LinuxFamily != "" {
		t.Errorf("LinuxFamily = %q, want empty", constraint.LinuxFamily)
	}
}

func TestBrewInstallAction_Validate(t *testing.T) {
	t.Parallel()
	action := &BrewInstallAction{}

	tests := []struct {
		name    string
		params  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "missing packages",
			params:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "valid packages",
			params: map[string]interface{}{
				"packages": []interface{}{"openssl", "libyaml"},
			},
			wantErr: false,
		},
		{
			name: "with tap",
			params: map[string]interface{}{
				"packages": []interface{}{"some-tool"},
				"tap":      "owner/repo",
			},
			wantErr: false,
		},
		{
			name: "with optional fields",
			params: map[string]interface{}{
				"packages":       []interface{}{"curl"},
				"fallback":       "Install via website",
				"unless_command": "curl",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := action.Validate(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBrewInstallAction_Execute(t *testing.T) {
	t.Parallel()
	action := &BrewInstallAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"packages": []interface{}{"openssl", "libyaml"},
	})
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
}

func TestBrewInstallAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := &BrewInstallAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() should return true")
	}
}

func TestBrewCaskAction_Name(t *testing.T) {
	t.Parallel()
	action := &BrewCaskAction{}
	if action.Name() != "brew_cask" {
		t.Errorf("Name() = %q, want %q", action.Name(), "brew_cask")
	}
}

func TestBrewCaskAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &BrewCaskAction{}
	constraint := action.ImplicitConstraint()

	if constraint == nil {
		t.Fatal("ImplicitConstraint() returned nil")
	}
	if constraint.OS != "darwin" {
		t.Errorf("OS = %q, want %q", constraint.OS, "darwin")
	}
	if constraint.LinuxFamily != "" {
		t.Errorf("LinuxFamily = %q, want empty", constraint.LinuxFamily)
	}
}

func TestBrewCaskAction_Validate(t *testing.T) {
	t.Parallel()
	action := &BrewCaskAction{}

	tests := []struct {
		name    string
		params  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "missing packages",
			params:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "valid packages",
			params: map[string]interface{}{
				"packages": []interface{}{"docker", "visual-studio-code"},
			},
			wantErr: false,
		},
		{
			name: "with tap",
			params: map[string]interface{}{
				"packages": []interface{}{"some-app"},
				"tap":      "owner/cask-repo",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := action.Validate(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBrewCaskAction_Execute(t *testing.T) {
	t.Parallel()
	action := &BrewCaskAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"packages": []interface{}{"docker"},
	})
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
}

func TestBrewCaskAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := &BrewCaskAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() should return true")
	}
}
