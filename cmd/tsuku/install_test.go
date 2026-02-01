package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/registry"
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

	freshFlag := installCmd.Flags().Lookup("fresh")
	if freshFlag == nil {
		t.Error("--fresh flag not registered")
	} else {
		if freshFlag.DefValue != "false" {
			t.Errorf("--fresh default = %q, want %q", freshFlag.DefValue, "false")
		}
	}
}

func TestInstallCmdUsage(t *testing.T) {
	// Use changed from <tool>... to [tool]... to allow --plan without tool arg
	if installCmd.Use != "install [tool]..." {
		t.Errorf("installCmd.Use = %q, want %q", installCmd.Use, "install [tool]...")
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
	// Verify plan-based installation examples
	if !strings.Contains(installCmd.Long, "--plan plan.json") {
		t.Error("installCmd.Long should contain --plan file example")
	}
	if !strings.Contains(installCmd.Long, "tsuku eval rg | tsuku install --plan -") {
		t.Error("installCmd.Long should contain --plan stdin example")
	}
}

func TestClassifyInstallError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "not found registry error",
			err: &registry.RegistryError{
				Type:    registry.ErrTypeNotFound,
				Recipe:  "nonexistent",
				Message: "recipe not found",
			},
			want: ExitRecipeNotFound,
		},
		{
			name: "network registry error",
			err: &registry.RegistryError{
				Type:    registry.ErrTypeNetwork,
				Message: "connection failed",
			},
			want: ExitNetwork,
		},
		{
			name: "DNS registry error",
			err: &registry.RegistryError{
				Type:    registry.ErrTypeDNS,
				Message: "DNS resolution failed",
			},
			want: ExitNetwork,
		},
		{
			name: "timeout registry error",
			err: &registry.RegistryError{
				Type:    registry.ErrTypeTimeout,
				Message: "request timed out",
			},
			want: ExitNetwork,
		},
		{
			name: "connection registry error",
			err: &registry.RegistryError{
				Type:    registry.ErrTypeConnection,
				Message: "connection refused",
			},
			want: ExitNetwork,
		},
		{
			name: "TLS registry error",
			err: &registry.RegistryError{
				Type:    registry.ErrTypeTLS,
				Message: "certificate error",
			},
			want: ExitNetwork,
		},
		{
			name: "wrapped not found error",
			err: fmt.Errorf("install failed: %w", &registry.RegistryError{
				Type:    registry.ErrTypeNotFound,
				Recipe:  "missing-tool",
				Message: "recipe not found",
			}),
			want: ExitRecipeNotFound,
		},
		{
			name: "dependency failure",
			err:  fmt.Errorf("failed to install dependency 'dav1d': registry: recipe dav1d not found in registry"),
			want: ExitDependencyFailed,
		},
		{
			name: "wrapped dependency failure",
			err:  fmt.Errorf("install error: %w", fmt.Errorf("failed to install dependency 'libx265': some error")),
			want: ExitDependencyFailed,
		},
		{
			name: "generic install error",
			err:  fmt.Errorf("extraction failed: bad tarball"),
			want: ExitInstallFailed,
		},
		{
			name: "parsing registry error falls through to default",
			err: &registry.RegistryError{
				Type:    registry.ErrTypeParsing,
				Message: "invalid TOML",
			},
			want: ExitInstallFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyInstallError(tt.err)
			if got != tt.want {
				t.Errorf("classifyInstallError() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCategoryFromExitCode(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{ExitRecipeNotFound, "recipe_not_found"},
		{ExitNetwork, "network_error"},
		{ExitDependencyFailed, "missing_dep"},
		{ExitInstallFailed, "install_failed"},
		{99, "install_failed"},
	}
	for _, tt := range tests {
		got := categoryFromExitCode(tt.code)
		if got != tt.want {
			t.Errorf("categoryFromExitCode(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestExtractMissingRecipes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want []string
	}{
		{
			name: "single missing recipe",
			err:  fmt.Errorf("registry: recipe dav1d not found in registry"),
			want: []string{"dav1d"},
		},
		{
			name: "multiple missing recipes",
			err:  fmt.Errorf("failed to install dependency 'foo': registry: recipe foo not found in registry\nfailed to install dependency 'bar': registry: recipe bar not found in registry"),
			want: []string{"foo", "bar"},
		},
		{
			name: "deduplicates",
			err:  fmt.Errorf("recipe dav1d not found in registry\nretry: recipe dav1d not found in registry"),
			want: []string{"dav1d"},
		},
		{
			name: "no matches returns empty slice",
			err:  fmt.Errorf("extraction failed: bad tarball"),
			want: []string{},
		},
		{
			name: "wrapped error",
			err:  fmt.Errorf("install error: %w", fmt.Errorf("registry: recipe libx265 not found in registry")),
			want: []string{"libx265"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMissingRecipes(tt.err)
			if len(got) != len(tt.want) {
				t.Fatalf("extractMissingRecipes() returned %d items, want %d: %v", len(got), len(tt.want), got)
			}
			for i, name := range tt.want {
				if got[i] != name {
					t.Errorf("extractMissingRecipes()[%d] = %q, want %q", i, got[i], name)
				}
			}
		})
	}
}

func TestInstallErrorJSON(t *testing.T) {
	resp := installError{
		Status:         "error",
		Category:       "missing_dep",
		Message:        "failed to install dependency 'foo'",
		MissingRecipes: []string{"foo", "bar"},
		ExitCode:       8,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if parsed["status"] != "error" {
		t.Errorf("status = %v, want %q", parsed["status"], "error")
	}
	if parsed["category"] != "missing_dep" {
		t.Errorf("category = %v, want %q", parsed["category"], "missing_dep")
	}
	if parsed["exit_code"].(float64) != 8 {
		t.Errorf("exit_code = %v, want 8", parsed["exit_code"])
	}
	recipes := parsed["missing_recipes"].([]interface{})
	if len(recipes) != 2 {
		t.Errorf("missing_recipes length = %d, want 2", len(recipes))
	}
}

func TestInstallJSONFlag(t *testing.T) {
	flag := installCmd.Flags().Lookup("json")
	if flag == nil {
		t.Fatal("--json flag not registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("--json default = %q, want %q", flag.DefValue, "false")
	}
}
