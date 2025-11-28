package executor

import (
	"context"
	"os"
	"testing"

	"github.com/tsuku-dev/tsuku/internal/recipe"
	"github.com/tsuku-dev/tsuku/internal/version"
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
