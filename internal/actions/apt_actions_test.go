package actions

import "testing"

func TestAptInstallAction_Name(t *testing.T) {
	t.Parallel()
	action := &AptInstallAction{}
	if action.Name() != "apt_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "apt_install")
	}
}

func TestAptInstallAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &AptInstallAction{}
	constraint := action.ImplicitConstraint()

	if constraint == nil {
		t.Fatal("ImplicitConstraint() returned nil")
	}
	if constraint.OS != "linux" {
		t.Errorf("OS = %q, want %q", constraint.OS, "linux")
	}
	if constraint.LinuxFamily != "debian" {
		t.Errorf("LinuxFamily = %q, want %q", constraint.LinuxFamily, "debian")
	}
}

func TestAptInstallAction_Validate(t *testing.T) {
	t.Parallel()
	action := &AptInstallAction{}

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
				"packages": []interface{}{"build-essential", "libssl-dev"},
			},
			wantErr: false,
		},
		{
			name: "with optional fields",
			params: map[string]interface{}{
				"packages":       []interface{}{"curl"},
				"fallback":       "Manual install required",
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

func TestAptInstallAction_Execute(t *testing.T) {
	t.Parallel()
	action := &AptInstallAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"packages": []interface{}{"build-essential", "libssl-dev"},
	})
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
}

func TestAptInstallAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := &AptInstallAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() should return true")
	}
}

func TestAptRepoAction_Name(t *testing.T) {
	t.Parallel()
	action := &AptRepoAction{}
	if action.Name() != "apt_repo" {
		t.Errorf("Name() = %q, want %q", action.Name(), "apt_repo")
	}
}

func TestAptRepoAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &AptRepoAction{}
	constraint := action.ImplicitConstraint()

	if constraint == nil {
		t.Fatal("ImplicitConstraint() returned nil")
	}
	if constraint.OS != "linux" {
		t.Errorf("OS = %q, want %q", constraint.OS, "linux")
	}
	if constraint.LinuxFamily != "debian" {
		t.Errorf("LinuxFamily = %q, want %q", constraint.LinuxFamily, "debian")
	}
}

func TestAptRepoAction_Validate(t *testing.T) {
	t.Parallel()
	action := &AptRepoAction{}

	tests := []struct {
		name    string
		params  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "missing all fields",
			params:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "missing key_url",
			params: map[string]interface{}{
				"url": "https://download.docker.com/linux/ubuntu",
			},
			wantErr: true,
		},
		{
			name: "missing key_sha256",
			params: map[string]interface{}{
				"url":     "https://download.docker.com/linux/ubuntu",
				"key_url": "https://download.docker.com/linux/ubuntu/gpg",
			},
			wantErr: true,
		},
		{
			name: "valid params",
			params: map[string]interface{}{
				"url":        "https://download.docker.com/linux/ubuntu",
				"key_url":    "https://download.docker.com/linux/ubuntu/gpg",
				"key_sha256": "1500c1f56fa9e26b9b8f42452a553675796ade0807cdce11975eb98170b3a570",
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

func TestAptPPAAction_Name(t *testing.T) {
	t.Parallel()
	action := &AptPPAAction{}
	if action.Name() != "apt_ppa" {
		t.Errorf("Name() = %q, want %q", action.Name(), "apt_ppa")
	}
}

func TestAptPPAAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &AptPPAAction{}
	constraint := action.ImplicitConstraint()

	if constraint == nil {
		t.Fatal("ImplicitConstraint() returned nil")
	}
	if constraint.OS != "linux" {
		t.Errorf("OS = %q, want %q", constraint.OS, "linux")
	}
	if constraint.LinuxFamily != "debian" {
		t.Errorf("LinuxFamily = %q, want %q", constraint.LinuxFamily, "debian")
	}
}

func TestAptPPAAction_Validate(t *testing.T) {
	t.Parallel()
	action := &AptPPAAction{}

	tests := []struct {
		name    string
		params  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "missing ppa",
			params:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "valid ppa",
			params: map[string]interface{}{
				"ppa": "deadsnakes/ppa",
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
