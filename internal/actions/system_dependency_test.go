package actions

import (
	"errors"
	"testing"
)

func TestSystemDependency_Name(t *testing.T) {
	action := &SystemDependencyAction{}
	if action.Name() != "system_dependency" {
		t.Errorf("Name() = %q, want %q", action.Name(), "system_dependency")
	}
}

func TestSystemDependency_IsDeterministic(t *testing.T) {
	action := &SystemDependencyAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestSystemDependency_Preflight_Valid(t *testing.T) {
	action := &SystemDependencyAction{}
	params := map[string]interface{}{
		"name":     "zlib",
		"packages": map[string]interface{}{"alpine": "zlib-dev"},
	}

	result := action.Preflight(params)
	if result.HasErrors() {
		t.Errorf("Preflight() has errors: %v", result.Errors)
	}
}

func TestSystemDependency_Preflight_MissingName(t *testing.T) {
	action := &SystemDependencyAction{}
	params := map[string]interface{}{
		"packages": map[string]interface{}{"alpine": "zlib-dev"},
	}

	result := action.Preflight(params)
	if !result.HasErrors() {
		t.Error("Preflight() should have errors for missing name")
	}
}

func TestSystemDependency_Preflight_MissingPackages(t *testing.T) {
	action := &SystemDependencyAction{}
	params := map[string]interface{}{
		"name": "zlib",
	}

	result := action.Preflight(params)
	if !result.HasErrors() {
		t.Error("Preflight() should have errors for missing packages")
	}
}

func TestDependencyMissingError_Error(t *testing.T) {
	err := &DependencyMissingError{
		Library: "zlib",
		Package: "zlib-dev",
		Command: "sudo apk add zlib-dev",
		Family:  "alpine",
	}

	expected := "missing system dependency: zlib (install with: sudo apk add zlib-dev)"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestIsDependencyMissing(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "DependencyMissingError",
			err:  &DependencyMissingError{Library: "zlib"},
			want: true,
		},
		{
			name: "wrapped DependencyMissingError",
			err:  errors.New("wrapped: " + (&DependencyMissingError{Library: "zlib"}).Error()),
			want: false, // Simple string wrapping doesn't preserve type
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "other error",
			err:  errors.New("some other error"),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsDependencyMissing(tc.err); got != tc.want {
				t.Errorf("IsDependencyMissing() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAsDependencyMissing(t *testing.T) {
	depErr := &DependencyMissingError{
		Library: "zlib",
		Package: "zlib-dev",
		Command: "sudo apk add zlib-dev",
		Family:  "alpine",
	}

	tests := []struct {
		name    string
		err     error
		wantNil bool
	}{
		{
			name:    "DependencyMissingError",
			err:     depErr,
			wantNil: false,
		},
		{
			name:    "nil error",
			err:     nil,
			wantNil: true,
		},
		{
			name:    "other error",
			err:     errors.New("some other error"),
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := AsDependencyMissing(tc.err)
			if tc.wantNil && result != nil {
				t.Errorf("AsDependencyMissing() = %v, want nil", result)
			}
			if !tc.wantNil && result == nil {
				t.Error("AsDependencyMissing() = nil, want non-nil")
			}
			if !tc.wantNil && result != nil {
				if result.Library != depErr.Library {
					t.Errorf("AsDependencyMissing().Library = %q, want %q", result.Library, depErr.Library)
				}
			}
		})
	}
}

func TestParsePackagesMap(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    map[string]string
		wantErr bool
	}{
		{
			name: "map[string]interface{} with strings",
			input: map[string]interface{}{
				"alpine": "zlib-dev",
				"debian": "zlib1g-dev",
			},
			want:    map[string]string{"alpine": "zlib-dev", "debian": "zlib1g-dev"},
			wantErr: false,
		},
		{
			name: "map[string]string",
			input: map[string]string{
				"alpine": "zlib-dev",
			},
			want:    map[string]string{"alpine": "zlib-dev"},
			wantErr: false,
		},
		{
			name: "map with non-string value",
			input: map[string]interface{}{
				"alpine": 123,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty map",
			input:   map[string]interface{}{},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "wrong type",
			input:   "not a map",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePackagesMap(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("parsePackagesMap() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if !tc.wantErr {
				for k, v := range tc.want {
					if got[k] != v {
						t.Errorf("parsePackagesMap()[%q] = %q, want %q", k, got[k], v)
					}
				}
			}
		})
	}
}

func TestGetInstallCommand(t *testing.T) {
	tests := []struct {
		name   string
		pkg    string
		family string
		want   string // We check contains because root prefix varies
	}{
		{
			name:   "alpine",
			pkg:    "zlib-dev",
			family: "alpine",
			want:   "apk add zlib-dev",
		},
		{
			name:   "unknown family",
			pkg:    "zlib-dev",
			family: "unknown",
			want:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getInstallCommand(tc.pkg, tc.family)
			// For non-empty expected, check that it contains the core command
			if tc.want != "" {
				if len(got) < len(tc.want) {
					t.Errorf("getInstallCommand() = %q, want to contain %q", got, tc.want)
					return
				}
				// Command should end with the expected suffix (ignoring prefix)
				suffix := got[len(got)-len(tc.want):]
				if suffix != tc.want {
					t.Errorf("getInstallCommand() = %q, want to end with %q", got, tc.want)
				}
			} else {
				if got != tc.want {
					t.Errorf("getInstallCommand() = %q, want %q", got, tc.want)
				}
			}
		})
	}
}

func TestIsPackageInstalled_UnknownFamily(t *testing.T) {
	// Unknown family should return false
	if isPackageInstalled("some-pkg", "unknown") {
		t.Error("isPackageInstalled() for unknown family should return false")
	}
}

func TestSystemDependency_ActionRegistered(t *testing.T) {
	action := Get("system_dependency")
	if action == nil {
		t.Error("system_dependency action not registered")
	}
	if _, ok := action.(*SystemDependencyAction); !ok {
		t.Errorf("Get(\"system_dependency\") returned %T, want *SystemDependencyAction", action)
	}
}
