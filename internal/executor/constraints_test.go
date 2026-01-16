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
