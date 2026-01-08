package recipe

import (
	"strings"
	"testing"
)

// mockLookup creates a ConstraintLookup for testing with predefined action constraints.
func mockLookup(constraints map[string]*Constraint) ConstraintLookup {
	return func(actionName string) (*Constraint, bool) {
		// Check if action is known
		constraint, known := constraints[actionName]
		return constraint, known
	}
}

func TestComputeAnalysis_UnknownAction(t *testing.T) {
	lookup := mockLookup(map[string]*Constraint{
		"download": nil,
	})

	_, err := ComputeAnalysis("unknown_action", nil, nil, lookup)
	if err == nil {
		t.Fatal("expected error for unknown action, got nil")
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("expected error to contain 'unknown action', got: %v", err)
	}
}

func TestComputeAnalysis_NoConstraintNoWhen(t *testing.T) {
	// Action with no implicit constraint, no when clause
	lookup := mockLookup(map[string]*Constraint{
		"download": nil, // no constraint
	})

	analysis, err := ComputeAnalysis("download", nil, nil, lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.Constraint != nil {
		t.Errorf("expected nil constraint, got: %+v", analysis.Constraint)
	}
	if analysis.FamilyVarying {
		t.Error("expected FamilyVarying=false, got true")
	}
}

func TestComputeAnalysis_ImplicitConstraintOnly(t *testing.T) {
	// apt_install has implicit constraint: linux/debian
	lookup := mockLookup(map[string]*Constraint{
		"apt_install": {OS: "linux", LinuxFamily: "debian"},
	})

	analysis, err := ComputeAnalysis("apt_install", nil, nil, lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.Constraint == nil {
		t.Fatal("expected non-nil constraint")
	}
	if analysis.Constraint.OS != "linux" {
		t.Errorf("expected OS=linux, got %q", analysis.Constraint.OS)
	}
	if analysis.Constraint.LinuxFamily != "debian" {
		t.Errorf("expected LinuxFamily=debian, got %q", analysis.Constraint.LinuxFamily)
	}
	if analysis.FamilyVarying {
		t.Error("expected FamilyVarying=false, got true")
	}
}

func TestComputeAnalysis_ImplicitWithCompatibleWhen(t *testing.T) {
	// apt_install (linux/debian) + when.arch: amd64 -> linux/debian/amd64
	lookup := mockLookup(map[string]*Constraint{
		"apt_install": {OS: "linux", LinuxFamily: "debian"},
	})

	when := &WhenClause{Arch: "amd64"}
	analysis, err := ComputeAnalysis("apt_install", when, nil, lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.Constraint == nil {
		t.Fatal("expected non-nil constraint")
	}
	if analysis.Constraint.OS != "linux" {
		t.Errorf("expected OS=linux, got %q", analysis.Constraint.OS)
	}
	if analysis.Constraint.LinuxFamily != "debian" {
		t.Errorf("expected LinuxFamily=debian, got %q", analysis.Constraint.LinuxFamily)
	}
	if analysis.Constraint.Arch != "amd64" {
		t.Errorf("expected Arch=amd64, got %q", analysis.Constraint.Arch)
	}
}

func TestComputeAnalysis_ImplicitWithConflictingFamily(t *testing.T) {
	// apt_install (debian) + when.linux_family: rhel -> error
	lookup := mockLookup(map[string]*Constraint{
		"apt_install": {OS: "linux", LinuxFamily: "debian"},
	})

	when := &WhenClause{LinuxFamily: "rhel"}
	_, err := ComputeAnalysis("apt_install", when, nil, lookup)
	if err == nil {
		t.Fatal("expected error for conflicting linux_family, got nil")
	}
	if !strings.Contains(err.Error(), "linux_family conflict") {
		t.Errorf("expected error about linux_family conflict, got: %v", err)
	}
}

func TestComputeAnalysis_ImplicitWithConflictingOS(t *testing.T) {
	// apt_install (linux) + when.os: darwin -> error
	lookup := mockLookup(map[string]*Constraint{
		"apt_install": {OS: "linux", LinuxFamily: "debian"},
	})

	when := &WhenClause{OS: []string{"darwin"}}
	_, err := ComputeAnalysis("apt_install", when, nil, lookup)
	if err == nil {
		t.Fatal("expected error for conflicting OS, got nil")
	}
	if !strings.Contains(err.Error(), "OS conflict") {
		t.Errorf("expected error about OS conflict, got: %v", err)
	}
}

func TestComputeAnalysis_LinuxFamilyInterpolation(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]interface{}
		want   bool
	}{
		{
			name:   "simple string with linux_family",
			params: map[string]interface{}{"url": "https://example.com/{{linux_family}}.tar.gz"},
			want:   true,
		},
		{
			name: "nested map with linux_family",
			params: map[string]interface{}{
				"config": map[string]interface{}{
					"path": "/opt/{{linux_family}}/bin",
				},
			},
			want: true,
		},
		{
			name: "array with linux_family",
			params: map[string]interface{}{
				"commands": []interface{}{
					"echo hello",
					"install-{{linux_family}}.sh",
				},
			},
			want: true,
		},
		{
			name:   "no interpolation",
			params: map[string]interface{}{"url": "https://example.com/file.tar.gz"},
			want:   false,
		},
		{
			name:   "empty params",
			params: map[string]interface{}{},
			want:   false,
		},
		{
			name:   "nil params",
			params: nil,
			want:   false,
		},
	}

	lookup := mockLookup(map[string]*Constraint{
		"download": nil,
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis, err := ComputeAnalysis("download", nil, tt.params, lookup)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if analysis.FamilyVarying != tt.want {
				t.Errorf("FamilyVarying = %v, want %v", analysis.FamilyVarying, tt.want)
			}
		})
	}
}

func TestComputeAnalysis_ConstrainedPlusVarying(t *testing.T) {
	// Edge case: step with BOTH family constraint AND {{linux_family}} interpolation
	// This is valid: step only runs on debian, but uses interpolation for that family
	lookup := mockLookup(map[string]*Constraint{
		"download": nil,
	})

	when := &WhenClause{LinuxFamily: "debian"}
	params := map[string]interface{}{
		"url": "https://example.com/{{linux_family}}-tool.tar.gz",
	}

	analysis, err := ComputeAnalysis("download", when, params, lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have constraint
	if analysis.Constraint == nil {
		t.Fatal("expected non-nil constraint")
	}
	if analysis.Constraint.LinuxFamily != "debian" {
		t.Errorf("expected LinuxFamily=debian, got %q", analysis.Constraint.LinuxFamily)
	}

	// Should also be family varying
	if !analysis.FamilyVarying {
		t.Error("expected FamilyVarying=true for constrained step with interpolation")
	}
}

func TestComputeAnalysis_OtherKnownVars(t *testing.T) {
	// Test detection of other known variables (os, arch) for future extensibility
	tests := []struct {
		name     string
		params   map[string]interface{}
		wantOS   bool
		wantArch bool
	}{
		{
			name:     "os interpolation",
			params:   map[string]interface{}{"url": "https://example.com/{{os}}/file.tar.gz"},
			wantOS:   true,
			wantArch: false,
		},
		{
			name:     "arch interpolation",
			params:   map[string]interface{}{"url": "https://example.com/{{arch}}/file.tar.gz"},
			wantOS:   false,
			wantArch: true,
		},
		{
			name:     "both os and arch",
			params:   map[string]interface{}{"url": "https://example.com/{{os}}-{{arch}}/file.tar.gz"},
			wantOS:   true,
			wantArch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify detectInterpolatedVars detects all known variables
			vars := detectInterpolatedVars(tt.params)
			if vars["os"] != tt.wantOS {
				t.Errorf("os detection = %v, want %v", vars["os"], tt.wantOS)
			}
			if vars["arch"] != tt.wantArch {
				t.Errorf("arch detection = %v, want %v", vars["arch"], tt.wantArch)
			}
		})
	}
}

func TestComputeAnalysis_WhenClauseExtends(t *testing.T) {
	// When clause can extend an unconstrained action
	lookup := mockLookup(map[string]*Constraint{
		"download": nil, // unconstrained
	})

	when := &WhenClause{
		OS:          []string{"linux"},
		LinuxFamily: "debian",
		Arch:        "amd64",
	}

	analysis, err := ComputeAnalysis("download", when, nil, lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.Constraint == nil {
		t.Fatal("expected non-nil constraint")
	}
	if analysis.Constraint.OS != "linux" {
		t.Errorf("expected OS=linux, got %q", analysis.Constraint.OS)
	}
	if analysis.Constraint.LinuxFamily != "debian" {
		t.Errorf("expected LinuxFamily=debian, got %q", analysis.Constraint.LinuxFamily)
	}
	if analysis.Constraint.Arch != "amd64" {
		t.Errorf("expected Arch=amd64, got %q", analysis.Constraint.Arch)
	}
}

func TestComputeAnalysis_EmptyWhenClause(t *testing.T) {
	// Empty when clause should not affect result
	lookup := mockLookup(map[string]*Constraint{
		"apt_install": {OS: "linux", LinuxFamily: "debian"},
	})

	when := &WhenClause{} // all fields empty

	analysis, err := ComputeAnalysis("apt_install", when, nil, lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still have implicit constraint
	if analysis.Constraint == nil {
		t.Fatal("expected non-nil constraint from implicit")
	}
	if analysis.Constraint.LinuxFamily != "debian" {
		t.Errorf("expected LinuxFamily=debian, got %q", analysis.Constraint.LinuxFamily)
	}
}

func TestComputeAnalysis_RedundantWhenClause(t *testing.T) {
	// Redundant when clause (matches implicit) should succeed
	lookup := mockLookup(map[string]*Constraint{
		"apt_install": {OS: "linux", LinuxFamily: "debian"},
	})

	when := &WhenClause{
		OS:          []string{"linux"},
		LinuxFamily: "debian",
	}

	analysis, err := ComputeAnalysis("apt_install", when, nil, lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if analysis.Constraint == nil {
		t.Fatal("expected non-nil constraint")
	}
	if analysis.Constraint.OS != "linux" {
		t.Errorf("expected OS=linux, got %q", analysis.Constraint.OS)
	}
	if analysis.Constraint.LinuxFamily != "debian" {
		t.Errorf("expected LinuxFamily=debian, got %q", analysis.Constraint.LinuxFamily)
	}
}

func TestComputeAnalysis_DarwinWithFamily(t *testing.T) {
	// darwin + linux_family is invalid
	lookup := mockLookup(map[string]*Constraint{
		"brew_install": {OS: "darwin"},
	})

	when := &WhenClause{LinuxFamily: "debian"}
	_, err := ComputeAnalysis("brew_install", when, nil, lookup)
	if err == nil {
		t.Fatal("expected error for darwin + linux_family, got nil")
	}
	// Should fail validation in MergeWhenClause
	if !strings.Contains(err.Error(), "linux_family") {
		t.Errorf("expected error about linux_family, got: %v", err)
	}
}
