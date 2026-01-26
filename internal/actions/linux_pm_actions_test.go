package actions

import (
	"strings"
	"testing"
)

func TestPacmanInstallAction_Name(t *testing.T) {
	t.Parallel()
	action := &PacmanInstallAction{}
	if action.Name() != "pacman_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "pacman_install")
	}
}

func TestPacmanInstallAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &PacmanInstallAction{}
	constraint := action.ImplicitConstraint()

	if constraint == nil {
		t.Fatal("ImplicitConstraint() returned nil")
	}
	if constraint.OS != "linux" {
		t.Errorf("OS = %q, want %q", constraint.OS, "linux")
	}
	if constraint.LinuxFamily != "arch" {
		t.Errorf("LinuxFamily = %q, want %q", constraint.LinuxFamily, "arch")
	}
}

func TestPacmanInstallAction_Validate(t *testing.T) {
	t.Parallel()
	action := &PacmanInstallAction{}

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
				"packages": []interface{}{"base-devel", "openssl"},
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

func TestPacmanInstallAction_Execute(t *testing.T) {
	t.Parallel()
	action := &PacmanInstallAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"packages": []interface{}{"base-devel"},
	})
	// On non-Arch systems, this returns DependencyMissingError
	// (pacman command not available, so packages are reported as missing)
	if err != nil {
		if !IsDependencyMissing(err) {
			t.Errorf("Execute() error = %v, want DependencyMissingError", err)
		}
		depErr := AsDependencyMissing(err)
		if depErr.Family != "arch" {
			t.Errorf("DependencyMissingError.Family = %q, want %q", depErr.Family, "arch")
		}
		if !strings.Contains(depErr.Command, "pacman -S") {
			t.Errorf("DependencyMissingError.Command = %q, want to contain 'pacman -S'", depErr.Command)
		}
	}
	// If no error, packages are installed (running on Arch)
}

func TestPacmanInstallAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := &PacmanInstallAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() should return true")
	}
}

func TestPacmanInstallAction_Preflight(t *testing.T) {
	t.Parallel()
	action := &PacmanInstallAction{}

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
				"packages": []interface{}{"base-devel", "openssl"},
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

func TestApkInstallAction_Name(t *testing.T) {
	t.Parallel()
	action := &ApkInstallAction{}
	if action.Name() != "apk_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "apk_install")
	}
}

func TestApkInstallAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &ApkInstallAction{}
	constraint := action.ImplicitConstraint()

	if constraint == nil {
		t.Fatal("ImplicitConstraint() returned nil")
	}
	if constraint.OS != "linux" {
		t.Errorf("OS = %q, want %q", constraint.OS, "linux")
	}
	if constraint.LinuxFamily != "alpine" {
		t.Errorf("LinuxFamily = %q, want %q", constraint.LinuxFamily, "alpine")
	}
}

func TestApkInstallAction_Validate(t *testing.T) {
	t.Parallel()
	action := &ApkInstallAction{}

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
				"packages": []interface{}{"build-base", "openssl-dev"},
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

func TestApkInstallAction_Execute(t *testing.T) {
	t.Parallel()
	action := &ApkInstallAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"packages": []interface{}{"curl"},
	})
	// On non-Alpine systems, this returns DependencyMissingError
	// (apk command not available, so packages are reported as missing)
	if err != nil {
		if !IsDependencyMissing(err) {
			t.Errorf("Execute() error = %v, want DependencyMissingError", err)
		}
		depErr := AsDependencyMissing(err)
		if depErr.Family != "alpine" {
			t.Errorf("DependencyMissingError.Family = %q, want %q", depErr.Family, "alpine")
		}
		if len(depErr.Packages) == 0 {
			t.Error("DependencyMissingError.Packages should not be empty")
		}
		if !strings.Contains(depErr.Command, "apk add") {
			t.Errorf("DependencyMissingError.Command = %q, want to contain 'apk add'", depErr.Command)
		}
	}
	// If no error, packages are installed (running on Alpine)
}

func TestApkInstallAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := &ApkInstallAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() should return true")
	}
}

func TestApkInstallAction_Preflight(t *testing.T) {
	t.Parallel()
	action := &ApkInstallAction{}

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
				"packages": []interface{}{"build-base", "openssl-dev"},
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

func TestZypperInstallAction_Name(t *testing.T) {
	t.Parallel()
	action := &ZypperInstallAction{}
	if action.Name() != "zypper_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "zypper_install")
	}
}

func TestZypperInstallAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &ZypperInstallAction{}
	constraint := action.ImplicitConstraint()

	if constraint == nil {
		t.Fatal("ImplicitConstraint() returned nil")
	}
	if constraint.OS != "linux" {
		t.Errorf("OS = %q, want %q", constraint.OS, "linux")
	}
	if constraint.LinuxFamily != "suse" {
		t.Errorf("LinuxFamily = %q, want %q", constraint.LinuxFamily, "suse")
	}
}

func TestZypperInstallAction_Validate(t *testing.T) {
	t.Parallel()
	action := &ZypperInstallAction{}

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
				"packages": []interface{}{"gcc", "libopenssl-devel"},
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

func TestZypperInstallAction_Execute(t *testing.T) {
	t.Parallel()
	action := &ZypperInstallAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"packages": []interface{}{"curl"},
	})
	// On non-SUSE systems, this returns DependencyMissingError
	// (rpm command not available, so packages are reported as missing)
	if err != nil {
		if !IsDependencyMissing(err) {
			t.Errorf("Execute() error = %v, want DependencyMissingError", err)
		}
		depErr := AsDependencyMissing(err)
		if depErr.Family != "suse" {
			t.Errorf("DependencyMissingError.Family = %q, want %q", depErr.Family, "suse")
		}
		if !strings.Contains(depErr.Command, "zypper install") {
			t.Errorf("DependencyMissingError.Command = %q, want to contain 'zypper install'", depErr.Command)
		}
	}
	// If no error, packages are installed (running on SUSE)
}

func TestZypperInstallAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := &ZypperInstallAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() should return true")
	}
}

func TestZypperInstallAction_Preflight(t *testing.T) {
	t.Parallel()
	action := &ZypperInstallAction{}

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
				"packages": []interface{}{"gcc", "libopenssl-devel"},
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

// TestAllInstallActions_ImplementSystemAction verifies that all install actions
// implement the SystemAction interface correctly.
func TestAllInstallActions_ImplementSystemAction(t *testing.T) {
	t.Parallel()

	actions := []struct {
		name       string
		action     SystemAction
		wantOS     string
		wantFamily string
	}{
		{"apt_install", &AptInstallAction{}, "linux", "debian"},
		{"apt_repo", &AptRepoAction{}, "linux", "debian"},
		{"apt_ppa", &AptPPAAction{}, "linux", "debian"},
		{"brew_install", &BrewInstallAction{}, "darwin", ""},
		{"brew_cask", &BrewCaskAction{}, "darwin", ""},
		{"dnf_install", &DnfInstallAction{}, "linux", "rhel"},
		{"dnf_repo", &DnfRepoAction{}, "linux", "rhel"},
		{"pacman_install", &PacmanInstallAction{}, "linux", "arch"},
		{"apk_install", &ApkInstallAction{}, "linux", "alpine"},
		{"zypper_install", &ZypperInstallAction{}, "linux", "suse"},
	}

	for _, tt := range actions {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Verify Name() returns correct name
			if tt.action.Name() != tt.name {
				t.Errorf("Name() = %q, want %q", tt.action.Name(), tt.name)
			}

			// Verify ImplicitConstraint() returns correct constraint
			constraint := tt.action.ImplicitConstraint()
			if constraint == nil {
				t.Fatal("ImplicitConstraint() returned nil")
			}
			if constraint.OS != tt.wantOS {
				t.Errorf("Constraint.OS = %q, want %q", constraint.OS, tt.wantOS)
			}
			if constraint.LinuxFamily != tt.wantFamily {
				t.Errorf("Constraint.LinuxFamily = %q, want %q", constraint.LinuxFamily, tt.wantFamily)
			}
		})
	}
}

func TestPacmanInstallAction_Describe(t *testing.T) {
	t.Parallel()
	action := &PacmanInstallAction{}

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
				"packages": []interface{}{"base-devel"},
			},
			want: "sudo pacman -S --noconfirm base-devel",
		},
		{
			name: "multiple packages",
			params: map[string]interface{}{
				"packages": []interface{}{"base-devel", "openssl"},
			},
			want: "sudo pacman -S --noconfirm base-devel openssl",
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

func TestApkInstallAction_Describe(t *testing.T) {
	t.Parallel()
	action := &ApkInstallAction{}

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
				"packages": []interface{}{"build-base"},
			},
			want: "sudo apk add build-base",
		},
		{
			name: "multiple packages",
			params: map[string]interface{}{
				"packages": []interface{}{"build-base", "openssl-dev"},
			},
			want: "sudo apk add build-base openssl-dev",
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

func TestZypperInstallAction_Describe(t *testing.T) {
	t.Parallel()
	action := &ZypperInstallAction{}

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
				"packages": []interface{}{"gcc"},
			},
			want: "sudo zypper install -y gcc",
		},
		{
			name: "multiple packages",
			params: map[string]interface{}{
				"packages": []interface{}{"gcc", "libopenssl-devel"},
			},
			want: "sudo zypper install -y gcc libopenssl-devel",
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
