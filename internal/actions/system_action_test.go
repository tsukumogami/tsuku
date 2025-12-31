package actions

import "testing"

func TestConstraint_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		constraint Constraint
		want       string
	}{
		{
			name:       "darwin only",
			constraint: Constraint{OS: "darwin"},
			want:       "darwin",
		},
		{
			name:       "linux with family",
			constraint: Constraint{OS: "linux", LinuxFamily: "debian"},
			want:       "linux/debian",
		},
		{
			name:       "linux without family",
			constraint: Constraint{OS: "linux"},
			want:       "linux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.constraint.String(); got != tt.want {
				t.Errorf("Constraint.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractBaseSystemFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		params            map[string]interface{}
		wantFallback      string
		wantUnlessCommand string
	}{
		{
			name:              "empty params",
			params:            map[string]interface{}{},
			wantFallback:      "",
			wantUnlessCommand: "",
		},
		{
			name: "with fallback",
			params: map[string]interface{}{
				"fallback": "Install manually from website",
			},
			wantFallback:      "Install manually from website",
			wantUnlessCommand: "",
		},
		{
			name: "with unless_command",
			params: map[string]interface{}{
				"unless_command": "docker",
			},
			wantFallback:      "",
			wantUnlessCommand: "docker",
		},
		{
			name: "with both fields",
			params: map[string]interface{}{
				"fallback":       "Visit https://example.com",
				"unless_command": "curl",
			},
			wantFallback:      "Visit https://example.com",
			wantUnlessCommand: "curl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fields := ExtractBaseSystemFields(tt.params)
			if fields.Fallback != tt.wantFallback {
				t.Errorf("Fallback = %q, want %q", fields.Fallback, tt.wantFallback)
			}
			if fields.UnlessCommand != tt.wantUnlessCommand {
				t.Errorf("UnlessCommand = %q, want %q", fields.UnlessCommand, tt.wantUnlessCommand)
			}
		})
	}
}

func TestValidatePackages(t *testing.T) {
	t.Parallel()

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
			name: "empty packages",
			params: map[string]interface{}{
				"packages": []interface{}{},
			},
			wantErr: true,
		},
		{
			name: "valid packages",
			params: map[string]interface{}{
				"packages": []interface{}{"curl", "wget"},
			},
			wantErr: false,
		},
		{
			name: "wrong type",
			params: map[string]interface{}{
				"packages": "not-a-slice",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ValidatePackages(tt.params, "test_action")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePackages() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
