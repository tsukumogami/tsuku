package main

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestIsSystemRequiredRecipe(t *testing.T) {
	tests := []struct {
		name     string
		recipe   *recipe.Recipe
		expected bool
	}{
		{
			name:     "nil recipe",
			recipe:   nil,
			expected: false,
		},
		{
			name: "empty steps",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{},
			},
			expected: false,
		},
		{
			name: "only require_system steps",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "require_system"},
					{Action: "require_system"},
				},
			},
			expected: true,
		},
		{
			name: "single require_system step",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "require_system"},
				},
			},
			expected: true,
		},
		{
			name: "mixed steps - not system required",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "download"},
					{Action: "extract"},
				},
			},
			expected: false,
		},
		{
			name: "has require_system but also other actions",
			recipe: &recipe.Recipe{
				Steps: []recipe.Step{
					{Action: "require_system"},
					{Action: "download"},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSystemRequiredRecipe(tt.recipe)
			if result != tt.expected {
				t.Errorf("isSystemRequiredRecipe() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMergeDeps(t *testing.T) {
	tests := []struct {
		name     string
		deps     actions.ResolvedDeps
		expected map[string]string
	}{
		{
			name: "empty deps",
			deps: actions.ResolvedDeps{
				InstallTime: map[string]string{},
				Runtime:     map[string]string{},
			},
			expected: map[string]string{},
		},
		{
			name: "only install deps",
			deps: actions.ResolvedDeps{
				InstallTime: map[string]string{"a": "1.0", "b": "2.0"},
				Runtime:     map[string]string{},
			},
			expected: map[string]string{"a": "1.0", "b": "2.0"},
		},
		{
			name: "only runtime deps",
			deps: actions.ResolvedDeps{
				InstallTime: map[string]string{},
				Runtime:     map[string]string{"c": "3.0"},
			},
			expected: map[string]string{"c": "3.0"},
		},
		{
			name: "both install and runtime deps",
			deps: actions.ResolvedDeps{
				InstallTime: map[string]string{"a": "1.0"},
				Runtime:     map[string]string{"b": "2.0"},
			},
			expected: map[string]string{"a": "1.0", "b": "2.0"},
		},
		{
			name: "runtime overwrites install if same key",
			deps: actions.ResolvedDeps{
				InstallTime: map[string]string{"a": "1.0"},
				Runtime:     map[string]string{"a": "2.0"},
			},
			expected: map[string]string{"a": "2.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeDeps(tt.deps)
			if len(result) != len(tt.expected) {
				t.Errorf("mergeDeps() returned %d items, want %d", len(result), len(tt.expected))
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("mergeDeps()[%s] = %s, want %s", k, result[k], v)
				}
			}
		})
	}
}

func TestDepStatus(t *testing.T) {
	// Test DepStatus struct creation
	status := DepStatus{
		Name:     "docker",
		Type:     "system-required",
		Status:   "installed",
		Version:  "24.0.0",
		Required: "20.0.0",
	}

	if status.Name != "docker" {
		t.Errorf("DepStatus.Name = %q, want %q", status.Name, "docker")
	}
	if status.Type != "system-required" {
		t.Errorf("DepStatus.Type = %q, want %q", status.Type, "system-required")
	}
	if status.Status != "installed" {
		t.Errorf("DepStatus.Status = %q, want %q", status.Status, "installed")
	}
}

func TestCheckDepsOutput(t *testing.T) {
	// Test CheckDepsOutput struct
	output := CheckDepsOutput{
		Tool: "myapp",
		Dependencies: []DepStatus{
			{Name: "dep1", Type: "provisionable", Status: "installed"},
			{Name: "dep2", Type: "system-required", Status: "missing"},
		},
		AllSatisfied: false,
	}

	if output.Tool != "myapp" {
		t.Errorf("CheckDepsOutput.Tool = %q, want %q", output.Tool, "myapp")
	}
	if len(output.Dependencies) != 2 {
		t.Errorf("len(CheckDepsOutput.Dependencies) = %d, want %d", len(output.Dependencies), 2)
	}
	if output.AllSatisfied {
		t.Errorf("CheckDepsOutput.AllSatisfied = true, want false")
	}
}
