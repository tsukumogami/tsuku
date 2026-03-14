package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestIsValidPyPIVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		version  string
		expected bool
	}{
		// Valid versions
		{"simple version", "1.0.0", true},
		{"two digit version", "24.10.0", true},
		{"single number", "1", true},
		{"two numbers", "1.0", true},
		{"release candidate", "1.2.3rc1", true},
		{"alpha release", "2.0.0a1", true},
		{"beta release", "3.0.0b2", true},
		{"dev release", "1.0.0dev1", true},
		{"post release", "1.0.0post1", true},

		// Invalid versions - empty
		{"empty string", "", false},

		// Invalid versions - too long
		{"too long", string(make([]byte, 51)), false},

		// Invalid versions - invalid characters
		{"command injection semicolon", "1.0.0; rm -rf /", false},
		{"path traversal", "../etc/passwd", false},
		{"subshell injection", "$(evil)", false},
		{"backtick injection", "`evil`", false},
		{"uppercase letters", "1.0.0RC1", false},
		{"space in version", "1.0 0", false},
		{"pipe in version", "1.0.0|cat", false},
		{"ampersand in version", "1.0.0&cmd", false},
		{"double ampersand", "1.0.0&&cmd", false},

		// Invalid versions - wrong structure
		{"starts with letter", "v1.0.0", false},
		{"starts with dot", ".1.0.0", false},
		{"dot in release tag", "1.0.0rc.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For too long test, generate proper string
			version := tt.version
			if tt.name == "too long" {
				version = "1"
				for i := 0; i < 50; i++ {
					version += "0"
				}
			}

			result := isValidPyPIVersion(version)
			if result != tt.expected {
				t.Errorf("isValidPyPIVersion(%q) = %v, want %v", tt.version, result, tt.expected)
			}
		})
	}
}

func TestPipxInstallAction_Name(t *testing.T) {
	t.Parallel()
	action := &PipxInstallAction{}
	if action.Name() != "pipx_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "pipx_install")
	}
}

func TestPipxInstallAction_ImplementsDecomposable(t *testing.T) {
	t.Parallel()
	var _ Decomposable = (*PipxInstallAction)(nil)

	// Also verify via IsDecomposable
	if !IsDecomposable("pipx_install") {
		t.Error("pipx_install should implement Decomposable")
	}
}

func TestPipxInstallAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := &PipxInstallAction{}
	deps := action.Dependencies()

	// Should require python-standalone at install time
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "python-standalone" {
		t.Errorf("InstallTime = %v, want [python-standalone]", deps.InstallTime)
	}

	// Should require python-standalone at runtime
	if len(deps.Runtime) != 1 || deps.Runtime[0] != "python-standalone" {
		t.Errorf("Runtime = %v, want [python-standalone]", deps.Runtime)
	}

	// Should require python-standalone at eval time for Decompose
	if len(deps.EvalTime) != 1 || deps.EvalTime[0] != "python-standalone" {
		t.Errorf("EvalTime = %v, want [python-standalone]", deps.EvalTime)
	}
}

func TestPipxInstallAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := &PipxInstallAction{}

	// pipx_install needs network to download packages from PyPI
	nv, ok := interface{}(action).(NetworkValidator)
	if !ok {
		t.Fatal("pipx_install should implement NetworkValidator")
	}
	if !nv.RequiresNetwork() {
		t.Error("pipx_install should require network")
	}
}

func TestPipxInstallAction_Decompose_Validation(t *testing.T) {
	t.Parallel()
	action := &PipxInstallAction{}

	tests := []struct {
		name        string
		version     string
		params      map[string]interface{}
		errContains string
	}{
		{
			name:        "missing package",
			version:     "1.0.0",
			params:      map[string]interface{}{},
			errContains: "requires 'package' parameter",
		},
		{
			name:    "missing executables",
			version: "1.0.0",
			params: map[string]interface{}{
				"package": "ruff",
			},
			errContains: "requires 'executables' parameter",
		},
		{
			name:    "empty executables",
			version: "1.0.0",
			params: map[string]interface{}{
				"package":     "ruff",
				"executables": []string{},
			},
			errContains: "requires 'executables' parameter",
		},
		{
			name:    "missing version",
			version: "",
			params: map[string]interface{}{
				"package":     "ruff",
				"executables": []string{"ruff"},
			},
			errContains: "requires a resolved version",
		},
		{
			name:    "invalid version",
			version: "evil;inject",
			params: map[string]interface{}{
				"package":     "flask",
				"executables": []any{"flask"},
			},
			errContains: "version",
		},
		{
			name:    "executables key absent",
			version: "1.0.0",
			params: map[string]interface{}{
				"package": "flask",
			},
			errContains: "executables",
		},
		{
			name:    "empty version",
			version: "",
			params: map[string]interface{}{
				"package":     "flask",
				"executables": []any{"flask"},
			},
			errContains: "version",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := &EvalContext{
				Context:    context.Background(),
				Version:    tc.version,
				VersionTag: "v" + tc.version,
				OS:         "linux",
				Arch:       "amd64",
			}
			_, err := action.Decompose(ctx, tc.params)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("error = %q, want containing %q", err.Error(), tc.errContains)
			}
		})
	}
}

func TestPipxInstallAction_Decompose_InvalidPackage(t *testing.T) {
	t.Parallel()
	action := &PipxInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "1.0.0",
	}

	tests := []struct {
		name     string
		package_ string
	}{
		{"command injection", "ruff; rm -rf /"},
		{"subshell", "$(evil)"},
		{"pipe", "ruff|cat"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]interface{}{
				"package":     tc.package_,
				"executables": []string{"ruff"},
			}

			_, err := action.Decompose(ctx, params)
			if err == nil {
				t.Fatal("expected error for invalid package name")
			}
		})
	}
}

func TestIsValidPyPIPackage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		package_ string
		expected bool
	}{
		// Valid package names
		{"simple", "ruff", true},
		{"with hyphen", "black-macchiato", true},
		{"with underscore", "typing_extensions", true},
		{"with dot", "zope.interface", true},
		{"with numbers", "python3-openid", true},
		{"numeric start after first", "oauth2client", true},

		// Invalid package names
		{"empty", "", false},
		{"too long", string(make([]byte, 201)), false},
		{"command injection", "ruff; rm -rf /", false},
		{"subshell", "$(evil)", false},
		{"pipe", "ruff|cat", false},
		{"ampersand", "ruff&cmd", false},
		{"backtick", "`evil`", false},
		{"newline", "ruff\nevil", false},
		{"space", "ruff evil", false},
		{"starts with hyphen", "-ruff", false},
		{"starts with dot", ".ruff", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// For too long test, generate proper string
			pkg := tc.package_
			if tc.name == "too long" {
				pkg = "a"
				for i := 0; i < 200; i++ {
					pkg += "a"
				}
			}

			result := isValidPyPIPackage(pkg)
			if result != tc.expected {
				t.Errorf("isValidPyPIPackage(%q) = %v, want %v", tc.package_, result, tc.expected)
			}
		})
	}
}

func TestParseWheelFilename(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		filename string
		want     *wheelInfo
	}{
		{
			name:     "simple wheel",
			filename: "ruff-0.1.0-py3-none-any.whl",
			want: &wheelInfo{
				name:     "ruff",
				version:  "0.1.0",
				python:   "py3",
				abi:      "none",
				platform: "any",
			},
		},
		{
			name:     "wheel with underscore in name",
			filename: "typing_extensions-4.8.0-py3-none-any.whl",
			want: &wheelInfo{
				name:     "typing-extensions", // normalized
				version:  "4.8.0",
				python:   "py3",
				abi:      "none",
				platform: "any",
			},
		},
		{
			name:     "platform-specific wheel",
			filename: "numpy-1.26.0-cp311-cp311-manylinux_2_17_x86_64.whl",
			want: &wheelInfo{
				name:     "numpy",
				version:  "1.26.0",
				python:   "cp311",
				abi:      "cp311",
				platform: "manylinux_2_17_x86_64",
			},
		},
		{
			name:     "not a wheel",
			filename: "package-1.0.0.tar.gz",
			want:     nil,
		},
		{
			name:     "too few parts",
			filename: "invalid.whl",
			want:     nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseWheelFilename(tc.filename)
			if tc.want == nil {
				if got != nil {
					t.Errorf("parseWheelFilename(%q) = %+v, want nil", tc.filename, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseWheelFilename(%q) = nil, want %+v", tc.filename, tc.want)
			}
			if got.name != tc.want.name || got.version != tc.want.version ||
				got.python != tc.want.python || got.abi != tc.want.abi ||
				got.platform != tc.want.platform {
				t.Errorf("parseWheelFilename(%q) = %+v, want %+v", tc.filename, got, tc.want)
			}
		})
	}
}

func TestDetectPythonNativeAddons(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		requirements string
		want         bool
	}{
		{
			name:         "pure python",
			requirements: "click==8.1.0\ntyping-extensions==4.8.0",
			want:         false,
		},
		{
			name:         "manylinux wheel",
			requirements: "numpy==1.26.0 # manylinux_2_17_x86_64",
			want:         true,
		},
		{
			name:         "macosx wheel",
			requirements: "pillow==10.0.0 # macosx_10_10_x86_64",
			want:         true,
		},
		{
			name:         "win_amd64 wheel",
			requirements: "package==1.0.0 # win_amd64",
			want:         true,
		},
		{
			name:         "known native package",
			requirements: "numpy==1.26.0\nclick==8.1.0",
			want:         true,
		},
		{
			name:         "scipy",
			requirements: "scipy==1.11.0",
			want:         true,
		},
		{
			name:         "cryptography",
			requirements: "cryptography==41.0.0",
			want:         true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectPythonNativeAddons(tc.requirements)
			if got != tc.want {
				t.Errorf("detectPythonNativeAddons() = %v, want %v", got, tc.want)
			}
		})
	}
}

// -- pipx_install.go: Execute early validation --

func TestPipxInstallAction_Execute_Validation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		version     string
		params      map[string]any
		errContains string
	}{
		{
			name:        "missing package",
			version:     "1.0.0",
			params:      map[string]any{},
			errContains: "package",
		},
		{
			name:        "invalid version",
			version:     "evil;inject",
			params:      map[string]any{"package": "flask"},
			errContains: "version",
		},
		{
			name:        "missing executables",
			version:     "1.0.0",
			params:      map[string]any{"package": "flask"},
			errContains: "executables",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			action := &PipxInstallAction{}
			ctx := &ExecutionContext{
				Context: context.Background(),
				WorkDir: t.TempDir(),
				Version: tt.version,
				OS:      "linux",
				Arch:    "amd64",
				Recipe:  &recipe.Recipe{},
			}
			err := action.Execute(ctx, tt.params)
			if err == nil || !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Expected error containing %q, got %v", tt.errContains, err)
			}
		})
	}
}

// -- go_build.go: Execute early validation --

// -- pipx_install.go: isValidPyPIPackage additional cases --

func TestIsValidPyPIPackage_WithSpaces(t *testing.T) {
	t.Parallel()
	if isValidPyPIPackage("evil package") {
		t.Error("Expected false for package with spaces")
	}
}

func TestIsValidPyPIPackage_WithNewline(t *testing.T) {
	t.Parallel()
	if isValidPyPIPackage("evil\npackage") {
		t.Error("Expected false for package with newline")
	}
}

// -- pipx_install.go: computeFileSHA256 (0% covered, pure function) --

func TestComputeFileSHA256_Success(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "testfile")
	if err := os.WriteFile(path, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}
	hash, err := computeFileSHA256(path)
	if err != nil {
		t.Fatalf("computeFileSHA256() error = %v", err)
	}
	// SHA256 of "hello world\n"
	expected := "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
	if hash != expected {
		t.Errorf("computeFileSHA256() = %q, want %q", hash, expected)
	}
}

func TestComputeFileSHA256_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := computeFileSHA256("/nonexistent/file")
	if err == nil {
		t.Error("Expected error for missing file")
	}
}
