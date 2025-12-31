package actions

import (
	"strings"
	"testing"
)

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

func TestAptInstallAction_Preflight(t *testing.T) {
	t.Parallel()
	action := &AptInstallAction{}

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
				"packages": []interface{}{"build-essential", "libssl-dev"},
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

func TestAptRepoAction_Preflight(t *testing.T) {
	t.Parallel()
	action := &AptRepoAction{}

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
				"url":     "https://download.docker.com/linux/ubuntu",
				"key_url": "https://download.docker.com/linux/ubuntu/gpg",
			},
			wantErrors: 1,
			wantErrMsg: "requires 'key_sha256' parameter",
		},
		{
			name: "valid params",
			params: map[string]interface{}{
				"url":        "https://download.docker.com/linux/ubuntu",
				"key_url":    "https://download.docker.com/linux/ubuntu/gpg",
				"key_sha256": "1500c1f56fa9e26b9b8f42452a553675796ade0807cdce11975eb98170b3a570",
			},
			wantErrors: 0,
		},
		{
			name: "http url rejected",
			params: map[string]interface{}{
				"url":        "http://download.docker.com/linux/ubuntu",
				"key_url":    "https://download.docker.com/linux/ubuntu/gpg",
				"key_sha256": "1500c1f56fa9e26b9b8f42452a553675796ade0807cdce11975eb98170b3a570",
			},
			wantErrors: 1,
			wantErrMsg: "must use HTTPS",
		},
		{
			name: "http key_url rejected",
			params: map[string]interface{}{
				"url":        "https://download.docker.com/linux/ubuntu",
				"key_url":    "http://download.docker.com/linux/ubuntu/gpg",
				"key_sha256": "1500c1f56fa9e26b9b8f42452a553675796ade0807cdce11975eb98170b3a570",
			},
			wantErrors: 1,
			wantErrMsg: "must use HTTPS",
		},
		{
			name: "both http urls rejected",
			params: map[string]interface{}{
				"url":        "http://download.docker.com/linux/ubuntu",
				"key_url":    "http://download.docker.com/linux/ubuntu/gpg",
				"key_sha256": "1500c1f56fa9e26b9b8f42452a553675796ade0807cdce11975eb98170b3a570",
			},
			wantErrors: 2,
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

func TestAptPPAAction_Preflight(t *testing.T) {
	t.Parallel()
	action := &AptPPAAction{}

	tests := []struct {
		name       string
		params     map[string]interface{}
		wantErrors int
		wantErrMsg string
	}{
		{
			name:       "missing ppa",
			params:     map[string]interface{}{},
			wantErrors: 1,
			wantErrMsg: "requires 'ppa' parameter",
		},
		{
			name:       "empty ppa",
			params:     map[string]interface{}{"ppa": ""},
			wantErrors: 1,
			wantErrMsg: "requires 'ppa' parameter",
		},
		{
			name:       "valid ppa",
			params:     map[string]interface{}{"ppa": "deadsnakes/ppa"},
			wantErrors: 0,
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

func TestAptInstallAction_Describe(t *testing.T) {
	t.Parallel()
	action := &AptInstallAction{}

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
				"packages": []interface{}{"curl"},
			},
			want: "sudo apt-get install -y curl",
		},
		{
			name: "multiple packages",
			params: map[string]interface{}{
				"packages": []interface{}{"build-essential", "libssl-dev"},
			},
			want: "sudo apt-get install -y build-essential libssl-dev",
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

func TestAptRepoAction_Describe(t *testing.T) {
	t.Parallel()
	action := &AptRepoAction{}

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
			name: "missing key_url",
			params: map[string]interface{}{
				"url": "https://download.docker.com/linux/ubuntu",
			},
			want: "",
		},
		{
			name: "valid params",
			params: map[string]interface{}{
				"url":     "https://download.docker.com/linux/ubuntu",
				"key_url": "https://download.docker.com/linux/ubuntu/gpg",
			},
			want: "curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/repo.gpg && " +
				"echo \"deb [signed-by=/etc/apt/keyrings/repo.gpg] https://download.docker.com/linux/ubuntu stable main\" | " +
				"sudo tee /etc/apt/sources.list.d/repo.list",
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

func TestAptPPAAction_Describe(t *testing.T) {
	t.Parallel()
	action := &AptPPAAction{}

	tests := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{
			name:   "missing ppa",
			params: map[string]interface{}{},
			want:   "",
		},
		{
			name:   "valid ppa",
			params: map[string]interface{}{"ppa": "deadsnakes/ppa"},
			want:   "sudo add-apt-repository ppa:deadsnakes/ppa",
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
