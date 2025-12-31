package actions

import (
	"context"
	"strings"
	"testing"
)

// TestGroupAddAction tests the GroupAddAction struct.
func TestGroupAddAction_Name(t *testing.T) {
	action := &GroupAddAction{}
	if got := action.Name(); got != "group_add" {
		t.Errorf("Name() = %q, want %q", got, "group_add")
	}
}

func TestGroupAddAction_IsDeterministic(t *testing.T) {
	action := &GroupAddAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestGroupAddAction_Preflight(t *testing.T) {
	tests := []struct {
		name       string
		params     map[string]interface{}
		wantErrors int
		wantErrMsg string
	}{
		{
			name:       "valid group",
			params:     map[string]interface{}{"group": "docker"},
			wantErrors: 0,
		},
		{
			name:       "valid group with underscore",
			params:     map[string]interface{}{"group": "_docker"},
			wantErrors: 0,
		},
		{
			name:       "valid group with hyphen",
			params:     map[string]interface{}{"group": "docker-users"},
			wantErrors: 0,
		},
		{
			name:       "missing group",
			params:     map[string]interface{}{},
			wantErrors: 1,
			wantErrMsg: "requires 'group' parameter",
		},
		{
			name:       "empty group",
			params:     map[string]interface{}{"group": ""},
			wantErrors: 1,
			wantErrMsg: "cannot be empty",
		},
		{
			name:       "invalid group - starts with number",
			params:     map[string]interface{}{"group": "1docker"},
			wantErrors: 1,
			wantErrMsg: "invalid characters",
		},
		{
			name:       "invalid group - contains space",
			params:     map[string]interface{}{"group": "docker users"},
			wantErrors: 1,
			wantErrMsg: "invalid characters",
		},
		{
			name:       "invalid group - contains slash",
			params:     map[string]interface{}{"group": "docker/users"},
			wantErrors: 1,
			wantErrMsg: "invalid characters",
		},
	}

	action := &GroupAddAction{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := action.Preflight(tt.params)
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Preflight() errors = %v, want %d", result.Errors, tt.wantErrors)
			}
			if tt.wantErrMsg != "" && len(result.Errors) > 0 {
				if !strings.Contains(result.Errors[0], tt.wantErrMsg) {
					t.Errorf("Preflight() error = %q, want to contain %q", result.Errors[0], tt.wantErrMsg)
				}
			}
		})
	}
}

func TestGroupAddAction_Execute(t *testing.T) {
	action := &GroupAddAction{}
	ctx := &ExecutionContext{Context: context.Background()}

	t.Run("valid execution", func(t *testing.T) {
		params := map[string]interface{}{"group": "docker"}
		err := action.Execute(ctx, params)
		if err != nil {
			t.Errorf("Execute() error = %v, want nil", err)
		}
	})

	t.Run("missing group", func(t *testing.T) {
		params := map[string]interface{}{}
		err := action.Execute(ctx, params)
		if err == nil {
			t.Error("Execute() error = nil, want error")
		}
	})
}

// TestServiceEnableAction tests the ServiceEnableAction struct.
func TestServiceEnableAction_Name(t *testing.T) {
	action := &ServiceEnableAction{}
	if got := action.Name(); got != "service_enable" {
		t.Errorf("Name() = %q, want %q", got, "service_enable")
	}
}

func TestServiceEnableAction_IsDeterministic(t *testing.T) {
	action := &ServiceEnableAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestServiceEnableAction_Preflight(t *testing.T) {
	tests := []struct {
		name       string
		params     map[string]interface{}
		wantErrors int
		wantErrMsg string
	}{
		{
			name:       "valid service",
			params:     map[string]interface{}{"service": "docker"},
			wantErrors: 0,
		},
		{
			name:       "valid service with dot",
			params:     map[string]interface{}{"service": "docker.service"},
			wantErrors: 0,
		},
		{
			name:       "valid service with @",
			params:     map[string]interface{}{"service": "getty@tty1"},
			wantErrors: 0,
		},
		{
			name:       "missing service",
			params:     map[string]interface{}{},
			wantErrors: 1,
			wantErrMsg: "requires 'service' parameter",
		},
		{
			name:       "empty service",
			params:     map[string]interface{}{"service": ""},
			wantErrors: 1,
			wantErrMsg: "cannot be empty",
		},
		{
			name:       "invalid service - contains space",
			params:     map[string]interface{}{"service": "docker service"},
			wantErrors: 1,
			wantErrMsg: "invalid characters",
		},
		{
			name:       "invalid service - contains slash",
			params:     map[string]interface{}{"service": "docker/service"},
			wantErrors: 1,
			wantErrMsg: "invalid characters",
		},
	}

	action := &ServiceEnableAction{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := action.Preflight(tt.params)
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Preflight() errors = %v, want %d", result.Errors, tt.wantErrors)
			}
			if tt.wantErrMsg != "" && len(result.Errors) > 0 {
				if !strings.Contains(result.Errors[0], tt.wantErrMsg) {
					t.Errorf("Preflight() error = %q, want to contain %q", result.Errors[0], tt.wantErrMsg)
				}
			}
		})
	}
}

func TestServiceEnableAction_Execute(t *testing.T) {
	action := &ServiceEnableAction{}
	ctx := &ExecutionContext{Context: context.Background()}

	t.Run("valid execution", func(t *testing.T) {
		params := map[string]interface{}{"service": "docker"}
		err := action.Execute(ctx, params)
		if err != nil {
			t.Errorf("Execute() error = %v, want nil", err)
		}
	})

	t.Run("missing service", func(t *testing.T) {
		params := map[string]interface{}{}
		err := action.Execute(ctx, params)
		if err == nil {
			t.Error("Execute() error = nil, want error")
		}
	})
}

// TestServiceStartAction tests the ServiceStartAction struct.
func TestServiceStartAction_Name(t *testing.T) {
	action := &ServiceStartAction{}
	if got := action.Name(); got != "service_start" {
		t.Errorf("Name() = %q, want %q", got, "service_start")
	}
}

func TestServiceStartAction_IsDeterministic(t *testing.T) {
	action := &ServiceStartAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestServiceStartAction_Preflight(t *testing.T) {
	tests := []struct {
		name       string
		params     map[string]interface{}
		wantErrors int
		wantErrMsg string
	}{
		{
			name:       "valid service",
			params:     map[string]interface{}{"service": "docker"},
			wantErrors: 0,
		},
		{
			name:       "missing service",
			params:     map[string]interface{}{},
			wantErrors: 1,
			wantErrMsg: "requires 'service' parameter",
		},
		{
			name:       "empty service",
			params:     map[string]interface{}{"service": ""},
			wantErrors: 1,
			wantErrMsg: "cannot be empty",
		},
	}

	action := &ServiceStartAction{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := action.Preflight(tt.params)
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Preflight() errors = %v, want %d", result.Errors, tt.wantErrors)
			}
			if tt.wantErrMsg != "" && len(result.Errors) > 0 {
				if !strings.Contains(result.Errors[0], tt.wantErrMsg) {
					t.Errorf("Preflight() error = %q, want to contain %q", result.Errors[0], tt.wantErrMsg)
				}
			}
		})
	}
}

func TestServiceStartAction_Execute(t *testing.T) {
	action := &ServiceStartAction{}
	ctx := &ExecutionContext{Context: context.Background()}

	t.Run("valid execution", func(t *testing.T) {
		params := map[string]interface{}{"service": "docker"}
		err := action.Execute(ctx, params)
		if err != nil {
			t.Errorf("Execute() error = %v, want nil", err)
		}
	})

	t.Run("missing service", func(t *testing.T) {
		params := map[string]interface{}{}
		err := action.Execute(ctx, params)
		if err == nil {
			t.Error("Execute() error = nil, want error")
		}
	})
}

// TestRequireCommandAction tests the RequireCommandAction struct.
func TestRequireCommandAction_Name(t *testing.T) {
	action := &RequireCommandAction{}
	if got := action.Name(); got != "require_command" {
		t.Errorf("Name() = %q, want %q", got, "require_command")
	}
}

func TestRequireCommandAction_IsDeterministic(t *testing.T) {
	action := &RequireCommandAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestRequireCommandAction_Preflight(t *testing.T) {
	tests := []struct {
		name       string
		params     map[string]interface{}
		wantErrors int
		wantErrMsg string
	}{
		{
			name:       "valid command",
			params:     map[string]interface{}{"command": "docker"},
			wantErrors: 0,
		},
		{
			name:       "valid command with hyphen",
			params:     map[string]interface{}{"command": "docker-compose"},
			wantErrors: 0,
		},
		{
			name: "valid command with version check",
			params: map[string]interface{}{
				"command":       "docker",
				"version_flag":  "--version",
				"version_regex": `Docker version (\d+\.\d+\.\d+)`,
				"min_version":   "20.0.0",
			},
			wantErrors: 0,
		},
		{
			name:       "missing command",
			params:     map[string]interface{}{},
			wantErrors: 1,
			wantErrMsg: "requires 'command' parameter",
		},
		{
			name:       "empty command",
			params:     map[string]interface{}{"command": ""},
			wantErrors: 1,
			wantErrMsg: "cannot be empty",
		},
		{
			name:       "invalid command - contains path",
			params:     map[string]interface{}{"command": "/usr/bin/docker"},
			wantErrors: 1,
			wantErrMsg: "invalid characters",
		},
		{
			name:       "invalid command - contains shell metachar",
			params:     map[string]interface{}{"command": "docker; rm -rf /"},
			wantErrors: 1,
			wantErrMsg: "invalid characters",
		},
		{
			name:       "invalid command - contains pipe",
			params:     map[string]interface{}{"command": "docker|grep"},
			wantErrors: 1,
			wantErrMsg: "invalid characters",
		},
		{
			name: "min_version without version_flag",
			params: map[string]interface{}{
				"command":       "docker",
				"version_regex": `(\d+)`,
				"min_version":   "20.0.0",
			},
			wantErrors: 1,
			wantErrMsg: "requires 'version_flag'",
		},
		{
			name: "min_version without version_regex",
			params: map[string]interface{}{
				"command":      "docker",
				"version_flag": "--version",
				"min_version":  "20.0.0",
			},
			wantErrors: 1,
			wantErrMsg: "requires 'version_regex'",
		},
		{
			name: "invalid version_regex",
			params: map[string]interface{}{
				"command":       "docker",
				"version_flag":  "--version",
				"version_regex": "[invalid",
			},
			wantErrors: 1,
			wantErrMsg: "is invalid",
		},
	}

	action := &RequireCommandAction{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := action.Preflight(tt.params)
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Preflight() errors = %v, want %d", result.Errors, tt.wantErrors)
			}
			if tt.wantErrMsg != "" && len(result.Errors) > 0 {
				if !strings.Contains(result.Errors[0], tt.wantErrMsg) {
					t.Errorf("Preflight() error = %q, want to contain %q", result.Errors[0], tt.wantErrMsg)
				}
			}
		})
	}
}

func TestRequireCommandAction_Execute(t *testing.T) {
	action := &RequireCommandAction{}
	ctx := &ExecutionContext{Context: context.Background()}

	t.Run("command exists - ls", func(t *testing.T) {
		params := map[string]interface{}{"command": "ls"}
		err := action.Execute(ctx, params)
		if err != nil {
			t.Errorf("Execute() error = %v, want nil", err)
		}
	})

	t.Run("command not found", func(t *testing.T) {
		params := map[string]interface{}{"command": "nonexistent_command_12345"}
		err := action.Execute(ctx, params)
		if err == nil {
			t.Error("Execute() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "not found in PATH") {
			t.Errorf("Execute() error = %q, want to contain 'not found in PATH'", err.Error())
		}
	})

	t.Run("missing command param", func(t *testing.T) {
		params := map[string]interface{}{}
		err := action.Execute(ctx, params)
		if err == nil {
			t.Error("Execute() error = nil, want error")
		}
	})
}

// TestManualAction tests the ManualAction struct.
func TestManualAction_Name(t *testing.T) {
	action := &ManualAction{}
	if got := action.Name(); got != "manual" {
		t.Errorf("Name() = %q, want %q", got, "manual")
	}
}

func TestManualAction_IsDeterministic(t *testing.T) {
	action := &ManualAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestManualAction_Preflight(t *testing.T) {
	tests := []struct {
		name       string
		params     map[string]interface{}
		wantErrors int
		wantErrMsg string
	}{
		{
			name:       "valid text",
			params:     map[string]interface{}{"text": "Please install Docker manually."},
			wantErrors: 0,
		},
		{
			name:       "valid multiline text",
			params:     map[string]interface{}{"text": "Step 1: Download\nStep 2: Install\nStep 3: Configure"},
			wantErrors: 0,
		},
		{
			name:       "missing text",
			params:     map[string]interface{}{},
			wantErrors: 1,
			wantErrMsg: "requires 'text' parameter",
		},
		{
			name:       "empty text",
			params:     map[string]interface{}{"text": ""},
			wantErrors: 1,
			wantErrMsg: "cannot be empty",
		},
	}

	action := &ManualAction{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := action.Preflight(tt.params)
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Preflight() errors = %v, want %d", result.Errors, tt.wantErrors)
			}
			if tt.wantErrMsg != "" && len(result.Errors) > 0 {
				if !strings.Contains(result.Errors[0], tt.wantErrMsg) {
					t.Errorf("Preflight() error = %q, want to contain %q", result.Errors[0], tt.wantErrMsg)
				}
			}
		})
	}
}

func TestManualAction_Execute(t *testing.T) {
	action := &ManualAction{}
	ctx := &ExecutionContext{Context: context.Background()}

	t.Run("valid execution", func(t *testing.T) {
		params := map[string]interface{}{"text": "Please install Docker manually."}
		err := action.Execute(ctx, params)
		if err != nil {
			t.Errorf("Execute() error = %v, want nil", err)
		}
	})

	t.Run("missing text", func(t *testing.T) {
		params := map[string]interface{}{}
		err := action.Execute(ctx, params)
		if err == nil {
			t.Error("Execute() error = nil, want error")
		}
	})
}

// Test helper functions
func TestIsValidGroupName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"simple", "docker", true},
		{"with underscore", "docker_users", true},
		{"with hyphen", "docker-users", true},
		{"starts with underscore", "_docker", true},
		{"uppercase", "Docker", true},
		{"starts with number", "1docker", false},
		{"contains space", "docker users", false},
		{"contains slash", "docker/users", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidGroupName(tt.input); got != tt.want {
				t.Errorf("isValidGroupName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidServiceName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"simple", "docker", true},
		{"with dot", "docker.service", true},
		{"with @", "getty@tty1", true},
		{"with hyphen", "docker-engine", true},
		{"contains space", "docker service", false},
		{"contains slash", "docker/service", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidServiceName(tt.input); got != tt.want {
				t.Errorf("isValidServiceName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidCommandName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"simple", "docker", true},
		{"with hyphen", "docker-compose", true},
		{"with underscore", "docker_compose", true},
		{"contains path", "/usr/bin/docker", false},
		{"contains semicolon", "docker;rm", false},
		{"contains pipe", "docker|grep", false},
		{"contains ampersand", "docker&", false},
		{"contains space", "docker compose", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidCommandName(tt.input); got != tt.want {
				t.Errorf("isValidCommandName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestVersionMeetsMinimum(t *testing.T) {
	tests := []struct {
		name     string
		detected string
		minimum  string
		want     bool
	}{
		{"equal", "1.2.3", "1.2.3", true},
		{"greater major", "2.0.0", "1.2.3", true},
		{"greater minor", "1.3.0", "1.2.3", true},
		{"greater patch", "1.2.4", "1.2.3", true},
		{"less major", "0.9.0", "1.2.3", false},
		{"less minor", "1.1.0", "1.2.3", false},
		{"less patch", "1.2.2", "1.2.3", false},
		{"with v prefix", "v1.2.3", "1.2.3", true},
		{"both v prefix", "v1.2.3", "v1.2.3", true},
		{"longer detected", "1.2.3.4", "1.2.3", true},
		{"longer minimum", "1.2", "1.2.3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionMeetsMinimum(tt.detected, tt.minimum); got != tt.want {
				t.Errorf("versionMeetsMinimum(%q, %q) = %v, want %v", tt.detected, tt.minimum, got, tt.want)
			}
		})
	}
}

// Test action registration
func TestSystemConfigActionsRegistered(t *testing.T) {
	actions := []string{
		"group_add",
		"service_enable",
		"service_start",
		"require_command",
		"manual",
	}

	for _, name := range actions {
		t.Run(name, func(t *testing.T) {
			action := Get(name)
			if action == nil {
				t.Errorf("Get(%q) = nil, want action", name)
			}
			if action != nil && action.Name() != name {
				t.Errorf("Get(%q).Name() = %q, want %q", name, action.Name(), name)
			}
		})
	}
}

// Describe() tests for config actions

func TestGroupAddAction_Describe(t *testing.T) {
	action := &GroupAddAction{}

	tests := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{
			name:   "missing group",
			params: map[string]interface{}{},
			want:   "",
		},
		{
			name:   "valid group",
			params: map[string]interface{}{"group": "docker"},
			want:   "sudo usermod -aG docker $USER",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := action.Describe(tt.params)
			if got != tt.want {
				t.Errorf("Describe() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServiceEnableAction_Describe(t *testing.T) {
	action := &ServiceEnableAction{}

	tests := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{
			name:   "missing service",
			params: map[string]interface{}{},
			want:   "",
		},
		{
			name:   "valid service",
			params: map[string]interface{}{"service": "docker"},
			want:   "sudo systemctl enable docker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := action.Describe(tt.params)
			if got != tt.want {
				t.Errorf("Describe() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServiceStartAction_Describe(t *testing.T) {
	action := &ServiceStartAction{}

	tests := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{
			name:   "missing service",
			params: map[string]interface{}{},
			want:   "",
		},
		{
			name:   "valid service",
			params: map[string]interface{}{"service": "docker"},
			want:   "sudo systemctl start docker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := action.Describe(tt.params)
			if got != tt.want {
				t.Errorf("Describe() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRequireCommandAction_Describe(t *testing.T) {
	action := &RequireCommandAction{}

	tests := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{
			name:   "missing command",
			params: map[string]interface{}{},
			want:   "",
		},
		{
			name:   "command only",
			params: map[string]interface{}{"command": "docker"},
			want:   "Requires: docker",
		},
		{
			name: "command with version",
			params: map[string]interface{}{
				"command":     "docker",
				"min_version": "20.0.0",
			},
			want: "Requires: docker (version >= 20.0.0)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := action.Describe(tt.params)
			if got != tt.want {
				t.Errorf("Describe() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestManualAction_Describe(t *testing.T) {
	action := &ManualAction{}

	tests := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{
			name:   "missing text",
			params: map[string]interface{}{},
			want:   "",
		},
		{
			name:   "valid text",
			params: map[string]interface{}{"text": "Please install Docker manually."},
			want:   "Please install Docker manually.",
		},
		{
			name:   "multiline text",
			params: map[string]interface{}{"text": "Step 1: Download\nStep 2: Install"},
			want:   "Step 1: Download\nStep 2: Install",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := action.Describe(tt.params)
			if got != tt.want {
				t.Errorf("Describe() = %q, want %q", got, tt.want)
			}
		})
	}
}
