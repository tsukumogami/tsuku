package actions

import (
	"context"
	"testing"
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
