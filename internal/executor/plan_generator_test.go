package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
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
		wantLen     int  // expected hash length
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
