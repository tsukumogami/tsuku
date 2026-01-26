package actions

import (
	"strings"
	"testing"
)

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

	// On non-RHEL systems, packages won't be installed, so we expect DependencyMissingError.
	// On RHEL systems with the packages installed, we expect nil.
	if err != nil {
		depErr := AsDependencyMissing(err)
		if depErr == nil {
			t.Errorf("Execute() error = %v, want DependencyMissingError or nil", err)
		} else {
			// Verify error contains expected information
			if depErr.Family != "rhel" {
				t.Errorf("DependencyMissingError.Family = %q, want %q", depErr.Family, "rhel")
			}
			if len(depErr.Packages) == 0 {
				t.Error("DependencyMissingError.Packages should not be empty")
			}
			if !strings.Contains(depErr.Command, "dnf install") {
				t.Errorf("DependencyMissingError.Command = %q, want to contain 'dnf install'", depErr.Command)
			}
		}
	}
}

func TestDnfInstallAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := &DnfInstallAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() should return true")
	}
}

func TestDnfInstallAction_Preflight(t *testing.T) {
	t.Parallel()
	action := &DnfInstallAction{}

	tests := []struct {
		name       string
		params     map[string]interface{}
		wantErrors int
		wantErrMsg string
	}{
		{
			name:       "missing packages",
			params:     map[string]interface{}{},
			wantErrors: 1,
			wantErrMsg: "requires non-empty 'packages' parameter",
		},
		{
			name: "valid packages",
			params: map[string]interface{}{
				"packages": []interface{}{"gcc", "openssl-devel"},
			},
			wantErrors: 0,
		},
		{
			name: "empty packages",
			params: map[string]interface{}{
				"packages": []interface{}{},
			},
			wantErrors: 1,
			wantErrMsg: "requires non-empty 'packages' parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := action.Preflight(tt.params)
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Preflight() errors = %v, want %d errors", result.Errors, tt.wantErrors)
			}
			if tt.wantErrMsg != "" && len(result.Errors) > 0 {
				if !strings.Contains(result.Errors[0], tt.wantErrMsg) {
					t.Errorf("Preflight() error = %q, want to contain %q", result.Errors[0], tt.wantErrMsg)
				}
			}
		})
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

func TestDnfRepoAction_Preflight(t *testing.T) {
	t.Parallel()
	action := &DnfRepoAction{}

	tests := []struct {
		name       string
		params     map[string]interface{}
		wantErrors int
		wantErrMsg string
	}{
		{
			name:       "missing all fields",
			params:     map[string]interface{}{},
			wantErrors: 3,
			wantErrMsg: "requires",
		},
		{
			name: "missing key_sha256",
			params: map[string]interface{}{
				"url":     "https://download.docker.com/linux/fedora/docker-ce.repo",
				"key_url": "https://download.docker.com/linux/fedora/gpg",
			},
			wantErrors: 1,
			wantErrMsg: "requires 'key_sha256' parameter",
		},
		{
			name: "valid params",
			params: map[string]interface{}{
				"url":        "https://download.docker.com/linux/fedora/docker-ce.repo",
				"key_url":    "https://download.docker.com/linux/fedora/gpg",
				"key_sha256": "abcd1234567890",
			},
			wantErrors: 0,
		},
		{
			name: "http url rejected",
			params: map[string]interface{}{
				"url":        "http://download.docker.com/linux/fedora/docker-ce.repo",
				"key_url":    "https://download.docker.com/linux/fedora/gpg",
				"key_sha256": "abcd1234567890",
			},
			wantErrors: 1,
			wantErrMsg: "must use HTTPS",
		},
		{
			name: "http key_url rejected",
			params: map[string]interface{}{
				"url":        "https://download.docker.com/linux/fedora/docker-ce.repo",
				"key_url":    "http://download.docker.com/linux/fedora/gpg",
				"key_sha256": "abcd1234567890",
			},
			wantErrors: 1,
			wantErrMsg: "must use HTTPS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := action.Preflight(tt.params)
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Preflight() errors = %v, want %d errors", result.Errors, tt.wantErrors)
			}
			if tt.wantErrMsg != "" && len(result.Errors) > 0 {
				found := false
				for _, err := range result.Errors {
					if strings.Contains(err, tt.wantErrMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Preflight() errors = %v, want to contain %q", result.Errors, tt.wantErrMsg)
				}
			}
		})
	}
}

func TestDnfInstallAction_Describe(t *testing.T) {
	t.Parallel()
	action := &DnfInstallAction{}

	tests := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{
			name:   "missing packages",
			params: map[string]interface{}{},
			want:   "",
		},
		{
			name: "single package",
			params: map[string]interface{}{
				"packages": []interface{}{"docker-ce"},
			},
			want: "sudo dnf install -y docker-ce",
		},
		{
			name: "multiple packages",
			params: map[string]interface{}{
				"packages": []interface{}{"docker-ce", "containerd.io"},
			},
			want: "sudo dnf install -y docker-ce containerd.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := action.Describe(tt.params)
			if got != tt.want {
				t.Errorf("Describe() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDnfRepoAction_Describe(t *testing.T) {
	t.Parallel()
	action := &DnfRepoAction{}

	tests := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{
			name:   "missing url",
			params: map[string]interface{}{},
			want:   "",
		},
		{
			name: "valid url",
			params: map[string]interface{}{
				"url": "https://download.docker.com/linux/fedora/docker-ce.repo",
			},
			want: "sudo dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := action.Describe(tt.params)
			if got != tt.want {
				t.Errorf("Describe() = %q, want %q", got, tt.want)
			}
		})
	}
}
