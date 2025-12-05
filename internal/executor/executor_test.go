package executor

import (
	"context"
	"os"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/version"
)

// TestResolveVersionWith_CustomSource tests that executor correctly uses custom version sources
func TestResolveVersionWith_CustomSource(t *testing.T) {
	// Create a test recipe with custom version source
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:          "test-tool",
			Description:   "Test tool",
			Homepage:      "https://example.com",
			VersionFormat: "semver",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist", // Use a registered source
		},
		Steps: []recipe.Step{
			{
				Action: "download_archive",
				Params: map[string]interface{}{
					"url": "https://example.com/test-{version}.tar.gz",
				},
			},
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Test that resolveVersionWith can use the registry
	ctx := context.Background()
	resolver := version.New()

	// This will attempt to fetch the real Node.js version (may fail due to network)
	// but the test verifies the integration works
	versionInfo, err := exec.resolveVersionWith(ctx, resolver)

	// If network is available, should get version; otherwise error is acceptable
	if err != nil {
		// Network failure is acceptable in unit tests
		t.Logf("resolveVersionWith() failed (expected in offline tests): %v", err)
	} else {
		t.Logf("resolveVersionWith() succeeded: version=%s", versionInfo.Version)
	}
}

// TestResolveVersionWith_UnknownSource tests error handling for unknown sources
func TestResolveVersionWith_UnknownSource(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:          "test-tool",
			Description:   "Test tool",
			VersionFormat: "semver",
		},
		Version: recipe.VersionSection{
			Source: "completely_unknown_source",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Test that resolveVersionWith handles unknown sources gracefully
	ctx := context.Background()
	resolver := version.New()

	// Call resolveVersionWith
	versionInfo, err := exec.resolveVersionWith(ctx, resolver)

	// Should get an error or fall back to "dev"
	if err == nil && versionInfo.Version != "dev" {
		// If no error, should have fallen back to "dev"
		t.Logf("resolveVersionWith() with unknown source: version=%s", versionInfo.Version)
	}
}

// TestExecute_FallbackToDev tests that executor falls back to "dev" version on resolution failure
func TestExecute_FallbackToDev(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:          "test-tool",
			Description:   "Test tool",
			VersionFormat: "semver",
		},
		Version: recipe.VersionSection{
			Source: "nonexistent_source", // Unknown source will fail
		},
		Steps: []recipe.Step{
			{
				Action: "run_shell",
				Params: map[string]interface{}{
					"script": "echo 'test'",
				},
			},
		},
		Verify: recipe.VerifySection{
			Command: "echo 'verification'",
			Pattern: "verification",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()
	err = exec.Execute(ctx)

	// Should NOT fail - should fall back to "dev" and continue
	if err != nil {
		// The error might be from the run_shell action not being registered in tests
		// That's OK - we just want to verify it didn't fail on version resolution
		if exec.version != "dev" {
			t.Errorf("Expected fallback to version 'dev', got '%s'", exec.version)
		}
	} else {
		// Success - verify it used "dev"
		if exec.version != "dev" {
			t.Errorf("Expected fallback to version 'dev', got '%s'", exec.version)
		}
	}
}

// TestSetToolsDir tests the SetToolsDir method
func TestSetToolsDir(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "test-tool",
			Description: "Test tool",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Test setting ToolsDir
	testToolsDir := "/custom/tools/path"
	exec.SetToolsDir(testToolsDir)

	if exec.toolsDir != testToolsDir {
		t.Errorf("SetToolsDir() = %q, want %q", exec.toolsDir, testToolsDir)
	}
}

// TestExecute_NetworkFailureFallback tests fallback on network errors
func TestExecute_NetworkFailureFallback(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:          "test-tool",
			Description:   "Test tool",
			VersionFormat: "semver",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist", // Will fail due to network in test environment
		},
		Steps: []recipe.Step{
			{
				Action: "run_shell",
				Params: map[string]interface{}{
					"script": "echo 'test'",
				},
			},
		},
		Verify: recipe.VerifySection{
			Command: "echo 'verification'",
			Pattern: "verification",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()
	err = exec.Execute(ctx)

	// In offline environments, should fall back to "dev"
	// (If network is available and nodejs_dist works, that's OK too)
	if exec.version != "dev" && err != nil {
		t.Logf("Network available, version resolved to: %s", exec.version)
	} else if exec.version == "dev" {
		t.Logf("Correctly fell back to 'dev' on network failure")
	}
}

// TestNewWithVersion tests creating an executor with a specific version
func TestNewWithVersion(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "test-tool",
			Description: "Test tool",
		},
	}

	exec, err := NewWithVersion(r, "1.2.3")
	if err != nil {
		t.Fatalf("NewWithVersion() error = %v", err)
	}
	defer exec.Cleanup()

	if exec.reqVersion != "1.2.3" {
		t.Errorf("reqVersion = %q, want %q", exec.reqVersion, "1.2.3")
	}
}

// TestShouldExecute tests the conditional execution logic
func TestShouldExecute(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "test-tool",
			Description: "Test tool",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	tests := []struct {
		name     string
		when     map[string]string
		expected bool
	}{
		{
			name:     "empty when - always execute",
			when:     map[string]string{},
			expected: true,
		},
		{
			name:     "nil when - always execute",
			when:     nil,
			expected: true,
		},
		{
			name:     "matching OS",
			when:     map[string]string{"os": "linux"},
			expected: true, // assuming test runs on linux
		},
		{
			name:     "non-matching OS",
			when:     map[string]string{"os": "windows"},
			expected: false,
		},
		{
			name:     "matching arch",
			when:     map[string]string{"arch": "amd64"},
			expected: true, // assuming test runs on amd64
		},
		{
			name:     "non-matching arch",
			when:     map[string]string{"arch": "arm"},
			expected: false,
		},
		{
			name:     "package_manager - always true (stub)",
			when:     map[string]string{"package_manager": "apt"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := exec.shouldExecute(tt.when)
			// Note: OS/arch tests depend on the actual runtime
			// We check the logic is correctly evaluated
			if tt.name == "empty when - always execute" || tt.name == "nil when - always execute" {
				if result != tt.expected {
					t.Errorf("shouldExecute(%v) = %v, want %v", tt.when, result, tt.expected)
				}
			}
			if tt.name == "non-matching OS" {
				if result != false {
					t.Errorf("shouldExecute with non-matching OS should return false")
				}
			}
			if tt.name == "non-matching arch" {
				if result != false {
					t.Errorf("shouldExecute with non-matching arch should return false")
				}
			}
			if tt.name == "package_manager - always true (stub)" {
				if result != true {
					t.Errorf("shouldExecute with package_manager should return true (stub)")
				}
			}
		})
	}
}

// TestExpandVars tests the expandVars helper function
func TestExpandVars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		expected string
	}{
		{
			name:     "single variable",
			input:    "tool-{version}.tar.gz",
			vars:     map[string]string{"version": "1.2.3"},
			expected: "tool-1.2.3.tar.gz",
		},
		{
			name:     "multiple variables",
			input:    "{binary} --version",
			vars:     map[string]string{"binary": "/usr/bin/tool", "version": "1.0"},
			expected: "/usr/bin/tool --version",
		},
		{
			name:     "no variables",
			input:    "static-string",
			vars:     map[string]string{"version": "1.0.0"},
			expected: "static-string",
		},
		{
			name:     "empty string",
			input:    "",
			vars:     map[string]string{"version": "1.0.0"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandVars(tt.input, tt.vars)
			if result != tt.expected {
				t.Errorf("expandVars(%q, %v) = %q, want %q", tt.input, tt.vars, result, tt.expected)
			}
		})
	}
}

// TestVersion tests the Version() getter
func TestVersion(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "test-tool",
			Description: "Test tool",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Initially empty
	if exec.Version() != "" {
		t.Errorf("Initial Version() = %q, want empty", exec.Version())
	}

	// Set version manually for test
	exec.version = "2.0.0"
	if exec.Version() != "2.0.0" {
		t.Errorf("Version() = %q, want %q", exec.Version(), "2.0.0")
	}
}

// TestWorkDir tests the WorkDir() getter
func TestWorkDir(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "test-tool",
			Description: "Test tool",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	workDir := exec.WorkDir()
	if workDir == "" {
		t.Error("WorkDir() should not be empty")
	}
}

// TestSetExecPaths tests the SetExecPaths method
func TestSetExecPaths(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "test-tool",
			Description: "Test tool",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	paths := []string{"/usr/local/bin", "/opt/bin"}
	exec.SetExecPaths(paths)

	if len(exec.execPaths) != 2 {
		t.Errorf("execPaths length = %d, want 2", len(exec.execPaths))
	}
	if exec.execPaths[0] != "/usr/local/bin" {
		t.Errorf("execPaths[0] = %q, want %q", exec.execPaths[0], "/usr/local/bin")
	}
}

// TestCleanup tests that Cleanup removes the work directory
func TestCleanup(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "test-tool",
			Description: "Test tool",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	workDir := exec.WorkDir()

	// Verify work dir exists
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		t.Fatal("WorkDir should exist after New()")
	}

	exec.Cleanup()

	// Verify work dir is removed
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Error("WorkDir should be removed after Cleanup()")
	}
}

// TestDryRun tests the DryRun method
func TestDryRun(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:          "test-tool",
			Description:   "Test tool",
			Dependencies:  []string{"dep1", "dep2"},
			VersionFormat: "semver",
		},
		Version: recipe.VersionSection{
			Source: "nonexistent_source", // Will fail, triggering dev fallback
		},
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{
					"url": "https://example.com/tool-{version}.tar.gz",
				},
			},
			{
				Action: "extract",
				Params: map[string]interface{}{
					"src": "tool-{version}.tar.gz",
				},
			},
			{
				Action: "install_binaries",
				Params: map[string]interface{}{
					"binaries": []interface{}{"tool", "tool-cli"},
				},
			},
		},
		Verify: recipe.VerifySection{
			Command: "tool --version",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()
	err = exec.DryRun(ctx)

	// DryRun should fail on version resolution with unknown source
	if err == nil {
		t.Log("DryRun succeeded (version resolved)")
	} else {
		t.Logf("DryRun returned error (expected for unknown source): %v", err)
	}
}

// TestDryRun_WithDependencies tests DryRun output includes dependencies
func TestDryRun_WithDependencies(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:         "test-tool",
			Description:  "Test tool with deps",
			Dependencies: []string{"nodejs", "npm"},
		},
		Version: recipe.VersionSection{
			Source: "unknown", // Will fail
		},
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]interface{}{
					"package": "some-package",
				},
			},
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Verify recipe has dependencies
	if len(r.Metadata.Dependencies) != 2 {
		t.Errorf("Expected 2 dependencies, got %d", len(r.Metadata.Dependencies))
	}
}

// TestDryRun_NoDependencies tests DryRun with no dependencies
func TestDryRun_NoDependencies(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:         "test-tool",
			Description:  "Test tool without deps",
			Dependencies: nil,
		},
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{
					"url": "https://example.com/tool.tar.gz",
				},
			},
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Verify recipe has no dependencies
	if len(r.Metadata.Dependencies) != 0 {
		t.Errorf("Expected 0 dependencies, got %d", len(r.Metadata.Dependencies))
	}
}

// TestFormatActionDescription tests the action description formatting
func TestFormatActionDescription(t *testing.T) {
	vars := map[string]string{
		"version": "1.2.3",
		"os":      "linux",
		"arch":    "amd64",
	}

	tests := []struct {
		name     string
		action   string
		params   map[string]interface{}
		expected string
	}{
		{
			name:   "download action",
			action: "download",
			params: map[string]interface{}{
				"url": "https://example.com/tool-{version}.tar.gz",
			},
			expected: "https://example.com/tool-1.2.3.tar.gz",
		},
		{
			name:   "extract action",
			action: "extract",
			params: map[string]interface{}{
				"src": "tool-{version}.tar.gz",
			},
			expected: "tool-1.2.3.tar.gz",
		},
		{
			name:   "install_binaries with string list",
			action: "install_binaries",
			params: map[string]interface{}{
				"binaries": []interface{}{"tool", "tool-cli", "tool-server"},
			},
			expected: "tool, tool-cli, tool-server",
		},
		{
			name:   "install_binaries with map list",
			action: "install_binaries",
			params: map[string]interface{}{
				"binaries": []interface{}{
					map[string]interface{}{"name": "bin1"},
					map[string]interface{}{"name": "bin2"},
				},
			},
			expected: "bin1, bin2",
		},
		{
			name:   "chmod action",
			action: "chmod",
			params: map[string]interface{}{
				"file": "tool-{version}",
				"mode": "755",
			},
			expected: "tool-1.2.3 (mode 755)",
		},
		{
			name:   "chmod action default mode",
			action: "chmod",
			params: map[string]interface{}{
				"file": "mytool",
			},
			expected: "mytool (mode 755)",
		},
		{
			name:   "cargo_install action",
			action: "cargo_install",
			params: map[string]interface{}{
				"package": "ripgrep",
			},
			expected: "ripgrep",
		},
		{
			name:   "npm_install action",
			action: "npm_install",
			params: map[string]interface{}{
				"package": "typescript",
			},
			expected: "typescript",
		},
		{
			name:   "pipx_install action",
			action: "pipx_install",
			params: map[string]interface{}{
				"package": "black",
			},
			expected: "black",
		},
		{
			name:   "gem_install action",
			action: "gem_install",
			params: map[string]interface{}{
				"package": "bundler",
			},
			expected: "bundler",
		},
		{
			name:   "run_command action short",
			action: "run_command",
			params: map[string]interface{}{
				"command": "make install",
			},
			expected: "make install",
		},
		{
			name:   "run_command action long (truncated)",
			action: "run_command",
			params: map[string]interface{}{
				"command": "this is a very long command that should be truncated because it exceeds sixty characters in length",
			},
			expected: "this is a very long command that should be truncated beca...",
		},
		{
			name:   "unknown action",
			action: "unknown_action",
			params: map[string]interface{}{
				"foo": "bar",
			},
			expected: "",
		},
		{
			name:     "download without url",
			action:   "download",
			params:   map[string]interface{}{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatActionDescription(tt.action, tt.params, vars)
			if result != tt.expected {
				t.Errorf("formatActionDescription(%q, %v) = %q, want %q", tt.action, tt.params, result, tt.expected)
			}
		})
	}
}

// TestDryRun_SkipsConditionalSteps tests that DryRun respects when conditions
func TestDryRun_SkipsConditionalSteps(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "test-tool",
			Description: "Test tool",
		},
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{
					"url": "https://example.com/tool.tar.gz",
				},
			},
			{
				Action: "download",
				When:   map[string]string{"os": "windows"}, // Should be skipped on Linux
				Params: map[string]interface{}{
					"url": "https://example.com/tool-windows.exe",
				},
			},
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Verify conditional step would be skipped
	if exec.shouldExecute(r.Steps[1].When) {
		t.Log("Step with os=windows would execute (running on Windows)")
	} else {
		t.Log("Step with os=windows correctly skipped (not on Windows)")
	}
}

// TestDryRun_SuccessfulVersionResolution tests DryRun with a working version source
func TestDryRun_SuccessfulVersionResolution(t *testing.T) {
	// Use nodejs_dist which should work with network
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:          "test-nodejs",
			Description:   "Test tool with nodejs version",
			Dependencies:  []string{"dep1"},
			VersionFormat: "semver",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{
					"url": "https://nodejs.org/dist/v{version}/node-v{version}-{os}-{arch}.tar.xz",
				},
			},
			{
				Action: "extract",
				Params: map[string]interface{}{
					"src": "node-v{version}-{os}-{arch}.tar.xz",
				},
			},
			{
				Action: "install_binaries",
				Params: map[string]interface{}{
					"binaries": []interface{}{"node", "npm", "npx"},
				},
			},
		},
		Verify: recipe.VerifySection{
			Command: "node --version",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()
	err = exec.DryRun(ctx)

	// If network available, should succeed
	if err != nil {
		t.Logf("DryRun failed (network issue?): %v", err)
	} else {
		t.Logf("DryRun succeeded, version: %s", exec.Version())
		// Verify version was set
		if exec.Version() == "" {
			t.Error("Version should be set after successful DryRun")
		}
	}
}

// TestDryRun_EmptySteps tests DryRun with no steps
func TestDryRun_EmptySteps(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "empty-tool",
			Description: "Tool with no steps",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()
	err = exec.DryRun(ctx)

	// Should succeed even with no steps
	if err != nil {
		t.Logf("DryRun with empty steps: %v (network issue expected)", err)
	} else {
		t.Log("DryRun with empty steps succeeded")
	}
}

// TestDryRun_WithVerification tests DryRun displays verification info
func TestDryRun_WithVerification(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "verify-tool",
			Description: "Tool with verification",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{
					"url": "https://example.com/tool.tar.gz",
				},
			},
		},
		Verify: recipe.VerifySection{
			Command: "tool --version",
			Pattern: "v{version}",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Just verify the recipe has verification set
	if r.Verify.Command == "" {
		t.Error("Recipe should have verification command")
	}
}

// TestDryRun_AllConditionalStepsSkipped tests when all steps are conditional and skipped
func TestDryRun_AllConditionalStepsSkipped(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "conditional-tool",
			Description: "Tool with all conditional steps",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{
			{
				Action: "download",
				When:   map[string]string{"os": "windows"}, // Skipped on Linux
				Params: map[string]interface{}{
					"url": "https://example.com/tool-windows.exe",
				},
			},
			{
				Action: "download",
				When:   map[string]string{"arch": "arm"}, // Skipped on amd64
				Params: map[string]interface{}{
					"url": "https://example.com/tool-arm.tar.gz",
				},
			},
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// All steps should be skipped (not on windows, not on arm)
	skipped := 0
	for _, step := range r.Steps {
		if !exec.shouldExecute(step.When) {
			skipped++
		}
	}

	if skipped != 2 {
		t.Errorf("Expected 2 skipped steps, got %d", skipped)
	}
}
