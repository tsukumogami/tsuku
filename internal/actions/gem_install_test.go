package actions

import (
	"context"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestIsValidGemName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid gem names
		{"simple name", "bundler", true},
		{"with hyphen", "factory-bot", true},
		{"with underscore", "rspec_support", true},
		{"mixed case", "MyGem", true},
		{"with numbers", "rails5", true},
		{"hyphen and underscore", "my-gem_name", true},

		// Invalid gem names
		{"empty", "", false},
		{"starts with number", "1gem", false},
		{"starts with hyphen", "-gem", false},
		{"starts with underscore", "_gem", false},
		{"contains dot", "my.gem", false},
		{"contains space", "my gem", false},
		{"contains slash", "my/gem", false},
		{"contains at", "@scope/gem", false},
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},

		// Security test cases
		{"injection semicolon", "gem;echo", false},
		{"injection backtick", "gem`pwd`", false},
		{"injection dollar", "gem$()", false},
		{"path traversal", "../../etc/passwd", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidGemName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidGemName(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidGemVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Valid versions
		{"simple", "1.0.0", true},
		{"two parts", "1.2", true},
		{"four parts", "1.2.3.4", true},
		{"with pre", "1.0.0.pre", true},
		{"with rc", "1.0.0.rc1", true},
		{"with beta", "1.0.0.beta.2", true},
		{"with hyphen pre", "1.0.0-pre.1", true},
		{"with alpha", "1.0.0alpha", true},

		// Invalid versions
		{"empty", "", false},
		{"starts with letter", "v1.0.0", false},
		{"contains plus", "1.0.0+build", false},
		{"contains space", "1.0 .0", false},
		{"too long", "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0", false},

		// Security test cases
		{"injection semicolon", "1.0.0;echo", false},
		{"injection backtick", "1.0.0`pwd`", false},
		{"injection dollar", "1.0.0$()", false},
		{"injection pipe", "1.0.0|cat", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidGemVersion(tt.input)
			if result != tt.expected {
				t.Errorf("isValidGemVersion(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGemInstallAction_Execute_Validation(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}

	tests := []struct {
		name        string
		params      map[string]interface{}
		version     string
		expectError string
	}{
		{
			name:        "missing gem parameter",
			params:      map[string]interface{}{},
			version:     "1.0.0",
			expectError: "requires 'gem' parameter",
		},
		{
			name: "invalid gem name",
			params: map[string]interface{}{
				"gem":         "invalid;gem",
				"executables": []interface{}{"exe"},
			},
			version:     "1.0.0",
			expectError: "invalid gem name",
		},
		{
			name: "missing executables",
			params: map[string]interface{}{
				"gem": "bundler",
			},
			version:     "1.0.0",
			expectError: "requires 'executables' parameter",
		},
		{
			name: "empty executables",
			params: map[string]interface{}{
				"gem":         "bundler",
				"executables": []interface{}{},
			},
			version:     "1.0.0",
			expectError: "requires 'executables' parameter",
		},
		{
			name: "invalid executable with path",
			params: map[string]interface{}{
				"gem":         "bundler",
				"executables": []interface{}{"../bin/exe"},
			},
			version:     "1.0.0",
			expectError: "must not contain path separators",
		},
		{
			name: "invalid executable with shell metacharacter",
			params: map[string]interface{}{
				"gem":         "bundler",
				"executables": []interface{}{"exe;rm"},
			},
			version:     "1.0.0",
			expectError: "contains shell metacharacters",
		},
		{
			name: "invalid version",
			params: map[string]interface{}{
				"gem":         "bundler",
				"executables": []interface{}{"bundle"},
			},
			version:     ";echo hack",
			expectError: "invalid version format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &ExecutionContext{
				Version:    tt.version,
				InstallDir: "/tmp/test",
			}

			err := action.Execute(ctx, tt.params)
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectError)
				return
			}

			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestGemInstallAction_Name(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	if action.Name() != "gem_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "gem_install")
	}
}

func TestGemInstallAction_Decompose_Validation(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}

	tests := []struct {
		name        string
		params      map[string]interface{}
		version     string
		expectError string
	}{
		{
			name:        "missing gem parameter",
			params:      map[string]interface{}{},
			version:     "1.0.0",
			expectError: "requires 'gem' parameter",
		},
		{
			name: "invalid gem name",
			params: map[string]interface{}{
				"gem":         "invalid;gem",
				"executables": []interface{}{"exe"},
			},
			version:     "1.0.0",
			expectError: "invalid gem name",
		},
		{
			name: "missing executables",
			params: map[string]interface{}{
				"gem": "bundler",
			},
			version:     "1.0.0",
			expectError: "requires 'executables' parameter",
		},
		{
			name: "missing version",
			params: map[string]interface{}{
				"gem":         "bundler",
				"executables": []interface{}{"bundle"},
			},
			version:     "",
			expectError: "requires a resolved version",
		},
		{
			name: "invalid version",
			params: map[string]interface{}{
				"gem":         "bundler",
				"executables": []interface{}{"bundle"},
			},
			version:     ";echo hack",
			expectError: "invalid version format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &EvalContext{
				Version: tt.version,
			}

			_, err := action.Decompose(ctx, tt.params)
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectError)
				return
			}

			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestGemInstallAction_ImplementsDecomposable(t *testing.T) {
	// Verify GemInstallAction implements Decomposable interface
	var _ Decomposable = (*GemInstallAction)(nil)
}

func TestGemInstallAction_Dependencies(t *testing.T) {
	action := &GemInstallAction{}
	deps := action.Dependencies()

	// Check EvalTime dependency (needed for bundle lock)
	if len(deps.EvalTime) != 1 || deps.EvalTime[0] != "ruby" {
		t.Errorf("EvalTime dependencies = %v, want [ruby]", deps.EvalTime)
	}

	// Check InstallTime dependency
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "ruby" {
		t.Errorf("InstallTime dependencies = %v, want [ruby]", deps.InstallTime)
	}

	// Check Runtime dependency
	if len(deps.Runtime) != 1 || deps.Runtime[0] != "ruby" {
		t.Errorf("Runtime dependencies = %v, want [ruby]", deps.Runtime)
	}
}

func TestCountLockfileGems(t *testing.T) {
	tests := []struct {
		name     string
		lockData string
		expected int
	}{
		{
			name:     "empty",
			lockData: "",
			expected: 0,
		},
		{
			name: "single gem",
			lockData: `GEM
  remote: https://rubygems.org/
  specs:
    bundler (2.4.0)

PLATFORMS
  ruby
`,
			expected: 1,
		},
		{
			name: "multiple gems",
			lockData: `GEM
  remote: https://rubygems.org/
  specs:
    bundler (2.4.0)
    rake (13.0.6)
    rspec (3.12.0)
      rspec-core (~> 3.12.0)

PLATFORMS
  ruby
`,
			expected: 3,
		},
		{
			name: "gems with dependencies",
			lockData: `GEM
  remote: https://rubygems.org/
  specs:
    nokogiri (1.13.10)
      mini_portile2 (~> 2.8.0)
      racc (~> 1.4)
    racc (1.6.2)
    mini_portile2 (2.8.5)

PLATFORMS
  ruby
`,
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countLockfileGems(tt.lockData)
			if result != tt.expected {
				t.Errorf("countLockfileGems() = %d, want %d", result, tt.expected)
			}
		})
	}
}

// -- gem_install.go: Decompose additional paths --

func TestGemInstallAction_Decompose_MissingGem(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "1.0.0",
	}
	_, err := action.Decompose(ctx, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "gem") {
		t.Errorf("Expected gem error, got %v", err)
	}
}

func TestGemInstallAction_Decompose_InvalidGemName(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "1.0.0",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"gem":         "!invalid",
		"executables": []any{"tool"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("Expected invalid error, got %v", err)
	}
}

func TestGemInstallAction_Decompose_MissingExecutables(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "1.0.0",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"gem": "rails",
	})
	if err == nil || !strings.Contains(err.Error(), "executables") {
		t.Errorf("Expected executables error, got %v", err)
	}
}

func TestGemInstallAction_Decompose_MissingVersion(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"gem":         "rails",
		"executables": []any{"rails"},
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Expected version error, got %v", err)
	}
}

func TestGemInstallAction_Decompose_InvalidVersion(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &EvalContext{
		Context: context.Background(),
		Version: "invalid!",
	}
	_, err := action.Decompose(ctx, map[string]any{
		"gem":         "rails",
		"executables": []any{"rails"},
	})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("Expected version error, got %v", err)
	}
}

// -- gem_install.go: Execute additional validation --

func TestGemInstallAction_Execute_ControlCharInExecutable(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"gem":         "rails",
		"executables": []any{"evil\x00name"},
	})
	if err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Errorf("Expected control characters error, got %v", err)
	}
}

func TestGemInstallAction_Execute_ShellMetacharInExecutable(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"gem":         "rails",
		"executables": []any{"evil$(cmd)"},
	})
	if err == nil || !strings.Contains(err.Error(), "shell metacharacters") {
		t.Errorf("Expected shell metacharacters error, got %v", err)
	}
}

func TestGemInstallAction_Execute_TooLongExecutableName(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"gem":         "rails",
		"executables": []any{strings.Repeat("a", 257)},
	})
	if err == nil || !strings.Contains(err.Error(), "length") {
		t.Errorf("Expected length error, got %v", err)
	}
}

// -- gem_install.go: Execute param validation --

func TestGemInstallAction_Execute_MissingGem(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing gem param")
	}
}

func TestGemInstallAction_Execute_InvalidGemName(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"gem": "invalid gem name!",
	})
	if err == nil {
		t.Error("Expected error for invalid gem name")
	}
}

func TestGemInstallAction_Execute_InvalidVersion(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "invalid version!@#",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"gem": "rails",
	})
	if err == nil {
		t.Error("Expected error for invalid version")
	}
}

func TestGemInstallAction_Execute_MissingExecutables(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"gem": "rails",
	})
	if err == nil {
		t.Error("Expected error for missing executables")
	}
}

func TestGemInstallAction_Execute_InvalidExecutableName(t *testing.T) {
	t.Parallel()
	action := &GemInstallAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"gem":         "rails",
		"executables": []any{"../evil"},
	})
	if err == nil {
		t.Error("Expected error for invalid executable name with path separator")
	}
}
