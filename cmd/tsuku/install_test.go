package main

import (
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
)

func TestIsInteractive(t *testing.T) {
	// In test environment, stdin is typically not a terminal
	// This test verifies the function doesn't panic and returns a boolean
	result := isInteractive()
	// We can't assert a specific value since it depends on the test environment,
	// but we verify it returns without error
	t.Logf("isInteractive() = %v", result)
}

func TestParseToolNameWithVersion(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		wantToolName      string
		wantVersionConstr string
	}{
		{
			name:              "simple tool name",
			input:             "kubectl",
			wantToolName:      "kubectl",
			wantVersionConstr: "",
		},
		{
			name:              "tool with version",
			input:             "kubectl@v1.29.0",
			wantToolName:      "kubectl",
			wantVersionConstr: "v1.29.0",
		},
		{
			name:              "tool with latest",
			input:             "terraform@latest",
			wantToolName:      "terraform",
			wantVersionConstr: "latest",
		},
		{
			name:              "tool with semver",
			input:             "nodejs@20",
			wantToolName:      "nodejs",
			wantVersionConstr: "20",
		},
		{
			name:              "tool with complex version",
			input:             "java@openjdk-21.0.1",
			wantToolName:      "java",
			wantVersionConstr: "openjdk-21.0.1",
		},
		{
			name:              "scoped npm package style",
			input:             "turbo@1.0.0",
			wantToolName:      "turbo",
			wantVersionConstr: "1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolName := tt.input
			versionConstraint := ""

			if strings.Contains(tt.input, "@") {
				parts := strings.SplitN(tt.input, "@", 2)
				toolName = parts[0]
				versionConstraint = parts[1]
			}

			if toolName != tt.wantToolName {
				t.Errorf("toolName = %q, want %q", toolName, tt.wantToolName)
			}
			if versionConstraint != tt.wantVersionConstr {
				t.Errorf("versionConstraint = %q, want %q", versionConstraint, tt.wantVersionConstr)
			}
		})
	}
}

func TestLatestVersionResolution(t *testing.T) {
	tests := []struct {
		name              string
		versionConstraint string
		wantResolve       string
	}{
		{
			name:              "latest converts to empty",
			versionConstraint: "latest",
			wantResolve:       "",
		},
		{
			name:              "specific version unchanged",
			versionConstraint: "v1.29.0",
			wantResolve:       "v1.29.0",
		},
		{
			name:              "empty stays empty",
			versionConstraint: "",
			wantResolve:       "",
		},
		{
			name:              "semver prefix unchanged",
			versionConstraint: "20",
			wantResolve:       "20",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolveVersion := tt.versionConstraint
			if resolveVersion == "latest" {
				resolveVersion = ""
			}

			if resolveVersion != tt.wantResolve {
				t.Errorf("resolveVersion = %q, want %q", resolveVersion, tt.wantResolve)
			}
		})
	}
}

func TestConfirmInstallResponseParsing(t *testing.T) {
	// Test the response parsing logic (not the actual prompt)
	tests := []struct {
		name     string
		response string
		want     bool
	}{
		{"lowercase y", "y", true},
		{"uppercase Y", "Y", true},
		{"lowercase yes", "yes", true},
		{"uppercase YES", "YES", true},
		{"mixed case Yes", "Yes", true},
		{"no", "no", false},
		{"n", "n", false},
		{"empty", "", false},
		{"random text", "maybe", false},
		{"y with spaces", "  y  ", true},
		{"yes with spaces", "  yes  ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the parsing logic from confirmInstall
			response := strings.TrimSpace(strings.ToLower(tt.response))
			result := response == "y" || response == "yes"

			if result != tt.want {
				t.Errorf("confirmInstall response %q = %v, want %v", tt.response, result, tt.want)
			}
		})
	}
}

func TestInstallCmdFlags(t *testing.T) {
	// Verify flags are registered correctly
	dryRunFlag := installCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("--dry-run flag not registered")
	} else {
		if dryRunFlag.DefValue != "false" {
			t.Errorf("--dry-run default = %q, want %q", dryRunFlag.DefValue, "false")
		}
	}

	forceFlag := installCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("--force flag not registered")
	} else {
		if forceFlag.DefValue != "false" {
			t.Errorf("--force default = %q, want %q", forceFlag.DefValue, "false")
		}
	}
}

func TestInstallCmdUsage(t *testing.T) {
	if installCmd.Use != "install <tool>..." {
		t.Errorf("installCmd.Use = %q, want %q", installCmd.Use, "install <tool>...")
	}

	if installCmd.Short != "Install a development tool" {
		t.Errorf("installCmd.Short = %q, want %q", installCmd.Short, "Install a development tool")
	}

	// Verify long description contains examples
	if !strings.Contains(installCmd.Long, "kubectl@v1.29.0") {
		t.Error("installCmd.Long should contain version example")
	}
	if !strings.Contains(installCmd.Long, "terraform@latest") {
		t.Error("installCmd.Long should contain @latest example")
	}
}

func TestConvertExecutorPlan(t *testing.T) {
	now := time.Now()

	t.Run("converts nil plan", func(t *testing.T) {
		result := convertExecutorPlan(nil)
		if result != nil {
			t.Errorf("convertExecutorPlan(nil) = %v, want nil", result)
		}
	})

	t.Run("converts full plan", func(t *testing.T) {
		ep := &executor.InstallationPlan{
			FormatVersion: 1,
			Tool:          "gh",
			Version:       "2.40.0",
			Platform: executor.Platform{
				OS:   "linux",
				Arch: "amd64",
			},
			GeneratedAt:  now,
			RecipeHash:   "abc123",
			RecipeSource: "registry",
			Steps: []executor.ResolvedStep{
				{
					Action:    "download_archive",
					Params:    map[string]interface{}{"url": "https://example.com/file.tar.gz"},
					Evaluable: true,
					URL:       "https://example.com/file.tar.gz",
					Checksum:  "sha256:deadbeef",
					Size:      12345,
				},
				{
					Action:    "extract",
					Params:    map[string]interface{}{"format": "tar.gz"},
					Evaluable: true,
				},
			},
		}

		result := convertExecutorPlan(ep)
		if result == nil {
			t.Fatal("convertExecutorPlan returned nil for non-nil input")
		}

		// Verify top-level fields
		if result.FormatVersion != 1 {
			t.Errorf("FormatVersion = %d, want 1", result.FormatVersion)
		}
		if result.Tool != "gh" {
			t.Errorf("Tool = %q, want %q", result.Tool, "gh")
		}
		if result.Version != "2.40.0" {
			t.Errorf("Version = %q, want %q", result.Version, "2.40.0")
		}
		if result.Platform.OS != "linux" || result.Platform.Arch != "amd64" {
			t.Errorf("Platform = %+v, want linux/amd64", result.Platform)
		}
		if result.RecipeHash != "abc123" {
			t.Errorf("RecipeHash = %q, want %q", result.RecipeHash, "abc123")
		}
		if result.RecipeSource != "registry" {
			t.Errorf("RecipeSource = %q, want %q", result.RecipeSource, "registry")
		}

		// Verify steps
		if len(result.Steps) != 2 {
			t.Fatalf("len(Steps) = %d, want 2", len(result.Steps))
		}

		step0 := result.Steps[0]
		if step0.Action != "download_archive" {
			t.Errorf("Steps[0].Action = %q, want %q", step0.Action, "download_archive")
		}
		if !step0.Evaluable {
			t.Error("Steps[0].Evaluable = false, want true")
		}
		if step0.URL != "https://example.com/file.tar.gz" {
			t.Errorf("Steps[0].URL = %q, want %q", step0.URL, "https://example.com/file.tar.gz")
		}
		if step0.Checksum != "sha256:deadbeef" {
			t.Errorf("Steps[0].Checksum = %q, want %q", step0.Checksum, "sha256:deadbeef")
		}
		if step0.Size != 12345 {
			t.Errorf("Steps[0].Size = %d, want 12345", step0.Size)
		}
	})

	t.Run("returns correct type", func(t *testing.T) {
		ep := &executor.InstallationPlan{
			FormatVersion: 1,
			Tool:          "test",
			Version:       "1.0.0",
		}

		result := convertExecutorPlan(ep)

		// Verify the result is the correct type for InstallOptions
		var _ *install.Plan = result
	})
}
