package actions

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestDependencyMissingError_Error(t *testing.T) {
	err := &DependencyMissingError{
		Packages: []string{"zlib-dev", "openssl-dev"},
		Command:  "sudo apk add zlib-dev openssl-dev",
		Family:   "alpine",
	}

	got := err.Error()
	if !strings.Contains(got, "zlib-dev") {
		t.Errorf("Error() should contain package name, got: %s", got)
	}
	if !strings.Contains(got, "sudo apk add") {
		t.Errorf("Error() should contain install command, got: %s", got)
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
			err:  &DependencyMissingError{Packages: []string{"pkg"}},
			want: true,
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
		Packages: []string{"zlib-dev"},
		Command:  "sudo apk add zlib-dev",
		Family:   "alpine",
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
		})
	}
}

func TestIsPackageInstalled_UnknownFamily(t *testing.T) {
	// Unknown family should return false
	if isPackageInstalled("some-pkg", "unknown") {
		t.Error("isPackageInstalled() for unknown family should return false")
	}
}

func TestCheckMissingPackages_UnknownFamily(t *testing.T) {
	// Unknown family - all packages should be reported as missing
	missing := checkMissingPackages([]string{"pkg1", "pkg2"}, "unknown")
	if len(missing) != 2 {
		t.Errorf("checkMissingPackages() for unknown family should return all packages, got %d", len(missing))
	}
}

func TestGetRootPrefix(t *testing.T) {
	// This test is tricky because it depends on the environment.
	// We can at least verify it returns something sensible.
	prefix := getRootPrefix()

	if os.Getuid() == 0 {
		if prefix != "" {
			t.Errorf("getRootPrefix() when root should return empty, got %q", prefix)
		}
	} else {
		// Should be either "sudo " or "doas "
		if prefix != "sudo " && prefix != "doas " {
			t.Errorf("getRootPrefix() when not root should return 'sudo ' or 'doas ', got %q", prefix)
		}
	}
}

func TestBuildInstallCommand(t *testing.T) {
	tests := []struct {
		name     string
		baseCmd  string
		packages []string
		contains string
	}{
		{
			name:     "single package",
			baseCmd:  "apk add",
			packages: []string{"zlib-dev"},
			contains: "apk add zlib-dev",
		},
		{
			name:     "multiple packages",
			baseCmd:  "apt-get install -y",
			packages: []string{"pkg1", "pkg2"},
			contains: "apt-get install -y pkg1 pkg2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildInstallCommand(tc.baseCmd, tc.packages)
			if !strings.Contains(got, tc.contains) {
				t.Errorf("buildInstallCommand() = %q, want to contain %q", got, tc.contains)
			}
		})
	}
}
