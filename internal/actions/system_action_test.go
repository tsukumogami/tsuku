package actions

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/platform"
)

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

func TestConstraint_MatchesTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		constraint Constraint
		target     platform.Target
		want       bool
	}{
		// Darwin constraints
		{
			name:       "darwin constraint matches darwin target",
			constraint: Constraint{OS: "darwin"},
			target:     platform.NewTarget("darwin/arm64", ""),
			want:       true,
		},
		{
			name:       "darwin constraint does not match linux target",
			constraint: Constraint{OS: "darwin"},
			target:     platform.NewTarget("linux/amd64", "debian"),
			want:       false,
		},
		// Linux constraints with family
		{
			name:       "debian constraint matches debian target",
			constraint: Constraint{OS: "linux", LinuxFamily: "debian"},
			target:     platform.NewTarget("linux/amd64", "debian"),
			want:       true,
		},
		{
			name:       "debian constraint does not match rhel target",
			constraint: Constraint{OS: "linux", LinuxFamily: "debian"},
			target:     platform.NewTarget("linux/amd64", "rhel"),
			want:       false,
		},
		{
			name:       "rhel constraint matches rhel target",
			constraint: Constraint{OS: "linux", LinuxFamily: "rhel"},
			target:     platform.NewTarget("linux/amd64", "rhel"),
			want:       true,
		},
		{
			name:       "arch constraint matches arch target",
			constraint: Constraint{OS: "linux", LinuxFamily: "arch"},
			target:     platform.NewTarget("linux/amd64", "arch"),
			want:       true,
		},
		{
			name:       "alpine constraint matches alpine target",
			constraint: Constraint{OS: "linux", LinuxFamily: "alpine"},
			target:     platform.NewTarget("linux/amd64", "alpine"),
			want:       true,
		},
		{
			name:       "suse constraint matches suse target",
			constraint: Constraint{OS: "linux", LinuxFamily: "suse"},
			target:     platform.NewTarget("linux/amd64", "suse"),
			want:       true,
		},
		// Linux constraint without family
		{
			name:       "linux-only constraint matches any linux family",
			constraint: Constraint{OS: "linux"},
			target:     platform.NewTarget("linux/amd64", "debian"),
			want:       true,
		},
		{
			name:       "linux-only constraint matches linux without family",
			constraint: Constraint{OS: "linux"},
			target:     platform.NewTarget("linux/amd64", ""),
			want:       true,
		},
		// Cross-OS mismatches
		{
			name:       "linux constraint does not match darwin",
			constraint: Constraint{OS: "linux", LinuxFamily: "debian"},
			target:     platform.NewTarget("darwin/arm64", ""),
			want:       false,
		},
		// Architecture is not checked (constraint is OS/family only)
		{
			name:       "constraint matches regardless of architecture",
			constraint: Constraint{OS: "linux", LinuxFamily: "debian"},
			target:     platform.NewTarget("linux/arm64", "debian"),
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.constraint.MatchesTarget(tt.target); got != tt.want {
				t.Errorf("Constraint.MatchesTarget() = %v, want %v", got, tt.want)
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

func TestSystemAction_IsExternallyManaged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action SystemAction
		want   bool
	}{
		// Package manager actions return true
		{"AptInstallAction", &AptInstallAction{}, true},
		{"AptRepoAction", &AptRepoAction{}, true},
		{"AptPPAAction", &AptPPAAction{}, true},
		{"BrewInstallAction", &BrewInstallAction{}, true},
		{"BrewCaskAction", &BrewCaskAction{}, true},
		{"DnfInstallAction", &DnfInstallAction{}, true},
		{"DnfRepoAction", &DnfRepoAction{}, true},
		{"PacmanInstallAction", &PacmanInstallAction{}, true},
		{"ApkInstallAction", &ApkInstallAction{}, true},
		{"ZypperInstallAction", &ZypperInstallAction{}, true},

		// Other system actions return false
		{"GroupAddAction", &GroupAddAction{}, false},
		{"ServiceEnableAction", &ServiceEnableAction{}, false},
		{"ServiceStartAction", &ServiceStartAction{}, false},
		{"RequireCommandAction", &RequireCommandAction{}, false},
		{"ManualAction", &ManualAction{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.action.IsExternallyManaged(); got != tt.want {
				t.Errorf("%s.IsExternallyManaged() = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
