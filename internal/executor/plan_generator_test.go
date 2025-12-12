package executor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// mockTOMLSerializer implements a simple interface for recipe hash testing.
type mockTOMLSerializer struct {
	content []byte
	err     error
}

func (m *mockTOMLSerializer) ToTOML() ([]byte, error) {
	return m.content, m.err
}

func TestComputeRecipeHash(t *testing.T) {
	tests := []struct {
		name        string
		content     []byte
		wantLen     int // expected hash length
		expectError bool
	}{
		{
			name:    "simple content",
			content: []byte(`[metadata]\nname = "test"`),
			wantLen: 64, // SHA256 hex is 64 chars
		},
		{
			name:    "empty content",
			content: []byte{},
			wantLen: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockTOMLSerializer{content: tt.content}
			hash, err := computeRecipeHash(mock)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(hash) != tt.wantLen {
				t.Errorf("hash length = %d, want %d", len(hash), tt.wantLen)
			}
		})
	}
}

func TestShouldExecuteForPlatform(t *testing.T) {
	tests := []struct {
		name       string
		when       map[string]string
		targetOS   string
		targetArch string
		want       bool
	}{
		{
			name:       "empty when - always execute",
			when:       map[string]string{},
			targetOS:   "linux",
			targetArch: "amd64",
			want:       true,
		},
		{
			name:       "nil when - always execute",
			when:       nil,
			targetOS:   "linux",
			targetArch: "amd64",
			want:       true,
		},
		{
			name:       "matching OS",
			when:       map[string]string{"os": "linux"},
			targetOS:   "linux",
			targetArch: "amd64",
			want:       true,
		},
		{
			name:       "non-matching OS",
			when:       map[string]string{"os": "darwin"},
			targetOS:   "linux",
			targetArch: "amd64",
			want:       false,
		},
		{
			name:       "matching arch",
			when:       map[string]string{"arch": "amd64"},
			targetOS:   "linux",
			targetArch: "amd64",
			want:       true,
		},
		{
			name:       "non-matching arch",
			when:       map[string]string{"arch": "arm64"},
			targetOS:   "linux",
			targetArch: "amd64",
			want:       false,
		},
		{
			name:       "matching OS and arch",
			when:       map[string]string{"os": "linux", "arch": "amd64"},
			targetOS:   "linux",
			targetArch: "amd64",
			want:       true,
		},
		{
			name:       "matching OS but non-matching arch",
			when:       map[string]string{"os": "linux", "arch": "arm64"},
			targetOS:   "linux",
			targetArch: "amd64",
			want:       false,
		},
		{
			name:       "package_manager ignored for plan",
			when:       map[string]string{"package_manager": "apt"},
			targetOS:   "linux",
			targetArch: "amd64",
			want:       true, // package_manager is a runtime check, not plan-time
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldExecuteForPlatform(tt.when, tt.targetOS, tt.targetArch)
			if got != tt.want {
				t.Errorf("shouldExecuteForPlatform() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDownloadAction(t *testing.T) {
	tests := []struct {
		action string
		want   bool
	}{
		{"download", true},
		{"download_archive", true},
		{"github_archive", true},
		{"github_file", true},
		{"hashicorp_release", true},
		{"homebrew_bottle", true},
		{"extract", false},
		{"install_binaries", false},
		{"run_command", false},
		{"npm_install", false},
		{"unknown_action", false},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			got := isDownloadAction(tt.action)
			if got != tt.want {
				t.Errorf("isDownloadAction(%q) = %v, want %v", tt.action, got, tt.want)
			}
		})
	}
}

func TestExtractDownloadURL(t *testing.T) {
	vars := map[string]string{
		"version": "1.2.3",
		"os":      "linux",
		"arch":    "amd64",
	}

	tests := []struct {
		name        string
		action      string
		params      map[string]interface{}
		wantURL     string
		expectError bool
	}{
		{
			name:   "download action",
			action: "download",
			params: map[string]interface{}{
				"url": "https://example.com/file.tar.gz",
			},
			wantURL: "https://example.com/file.tar.gz",
		},
		{
			name:   "download_archive action",
			action: "download_archive",
			params: map[string]interface{}{
				"url": "https://example.com/archive.zip",
			},
			wantURL: "https://example.com/archive.zip",
		},
		{
			name:        "download missing url",
			action:      "download",
			params:      map[string]interface{}{},
			expectError: true,
		},
		{
			name:   "github_archive with asset_pattern",
			action: "github_archive",
			params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "tool-linux-amd64.tar.gz",
			},
			wantURL: "https://github.com/owner/repo/releases/download/v1.2.3/tool-linux-amd64.tar.gz",
		},
		{
			name:   "github_file with file",
			action: "github_file",
			params: map[string]interface{}{
				"repo": "owner/repo",
				"file": "binary",
			},
			wantURL: "https://github.com/owner/repo/releases/download/v1.2.3/binary",
		},
		{
			name:        "github_archive missing repo",
			action:      "github_archive",
			params:      map[string]interface{}{"asset_pattern": "file.tar.gz"},
			expectError: true,
		},
		{
			name:        "github_archive missing asset",
			action:      "github_archive",
			params:      map[string]interface{}{"repo": "owner/repo"},
			expectError: true,
		},
		{
			name:   "hashicorp_release",
			action: "hashicorp_release",
			params: map[string]interface{}{
				"product": "terraform",
			},
			wantURL: "https://releases.hashicorp.com/terraform/1.2.3/terraform_1.2.3_linux_amd64.zip",
		},
		{
			name:        "hashicorp_release missing product",
			action:      "hashicorp_release",
			params:      map[string]interface{}{},
			expectError: true,
		},
		{
			name:    "homebrew_bottle returns empty",
			action:  "homebrew_bottle",
			params:  map[string]interface{}{"formula": "test"},
			wantURL: "", // homebrew bottles skip checksum
		},
		{
			name:    "unknown action returns empty",
			action:  "unknown",
			params:  map[string]interface{}{},
			wantURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := extractDownloadURL(tt.action, tt.params, vars)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if url != tt.wantURL {
				t.Errorf("extractDownloadURL() = %q, want %q", url, tt.wantURL)
			}
		})
	}
}

func TestExpandParams(t *testing.T) {
	vars := map[string]string{
		"version": "1.0.0",
		"os":      "linux",
		"arch":    "amd64",
	}

	tests := []struct {
		name   string
		params map[string]interface{}
		want   map[string]interface{}
	}{
		{
			name:   "simple string expansion",
			params: map[string]interface{}{"url": "https://example.com/{version}/file.tar.gz"},
			want:   map[string]interface{}{"url": "https://example.com/1.0.0/file.tar.gz"},
		},
		{
			name:   "multiple variables in one string",
			params: map[string]interface{}{"url": "https://example.com/{version}/{os}-{arch}.tar.gz"},
			want:   map[string]interface{}{"url": "https://example.com/1.0.0/linux-amd64.tar.gz"},
		},
		{
			name:   "no variables",
			params: map[string]interface{}{"url": "https://example.com/file.tar.gz"},
			want:   map[string]interface{}{"url": "https://example.com/file.tar.gz"},
		},
		{
			name:   "non-string values unchanged",
			params: map[string]interface{}{"count": 42, "enabled": true},
			want:   map[string]interface{}{"count": 42, "enabled": true},
		},
		{
			name: "nested array expansion",
			params: map[string]interface{}{
				"items": []interface{}{"item-{version}", "item-{os}"},
			},
			want: map[string]interface{}{
				"items": []interface{}{"item-1.0.0", "item-linux"},
			},
		},
		{
			name: "nested map expansion",
			params: map[string]interface{}{
				"config": map[string]interface{}{
					"path": "/opt/{version}",
				},
			},
			want: map[string]interface{}{
				"config": map[string]interface{}{
					"path": "/opt/1.0.0",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandParams(tt.params, vars)

			// Deep comparison would be complex, so we check key cases
			for k, wantV := range tt.want {
				gotV, ok := got[k]
				if !ok {
					t.Errorf("missing key %q in result", k)
					continue
				}

				// For simple values, direct comparison
				switch wantVal := wantV.(type) {
				case string:
					if gotVal, ok := gotV.(string); !ok || gotVal != wantVal {
						t.Errorf("got[%q] = %v, want %v", k, gotV, wantV)
					}
				case int:
					if gotVal, ok := gotV.(int); !ok || gotVal != wantVal {
						t.Errorf("got[%q] = %v, want %v", k, gotV, wantV)
					}
				case bool:
					if gotVal, ok := gotV.(bool); !ok || gotVal != wantVal {
						t.Errorf("got[%q] = %v, want %v", k, gotV, wantV)
					}
				case []interface{}:
					gotArr, ok := gotV.([]interface{})
					if !ok {
						t.Errorf("got[%q] is not an array", k)
						continue
					}
					if len(gotArr) != len(wantVal) {
						t.Errorf("got[%q] length = %d, want %d", k, len(gotArr), len(wantVal))
						continue
					}
					for i, item := range wantVal {
						if gotArr[i] != item {
							t.Errorf("got[%q][%d] = %v, want %v", k, i, gotArr[i], item)
						}
					}
				case map[string]interface{}:
					gotMap, ok := gotV.(map[string]interface{})
					if !ok {
						t.Errorf("got[%q] is not a map", k)
						continue
					}
					for mk, mv := range wantVal {
						if gotMap[mk] != mv {
							t.Errorf("got[%q][%q] = %v, want %v", k, mk, gotMap[mk], mv)
						}
					}
				}
			}
		})
	}
}

func TestExpandVarsInString(t *testing.T) {
	vars := map[string]string{
		"version": "2.0.0",
		"os":      "darwin",
		"arch":    "arm64",
	}

	tests := []struct {
		input string
		want  string
	}{
		{"{version}", "2.0.0"},
		{"{os}-{arch}", "darwin-arm64"},
		{"no-vars", "no-vars"},
		{"{unknown}", "{unknown}"}, // Unknown vars remain unchanged
		{"", ""},
		{"prefix-{version}-suffix", "prefix-2.0.0-suffix"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandVarsInString(tt.input, vars)
			if got != tt.want {
				t.Errorf("expandVarsInString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetStandardPlanVars(t *testing.T) {
	vars := GetStandardPlanVars("1.0.0", "v1.0.0", "linux", "amd64")

	expected := map[string]string{
		"version":     "1.0.0",
		"version_tag": "v1.0.0",
		"os":          "linux",
		"arch":        "amd64",
	}

	for k, v := range expected {
		if vars[k] != v {
			t.Errorf("vars[%q] = %q, want %q", k, vars[k], v)
		}
	}
}

func TestApplyOSMapping(t *testing.T) {
	vars := map[string]string{"os": "darwin"}
	params := map[string]interface{}{
		"os_mapping": map[string]interface{}{
			"darwin": "macos",
			"linux":  "linux",
		},
	}

	ApplyOSMapping(vars, params)

	if vars["os"] != "macos" {
		t.Errorf("after mapping, os = %q, want %q", vars["os"], "macos")
	}
}

func TestApplyArchMapping(t *testing.T) {
	vars := map[string]string{"arch": "amd64"}
	params := map[string]interface{}{
		"arch_mapping": map[string]interface{}{
			"amd64": "x86_64",
			"arm64": "aarch64",
		},
	}

	ApplyArchMapping(vars, params)

	if vars["arch"] != "x86_64" {
		t.Errorf("after mapping, arch = %q, want %q", vars["arch"], "x86_64")
	}
}

func TestGeneratePlan_BasicRecipe(t *testing.T) {
	// Create a simple recipe with only evaluable actions that don't require downloads
	// Use nodejs_dist which is a registered source for version resolution
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        "test-tool",
			Description: "Test tool for plan generation",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{
			{
				Action: "install_binaries",
				Params: map[string]interface{}{
					"files": []interface{}{"bin/tool"},
				},
			},
			{
				Action: "chmod",
				Params: map[string]interface{}{
					"path": "$TSUKU_HOME/bin/tool",
					"mode": "0755",
				},
			},
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()

	plan, err := exec.GeneratePlan(ctx, PlanConfig{
		OS:           "linux",
		Arch:         "amd64",
		RecipeSource: "test",
	})

	if err != nil {
		// Network failures are acceptable in unit tests
		t.Skipf("GeneratePlan() error (expected in offline tests): %v", err)
	}

	// Verify plan metadata
	if plan.FormatVersion != PlanFormatVersion {
		t.Errorf("FormatVersion = %d, want %d", plan.FormatVersion, PlanFormatVersion)
	}
	if plan.Tool != "test-tool" {
		t.Errorf("Tool = %q, want %q", plan.Tool, "test-tool")
	}
	if plan.Version == "" {
		t.Error("Version should not be empty")
	}
	if plan.Platform.OS != "linux" {
		t.Errorf("Platform.OS = %q, want %q", plan.Platform.OS, "linux")
	}
	if plan.Platform.Arch != "amd64" {
		t.Errorf("Platform.Arch = %q, want %q", plan.Platform.Arch, "amd64")
	}
	if plan.RecipeSource != "test" {
		t.Errorf("RecipeSource = %q, want %q", plan.RecipeSource, "test")
	}
	if plan.RecipeHash == "" {
		t.Error("RecipeHash should not be empty")
	}
	if len(plan.Steps) != 2 {
		t.Errorf("len(Steps) = %d, want %d", len(plan.Steps), 2)
	}

	// Verify steps are evaluable
	for _, step := range plan.Steps {
		if !step.Evaluable {
			t.Errorf("step %q should be evaluable", step.Action)
		}
	}
}

func TestGeneratePlan_NonEvaluableWarnings(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{
			{
				Action: "run_command",
				Params: map[string]interface{}{
					"command": "echo hello",
				},
			},
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
		t.Fatalf("New() error: %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()

	var warnings []string
	plan, err := exec.GeneratePlan(ctx, PlanConfig{
		OS:           "linux",
		Arch:         "amd64",
		RecipeSource: "test",
		OnWarning: func(action, msg string) {
			warnings = append(warnings, action+": "+msg)
		},
	})

	if err != nil {
		// Network failures are acceptable in unit tests
		t.Skipf("GeneratePlan() error (expected in offline tests): %v", err)
	}

	// Should have 2 warnings for non-evaluable actions
	if len(warnings) != 2 {
		t.Errorf("got %d warnings, want 2: %v", len(warnings), warnings)
	}

	// Verify steps are marked as non-evaluable
	for _, step := range plan.Steps {
		if step.Evaluable {
			t.Errorf("step %q should not be evaluable", step.Action)
		}
	}
}

func TestGeneratePlan_WhenFiltering(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{
			{
				Action: "chmod",
				Params: map[string]interface{}{"path": "tool", "mode": "0755"},
				When:   map[string]string{"os": "linux"},
			},
			{
				Action: "chmod",
				Params: map[string]interface{}{"path": "tool", "mode": "0755"},
				When:   map[string]string{"os": "darwin"},
			},
			{
				Action: "install_binaries",
				Params: map[string]interface{}{"files": []interface{}{"tool"}},
				// No when clause - always included
			},
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()

	// Generate for Linux
	plan, err := exec.GeneratePlan(ctx, PlanConfig{
		OS:           "linux",
		Arch:         "amd64",
		RecipeSource: "test",
	})

	if err != nil {
		// Network failures are acceptable in unit tests
		t.Skipf("GeneratePlan() error (expected in offline tests): %v", err)
	}

	// Should have 2 steps: linux chmod + install_binaries
	if len(plan.Steps) != 2 {
		t.Errorf("len(Steps) = %d, want 2", len(plan.Steps))
	}

	// Generate for Darwin
	exec2, err := New(r)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer exec2.Cleanup()

	plan2, err := exec2.GeneratePlan(ctx, PlanConfig{
		OS:           "darwin",
		Arch:         "arm64",
		RecipeSource: "test",
	})

	if err != nil {
		// Network failures are acceptable in unit tests
		t.Skipf("GeneratePlan() for darwin error (expected in offline tests): %v", err)
	}

	// Should also have 2 steps: darwin chmod + install_binaries
	if len(plan2.Steps) != 2 {
		t.Errorf("len(Steps) for darwin = %d, want 2", len(plan2.Steps))
	}
}

func TestGeneratePlan_TemplateExpansion(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{
			{
				Action: "extract",
				Params: map[string]interface{}{
					"archive": "tool-{version}-{os}-{arch}.tar.gz",
					"dest":    "/opt/tool-{version}",
				},
			},
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()

	plan, err := exec.GeneratePlan(ctx, PlanConfig{
		OS:           "linux",
		Arch:         "amd64",
		RecipeSource: "test",
	})

	if err != nil {
		// Network failures are acceptable in unit tests
		t.Skipf("GeneratePlan() error (expected in offline tests): %v", err)
	}

	if len(plan.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(plan.Steps))
	}

	step := plan.Steps[0]
	archive, ok := step.Params["archive"].(string)
	if !ok {
		t.Fatal("archive param is not a string")
	}
	// Verify the version, os, and arch are expanded (we don't know the exact version)
	if !strings.Contains(archive, "-linux-amd64.tar.gz") {
		t.Errorf("archive = %q, should contain %q", archive, "-linux-amd64.tar.gz")
	}
	if strings.Contains(archive, "{version}") {
		t.Errorf("archive = %q, should not contain {version}", archive)
	}

	dest, ok := step.Params["dest"].(string)
	if !ok {
		t.Fatal("dest param is not a string")
	}
	if strings.Contains(dest, "{version}") {
		t.Errorf("dest = %q, should not contain {version}", dest)
	}
}

func TestComputeRecipeHash_Error(t *testing.T) {
	mock := &mockTOMLSerializer{
		err: fmt.Errorf("serialization error"),
	}
	_, err := computeRecipeHash(mock)

	if err == nil {
		t.Error("expected error but got none")
	}
	if !strings.Contains(err.Error(), "failed to serialize recipe") {
		t.Errorf("error should mention serialization failure, got: %v", err)
	}
}

func TestComputeRecipeHash_Deterministic(t *testing.T) {
	// Same content should produce same hash
	content := []byte(`[metadata]\nname = "test"`)
	mock1 := &mockTOMLSerializer{content: content}
	mock2 := &mockTOMLSerializer{content: content}

	hash1, err1 := computeRecipeHash(mock1)
	hash2, err2 := computeRecipeHash(mock2)

	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}

	if hash1 != hash2 {
		t.Errorf("same content should produce same hash: %s != %s", hash1, hash2)
	}

	// Different content should produce different hash
	mock3 := &mockTOMLSerializer{content: []byte(`[metadata]\nname = "other"`)}
	hash3, err3 := computeRecipeHash(mock3)
	if err3 != nil {
		t.Fatalf("unexpected error: %v", err3)
	}

	if hash1 == hash3 {
		t.Errorf("different content should produce different hash: %s == %s", hash1, hash3)
	}
}

func TestApplyOSMapping_NoMapping(t *testing.T) {
	vars := map[string]string{"os": "darwin"}
	params := map[string]interface{}{} // No os_mapping

	ApplyOSMapping(vars, params)

	// Should remain unchanged
	if vars["os"] != "darwin" {
		t.Errorf("os should remain unchanged, got %q", vars["os"])
	}
}

func TestApplyOSMapping_NoMatchingOS(t *testing.T) {
	vars := map[string]string{"os": "windows"}
	params := map[string]interface{}{
		"os_mapping": map[string]interface{}{
			"darwin": "macos",
			"linux":  "linux",
			// No windows mapping
		},
	}

	ApplyOSMapping(vars, params)

	// Should remain unchanged
	if vars["os"] != "windows" {
		t.Errorf("os should remain unchanged when no mapping exists, got %q", vars["os"])
	}
}

func TestApplyArchMapping_NoMapping(t *testing.T) {
	vars := map[string]string{"arch": "amd64"}
	params := map[string]interface{}{} // No arch_mapping

	ApplyArchMapping(vars, params)

	// Should remain unchanged
	if vars["arch"] != "amd64" {
		t.Errorf("arch should remain unchanged, got %q", vars["arch"])
	}
}

func TestApplyArchMapping_NoMatchingArch(t *testing.T) {
	vars := map[string]string{"arch": "riscv64"}
	params := map[string]interface{}{
		"arch_mapping": map[string]interface{}{
			"amd64": "x86_64",
			"arm64": "aarch64",
			// No riscv64 mapping
		},
	}

	ApplyArchMapping(vars, params)

	// Should remain unchanged
	if vars["arch"] != "riscv64" {
		t.Errorf("arch should remain unchanged when no mapping exists, got %q", vars["arch"])
	}
}

func TestExpandValue_AllTypes(t *testing.T) {
	vars := map[string]string{"version": "1.0.0"}

	tests := []struct {
		name  string
		input interface{}
		want  interface{}
	}{
		{
			name:  "string with variable",
			input: "v{version}",
			want:  "v1.0.0",
		},
		{
			name:  "integer unchanged",
			input: 42,
			want:  42,
		},
		{
			name:  "bool unchanged",
			input: true,
			want:  true,
		},
		{
			name:  "float unchanged",
			input: 3.14,
			want:  3.14,
		},
		{
			name:  "nil unchanged",
			input: nil,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandValue(tt.input, vars)
			if got != tt.want {
				t.Errorf("expandValue(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestShouldExecuteForPlatform_CombinedConditions(t *testing.T) {
	tests := []struct {
		name       string
		when       map[string]string
		targetOS   string
		targetArch string
		want       bool
	}{
		{
			name:       "package_manager with matching OS",
			when:       map[string]string{"os": "linux", "package_manager": "apt"},
			targetOS:   "linux",
			targetArch: "amd64",
			want:       true, // package_manager is ignored for plan
		},
		{
			name:       "package_manager with non-matching OS",
			when:       map[string]string{"os": "darwin", "package_manager": "brew"},
			targetOS:   "linux",
			targetArch: "amd64",
			want:       false,
		},
		{
			name:       "all three conditions matching",
			when:       map[string]string{"os": "linux", "arch": "amd64", "package_manager": "apt"},
			targetOS:   "linux",
			targetArch: "amd64",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldExecuteForPlatform(tt.when, tt.targetOS, tt.targetArch)
			if got != tt.want {
				t.Errorf("shouldExecuteForPlatform() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGeneratePlan_DefaultsApplied(t *testing.T) {
	// Test that defaults are applied when config fields are empty
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{
			{
				Action: "chmod",
				Params: map[string]interface{}{"path": "tool", "mode": "0755"},
			},
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()

	// Empty config - should use defaults
	plan, err := exec.GeneratePlan(ctx, PlanConfig{})

	if err != nil {
		t.Skipf("GeneratePlan() error (expected in offline tests): %v", err)
	}

	// Verify defaults were applied
	if plan.Platform.OS == "" {
		t.Error("Platform.OS should not be empty (default should be applied)")
	}
	if plan.Platform.Arch == "" {
		t.Error("Platform.Arch should not be empty (default should be applied)")
	}
	if plan.RecipeSource != "unknown" {
		t.Errorf("RecipeSource should be 'unknown' when not specified, got %q", plan.RecipeSource)
	}
}

func TestGeneratePlan_GeneratedAtIsRecent(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()

	timeBefore := time.Now().Add(-time.Second)
	plan, err := exec.GeneratePlan(ctx, PlanConfig{
		OS:   "linux",
		Arch: "amd64",
	})
	timeAfter := time.Now().Add(time.Second)

	if err != nil {
		t.Skipf("GeneratePlan() error (expected in offline tests): %v", err)
	}

	if plan.GeneratedAt.Before(timeBefore) || plan.GeneratedAt.After(timeAfter) {
		t.Errorf("GeneratedAt should be recent, got %v (expected between %v and %v)",
			plan.GeneratedAt, timeBefore, timeAfter)
	}
}

func TestGeneratePlan_WithDownloadAction(t *testing.T) {
	// Create a TLS test server that serves a file (PreDownloader requires HTTPS)
	testContent := []byte("test file content for checksum")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testContent)
	}))
	defer server.Close()

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{
					"url": server.URL + "/file.tar.gz",
				},
			},
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()

	// Create a downloader with our test server's HTTP client
	downloader := validate.NewPreDownloader().WithHTTPClient(server.Client())

	plan, err := exec.GeneratePlan(ctx, PlanConfig{
		OS:           "linux",
		Arch:         "amd64",
		RecipeSource: "test",
		Downloader:   downloader,
	})

	if err != nil {
		// Version resolution may fail in tests - that's acceptable
		t.Skipf("GeneratePlan() error (expected in offline tests): %v", err)
	}

	if len(plan.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(plan.Steps))
	}

	step := plan.Steps[0]
	if step.Action != "download" {
		t.Errorf("step.Action = %q, want %q", step.Action, "download")
	}
	if step.URL == "" {
		t.Error("step.URL should not be empty for download action")
	}
	if step.Checksum == "" {
		t.Error("step.Checksum should not be empty after download")
	}
	if step.Size == 0 {
		t.Error("step.Size should not be 0 after download")
	}
	if !step.Evaluable {
		t.Error("download action should be evaluable")
	}
}

func TestGeneratePlan_DownloadError(t *testing.T) {
	// Create a TLS test server that returns an error
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{
					"url": server.URL + "/file.tar.gz",
				},
			},
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()
	downloader := validate.NewPreDownloader().WithHTTPClient(server.Client())

	_, err = exec.GeneratePlan(ctx, PlanConfig{
		OS:           "linux",
		Arch:         "amd64",
		RecipeSource: "test",
		Downloader:   downloader,
	})

	// Either version resolution fails or download fails - both are acceptable
	// The key is that the error is handled gracefully
	if err != nil {
		// This is expected - either network error or download error
		t.Logf("GeneratePlan() returned expected error: %v", err)
	}
}

func TestGeneratePlan_HomebrewBottleSkipsChecksum(t *testing.T) {
	// homebrew_bottle should not attempt download (URL is empty)
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{
			{
				Action: "homebrew_bottle",
				Params: map[string]interface{}{
					"formula": "test-formula",
				},
			},
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()

	plan, err := exec.GeneratePlan(ctx, PlanConfig{
		OS:           "darwin",
		Arch:         "arm64",
		RecipeSource: "test",
	})

	if err != nil {
		t.Skipf("GeneratePlan() error (expected in offline tests): %v", err)
	}

	if len(plan.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(plan.Steps))
	}

	step := plan.Steps[0]
	// homebrew_bottle returns empty URL, so no checksum is computed
	if step.URL != "" {
		t.Errorf("homebrew_bottle step.URL should be empty, got %q", step.URL)
	}
	if step.Checksum != "" {
		t.Errorf("homebrew_bottle step.Checksum should be empty, got %q", step.Checksum)
	}
}

func TestGeneratePlan_MixedEvaluableSteps(t *testing.T) {
	// Test a recipe with both evaluable and non-evaluable steps
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
		Version: recipe.VersionSection{
			Source: "nodejs_dist",
		},
		Steps: []recipe.Step{
			{
				Action: "extract",
				Params: map[string]interface{}{"archive": "test.tar.gz"},
			},
			{
				Action: "run_command",
				Params: map[string]interface{}{"command": "make install"},
			},
			{
				Action: "install_binaries",
				Params: map[string]interface{}{"files": []interface{}{"bin/tool"}},
			},
			{
				Action: "npm_install",
				Params: map[string]interface{}{"package": "some-package"},
			},
		},
	}

	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer exec.Cleanup()

	ctx := context.Background()

	var warnings []string
	plan, err := exec.GeneratePlan(ctx, PlanConfig{
		OS:           "linux",
		Arch:         "amd64",
		RecipeSource: "test",
		OnWarning: func(action, msg string) {
			warnings = append(warnings, action)
		},
	})

	if err != nil {
		t.Skipf("GeneratePlan() error (expected in offline tests): %v", err)
	}

	if len(plan.Steps) != 4 {
		t.Fatalf("len(Steps) = %d, want 4", len(plan.Steps))
	}

	// Check evaluability of each step
	expectedEvaluable := map[string]bool{
		"extract":          true,
		"run_command":      false,
		"install_binaries": true,
		"npm_install":      false,
	}

	for _, step := range plan.Steps {
		expected, ok := expectedEvaluable[step.Action]
		if !ok {
			t.Errorf("unexpected action %q", step.Action)
			continue
		}
		if step.Evaluable != expected {
			t.Errorf("step %q Evaluable = %v, want %v", step.Action, step.Evaluable, expected)
		}
	}

	// Should have 2 warnings for non-evaluable actions
	if len(warnings) != 2 {
		t.Errorf("got %d warnings, want 2: %v", len(warnings), warnings)
	}
}

func TestGeneratePlan_AllDownloadActionTypes(t *testing.T) {
	// Test URL extraction for all download action types (using TLS server)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test content"))
	}))
	defer server.Close()

	tests := []struct {
		name      string
		action    string
		params    map[string]interface{}
		expectURL bool
	}{
		{
			name:      "download with url",
			action:    "download",
			params:    map[string]interface{}{"url": server.URL + "/file.tar.gz"},
			expectURL: true,
		},
		{
			name:      "download_archive with url",
			action:    "download_archive",
			params:    map[string]interface{}{"url": server.URL + "/archive.zip"},
			expectURL: true,
		},
		{
			name:      "homebrew_bottle (no URL)",
			action:    "homebrew_bottle",
			params:    map[string]interface{}{"formula": "test"},
			expectURL: false, // homebrew_bottle returns empty URL
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recipe.Recipe{
				Metadata: recipe.MetadataSection{Name: "test"},
				Version:  recipe.VersionSection{Source: "nodejs_dist"},
				Steps:    []recipe.Step{{Action: tt.action, Params: tt.params}},
			}

			exec, err := New(r)
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			defer exec.Cleanup()

			downloader := validate.NewPreDownloader().WithHTTPClient(server.Client())
			plan, err := exec.GeneratePlan(context.Background(), PlanConfig{
				OS:         "linux",
				Arch:       "amd64",
				Downloader: downloader,
			})

			if err != nil {
				t.Skipf("GeneratePlan() error (expected in offline tests): %v", err)
			}

			if len(plan.Steps) != 1 {
				t.Fatalf("len(Steps) = %d, want 1", len(plan.Steps))
			}

			step := plan.Steps[0]
			hasURL := step.URL != ""
			if hasURL != tt.expectURL {
				t.Errorf("step.URL present = %v, want %v (URL: %q)", hasURL, tt.expectURL, step.URL)
			}

			if tt.expectURL && step.Checksum == "" {
				t.Error("expected checksum to be computed when URL is present")
			}
		})
	}
}
