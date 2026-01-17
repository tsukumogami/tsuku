package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/actions"
)

func TestExtractConstraints_PipExec(t *testing.T) {
	// Create a test plan with pip_exec step containing locked_requirements
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "black",
		Version:       "26.1a1",
		Steps: []ResolvedStep{
			{
				Action: "pip_exec",
				Params: map[string]interface{}{
					"package": "black",
					"version": "26.1a1",
					"locked_requirements": `black==26.1a1 \
    --hash=sha256:6bef30dd59ee2f3cead8676fb20b02eb61e2a62242e1687bb487d83b4f2c4f5d
click==8.3.1 \
    --hash=sha256:981153a64e25f12d547d3426c367a4857371575ee7ad18df2a6183ab0545b2a6
mypy-extensions==1.1.0 \
    --hash=sha256:1be4cccdb0f2482337c4743e60421de3a356cd97508abadd57d47403e94f5505
`,
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	// Verify pip constraints were extracted
	if len(constraints.PipConstraints) != 3 {
		t.Errorf("Expected 3 pip constraints, got %d", len(constraints.PipConstraints))
	}

	// Check specific packages (normalized to lowercase with hyphens)
	expected := map[string]string{
		"black":           "26.1a1",
		"click":           "8.3.1",
		"mypy-extensions": "1.1.0",
	}

	for pkg, ver := range expected {
		got, ok := constraints.PipConstraints[pkg]
		if !ok {
			t.Errorf("Missing constraint for package %q", pkg)
			continue
		}
		if got != ver {
			t.Errorf("Package %q: expected version %q, got %q", pkg, ver, got)
		}
	}
}

func TestExtractConstraints_EmptyPlan(t *testing.T) {
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "kubectl",
		Version:       "1.29.0",
		Steps: []ResolvedStep{
			{
				Action: "download_file",
				Params: map[string]interface{}{
					"url":  "https://example.com/kubectl",
					"dest": "kubectl",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	// Should have empty pip constraints (initialized but empty map)
	if len(constraints.PipConstraints) != 0 {
		t.Errorf("Expected 0 pip constraints for non-pip plan, got %d", len(constraints.PipConstraints))
	}
}

func TestExtractConstraints_FromFile(t *testing.T) {
	// Create a temporary plan file
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "httpie",
		Version:       "3.2.4",
		Steps: []ResolvedStep{
			{
				Action: "pip_exec",
				Params: map[string]interface{}{
					"package": "httpie",
					"version": "3.2.4",
					"locked_requirements": `httpie==3.2.4 \
    --hash=sha256:abc123
requests==2.31.0 \
    --hash=sha256:def456
`,
				},
			},
		},
	}

	tempDir := t.TempDir()
	planPath := filepath.Join(tempDir, "golden.json")

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("Failed to marshal plan: %v", err)
	}
	if err := os.WriteFile(planPath, data, 0644); err != nil {
		t.Fatalf("Failed to write plan file: %v", err)
	}

	constraints, err := ExtractConstraints(planPath)
	if err != nil {
		t.Fatalf("ExtractConstraints failed: %v", err)
	}

	if len(constraints.PipConstraints) != 2 {
		t.Errorf("Expected 2 pip constraints, got %d", len(constraints.PipConstraints))
	}

	if ver, ok := constraints.PipConstraints["httpie"]; !ok || ver != "3.2.4" {
		t.Errorf("Expected httpie==3.2.4, got %q", ver)
	}
	if ver, ok := constraints.PipConstraints["requests"]; !ok || ver != "2.31.0" {
		t.Errorf("Expected requests==2.31.0, got %q", ver)
	}
}

func TestExtractConstraints_InvalidFile(t *testing.T) {
	_, err := ExtractConstraints("/nonexistent/path/to/plan.json")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

func TestExtractConstraints_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	invalidPath := filepath.Join(tempDir, "invalid.json")

	if err := os.WriteFile(invalidPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("Failed to write invalid file: %v", err)
	}

	_, err := ExtractConstraints(invalidPath)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestExtractConstraints_FromDependencies(t *testing.T) {
	// Test that constraints are extracted from dependency plans as well
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "myapp",
		Version:       "1.0.0",
		Dependencies: []DependencyPlan{
			{
				Tool:    "python-app",
				Version: "2.0.0",
				Steps: []ResolvedStep{
					{
						Action: "pip_exec",
						Params: map[string]interface{}{
							"package": "dependency-pkg",
							"version": "1.5.0",
							"locked_requirements": `dependency-pkg==1.5.0 \
    --hash=sha256:xyz789
`,
						},
					},
				},
			},
		},
		Steps: []ResolvedStep{
			{
				Action: "download_file",
				Params: map[string]interface{}{
					"url": "https://example.com/myapp",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	// Should have constraint from dependency
	if ver, ok := constraints.PipConstraints["dependency-pkg"]; !ok || ver != "1.5.0" {
		t.Errorf("Expected dependency-pkg==1.5.0 from dependencies, got %q", ver)
	}
}

func TestParsePipRequirements(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: map[string]string{},
		},
		{
			name: "simple package",
			input: `flask==2.3.0 \
    --hash=sha256:abc123
`,
			expected: map[string]string{
				"flask": "2.3.0",
			},
		},
		{
			name: "multiple packages",
			input: `flask==2.3.0 \
    --hash=sha256:abc123
werkzeug==2.3.0 \
    --hash=sha256:def456
jinja2==3.1.2 \
    --hash=sha256:ghi789
`,
			expected: map[string]string{
				"flask":    "2.3.0",
				"werkzeug": "2.3.0",
				"jinja2":   "3.1.2",
			},
		},
		{
			name: "package with underscore normalized to hyphen",
			input: `typing_extensions==4.15.0 \
    --hash=sha256:xyz
`,
			expected: map[string]string{
				"typing-extensions": "4.15.0",
			},
		},
		{
			name: "prerelease version",
			input: `black==26.1a1 \
    --hash=sha256:xyz
`,
			expected: map[string]string{
				"black": "26.1a1",
			},
		},
		{
			name: "package with dots in name",
			input: `zope.interface==6.0 \
    --hash=sha256:xyz
`,
			expected: map[string]string{
				"zope-interface": "6.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParsePipRequirements(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d packages, got %d", len(tt.expected), len(result))
			}
			for pkg, ver := range tt.expected {
				got, ok := result[pkg]
				if !ok {
					t.Errorf("Missing package %q", pkg)
					continue
				}
				if got != ver {
					t.Errorf("Package %q: expected version %q, got %q", pkg, ver, got)
				}
			}
		})
	}
}

func TestHasPipConstraints(t *testing.T) {
	tests := []struct {
		name        string
		constraints *actions.EvalConstraints
		expected    bool
	}{
		{
			name:        "nil constraints",
			constraints: nil,
			expected:    false,
		},
		{
			name: "empty constraints",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{},
			},
			expected: false,
		},
		{
			name: "with pip constraints",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{
					"flask": "2.3.0",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasPipConstraints(tt.constraints)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetPipConstraint(t *testing.T) {
	constraints := &actions.EvalConstraints{
		PipConstraints: map[string]string{
			"flask":             "2.3.0",
			"typing-extensions": "4.15.0",
		},
	}

	tests := []struct {
		name        string
		packageName string
		constraints *actions.EvalConstraints
		wantVersion string
		wantOK      bool
	}{
		{
			name:        "existing package",
			packageName: "flask",
			constraints: constraints,
			wantVersion: "2.3.0",
			wantOK:      true,
		},
		{
			name:        "package with underscore",
			packageName: "typing_extensions",
			constraints: constraints,
			wantVersion: "4.15.0",
			wantOK:      true,
		},
		{
			name:        "package with uppercase",
			packageName: "FLASK",
			constraints: constraints,
			wantVersion: "2.3.0",
			wantOK:      true,
		},
		{
			name:        "non-existent package",
			packageName: "nonexistent",
			constraints: constraints,
			wantVersion: "",
			wantOK:      false,
		},
		{
			name:        "nil constraints",
			packageName: "flask",
			constraints: nil,
			wantVersion: "",
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVersion, gotOK := GetPipConstraint(tt.constraints, tt.packageName)
			if gotOK != tt.wantOK {
				t.Errorf("GetPipConstraint() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("GetPipConstraint() version = %q, want %q", gotVersion, tt.wantVersion)
			}
		})
	}
}

func TestExtractConstraints_GoBuild(t *testing.T) {
	goSum := `github.com/go-delve/delve v1.9.0 h1:abc123
github.com/go-delve/delve v1.9.0/go.mod h1:def456
golang.org/x/sys v0.0.0-20220722155257-8c9f86f7a55f h1:xyz789
`
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "dlv",
		Version:       "1.9.0",
		Steps: []ResolvedStep{
			{
				Action: "go_build",
				Params: map[string]interface{}{
					"module":         "github.com/go-delve/delve/cmd/dlv",
					"install_module": "github.com/go-delve/delve/cmd/dlv",
					"version":        "v1.9.0",
					"executables":    []interface{}{"dlv"},
					"go_sum":         goSum,
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.GoSum != goSum {
		t.Errorf("Expected GoSum to be extracted, got %q", constraints.GoSum)
	}
}

func TestExtractConstraints_GoBuildInDependency(t *testing.T) {
	goSum := `github.com/example/tool v1.0.0 h1:abc123
`
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "myapp",
		Version:       "1.0.0",
		Dependencies: []DependencyPlan{
			{
				Tool:    "go-tool",
				Version: "1.0.0",
				Steps: []ResolvedStep{
					{
						Action: "go_build",
						Params: map[string]interface{}{
							"module":  "github.com/example/tool",
							"version": "v1.0.0",
							"go_sum":  goSum,
						},
					},
				},
			},
		},
		Steps: []ResolvedStep{
			{
				Action: "download_file",
				Params: map[string]interface{}{
					"url": "https://example.com/myapp",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.GoSum != goSum {
		t.Errorf("Expected GoSum from dependency to be extracted, got %q", constraints.GoSum)
	}
}

func TestExtractConstraints_GoBuildFirstWins(t *testing.T) {
	goSum1 := `first go.sum content`
	goSum2 := `second go.sum content`

	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "multi-tool",
		Version:       "1.0.0",
		Steps: []ResolvedStep{
			{
				Action: "go_build",
				Params: map[string]interface{}{
					"module":  "github.com/example/first",
					"version": "v1.0.0",
					"go_sum":  goSum1,
				},
			},
			{
				Action: "go_build",
				Params: map[string]interface{}{
					"module":  "github.com/example/second",
					"version": "v2.0.0",
					"go_sum":  goSum2,
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.GoSum != goSum1 {
		t.Errorf("Expected first GoSum to win, got %q", constraints.GoSum)
	}
}

func TestExtractConstraints_GoBuildEmptyGoSum(t *testing.T) {
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "tool",
		Version:       "1.0.0",
		Steps: []ResolvedStep{
			{
				Action: "go_build",
				Params: map[string]interface{}{
					"module":  "github.com/example/tool",
					"version": "v1.0.0",
					"go_sum":  "",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.GoSum != "" {
		t.Errorf("Expected empty GoSum for empty go_sum param, got %q", constraints.GoSum)
	}
}

func TestHasGoSumConstraint(t *testing.T) {
	tests := []struct {
		name        string
		constraints *actions.EvalConstraints
		expected    bool
	}{
		{
			name:        "nil constraints",
			constraints: nil,
			expected:    false,
		},
		{
			name: "empty constraints",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{},
				GoSum:          "",
			},
			expected: false,
		},
		{
			name: "with go sum",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{},
				GoSum:          "github.com/example/pkg v1.0.0 h1:abc123\n",
			},
			expected: true,
		},
		{
			name: "with pip but no go",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{
					"flask": "2.3.0",
				},
				GoSum: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasGoSumConstraint(tt.constraints)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExtractConstraints_GoVersion(t *testing.T) {
	goSum := "github.com/example/tool v1.0.0 h1:abc123\n"
	goVersion := "1.25.5"
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "tool",
		Version:       "1.0.0",
		Steps: []ResolvedStep{
			{
				Action: "go_build",
				Params: map[string]interface{}{
					"module":     "github.com/example/tool",
					"version":    "v1.0.0",
					"go_sum":     goSum,
					"go_version": goVersion,
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.GoVersion != goVersion {
		t.Errorf("Expected GoVersion %q, got %q", goVersion, constraints.GoVersion)
	}
	if constraints.GoSum != goSum {
		t.Errorf("Expected GoSum %q, got %q", goSum, constraints.GoSum)
	}
}

func TestExtractConstraints_GoVersionFirstWins(t *testing.T) {
	goVersion1 := "1.25.5"
	goVersion2 := "1.25.6"
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "tool",
		Version:       "1.0.0",
		Steps: []ResolvedStep{
			{
				Action: "go_build",
				Params: map[string]interface{}{
					"module":     "github.com/example/tool",
					"version":    "v1.0.0",
					"go_sum":     "sum1\n",
					"go_version": goVersion1,
				},
			},
			{
				Action: "go_build",
				Params: map[string]interface{}{
					"module":     "github.com/example/tool2",
					"version":    "v2.0.0",
					"go_sum":     "sum2\n",
					"go_version": goVersion2,
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.GoVersion != goVersion1 {
		t.Errorf("Expected first GoVersion to win (%q), got %q", goVersion1, constraints.GoVersion)
	}
}

func TestHasGoVersionConstraint(t *testing.T) {
	tests := []struct {
		name        string
		constraints *actions.EvalConstraints
		expected    bool
	}{
		{
			name:        "nil constraints",
			constraints: nil,
			expected:    false,
		},
		{
			name: "empty constraints",
			constraints: &actions.EvalConstraints{
				GoVersion: "",
			},
			expected: false,
		},
		{
			name: "with go version",
			constraints: &actions.EvalConstraints{
				GoVersion: "1.25.5",
			},
			expected: true,
		},
		{
			name: "with go sum but no version",
			constraints: &actions.EvalConstraints{
				GoSum:     "github.com/example/pkg v1.0.0 h1:abc123\n",
				GoVersion: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasGoVersionConstraint(tt.constraints)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetGoVersionConstraint(t *testing.T) {
	tests := []struct {
		name        string
		constraints *actions.EvalConstraints
		wantVersion string
		wantOK      bool
	}{
		{
			name:        "nil constraints",
			constraints: nil,
			wantVersion: "",
			wantOK:      false,
		},
		{
			name: "empty go version",
			constraints: &actions.EvalConstraints{
				GoVersion: "",
			},
			wantVersion: "",
			wantOK:      false,
		},
		{
			name: "with go version",
			constraints: &actions.EvalConstraints{
				GoVersion: "1.25.5",
			},
			wantVersion: "1.25.5",
			wantOK:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, ok := GetGoVersionConstraint(tt.constraints)
			if version != tt.wantVersion || ok != tt.wantOK {
				t.Errorf("GetGoVersionConstraint() = (%q, %v), want (%q, %v)",
					version, ok, tt.wantVersion, tt.wantOK)
			}
		})
	}
}

func TestExtractConstraints_CargoBuild(t *testing.T) {
	lockData := `# This file is automatically @generated by Cargo.
[[package]]
name = "ripgrep"
version = "14.0.0"
source = "registry+https://github.com/rust-lang/crates.io-index"

[[package]]
name = "regex"
version = "1.10.0"
source = "registry+https://github.com/rust-lang/crates.io-index"
`
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "ripgrep",
		Version:       "14.0.0",
		Steps: []ResolvedStep{
			{
				Action: "cargo_build",
				Params: map[string]interface{}{
					"crate":       "ripgrep",
					"version":     "14.0.0",
					"executables": []interface{}{"rg"},
					"lock_data":   lockData,
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.CargoLock != lockData {
		t.Errorf("Expected CargoLock to be extracted, got %q", constraints.CargoLock)
	}
}

func TestExtractConstraints_CargoBuildInDependency(t *testing.T) {
	lockData := `[[package]]
name = "dep-crate"
version = "1.0.0"
`
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "myapp",
		Version:       "1.0.0",
		Dependencies: []DependencyPlan{
			{
				Tool:    "rust-tool",
				Version: "1.0.0",
				Steps: []ResolvedStep{
					{
						Action: "cargo_build",
						Params: map[string]interface{}{
							"crate":     "rust-tool",
							"version":   "1.0.0",
							"lock_data": lockData,
						},
					},
				},
			},
		},
		Steps: []ResolvedStep{
			{
				Action: "download_file",
				Params: map[string]interface{}{
					"url": "https://example.com/myapp",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.CargoLock != lockData {
		t.Errorf("Expected CargoLock from dependency to be extracted, got %q", constraints.CargoLock)
	}
}

func TestExtractConstraints_CargoBuildFirstWins(t *testing.T) {
	lockData1 := `first Cargo.lock content`
	lockData2 := `second Cargo.lock content`

	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "multi-tool",
		Version:       "1.0.0",
		Steps: []ResolvedStep{
			{
				Action: "cargo_build",
				Params: map[string]interface{}{
					"crate":     "first-crate",
					"version":   "1.0.0",
					"lock_data": lockData1,
				},
			},
			{
				Action: "cargo_build",
				Params: map[string]interface{}{
					"crate":     "second-crate",
					"version":   "2.0.0",
					"lock_data": lockData2,
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.CargoLock != lockData1 {
		t.Errorf("Expected first CargoLock to win, got %q", constraints.CargoLock)
	}
}

func TestExtractConstraints_CargoBuildEmptyLockData(t *testing.T) {
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "tool",
		Version:       "1.0.0",
		Steps: []ResolvedStep{
			{
				Action: "cargo_build",
				Params: map[string]interface{}{
					"crate":     "some-crate",
					"version":   "1.0.0",
					"lock_data": "",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.CargoLock != "" {
		t.Errorf("Expected empty CargoLock for empty lock_data param, got %q", constraints.CargoLock)
	}
}

func TestHasCargoLockConstraint(t *testing.T) {
	tests := []struct {
		name        string
		constraints *actions.EvalConstraints
		expected    bool
	}{
		{
			name:        "nil constraints",
			constraints: nil,
			expected:    false,
		},
		{
			name: "empty constraints",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{},
				CargoLock:      "",
			},
			expected: false,
		},
		{
			name: "with cargo lock",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{},
				CargoLock:      "[[package]]\nname = \"ripgrep\"\n",
			},
			expected: true,
		},
		{
			name: "with pip but no cargo",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{
					"flask": "2.3.0",
				},
				CargoLock: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasCargoLockConstraint(tt.constraints)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExtractConstraints_NpmExec(t *testing.T) {
	packageLock := `{
  "name": "tsuku-npm-eval",
  "version": "0.0.0",
  "lockfileVersion": 3,
  "packages": {
    "node_modules/wrangler": {
      "version": "4.58.0"
    }
  }
}`
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "wrangler",
		Version:       "4.58.0",
		Steps: []ResolvedStep{
			{
				Action: "npm_exec",
				Params: map[string]interface{}{
					"package":      "wrangler",
					"version":      "4.58.0",
					"executables":  []interface{}{"wrangler"},
					"package_lock": packageLock,
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.NpmLock != packageLock {
		t.Errorf("Expected NpmLock to be extracted, got %q", constraints.NpmLock)
	}
}

func TestExtractConstraints_NpmExecInDependency(t *testing.T) {
	packageLock := `{"name": "dep-package", "lockfileVersion": 3}`
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "myapp",
		Version:       "1.0.0",
		Dependencies: []DependencyPlan{
			{
				Tool:    "npm-tool",
				Version: "1.0.0",
				Steps: []ResolvedStep{
					{
						Action: "npm_exec",
						Params: map[string]interface{}{
							"package":      "npm-tool",
							"version":      "1.0.0",
							"package_lock": packageLock,
						},
					},
				},
			},
		},
		Steps: []ResolvedStep{
			{
				Action: "download_file",
				Params: map[string]interface{}{
					"url": "https://example.com/myapp",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.NpmLock != packageLock {
		t.Errorf("Expected NpmLock from dependency to be extracted, got %q", constraints.NpmLock)
	}
}

func TestExtractConstraints_NpmExecFirstWins(t *testing.T) {
	packageLock1 := `first package-lock.json content`
	packageLock2 := `second package-lock.json content`

	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "multi-tool",
		Version:       "1.0.0",
		Steps: []ResolvedStep{
			{
				Action: "npm_exec",
				Params: map[string]interface{}{
					"package":      "first-package",
					"version":      "1.0.0",
					"package_lock": packageLock1,
				},
			},
			{
				Action: "npm_exec",
				Params: map[string]interface{}{
					"package":      "second-package",
					"version":      "2.0.0",
					"package_lock": packageLock2,
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.NpmLock != packageLock1 {
		t.Errorf("Expected first NpmLock to win, got %q", constraints.NpmLock)
	}
}

func TestExtractConstraints_NpmExecEmptyPackageLock(t *testing.T) {
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "tool",
		Version:       "1.0.0",
		Steps: []ResolvedStep{
			{
				Action: "npm_exec",
				Params: map[string]interface{}{
					"package":      "some-package",
					"version":      "1.0.0",
					"package_lock": "",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.NpmLock != "" {
		t.Errorf("Expected empty NpmLock for empty package_lock param, got %q", constraints.NpmLock)
	}
}

func TestHasNpmLockConstraint(t *testing.T) {
	tests := []struct {
		name        string
		constraints *actions.EvalConstraints
		expected    bool
	}{
		{
			name:        "nil constraints",
			constraints: nil,
			expected:    false,
		},
		{
			name: "empty constraints",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{},
				NpmLock:        "",
			},
			expected: false,
		},
		{
			name: "with npm lock",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{},
				NpmLock:        `{"name": "test", "lockfileVersion": 3}`,
			},
			expected: true,
		},
		{
			name: "with pip but no npm",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{
					"flask": "2.3.0",
				},
				NpmLock: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasNpmLockConstraint(tt.constraints)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExtractConstraints_GemExec(t *testing.T) {
	lockData := `GEM
  remote: https://rubygems.org/
  specs:
    jekyll (4.4.1)
      addressable (~> 2.4)
      colorator (~> 1.0)

PLATFORMS
  ruby

DEPENDENCIES
  jekyll (= 4.4.1)
`
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "jekyll",
		Version:       "4.4.1",
		Steps: []ResolvedStep{
			{
				Action: "gem_exec",
				Params: map[string]interface{}{
					"gem":         "jekyll",
					"version":     "4.4.1",
					"executables": []interface{}{"jekyll"},
					"lock_data":   lockData,
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.GemLock != lockData {
		t.Errorf("Expected GemLock to be extracted, got %q", constraints.GemLock)
	}
}

func TestExtractConstraints_GemExecInDependency(t *testing.T) {
	lockData := `GEM
  specs:
    dep-gem (1.0.0)
`
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "myapp",
		Version:       "1.0.0",
		Dependencies: []DependencyPlan{
			{
				Tool:    "ruby-tool",
				Version: "1.0.0",
				Steps: []ResolvedStep{
					{
						Action: "gem_exec",
						Params: map[string]interface{}{
							"gem":       "ruby-tool",
							"version":   "1.0.0",
							"lock_data": lockData,
						},
					},
				},
			},
		},
		Steps: []ResolvedStep{
			{
				Action: "download_file",
				Params: map[string]interface{}{
					"url": "https://example.com/myapp",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.GemLock != lockData {
		t.Errorf("Expected GemLock from dependency to be extracted, got %q", constraints.GemLock)
	}
}

func TestExtractConstraints_GemExecFirstWins(t *testing.T) {
	lockData1 := `first Gemfile.lock content`
	lockData2 := `second Gemfile.lock content`

	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "multi-tool",
		Version:       "1.0.0",
		Steps: []ResolvedStep{
			{
				Action: "gem_exec",
				Params: map[string]interface{}{
					"gem":       "first-gem",
					"version":   "1.0.0",
					"lock_data": lockData1,
				},
			},
			{
				Action: "gem_exec",
				Params: map[string]interface{}{
					"gem":       "second-gem",
					"version":   "2.0.0",
					"lock_data": lockData2,
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.GemLock != lockData1 {
		t.Errorf("Expected first GemLock to win, got %q", constraints.GemLock)
	}
}

func TestExtractConstraints_GemExecEmptyLockData(t *testing.T) {
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "tool",
		Version:       "1.0.0",
		Steps: []ResolvedStep{
			{
				Action: "gem_exec",
				Params: map[string]interface{}{
					"gem":       "some-gem",
					"version":   "1.0.0",
					"lock_data": "",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.GemLock != "" {
		t.Errorf("Expected empty GemLock for empty lock_data param, got %q", constraints.GemLock)
	}
}

func TestHasGemLockConstraint(t *testing.T) {
	tests := []struct {
		name        string
		constraints *actions.EvalConstraints
		expected    bool
	}{
		{
			name:        "nil constraints",
			constraints: nil,
			expected:    false,
		},
		{
			name: "empty constraints",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{},
				GemLock:        "",
			},
			expected: false,
		},
		{
			name: "with gem lock",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{},
				GemLock:        "GEM\n  specs:\n    jekyll (4.4.1)\n",
			},
			expected: true,
		},
		{
			name: "with pip but no gem",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{
					"flask": "2.3.0",
				},
				GemLock: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasGemLockConstraint(tt.constraints)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExtractConstraints_CpanInstall(t *testing.T) {
	snapshot := `# carton snapshot format: version 1.0
DISTRIBUTIONS
  Carton-1.0.35
    pathname: M/MI/MIYAGAWA/Carton-1.0.35.tar.gz
    provides:
      Carton 1.0.35
    requirements:
      perl 5.008001
`
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "carton",
		Version:       "1.0.35",
		Steps: []ResolvedStep{
			{
				Action: "cpan_install",
				Params: map[string]interface{}{
					"distribution": "Carton",
					"version":      "1.0.35",
					"executables":  []interface{}{"carton"},
					"snapshot":     snapshot,
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.CpanMeta != snapshot {
		t.Errorf("Expected CpanMeta to be extracted, got %q", constraints.CpanMeta)
	}
}

func TestExtractConstraints_CpanInstallInDependency(t *testing.T) {
	snapshot := `# carton snapshot format: version 1.0
DISTRIBUTIONS
  Some-Module-1.0.0
`
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "myapp",
		Version:       "1.0.0",
		Dependencies: []DependencyPlan{
			{
				Tool:    "perl-tool",
				Version: "1.0.0",
				Steps: []ResolvedStep{
					{
						Action: "cpan_install",
						Params: map[string]interface{}{
							"distribution": "perl-tool",
							"version":      "1.0.0",
							"snapshot":     snapshot,
						},
					},
				},
			},
		},
		Steps: []ResolvedStep{
			{
				Action: "download_file",
				Params: map[string]interface{}{
					"url": "https://example.com/myapp",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.CpanMeta != snapshot {
		t.Errorf("Expected CpanMeta from dependency to be extracted, got %q", constraints.CpanMeta)
	}
}

func TestExtractConstraints_CpanInstallFirstWins(t *testing.T) {
	snapshot1 := `first cpanfile.snapshot content`
	snapshot2 := `second cpanfile.snapshot content`

	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "multi-tool",
		Version:       "1.0.0",
		Steps: []ResolvedStep{
			{
				Action: "cpan_install",
				Params: map[string]interface{}{
					"distribution": "first-dist",
					"version":      "1.0.0",
					"snapshot":     snapshot1,
				},
			},
			{
				Action: "cpan_install",
				Params: map[string]interface{}{
					"distribution": "second-dist",
					"version":      "2.0.0",
					"snapshot":     snapshot2,
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.CpanMeta != snapshot1 {
		t.Errorf("Expected first CpanMeta to win, got %q", constraints.CpanMeta)
	}
}

func TestExtractConstraints_CpanInstallEmptySnapshot(t *testing.T) {
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "tool",
		Version:       "1.0.0",
		Steps: []ResolvedStep{
			{
				Action: "cpan_install",
				Params: map[string]interface{}{
					"distribution": "some-dist",
					"version":      "1.0.0",
					"snapshot":     "",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	if constraints.CpanMeta != "" {
		t.Errorf("Expected empty CpanMeta for empty snapshot param, got %q", constraints.CpanMeta)
	}
}

func TestHasCpanMetaConstraint(t *testing.T) {
	tests := []struct {
		name        string
		constraints *actions.EvalConstraints
		expected    bool
	}{
		{
			name:        "nil constraints",
			constraints: nil,
			expected:    false,
		},
		{
			name: "empty constraints",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{},
				CpanMeta:       "",
			},
			expected: false,
		},
		{
			name: "with cpan meta",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{},
				CpanMeta:       "# carton snapshot format: version 1.0\nDISTRIBUTIONS\n",
			},
			expected: true,
		},
		{
			name: "with pip but no cpan",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{
					"flask": "2.3.0",
				},
				CpanMeta: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasCpanMetaConstraint(tt.constraints)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// Tests for DependencyVersions extraction

func TestExtractConstraints_DependencyVersions(t *testing.T) {
	// Test that dependency versions are extracted from the dependency tree
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "black",
		Version:       "26.1a1",
		Dependencies: []DependencyPlan{
			{
				Tool:    "python-standalone",
				Version: "20260113",
				Steps: []ResolvedStep{
					{
						Action: "download_file",
						Params: map[string]interface{}{
							"url": "https://example.com/python.tar.zst",
						},
					},
				},
			},
		},
		Steps: []ResolvedStep{
			{
				Action: "pip_exec",
				Params: map[string]interface{}{
					"package": "black",
					"version": "26.1a1",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	// Verify dependency version was extracted
	if ver, ok := constraints.DependencyVersions["python-standalone"]; !ok || ver != "20260113" {
		t.Errorf("Expected python-standalone==20260113, got %q", ver)
	}
}

func TestExtractConstraints_DependencyVersionsNested(t *testing.T) {
	// Test that nested dependencies are also extracted
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "myapp",
		Version:       "1.0.0",
		Dependencies: []DependencyPlan{
			{
				Tool:    "rust-tool",
				Version: "1.0.0",
				Dependencies: []DependencyPlan{
					{
						Tool:    "rust",
						Version: "1.75.0",
						Steps: []ResolvedStep{
							{
								Action: "download_file",
								Params: map[string]interface{}{
									"url": "https://example.com/rust.tar.xz",
								},
							},
						},
					},
				},
				Steps: []ResolvedStep{
					{
						Action: "cargo_build",
						Params: map[string]interface{}{
							"crate":   "rust-tool",
							"version": "1.0.0",
						},
					},
				},
			},
		},
		Steps: []ResolvedStep{
			{
				Action: "download_file",
				Params: map[string]interface{}{
					"url": "https://example.com/myapp",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	// Verify both dependency versions were extracted
	if ver, ok := constraints.DependencyVersions["rust-tool"]; !ok || ver != "1.0.0" {
		t.Errorf("Expected rust-tool==1.0.0, got %q", ver)
	}
	if ver, ok := constraints.DependencyVersions["rust"]; !ok || ver != "1.75.0" {
		t.Errorf("Expected rust==1.75.0, got %q", ver)
	}
}

func TestExtractConstraints_DependencyVersionsFirstWins(t *testing.T) {
	// Test first-encountered-wins semantics for version conflicts
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "myapp",
		Version:       "1.0.0",
		Dependencies: []DependencyPlan{
			{
				Tool:    "toolA",
				Version: "1.0.0",
				Dependencies: []DependencyPlan{
					{
						Tool:    "shared-dep",
						Version: "2.0.0", // First occurrence
						Steps:   []ResolvedStep{},
					},
				},
				Steps: []ResolvedStep{},
			},
			{
				Tool:    "toolB",
				Version: "1.0.0",
				Dependencies: []DependencyPlan{
					{
						Tool:    "shared-dep",
						Version: "3.0.0", // Later occurrence - should be ignored
						Steps:   []ResolvedStep{},
					},
				},
				Steps: []ResolvedStep{},
			},
		},
		Steps: []ResolvedStep{},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	// First-encountered version should win
	if ver, ok := constraints.DependencyVersions["shared-dep"]; !ok || ver != "2.0.0" {
		t.Errorf("Expected first-encountered shared-dep==2.0.0, got %q", ver)
	}
}

func TestExtractConstraints_DependencyVersionsEmpty(t *testing.T) {
	// Test plan with no dependencies
	plan := &InstallationPlan{
		FormatVersion: 3,
		Tool:          "kubectl",
		Version:       "1.29.0",
		Dependencies:  []DependencyPlan{},
		Steps: []ResolvedStep{
			{
				Action: "download_file",
				Params: map[string]interface{}{
					"url": "https://example.com/kubectl",
				},
			},
		},
	}

	constraints, err := ExtractConstraintsFromPlan(plan)
	if err != nil {
		t.Fatalf("ExtractConstraintsFromPlan failed: %v", err)
	}

	// Should have empty dependency versions (initialized but empty map)
	if len(constraints.DependencyVersions) != 0 {
		t.Errorf("Expected 0 dependency versions for plan with no dependencies, got %d", len(constraints.DependencyVersions))
	}
}

func TestHasDependencyVersionConstraints(t *testing.T) {
	tests := []struct {
		name        string
		constraints *actions.EvalConstraints
		expected    bool
	}{
		{
			name:        "nil constraints",
			constraints: nil,
			expected:    false,
		},
		{
			name: "empty constraints",
			constraints: &actions.EvalConstraints{
				PipConstraints:     map[string]string{},
				DependencyVersions: map[string]string{},
			},
			expected: false,
		},
		{
			name: "with dependency versions",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{},
				DependencyVersions: map[string]string{
					"python-standalone": "20260113",
				},
			},
			expected: true,
		},
		{
			name: "with pip but no dependency versions",
			constraints: &actions.EvalConstraints{
				PipConstraints: map[string]string{
					"flask": "2.3.0",
				},
				DependencyVersions: map[string]string{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasDependencyVersionConstraints(tt.constraints)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetDependencyVersionConstraint(t *testing.T) {
	constraints := &actions.EvalConstraints{
		DependencyVersions: map[string]string{
			"python-standalone": "20260113",
			"go":                "1.25.5",
		},
	}

	tests := []struct {
		name        string
		toolName    string
		constraints *actions.EvalConstraints
		wantVersion string
		wantOK      bool
	}{
		{
			name:        "existing tool",
			toolName:    "python-standalone",
			constraints: constraints,
			wantVersion: "20260113",
			wantOK:      true,
		},
		{
			name:        "another existing tool",
			toolName:    "go",
			constraints: constraints,
			wantVersion: "1.25.5",
			wantOK:      true,
		},
		{
			name:        "non-existent tool",
			toolName:    "nodejs",
			constraints: constraints,
			wantVersion: "",
			wantOK:      false,
		},
		{
			name:        "nil constraints",
			toolName:    "python-standalone",
			constraints: nil,
			wantVersion: "",
			wantOK:      false,
		},
		{
			name:     "empty dependency versions",
			toolName: "python-standalone",
			constraints: &actions.EvalConstraints{
				DependencyVersions: map[string]string{},
			},
			wantVersion: "",
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVersion, gotOK := GetDependencyVersionConstraint(tt.constraints, tt.toolName)
			if gotOK != tt.wantOK {
				t.Errorf("GetDependencyVersionConstraint() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("GetDependencyVersionConstraint() version = %q, want %q", gotVersion, tt.wantVersion)
			}
		})
	}
}
