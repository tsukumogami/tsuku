package actions

import (
	"context"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestFossilArchiveAction_Name(t *testing.T) {
	action := &FossilArchiveAction{}
	if action.Name() != "fossil_archive" {
		t.Errorf("Name() = %q, want %q", action.Name(), "fossil_archive")
	}
}

func TestFossilArchiveAction_IsDeterministic(t *testing.T) {
	action := &FossilArchiveAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestFossilArchiveAction_VersionToTag(t *testing.T) {
	action := &FossilArchiveAction{}

	tests := []struct {
		name             string
		version          string
		tagPrefix        string
		versionSeparator string
		expected         string
	}{
		{
			name:             "SQLite default format",
			version:          "3.46.0",
			tagPrefix:        "version-",
			versionSeparator: ".",
			expected:         "version-3.46.0",
		},
		{
			name:             "Tcl with dash separator",
			version:          "9.0.0",
			tagPrefix:        "core-",
			versionSeparator: "-",
			expected:         "core-9-0-0",
		},
		{
			name:             "Empty separator (no conversion)",
			version:          "3.46.0",
			tagPrefix:        "version-",
			versionSeparator: "",
			expected:         "version-3.46.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := action.versionToTag(tt.version, tt.tagPrefix, tt.versionSeparator)
			if result != tt.expected {
				t.Errorf("versionToTag(%q, %q, %q) = %q, want %q",
					tt.version, tt.tagPrefix, tt.versionSeparator, result, tt.expected)
			}
		})
	}
}

func TestFossilArchiveAction_Preflight(t *testing.T) {
	action := &FossilArchiveAction{}

	tests := []struct {
		name        string
		params      map[string]interface{}
		expectError bool
		errorCount  int
	}{
		{
			name: "Valid params",
			params: map[string]interface{}{
				"repo":         "https://sqlite.org/src",
				"project_name": "sqlite",
			},
			expectError: false,
			errorCount:  0,
		},
		{
			name: "Missing repo",
			params: map[string]interface{}{
				"project_name": "sqlite",
			},
			expectError: true,
			errorCount:  1,
		},
		{
			name: "Missing project_name",
			params: map[string]interface{}{
				"repo": "https://sqlite.org/src",
			},
			expectError: true,
			errorCount:  1,
		},
		{
			name: "HTTP repo (not HTTPS)",
			params: map[string]interface{}{
				"repo":         "http://sqlite.org/src",
				"project_name": "sqlite",
			},
			expectError: true,
			errorCount:  1,
		},
		{
			name:        "All missing",
			params:      map[string]interface{}{},
			expectError: true,
			errorCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := action.Preflight(tt.params)

			hasErrors := len(result.Errors) > 0
			if hasErrors != tt.expectError {
				t.Errorf("Preflight() hasErrors = %v, want %v", hasErrors, tt.expectError)
			}

			if len(result.Errors) != tt.errorCount {
				t.Errorf("Preflight() error count = %d, want %d", len(result.Errors), tt.errorCount)
				for _, err := range result.Errors {
					t.Logf("  Error: %s", err)
				}
			}
		})
	}
}

func TestFossilArchiveAction_Decompose(t *testing.T) {
	action := &FossilArchiveAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "3.46.0",
		VersionTag: "version-3.46.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	params := map[string]interface{}{
		"repo":         "https://sqlite.org/src",
		"project_name": "sqlite",
		"strip_dirs":   1,
	}

	steps, err := action.Decompose(ctx, params)
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}

	// Should have 2 steps: download_file, extract
	// (No chmod/install_binaries because Fossil archives contain source code)
	if len(steps) != 2 {
		t.Errorf("Decompose() returned %d steps, want 2", len(steps))
		for i, step := range steps {
			t.Logf("  Step %d: %s", i, step.Action)
		}
		return
	}

	// Verify step actions
	expectedActions := []string{"download_file", "extract"}
	for i, expected := range expectedActions {
		if steps[i].Action != expected {
			t.Errorf("Step %d action = %q, want %q", i, steps[i].Action, expected)
		}
	}

	// Verify download URL
	downloadURL, _ := GetString(steps[0].Params, "url")
	expectedURL := "https://sqlite.org/src/tarball/version-3.46.0/sqlite.tar.gz"
	if downloadURL != expectedURL {
		t.Errorf("Download URL = %q, want %q", downloadURL, expectedURL)
	}

	// Verify archive filename
	archiveFilename, _ := GetString(steps[0].Params, "dest")
	if archiveFilename != "sqlite.tar.gz" {
		t.Errorf("Archive filename = %q, want %q", archiveFilename, "sqlite.tar.gz")
	}
}

func TestFossilArchiveAction_Decompose_WithCustomTagFormat(t *testing.T) {
	action := &FossilArchiveAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "9.0.0",
		VersionTag: "core-9-0-0",
		OS:         "linux",
		Arch:       "amd64",
	}

	params := map[string]interface{}{
		"repo":              "https://core.tcl-lang.org/tcl",
		"project_name":      "tcl",
		"tag_prefix":        "core-",
		"version_separator": "-",
	}

	steps, err := action.Decompose(ctx, params)
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}

	if len(steps) != 2 {
		t.Fatalf("Decompose() returned %d steps, want 2", len(steps))
	}

	// Verify download URL with custom tag format
	downloadURL, _ := GetString(steps[0].Params, "url")
	expectedURL := "https://core.tcl-lang.org/tcl/tarball/core-9-0-0/tcl.tar.gz"
	if downloadURL != expectedURL {
		t.Errorf("Download URL = %q, want %q", downloadURL, expectedURL)
	}
}

func TestFossilArchiveAction_Decompose_MissingParams(t *testing.T) {
	action := &FossilArchiveAction{}

	ctx := &EvalContext{
		Context: context.Background(),
		Version: "3.46.0",
	}

	tests := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name: "Missing repo",
			params: map[string]interface{}{
				"project_name": "sqlite",
			},
		},
		{
			name: "Missing project_name",
			params: map[string]interface{}{
				"repo": "https://sqlite.org/src",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := action.Decompose(ctx, tt.params)
			if err == nil {
				t.Error("Decompose() expected error, got nil")
			}
		})
	}
}

// -- fossil_archive.go: Execute early validation --

func TestFossilArchiveAction_Execute_MissingRepo(t *testing.T) {
	t.Parallel()
	action := &FossilArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "repo") {
		t.Errorf("Expected repo error, got %v", err)
	}
}

// -- fossil_archive.go: versionToTag --

func TestFossilArchiveAction_VersionToTag_Direct(t *testing.T) {
	t.Parallel()
	action := &FossilArchiveAction{}

	tests := []struct {
		version          string
		tagPrefix        string
		versionSeparator string
		want             string
	}{
		{"3.46.0", "version-", ".", "version-3.46.0"},
		{"9.0.0", "core-", "-", "core-9-0-0"},
		{"1.2.3", "", ".", "1.2.3"},
		{"1.2.3", "v", ".", "v1.2.3"},
		{"1.2.3", "release-", "_", "release-1_2_3"},
	}
	for _, tt := range tests {
		got := action.versionToTag(tt.version, tt.tagPrefix, tt.versionSeparator)
		if got != tt.want {
			t.Errorf("versionToTag(%q, %q, %q) = %q, want %q",
				tt.version, tt.tagPrefix, tt.versionSeparator, got, tt.want)
		}
	}
}

// -- fossil_archive.go: Execute param validation --

func TestFossilArchiveAction_Execute_MissingProjectName(t *testing.T) {
	t.Parallel()
	action := &FossilArchiveAction{}
	ctx := newTestExecCtx(t)
	err := action.Execute(ctx, map[string]any{
		"repo": "https://sqlite.org/src",
	})
	if err == nil {
		t.Error("Expected error for missing project_name")
	}
}

// -- fossil_archive.go: Execute with full params (fails at download) --

func TestFossilArchiveAction_Execute_FullParams(t *testing.T) {
	t.Parallel()
	action := &FossilArchiveAction{}
	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: t.TempDir(),
		Version: "3.46.0",
		OS:      "linux",
		Arch:    "amd64",
		Recipe:  &recipe.Recipe{},
	}
	err := action.Execute(ctx, map[string]any{
		"repo":              "https://nonexistent.invalid/src",
		"project_name":      "test",
		"strip_dirs":        1,
		"tag_prefix":        "version-",
		"version_separator": "-",
	})
	// Should fail at download, not at param validation
	if err == nil {
		t.Error("Expected error (download should fail)")
	}
}

// -- fossil_archive.go: Decompose with custom tag format --

func TestFossilArchiveAction_Decompose_DefaultParams(t *testing.T) {
	t.Parallel()
	action := &FossilArchiveAction{}
	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "3.46.0",
		VersionTag: "version-3.46.0",
		OS:         "linux",
		Arch:       "amd64",
	}
	steps, err := action.Decompose(ctx, map[string]any{
		"repo":         "https://nonexistent.invalid/src",
		"project_name": "test",
	})
	if err != nil {
		t.Fatalf("Decompose() error = %v", err)
	}
	if len(steps) != 2 {
		t.Errorf("Decompose() returned %d steps, want 2", len(steps))
	}
}

func TestFossilArchiveAction_IsDeterministic_Direct(t *testing.T) {
	t.Parallel()
	action := FossilArchiveAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

// -- fossil_archive.go: Execute error paths --

func TestFossilArchiveAction_Execute_MissingParams(t *testing.T) {
	t.Parallel()
	action := &FossilArchiveAction{}
	ctx := &ExecutionContext{
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
	}

	t.Run("missing repo", func(t *testing.T) {
		err := action.Execute(ctx, map[string]any{
			"project_name": "test",
		})
		if err == nil {
			t.Error("Expected error for missing repo")
		}
	})

	t.Run("missing project_name", func(t *testing.T) {
		err := action.Execute(ctx, map[string]any{
			"repo": "https://example.com/src",
		})
		if err == nil {
			t.Error("Expected error for missing project_name")
		}
	})
}
