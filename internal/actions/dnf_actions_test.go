package actions

import "testing"

func TestDnfInstallAction_Name(t *testing.T) {
	t.Parallel()
	action := &DnfInstallAction{}
	if action.Name() != "dnf_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "dnf_install")
	}
}

func TestDnfInstallAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &DnfInstallAction{}
	constraint := action.ImplicitConstraint()

	if constraint == nil {
		t.Fatal("ImplicitConstraint() returned nil")
	}
	if constraint.OS != "linux" {
		t.Errorf("OS = %q, want %q", constraint.OS, "linux")
	}
	if constraint.LinuxFamily != "rhel" {
		t.Errorf("LinuxFamily = %q, want %q", constraint.LinuxFamily, "rhel")
	}
}

func TestDnfInstallAction_Validate(t *testing.T) {
	t.Parallel()
	action := &DnfInstallAction{}

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
				"packages": []interface{}{"gcc", "openssl-devel"},
			},
			wantErr: false,
		},
		{
			name: "with optional fields",
			params: map[string]interface{}{
				"packages":       []interface{}{"docker"},
				"fallback":       "See https://docs.docker.com",
				"unless_command": "docker",
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

func TestDnfInstallAction_Execute(t *testing.T) {
	t.Parallel()
	action := &DnfInstallAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"packages": []interface{}{"gcc", "openssl-devel"},
	})
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
}

func TestDnfInstallAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := &DnfInstallAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() should return true")
	}
}

func TestDnfRepoAction_Name(t *testing.T) {
	t.Parallel()
	action := &DnfRepoAction{}
	if action.Name() != "dnf_repo" {
		t.Errorf("Name() = %q, want %q", action.Name(), "dnf_repo")
	}
}

func TestDnfRepoAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &DnfRepoAction{}
	constraint := action.ImplicitConstraint()

	if constraint == nil {
		t.Fatal("ImplicitConstraint() returned nil")
	}
	if constraint.OS != "linux" {
		t.Errorf("OS = %q, want %q", constraint.OS, "linux")
	}
	if constraint.LinuxFamily != "rhel" {
		t.Errorf("LinuxFamily = %q, want %q", constraint.LinuxFamily, "rhel")
	}
}

func TestDnfRepoAction_Validate(t *testing.T) {
	t.Parallel()
	action := &DnfRepoAction{}

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
				"url": "https://download.docker.com/linux/fedora/docker-ce.repo",
			},
			wantErr: true,
		},
		{
			name: "missing key_sha256",
			params: map[string]interface{}{
				"url":     "https://download.docker.com/linux/fedora/docker-ce.repo",
				"key_url": "https://download.docker.com/linux/fedora/gpg",
			},
			wantErr: true,
		},
		{
			name: "valid params",
			params: map[string]interface{}{
				"url":        "https://download.docker.com/linux/fedora/docker-ce.repo",
				"key_url":    "https://download.docker.com/linux/fedora/gpg",
				"key_sha256": "abcd1234567890",
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
