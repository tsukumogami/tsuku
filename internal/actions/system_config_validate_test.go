package actions

import (
	"testing"
)

// Tests for Validate and ImplicitConstraint methods that were at 0% coverage.

func TestGroupAddAction_Validate(t *testing.T) {
	t.Parallel()
	action := &GroupAddAction{}

	tests := []struct {
		name    string
		params  map[string]any
		wantErr bool
	}{
		{
			name:    "valid group",
			params:  map[string]any{"group": "docker"},
			wantErr: false,
		},
		{
			name:    "missing group",
			params:  map[string]any{},
			wantErr: true,
		},
		{
			name:    "empty group",
			params:  map[string]any{"group": ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Validate(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGroupAddAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &GroupAddAction{}
	if c := action.ImplicitConstraint(); c != nil {
		t.Errorf("ImplicitConstraint() = %v, want nil", c)
	}
}

func TestGroupAddAction_IsExternallyManaged(t *testing.T) {
	t.Parallel()
	action := &GroupAddAction{}
	if action.IsExternallyManaged() {
		t.Error("IsExternallyManaged() = true, want false")
	}
}

func TestServiceEnableAction_Validate(t *testing.T) {
	t.Parallel()
	action := &ServiceEnableAction{}

	tests := []struct {
		name    string
		params  map[string]any
		wantErr bool
	}{
		{
			name:    "valid service",
			params:  map[string]any{"service": "docker"},
			wantErr: false,
		},
		{
			name:    "missing service",
			params:  map[string]any{},
			wantErr: true,
		},
		{
			name:    "empty service",
			params:  map[string]any{"service": ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Validate(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServiceEnableAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &ServiceEnableAction{}
	if c := action.ImplicitConstraint(); c != nil {
		t.Errorf("ImplicitConstraint() = %v, want nil", c)
	}
}

func TestServiceEnableAction_IsExternallyManaged(t *testing.T) {
	t.Parallel()
	action := &ServiceEnableAction{}
	if action.IsExternallyManaged() {
		t.Error("IsExternallyManaged() = true, want false")
	}
}

func TestServiceStartAction_Validate(t *testing.T) {
	t.Parallel()
	action := &ServiceStartAction{}

	tests := []struct {
		name    string
		params  map[string]any
		wantErr bool
	}{
		{
			name:    "valid service",
			params:  map[string]any{"service": "docker"},
			wantErr: false,
		},
		{
			name:    "missing service",
			params:  map[string]any{},
			wantErr: true,
		},
		{
			name:    "empty service",
			params:  map[string]any{"service": ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Validate(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServiceStartAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &ServiceStartAction{}
	if c := action.ImplicitConstraint(); c != nil {
		t.Errorf("ImplicitConstraint() = %v, want nil", c)
	}
}

func TestServiceStartAction_IsExternallyManaged(t *testing.T) {
	t.Parallel()
	action := &ServiceStartAction{}
	if action.IsExternallyManaged() {
		t.Error("IsExternallyManaged() = true, want false")
	}
}

func TestRequireCommandAction_Validate(t *testing.T) {
	t.Parallel()
	action := &RequireCommandAction{}

	tests := []struct {
		name    string
		params  map[string]any
		wantErr bool
	}{
		{
			name:    "valid command",
			params:  map[string]any{"command": "ls"},
			wantErr: false,
		},
		{
			name:    "missing command",
			params:  map[string]any{},
			wantErr: true,
		},
		{
			name:    "empty command",
			params:  map[string]any{"command": ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Validate(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRequireCommandAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &RequireCommandAction{}
	if c := action.ImplicitConstraint(); c != nil {
		t.Errorf("ImplicitConstraint() = %v, want nil", c)
	}
}

func TestRequireCommandAction_IsExternallyManaged(t *testing.T) {
	t.Parallel()
	action := &RequireCommandAction{}
	if action.IsExternallyManaged() {
		t.Error("IsExternallyManaged() = true, want false")
	}
}

func TestManualAction_Validate(t *testing.T) {
	t.Parallel()
	action := &ManualAction{}

	tests := []struct {
		name    string
		params  map[string]any
		wantErr bool
	}{
		{
			name:    "valid text",
			params:  map[string]any{"text": "Install manually"},
			wantErr: false,
		},
		{
			name:    "missing text",
			params:  map[string]any{},
			wantErr: true,
		},
		{
			name:    "empty text",
			params:  map[string]any{"text": ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := action.Validate(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManualAction_ImplicitConstraint(t *testing.T) {
	t.Parallel()
	action := &ManualAction{}
	if c := action.ImplicitConstraint(); c != nil {
		t.Errorf("ImplicitConstraint() = %v, want nil", c)
	}
}

func TestManualAction_IsExternallyManaged(t *testing.T) {
	t.Parallel()
	action := &ManualAction{}
	if action.IsExternallyManaged() {
		t.Error("IsExternallyManaged() = true, want false")
	}
}
