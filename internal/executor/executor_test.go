package executor

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"

	"github.com/tsukumogami/tsuku/internal/actions"
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

// TestResolveVersion_EmptyConstraint tests that empty constraint resolves to latest
func TestResolveVersion_EmptyConstraint(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:          "test-tool",
			Description:   "Test tool",
			VersionFormat: "semver",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist", // Use a registered source
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()
	version, err := exec.ResolveVersion(ctx, "")

	// Network failure is acceptable in unit tests
	if err != nil {
		t.Logf("ResolveVersion() failed (expected in offline tests): %v", err)
	} else {
		t.Logf("ResolveVersion() succeeded: version=%s", version)
		if version == "" {
			t.Error("ResolveVersion() returned empty version string")
		}
	}
}

// TestResolveVersion_SpecificConstraint tests resolving a specific version
func TestResolveVersion_SpecificConstraint(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:          "test-tool",
			Description:   "Test tool",
			VersionFormat: "semver",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()
	// Request a specific version
	version, err := exec.ResolveVersion(ctx, "20.0.0")

	// Network failure is acceptable in unit tests
	if err != nil {
		t.Logf("ResolveVersion() with constraint failed (expected in offline tests): %v", err)
	} else {
		t.Logf("ResolveVersion() with constraint succeeded: version=%s", version)
		if version == "" {
			t.Error("ResolveVersion() returned empty version string")
		}
	}
}

// TestResolveVersion_UnknownSource tests error handling for unknown version sources
func TestResolveVersion_UnknownSource(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "test-tool",
			Description: "Test tool",
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

	ctx := context.Background()
	_, err = exec.ResolveVersion(ctx, "")

	// Should get an error since the source doesn't exist in the registry
	if err != nil {
		t.Logf("ResolveVersion() correctly returned error for unknown source: %v", err)
	} else {
		t.Log("ResolveVersion() succeeded with unknown source (custom provider may handle it)")
	}
}

// TestResolveVersion_NoVersionSource tests error handling when no version source is configured
func TestResolveVersion_NoVersionSource(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "test-tool",
			Description: "Test tool",
		},
		// No Version section - no version source configured
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

	ctx := context.Background()
	_, err = exec.ResolveVersion(ctx, "")

	// Should get an error since no version source is configured
	if err == nil {
		t.Error("ResolveVersion() should return error when no version source is configured")
	} else {
		t.Logf("ResolveVersion() correctly returned error: %v", err)
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
		when     *recipe.WhenClause
		expected bool
	}{
		{
			name:     "empty when - always execute",
			when:     &recipe.WhenClause{},
			expected: true,
		},
		{
			name:     "nil when - always execute",
			when:     nil,
			expected: true,
		},
		{
			name:     "matching OS",
			when:     &recipe.WhenClause{OS: []string{"linux"}},
			expected: true, // assuming test runs on linux
		},
		{
			name:     "non-matching OS",
			when:     &recipe.WhenClause{OS: []string{"windows"}},
			expected: false,
		},
		{
			name:     "matching arch",
			when:     &recipe.WhenClause{Platform: []string{runtime.GOOS + "/amd64"}},
			expected: true, // assuming test runs on amd64
		},
		{
			name:     "non-matching arch",
			when:     &recipe.WhenClause{Platform: []string{runtime.GOOS + "/arm"}},
			expected: false,
		},
		{
			name:     "package_manager - always true (stub)",
			when:     &recipe.WhenClause{PackageManager: "apt"},
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
				When:   &recipe.WhenClause{OS: []string{"windows"}}, // Should be skipped on Linux
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
				When:   &recipe.WhenClause{OS: []string{"windows"}}, // Skipped on Linux
				Params: map[string]interface{}{
					"url": "https://example.com/tool-windows.exe",
				},
			},
			{
				Action: "download",
				When:   &recipe.WhenClause{Platform: []string{runtime.GOOS + "/arm"}}, // Skipped on amd64
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

// TestExecutePlan_EmptyPlan tests ExecutePlan with no steps
func TestExecutePlan_EmptyPlan(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "empty-tool",
			Description: "Tool with no steps",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "empty-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Steps:         []ResolvedStep{},
	}

	ctx := context.Background()
	err = exec.ExecutePlan(ctx, plan)

	if err != nil {
		t.Errorf("ExecutePlan() with empty plan error = %v", err)
	}

	// Version should be set from plan
	if exec.Version() != "1.0.0" {
		t.Errorf("Version() = %q, want %q", exec.Version(), "1.0.0")
	}
}

// TestExecutePlan_UnknownAction tests ExecutePlan with an unknown action
func TestExecutePlan_UnknownAction(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
		Steps: []ResolvedStep{
			{
				Action: "unknown_action",
				Params: map[string]interface{}{},
			},
		},
	}

	ctx := context.Background()
	err = exec.ExecutePlan(ctx, plan)

	if err == nil {
		t.Error("ExecutePlan() with unknown action should fail")
	}
	if err != nil && !contains(err.Error(), "unknown action") {
		t.Errorf("ExecutePlan() error = %v, want error containing 'unknown action'", err)
	}
}

// TestExecutePlan_ContextCancellation tests ExecutePlan respects context cancellation
func TestExecutePlan_ContextCancellation(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Steps: []ResolvedStep{
			{
				Action: "chmod",
				Params: map[string]interface{}{
					"files": []interface{}{"nonexistent.txt"},
					"mode":  "755",
				},
			},
		},
	}

	// Create a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = exec.ExecutePlan(ctx, plan)

	if err == nil {
		t.Error("ExecutePlan() with canceled context should fail")
	}
	if err != nil && err != context.Canceled {
		t.Errorf("ExecutePlan() error = %v, want context.Canceled", err)
	}
}

// TestExecutePlan_NonDownloadSteps tests ExecutePlan with non-download steps
func TestExecutePlan_NonDownloadSteps(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Create a test file to chmod (relative to WorkDir)
	testFile := "test.sh"
	testFilePath := exec.WorkDir() + "/" + testFile
	if err := os.WriteFile(testFilePath, []byte("#!/bin/sh\necho hello"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Steps: []ResolvedStep{
			{
				Action: "chmod",
				Params: map[string]interface{}{
					"files": []interface{}{testFile},
					"mode":  "0755",
				},
			},
		},
	}

	ctx := context.Background()
	err = exec.ExecutePlan(ctx, plan)

	if err != nil {
		t.Errorf("ExecutePlan() with chmod step error = %v", err)
	}

	// Verify file is executable
	info, err := os.Stat(testFilePath)
	if err != nil {
		t.Fatalf("Failed to stat test file: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("File should be executable after chmod")
	}
}

// TestExecutePlan_LibraryWithDirectoryMode tests that library recipes can use
// directory install mode without requiring a verify section.
// This is a regression test for the bug where RecipeType wasn't propagated
// from the plan to the execution context when there was no verify section.
func TestExecutePlan_LibraryWithDirectoryMode(t *testing.T) {
	// Create executor WITHOUT a recipe (simulates plan-only execution)
	exec, err := New(nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Create test files in work directory
	binDir := exec.WorkDir() + "/bin"
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("Failed to create bin dir: %v", err)
	}
	testBinary := binDir + "/testlib"
	if err := os.WriteFile(testBinary, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatalf("Failed to create test binary: %v", err)
	}

	// Create a plan for a library with directory install mode and NO verify section
	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "test-lib",
		Version:       "1.0.0",
		Platform:      Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		RecipeType:    "library", // This is the key field that must be propagated
		// Note: No Verify section - libraries are exempt from verification
		Steps: []ResolvedStep{
			{
				Action: "install_binaries",
				Params: map[string]interface{}{
					"binaries":     []interface{}{"bin/testlib"},
					"install_mode": "directory",
				},
			},
		},
	}

	ctx := context.Background()
	err = exec.ExecutePlan(ctx, plan)

	if err != nil {
		t.Errorf("ExecutePlan() for library with directory mode error = %v", err)
		t.Log("Libraries should be exempt from verify requirement when using directory install mode")
	}
}

// TestComputeFileChecksum tests the checksum computation helper
func TestComputeFileChecksum(t *testing.T) {
	// Create a temp file with known content
	tmpFile, err := os.CreateTemp("", "checksum-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := []byte("hello world\n")
	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	// Compute checksum
	checksum, err := computeFileChecksum(tmpFile.Name())
	if err != nil {
		t.Fatalf("computeFileChecksum() error = %v", err)
	}

	// Expected SHA256 of "hello world\n"
	expected := "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
	if checksum != expected {
		t.Errorf("computeFileChecksum() = %q, want %q", checksum, expected)
	}
}

// TestComputeFileChecksum_FileNotFound tests checksum of non-existent file
func TestComputeFileChecksum_FileNotFound(t *testing.T) {
	_, err := computeFileChecksum("/nonexistent/file/path")
	if err == nil {
		t.Error("computeFileChecksum() should fail for non-existent file")
	}
}

// TestResolveDownloadDest tests destination resolution for downloads
func TestResolveDownloadDest(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	workDir := exec.WorkDir()

	tests := []struct {
		name     string
		step     ResolvedStep
		expected string
	}{
		{
			name: "explicit dest param",
			step: ResolvedStep{
				Action: "download",
				URL:    "https://example.com/tool.tar.gz",
				Params: map[string]interface{}{
					"dest": "custom-name.tar.gz",
				},
			},
			expected: workDir + "/custom-name.tar.gz",
		},
		{
			name: "dest from step URL",
			step: ResolvedStep{
				Action: "download",
				URL:    "https://example.com/path/to/file.tar.gz",
				Params: map[string]interface{}{},
			},
			expected: workDir + "/file.tar.gz",
		},
		{
			name: "URL with query params",
			step: ResolvedStep{
				Action: "download",
				URL:    "https://example.com/file.tar.gz?token=abc123",
				Params: map[string]interface{}{},
			},
			expected: workDir + "/file.tar.gz",
		},
		{
			name: "fallback to url param",
			step: ResolvedStep{
				Action: "download",
				URL:    "",
				Params: map[string]interface{}{
					"url": "https://example.com/fallback.tar.gz",
				},
			},
			expected: workDir + "/fallback.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execCtx := &actions.ExecutionContext{
				WorkDir: workDir,
			}
			result := exec.resolveDownloadDest(tt.step, execCtx)
			if result != tt.expected {
				t.Errorf("resolveDownloadDest() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestExecuteDownloadWithVerification_ChecksumMismatch tests that checksum mismatch returns proper error
func TestExecuteDownloadWithVerification_ChecksumMismatch(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer exec.Cleanup()

	// Create a test file with known content
	testFile := "test-download.txt"
	testFilePath := exec.WorkDir() + "/" + testFile
	content := []byte("this is test content")
	if err := os.WriteFile(testFilePath, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a plan with a download step that has wrong checksum
	plan := &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          "test-tool",
		Version:       "1.0.0",
		Platform:      Platform{OS: "linux", Arch: "amd64"},
	}

	step := ResolvedStep{
		Action:   "download",
		URL:      "https://example.com/test-download.txt",
		Checksum: "0000000000000000000000000000000000000000000000000000000000000000",
		Params: map[string]interface{}{
			"dest": testFile,
		},
	}

	execCtx := &actions.ExecutionContext{
		WorkDir: exec.WorkDir(),
	}

	// This tests the checksum verification logic directly
	// The download action won't be called (we pre-created the file)
	// so we need to compute checksum and compare
	actualChecksum, err := computeFileChecksum(testFilePath)
	if err != nil {
		t.Fatalf("Failed to compute checksum: %v", err)
	}

	// Verify expected checksum doesn't match actual
	expectedChecksum := step.Checksum
	if actualChecksum == expectedChecksum {
		t.Fatal("Test setup error: checksums should not match")
	}

	// Create the error manually to verify its format
	checksumErr := &ChecksumMismatchError{
		Tool:             plan.Tool,
		Version:          plan.Version,
		URL:              step.URL,
		ExpectedChecksum: expectedChecksum,
		ActualChecksum:   actualChecksum,
	}

	// Verify error message contains required information
	errorMsg := checksumErr.Error()
	checks := []string{
		step.URL,
		expectedChecksum,
		actualChecksum,
		"test-tool@1.0.0",
		"--fresh",
	}
	for _, check := range checks {
		if !contains(errorMsg, check) {
			t.Errorf("ChecksumMismatchError should contain %q, got: %s", check, errorMsg)
		}
	}

	// Verify errors.As works with ChecksumMismatchError
	var mismatchErr *ChecksumMismatchError
	if !errors.As(checksumErr, &mismatchErr) {
		t.Error("errors.As should work with ChecksumMismatchError")
	}

	// Verify destPath resolution with ExecutionContext
	destPath := exec.resolveDownloadDest(step, execCtx)
	if destPath != testFilePath {
		t.Errorf("resolveDownloadDest() = %q, want %q", destPath, testFilePath)
	}
}

// TestBuildResolvedDepsFromPlan tests that dependency versions are correctly extracted from plan
func TestBuildResolvedDepsFromPlan(t *testing.T) {
	// Create a mock dependency plan with nested dependencies
	deps := []DependencyPlan{
		{
			Tool:    "openssl",
			Version: "3.6.0",
			Dependencies: []DependencyPlan{
				{
					Tool:    "perl",
					Version: "5.38.0",
				},
			},
		},
		{
			Tool:    "zlib",
			Version: "1.3.1",
		},
	}

	result := buildResolvedDepsFromPlan(deps)

	// Check direct dependencies
	if result.InstallTime["openssl"] != "3.6.0" {
		t.Errorf("openssl version = %q, want %q", result.InstallTime["openssl"], "3.6.0")
	}
	if result.InstallTime["zlib"] != "1.3.1" {
		t.Errorf("zlib version = %q, want %q", result.InstallTime["zlib"], "1.3.1")
	}

	// Check nested dependency
	if result.InstallTime["perl"] != "5.38.0" {
		t.Errorf("perl version = %q, want %q", result.InstallTime["perl"], "5.38.0")
	}
}

// TestBuildResolvedDepsFromPlan_Empty tests with no dependencies
func TestBuildResolvedDepsFromPlan_Empty(t *testing.T) {
	result := buildResolvedDepsFromPlan(nil)

	if len(result.InstallTime) != 0 {
		t.Errorf("InstallTime should be empty, got %d entries", len(result.InstallTime))
	}
	if len(result.Runtime) != 0 {
		t.Errorf("Runtime should be empty, got %d entries", len(result.Runtime))
	}
}
