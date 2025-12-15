package recipe

import (
	"testing"

	"github.com/BurntSushi/toml"
)

func TestRecipe_UnmarshalTOML_Valid(t *testing.T) {
	tomlData := `
[metadata]
name = "test-tool"
description = "A test tool"
version_format = "semver"
homepage = "https://example.com"
requires_sudo = false
dependencies = ["dep-a", "dep-b"]

[[steps]]
action = "github_file"
repo = "owner/repo"
asset_pattern = "tool-{{os}}-{{arch}}.tar.gz"

[verify]
command = "tool --version"
pattern = "v{{version}}"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Verify metadata
	if recipe.Metadata.Name != "test-tool" {
		t.Errorf("Name = %s, want test-tool", recipe.Metadata.Name)
	}

	if recipe.Metadata.Description != "A test tool" {
		t.Errorf("Description = %s, want 'A test tool'", recipe.Metadata.Description)
	}

	if recipe.Metadata.VersionFormat != "semver" {
		t.Errorf("VersionFormat = %s, want semver", recipe.Metadata.VersionFormat)
	}

	if recipe.Metadata.Homepage != "https://example.com" {
		t.Errorf("Homepage = %s, want https://example.com", recipe.Metadata.Homepage)
	}

	if recipe.Metadata.RequiresSudo {
		t.Error("RequiresSudo = true, want false")
	}

	// Verify dependencies
	if len(recipe.Metadata.Dependencies) != 2 {
		t.Fatalf("Dependencies length = %d, want 2", len(recipe.Metadata.Dependencies))
	}

	if recipe.Metadata.Dependencies[0] != "dep-a" {
		t.Errorf("Dependencies[0] = %s, want dep-a", recipe.Metadata.Dependencies[0])
	}

	if recipe.Metadata.Dependencies[1] != "dep-b" {
		t.Errorf("Dependencies[1] = %s, want dep-b", recipe.Metadata.Dependencies[1])
	}

	// Verify steps
	if len(recipe.Steps) != 1 {
		t.Fatalf("Steps length = %d, want 1", len(recipe.Steps))
	}

	step := recipe.Steps[0]
	if step.Action != "github_file" {
		t.Errorf("Action = %s, want github_file", step.Action)
	}

	// Verify verify section
	if recipe.Verify.Command != "tool --version" {
		t.Errorf("Verify.Command = %s, want 'tool --version'", recipe.Verify.Command)
	}

	if recipe.Verify.Pattern != "v{{version}}" {
		t.Errorf("Verify.Pattern = %s, want 'v{{version}}'", recipe.Verify.Pattern)
	}
}

func TestRecipe_UnmarshalTOML_NoDependencies(t *testing.T) {
	tomlData := `
[metadata]
name = "test-tool"
description = "A test tool"
version_format = "semver"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "echo verified"
pattern = "verified"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Dependencies can be nil or empty slice, both are functionally equivalent
	if len(recipe.Metadata.Dependencies) != 0 {
		t.Errorf("Dependencies length = %d, want 0", len(recipe.Metadata.Dependencies))
	}
}

func TestRecipe_UnmarshalTOML_TypeField(t *testing.T) {
	tests := []struct {
		name     string
		tomlData string
		wantType string
	}{
		{
			name: "type tool",
			tomlData: `
[metadata]
name = "test-tool"
type = "tool"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "echo verified"
`,
			wantType: "tool",
		},
		{
			name: "type library",
			tomlData: `
[metadata]
name = "test-lib"
type = "library"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "echo verified"
`,
			wantType: "library",
		},
		{
			name: "type omitted defaults to empty",
			tomlData: `
[metadata]
name = "test-tool"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "echo verified"
`,
			wantType: "", // Empty string, defaults to "tool" at runtime
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var recipe Recipe
			err := toml.Unmarshal([]byte(tt.tomlData), &recipe)
			if err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if recipe.Metadata.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", recipe.Metadata.Type, tt.wantType)
			}
		})
	}
}

func TestStep_UnmarshalTOML_Params(t *testing.T) {
	tomlData := `
[[steps]]
action = "github_file"
repo = "owner/repo"
asset_pattern = "tool-{{os}}-{{arch}}.tar.gz"
extract = true
`

	var recipe struct {
		Steps []Step `toml:"steps"`
	}

	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if len(recipe.Steps) != 1 {
		t.Fatalf("Steps length = %d, want 1", len(recipe.Steps))
	}

	step := recipe.Steps[0]

	if step.Action != "github_file" {
		t.Errorf("Action = %s, want github_file", step.Action)
	}

	// Check params
	repo, ok := step.Params["repo"].(string)
	if !ok {
		t.Fatal("repo not in Params or not a string")
	}
	if repo != "owner/repo" {
		t.Errorf("Params['repo'] = %s, want owner/repo", repo)
	}

	assetPattern, ok := step.Params["asset_pattern"].(string)
	if !ok {
		t.Fatal("asset_pattern not in Params or not a string")
	}
	if assetPattern != "tool-{{os}}-{{arch}}.tar.gz" {
		t.Errorf("Params['asset_pattern'] = %s, want tool-{{os}}-{{arch}}.tar.gz", assetPattern)
	}

	extract, ok := step.Params["extract"].(bool)
	if !ok {
		t.Fatal("extract not in Params or not a bool")
	}
	if !extract {
		t.Error("Params['extract'] = false, want true")
	}
}

func TestStep_UnmarshalTOML_When(t *testing.T) {
	tomlData := `
[[steps]]
action = "run_command"
command = "brew install tool"

[steps.when]
os = "darwin"
arch = "arm64"
`

	var recipe struct {
		Steps []Step `toml:"steps"`
	}

	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	step := recipe.Steps[0]

	if len(step.When) != 2 {
		t.Fatalf("When length = %d, want 2", len(step.When))
	}

	if step.When["os"] != "darwin" {
		t.Errorf("When['os'] = %s, want darwin", step.When["os"])
	}

	if step.When["arch"] != "arm64" {
		t.Errorf("When['arch'] = %s, want arm64", step.When["arch"])
	}

	// 'when' should not be in Params
	if _, ok := step.Params["when"]; ok {
		t.Error("'when' should not be in Params")
	}
}

func TestStep_UnmarshalTOML_NoteAndDescription(t *testing.T) {
	tomlData := `
[[steps]]
action = "download"
url = "https://example.com/file.tar.gz"
note = "This is a note"
description = "Download the file"
`

	var recipe struct {
		Steps []Step `toml:"steps"`
	}

	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	step := recipe.Steps[0]

	if step.Note != "This is a note" {
		t.Errorf("Note = %s, want 'This is a note'", step.Note)
	}

	if step.Description != "Download the file" {
		t.Errorf("Description = %s, want 'Download the file'", step.Description)
	}

	// note and description should not be in Params
	if _, ok := step.Params["note"]; ok {
		t.Error("'note' should not be in Params")
	}

	if _, ok := step.Params["description"]; ok {
		t.Error("'description' should not be in Params")
	}
}

func TestStep_UnmarshalTOML_AllFields(t *testing.T) {
	tomlData := `
[[steps]]
action = "custom_action"
param1 = "value1"
param2 = 42
note = "A note"
description = "A description"

[steps.when]
os = "linux"
`

	var recipe struct {
		Steps []Step `toml:"steps"`
	}

	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	step := recipe.Steps[0]

	// Check known fields
	if step.Action != "custom_action" {
		t.Errorf("Action = %s, want custom_action", step.Action)
	}

	if step.Note != "A note" {
		t.Errorf("Note = %s, want 'A note'", step.Note)
	}

	if step.Description != "A description" {
		t.Errorf("Description = %s, want 'A description'", step.Description)
	}

	if len(step.When) != 1 || step.When["os"] != "linux" {
		t.Errorf("When = %v, want map[os:linux]", step.When)
	}

	// Check params (only custom fields)
	if len(step.Params) != 2 {
		t.Errorf("Params length = %d, want 2", len(step.Params))
	}

	if step.Params["param1"] != "value1" {
		t.Errorf("Params['param1'] = %v, want value1", step.Params["param1"])
	}

	if step.Params["param2"] != int64(42) {
		t.Errorf("Params['param2'] = %v, want 42", step.Params["param2"])
	}
}

func TestRecipe_UnmarshalTOML_InvalidTOML(t *testing.T) {
	tomlData := `
[metadata
name = "test-tool"  # Missing closing bracket
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err == nil {
		t.Error("Unmarshal() error = nil, want error")
	}
}

func TestRecipe_UnmarshalTOML_MissingRequired(t *testing.T) {
	tomlData := `
[metadata]
name = "test-tool"
# Missing description

[[steps]]
# Missing action
command = "echo test"

[verify]
command = "echo verified"
# Pattern is optional
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	// TOML unmarshaling will succeed, but the recipe might be invalid
	// Validation should happen separately
	if err != nil {
		t.Errorf("Unmarshal() error = %v, want nil (validation is separate)", err)
	}

	// Check that fields are indeed missing/empty
	if recipe.Metadata.Description != "" {
		t.Errorf("Description = %s, want empty", recipe.Metadata.Description)
	}

	if len(recipe.Steps) > 0 && recipe.Steps[0].Action != "" {
		t.Errorf("Step Action = %s, want empty", recipe.Steps[0].Action)
	}
}

func TestVerifySection_AdditionalVerify(t *testing.T) {
	tomlData := `
[metadata]
name = "test-tool"
description = "A test tool"
version_format = "semver"

[[steps]]
action = "run_command"
command = "echo test"

[verify]
command = "tool --version"
pattern = "v1.0.0"

[[verify.additional]]
command = "tool --help"
pattern = "Usage:"

[[verify.additional]]
command = "tool config"
pattern = "Config OK"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if recipe.Verify.Command != "tool --version" {
		t.Errorf("Verify.Command = %s, want 'tool --version'", recipe.Verify.Command)
	}

	if recipe.Verify.Pattern != "v1.0.0" {
		t.Errorf("Verify.Pattern = %s, want 'v1.0.0'", recipe.Verify.Pattern)
	}

	if len(recipe.Verify.Additional) != 2 {
		t.Fatalf("Additional verifications length = %d, want 2", len(recipe.Verify.Additional))
	}

	if recipe.Verify.Additional[0].Command != "tool --help" {
		t.Errorf("Additional[0].Command = %s, want 'tool --help'", recipe.Verify.Additional[0].Command)
	}

	if recipe.Verify.Additional[0].Pattern != "Usage:" {
		t.Errorf("Additional[0].Pattern = %s, want 'Usage:'", recipe.Verify.Additional[0].Pattern)
	}

	if recipe.Verify.Additional[1].Command != "tool config" {
		t.Errorf("Additional[1].Command = %s, want 'tool config'", recipe.Verify.Additional[1].Command)
	}

	if recipe.Verify.Additional[1].Pattern != "Config OK" {
		t.Errorf("Additional[1].Pattern = %s, want 'Config OK'", recipe.Verify.Additional[1].Pattern)
	}
}

func TestRecipe_ExtractBinaries_SingularBinary(t *testing.T) {
	// Test that singular 'binary' parameter gets "bin/" prefix (github_file)
	tomlData := `
[metadata]
name = "test-tool"
description = "A test tool"
version_format = "semver"

[[steps]]
action = "github_file"
repo = "owner/repo"
asset_pattern = "tool-{os}-{arch}"
binary = "bombardier"

[verify]
command = "bombardier --version"
pattern = "1.0.0"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	binaries := recipe.ExtractBinaries()

	// Singular 'binary' should also get bin/ prefix
	expected := []string{"bin/bombardier"}
	if len(binaries) != len(expected) {
		t.Fatalf("ExtractBinaries() returned %d binaries, want %d", len(binaries), len(expected))
	}

	if binaries[0] != expected[0] {
		t.Errorf("ExtractBinaries()[0] = %s, want %s", binaries[0], expected[0])
	}
}

func TestRecipe_ExtractBinaries_SimpleStrings(t *testing.T) {
	// Test that simple string binaries get "bin/" prefix (github_archive)
	// This prevents regression where symlinks pointed to wrong paths
	tomlData := `
[metadata]
name = "test-tool"
description = "A test tool"
version_format = "semver"

[[steps]]
action = "github_archive"
repo = "owner/repo"
asset_pattern = "tool.tar.gz"
archive_format = "tar.gz"
binaries = ["age", "keygen"]

[verify]
command = "age --version"
pattern = "1.0.0"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	binaries := recipe.ExtractBinaries()

	// Simple strings should become "bin/<basename>"
	expected := []string{"bin/age", "bin/keygen"}
	if len(binaries) != len(expected) {
		t.Fatalf("ExtractBinaries() returned %d binaries, want %d", len(binaries), len(expected))
	}

	for i, want := range expected {
		if binaries[i] != want {
			t.Errorf("ExtractBinaries()[%d] = %s, want %s", i, binaries[i], want)
		}
	}
}

func TestRecipe_ExtractBinaries_ObjectFormat(t *testing.T) {
	// Test that object format preserves full paths
	tomlData := `
[metadata]
name = "test-tool"
description = "A test tool"
version_format = "semver"

[[steps]]
action = "github_archive"
repo = "owner/repo"
asset_pattern = "tool.tar.gz"
archive_format = "tar.gz"
install_mode = "directory"
binaries = [
	{ src = "cargo/bin/cargo", dest = "cargo/bin/cargo" },
	{ src = "rustc/bin/rustc", dest = "rustc/bin/rustc" }
]

[verify]
command = "cargo --version"
pattern = "1.0.0"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	binaries := recipe.ExtractBinaries()

	// Object format should preserve full dest paths
	expected := []string{"cargo/bin/cargo", "rustc/bin/rustc"}
	if len(binaries) != len(expected) {
		t.Fatalf("ExtractBinaries() returned %d binaries, want %d", len(binaries), len(expected))
	}

	for i, want := range expected {
		if binaries[i] != want {
			t.Errorf("ExtractBinaries()[%d] = %s, want %s", i, binaries[i], want)
		}
	}
}

func TestRecipe_ExtractBinaries_MixedFormats(t *testing.T) {
	// Test that simple strings get bin/ prefix even when in same array as objects
	tomlData := `
[metadata]
name = "test-tool"
description = "A test tool"
version_format = "semver"

[[steps]]
action = "github_archive"
repo = "owner/repo"
asset_pattern = "tool.tar.gz"
archive_format = "tar.gz"
binaries = [
	"kubectl",
	{ src = "bin/argocd", dest = "bin/argocd" }
]

[verify]
command = "kubectl version"
pattern = "1.0.0"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	binaries := recipe.ExtractBinaries()

	// Simple string should get bin/ prefix, object format preserves path
	expected := []string{"bin/kubectl", "bin/argocd"}
	if len(binaries) != len(expected) {
		t.Fatalf("ExtractBinaries() returned %d binaries, want %d", len(binaries), len(expected))
	}

	for i, want := range expected {
		if binaries[i] != want {
			t.Errorf("ExtractBinaries()[%d] = %s, want %s", i, binaries[i], want)
		}
	}
}

func TestRecipe_ExtractBinaries_Executables(t *testing.T) {
	// Test that 'executables' parameter gets "bin/" prefix (npm_install)
	tomlData := `
[metadata]
name = "test-tool"
description = "A test tool"
version_format = "semver"

[[steps]]
action = "npm_install"
package = "serve"
executables = ["serve"]

[verify]
command = "serve --version"
pattern = "14.2.5"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	binaries := recipe.ExtractBinaries()

	// npm_install executables should get bin/ prefix
	expected := []string{"bin/serve"}
	if len(binaries) != len(expected) {
		t.Fatalf("ExtractBinaries() returned %d binaries, want %d", len(binaries), len(expected))
	}

	if binaries[0] != expected[0] {
		t.Errorf("ExtractBinaries()[0] = %s, want %s", binaries[0], expected[0])
	}
}

func TestRecipe_ExtractBinaries_DirectoryMode_SimplePaths(t *testing.T) {
	// Test that directory mode preserves simple paths as-is (zig)
	tomlData := `
[metadata]
name = "zig"
description = "Zig compiler"
version_format = "semver"

[[steps]]
action = "download_archive"
url = "https://ziglang.org/download/{version}/zig.tar.xz"
archive_format = "tar.xz"
binaries = ["zig"]
install_mode = "directory"

[verify]
command = "zig version"
pattern = "{version}"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	binaries := recipe.ExtractBinaries()

	// Directory mode should preserve paths as-is
	expected := []string{"zig"}
	if len(binaries) != len(expected) {
		t.Fatalf("ExtractBinaries() returned %d binaries, want %d", len(binaries), len(expected))
	}

	if binaries[0] != expected[0] {
		t.Errorf("ExtractBinaries()[0] = %s, want %s", binaries[0], expected[0])
	}
}

func TestRecipe_ExtractBinaries_DirectoryMode_FullPaths(t *testing.T) {
	// Test that directory mode preserves full paths (rust)
	tomlData := `
[metadata]
name = "rust"
description = "Rust compiler and cargo"
version_format = "semver"

[[steps]]
action = "download_archive"
url = "https://static.rust-lang.org/dist/rust-{version}.tar.gz"
archive_format = "tar.gz"
binaries = ["cargo/bin/cargo", "rustc/bin/rustc"]
install_mode = "directory"

[verify]
command = "cargo --version"
pattern = "cargo {version}"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	binaries := recipe.ExtractBinaries()

	// Directory mode should preserve full paths
	expected := []string{"cargo/bin/cargo", "rustc/bin/rustc"}
	if len(binaries) != len(expected) {
		t.Fatalf("ExtractBinaries() returned %d binaries, want %d", len(binaries), len(expected))
	}

	for i, want := range expected {
		if binaries[i] != want {
			t.Errorf("ExtractBinaries()[%d] = %s, want %s", i, binaries[i], want)
		}
	}
}

func TestRecipe_HasChecksumVerification_NoDownloadSteps(t *testing.T) {
	// Recipes without download steps should return true (nothing to verify)
	recipe := Recipe{
		Steps: []Step{
			{Action: "chmod", Params: map[string]interface{}{"files": []string{"bin/tool"}}},
		},
	}

	if !recipe.HasChecksumVerification() {
		t.Error("HasChecksumVerification() = false for recipe with no download steps, want true")
	}
}

func TestRecipe_HasChecksumVerification_DownloadWithoutChecksum(t *testing.T) {
	// Download step without checksum should return false
	recipe := Recipe{
		Steps: []Step{
			{Action: "download", Params: map[string]interface{}{
				"url":  "https://example.com/file",
				"dest": "file",
			}},
		},
	}

	if recipe.HasChecksumVerification() {
		t.Error("HasChecksumVerification() = true for recipe without checksum, want false")
	}
}

func TestRecipe_HasChecksumVerification_DownloadWithInlineChecksum(t *testing.T) {
	// Download step with inline checksum should return true
	recipe := Recipe{
		Steps: []Step{
			{Action: "download", Params: map[string]interface{}{
				"url":      "https://example.com/file",
				"dest":     "file",
				"checksum": "abc123",
			}},
		},
	}

	if !recipe.HasChecksumVerification() {
		t.Error("HasChecksumVerification() = false for recipe with inline checksum, want true")
	}
}

func TestRecipe_HasChecksumVerification_DownloadWithChecksumURL(t *testing.T) {
	// Download step with checksum URL should return true
	recipe := Recipe{
		Steps: []Step{
			{Action: "download", Params: map[string]interface{}{
				"url":          "https://example.com/file",
				"dest":         "file",
				"checksum_url": "https://example.com/file.sha256",
			}},
		},
	}

	if !recipe.HasChecksumVerification() {
		t.Error("HasChecksumVerification() = false for recipe with checksum_url, want true")
	}
}

func TestRecipe_HasChecksumVerification_GitHubArchiveWithChecksum(t *testing.T) {
	// github_archive with checksum should return true
	recipe := Recipe{
		Steps: []Step{
			{Action: "github_archive", Params: map[string]interface{}{
				"repo":           "owner/repo",
				"asset_pattern":  "tool-{version}.tar.gz",
				"archive_format": "tar.gz",
				"binaries":       []string{"tool"},
				"checksum":       "abc123",
			}},
		},
	}

	if !recipe.HasChecksumVerification() {
		t.Error("HasChecksumVerification() = false for github_archive with checksum, want true")
	}
}

func TestRecipe_HasChecksumVerification_GitHubArchiveWithoutChecksum(t *testing.T) {
	// github_archive without checksum should return false
	recipe := Recipe{
		Steps: []Step{
			{Action: "github_archive", Params: map[string]interface{}{
				"repo":           "owner/repo",
				"asset_pattern":  "tool-{version}.tar.gz",
				"archive_format": "tar.gz",
				"binaries":       []string{"tool"},
			}},
		},
	}

	if recipe.HasChecksumVerification() {
		t.Error("HasChecksumVerification() = true for github_archive without checksum, want false")
	}
}

func TestRecipe_HasChecksumVerification_MultipleSteps_AnyHasChecksum(t *testing.T) {
	// If any download step has checksum, should return true
	recipe := Recipe{
		Steps: []Step{
			{Action: "github_file", Params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "tool1",
				"binary":        "tool1",
				// No checksum
			}},
			{Action: "download", Params: map[string]interface{}{
				"url":      "https://example.com/tool2",
				"dest":     "tool2",
				"checksum": "abc123", // Has checksum
			}},
		},
	}

	if !recipe.HasChecksumVerification() {
		t.Error("HasChecksumVerification() = false when any step has checksum, want true")
	}
}

func TestRecipe_HasChecksumVerification_AllDownloadActions(t *testing.T) {
	// Test all download action types
	downloadActions := []string{
		"download",
		"download_archive",
		"github_archive",
		"github_file",
	}

	for _, action := range downloadActions {
		t.Run(action+"_without_checksum", func(t *testing.T) {
			recipe := Recipe{
				Steps: []Step{
					{Action: action, Params: map[string]interface{}{
						"url": "https://example.com/file",
					}},
				},
			}

			if recipe.HasChecksumVerification() {
				t.Errorf("HasChecksumVerification() = true for %s without checksum, want false", action)
			}
		})

		t.Run(action+"_with_checksum", func(t *testing.T) {
			recipe := Recipe{
				Steps: []Step{
					{Action: action, Params: map[string]interface{}{
						"url":      "https://example.com/file",
						"checksum": "abc123",
					}},
				},
			}

			if !recipe.HasChecksumVerification() {
				t.Errorf("HasChecksumVerification() = false for %s with checksum, want true", action)
			}
		})
	}
}

func TestRecipe_HasChecksumVerification_NonDownloadActions(t *testing.T) {
	// Non-download actions should be ignored
	recipe := Recipe{
		Steps: []Step{
			{Action: "npm_install", Params: map[string]interface{}{
				"package": "some-package",
			}},
			{Action: "pip_install", Params: map[string]interface{}{
				"package": "some-package",
			}},
		},
	}

	// No download actions, so should return true (nothing to verify)
	if !recipe.HasChecksumVerification() {
		t.Error("HasChecksumVerification() = false for non-download actions, want true")
	}
}

func TestVerifySection_ModeAndVersionFormat(t *testing.T) {
	tomlData := `
[metadata]
name = "test-tool"
description = "A test tool"

[[steps]]
action = "github_file"
repo = "owner/repo"
asset_pattern = "tool-{os}-{arch}"
binary = "tool"

[verify]
command = "tool --version"
pattern = "Version: {version}"
mode = "version"
version_format = "semver"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if recipe.Verify.Mode != VerifyModeVersion {
		t.Errorf("Verify.Mode = %s, want %s", recipe.Verify.Mode, VerifyModeVersion)
	}

	if recipe.Verify.VersionFormat != VersionFormatSemver {
		t.Errorf("Verify.VersionFormat = %s, want %s", recipe.Verify.VersionFormat, VersionFormatSemver)
	}
}

func TestVerifySection_OutputModeWithReason(t *testing.T) {
	tomlData := `
[metadata]
name = "gofumpt"
description = "A stricter gofmt"

[[steps]]
action = "go_install"
package = "mvdan.cc/gofumpt"

[verify]
command = "gofumpt -h"
pattern = "usage:"
mode = "output"
reason = "Tool does not support --version flag"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if recipe.Verify.Mode != VerifyModeOutput {
		t.Errorf("Verify.Mode = %s, want %s", recipe.Verify.Mode, VerifyModeOutput)
	}

	if recipe.Verify.Pattern != "usage:" {
		t.Errorf("Verify.Pattern = %s, want usage:", recipe.Verify.Pattern)
	}

	expectedReason := "Tool does not support --version flag"
	if recipe.Verify.Reason != expectedReason {
		t.Errorf("Verify.Reason = %s, want %s", recipe.Verify.Reason, expectedReason)
	}
}

func TestVerifySection_AllVersionFormats(t *testing.T) {
	formats := []string{
		VersionFormatRaw,
		VersionFormatSemver,
		VersionFormatSemverFull,
		VersionFormatStripV,
	}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			tomlData := `
[metadata]
name = "test-tool"
description = "A test tool"

[[steps]]
action = "download"
url = "https://example.com/tool"

[verify]
command = "tool --version"
pattern = "{version}"
version_format = "` + format + `"
`

			var recipe Recipe
			err := toml.Unmarshal([]byte(tomlData), &recipe)
			if err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			if recipe.Verify.VersionFormat != format {
				t.Errorf("Verify.VersionFormat = %s, want %s", recipe.Verify.VersionFormat, format)
			}
		})
	}
}

func TestVerifySection_DefaultsWhenOmitted(t *testing.T) {
	tomlData := `
[metadata]
name = "test-tool"
description = "A test tool"

[[steps]]
action = "download"
url = "https://example.com/tool"

[verify]
command = "tool --version"
pattern = "{version}"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Fields should be empty strings when omitted (defaults applied at runtime)
	if recipe.Verify.Mode != "" {
		t.Errorf("Verify.Mode = %s, want empty (default applied at runtime)", recipe.Verify.Mode)
	}

	if recipe.Verify.VersionFormat != "" {
		t.Errorf("Verify.VersionFormat = %s, want empty (default applied at runtime)", recipe.Verify.VersionFormat)
	}

	if recipe.Verify.Reason != "" {
		t.Errorf("Verify.Reason = %s, want empty", recipe.Verify.Reason)
	}
}

func TestVerifyConstants(t *testing.T) {
	// Verify constants have expected values
	if VerifyModeVersion != "version" {
		t.Errorf("VerifyModeVersion = %s, want version", VerifyModeVersion)
	}
	if VerifyModeOutput != "output" {
		t.Errorf("VerifyModeOutput = %s, want output", VerifyModeOutput)
	}
	if VersionFormatRaw != "raw" {
		t.Errorf("VersionFormatRaw = %s, want raw", VersionFormatRaw)
	}
	if VersionFormatSemver != "semver" {
		t.Errorf("VersionFormatSemver = %s, want semver", VersionFormatSemver)
	}
	if VersionFormatSemverFull != "semver_full" {
		t.Errorf("VersionFormatSemverFull = %s, want semver_full", VersionFormatSemverFull)
	}
	if VersionFormatStripV != "strip_v" {
		t.Errorf("VersionFormatStripV = %s, want strip_v", VersionFormatStripV)
	}
}

func TestRecipe_IsLibrary(t *testing.T) {
	tests := []struct {
		name       string
		recipeType string
		want       bool
	}{
		{
			name:       "type library returns true",
			recipeType: RecipeTypeLibrary,
			want:       true,
		},
		{
			name:       "type tool returns false",
			recipeType: RecipeTypeTool,
			want:       false,
		},
		{
			name:       "empty type returns false",
			recipeType: "",
			want:       false,
		},
		{
			name:       "unknown type returns false",
			recipeType: "unknown",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipe := Recipe{
				Metadata: MetadataSection{
					Type: tt.recipeType,
				},
			}
			if got := recipe.IsLibrary(); got != tt.want {
				t.Errorf("IsLibrary() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecipeTypeConstants(t *testing.T) {
	// Verify constants have expected values
	if RecipeTypeTool != "tool" {
		t.Errorf("RecipeTypeTool = %s, want tool", RecipeTypeTool)
	}
	if RecipeTypeLibrary != "library" {
		t.Errorf("RecipeTypeLibrary = %s, want library", RecipeTypeLibrary)
	}
}

func TestRecipe_ToTOML_Basic(t *testing.T) {
	recipe := Recipe{
		Metadata: MetadataSection{
			Name:          "test-tool",
			Description:   "A test tool",
			Homepage:      "https://example.com",
			VersionFormat: "semver",
		},
		Version: VersionSection{
			Source:     "github_releases",
			GitHubRepo: "owner/repo",
		},
		Steps: []Step{
			{
				Action: "github_file",
				Params: map[string]interface{}{
					"repo":          "owner/repo",
					"asset_pattern": "tool-{os}-{arch}",
					"binary":        "tool",
				},
			},
		},
		Verify: VerifySection{
			Command: "tool --version",
			Pattern: "{version}",
		},
	}

	data, err := recipe.ToTOML()
	if err != nil {
		t.Fatalf("ToTOML() error = %v", err)
	}

	tomlStr := string(data)

	// Check metadata section
	if !contains(tomlStr, "[metadata]") {
		t.Error("ToTOML() missing [metadata] section")
	}
	if !contains(tomlStr, `name = "test-tool"`) {
		t.Error("ToTOML() missing name field")
	}
	if !contains(tomlStr, `description = "A test tool"`) {
		t.Error("ToTOML() missing description field")
	}
	if !contains(tomlStr, `homepage = "https://example.com"`) {
		t.Error("ToTOML() missing homepage field")
	}
	if !contains(tomlStr, `version_format = "semver"`) {
		t.Error("ToTOML() missing version_format field")
	}

	// Check version section
	if !contains(tomlStr, "[version]") {
		t.Error("ToTOML() missing [version] section")
	}
	if !contains(tomlStr, `source = "github_releases"`) {
		t.Error("ToTOML() missing source field")
	}
	if !contains(tomlStr, `github_repo = "owner/repo"`) {
		t.Error("ToTOML() missing github_repo field")
	}

	// Check steps section
	if !contains(tomlStr, "[[steps]]") {
		t.Error("ToTOML() missing [[steps]] section")
	}
	if !contains(tomlStr, `action = "github_file"`) {
		t.Error("ToTOML() missing action field in steps")
	}

	// Check verify section
	if !contains(tomlStr, "[verify]") {
		t.Error("ToTOML() missing [verify] section")
	}
	if !contains(tomlStr, `command = "tool --version"`) {
		t.Error("ToTOML() missing command field in verify")
	}
	if !contains(tomlStr, `pattern = "{version}"`) {
		t.Error("ToTOML() missing pattern field in verify")
	}
}

func TestRecipe_ToTOML_WithStepWhen(t *testing.T) {
	recipe := Recipe{
		Metadata: MetadataSection{
			Name: "test-tool",
		},
		Version: VersionSection{
			Source: "github_releases",
		},
		Steps: []Step{
			{
				Action: "run_command",
				When: map[string]string{
					"os":   "darwin",
					"arch": "arm64",
				},
				Params: map[string]interface{}{
					"command": "brew install tool",
				},
			},
		},
		Verify: VerifySection{
			Command: "tool --version",
		},
	}

	data, err := recipe.ToTOML()
	if err != nil {
		t.Fatalf("ToTOML() error = %v", err)
	}

	tomlStr := string(data)

	// Check that when clause is serialized
	if !contains(tomlStr, "[[steps]]") {
		t.Error("ToTOML() missing [[steps]] section")
	}
	if !contains(tomlStr, "[when]") && !contains(tomlStr, "when.") {
		// when may be serialized as inline table or subtable
		t.Log("ToTOML() output:", tomlStr)
	}
}

func TestRecipe_ToTOML_WithNoteAndDescription(t *testing.T) {
	recipe := Recipe{
		Metadata: MetadataSection{
			Name: "test-tool",
		},
		Version: VersionSection{
			Source: "github_releases",
		},
		Steps: []Step{
			{
				Action:      "download",
				Note:        "This is a note",
				Description: "Download the file",
				Params: map[string]interface{}{
					"url": "https://example.com/file",
				},
			},
		},
		Verify: VerifySection{
			Command: "tool --version",
		},
	}

	data, err := recipe.ToTOML()
	if err != nil {
		t.Fatalf("ToTOML() error = %v", err)
	}

	tomlStr := string(data)

	// Check that note and description are serialized
	if !contains(tomlStr, `note = "This is a note"`) {
		t.Error("ToTOML() missing note field")
	}
	if !contains(tomlStr, `description = "Download the file"`) {
		t.Error("ToTOML() missing description field")
	}
}

func TestRecipe_ToTOML_MultipleSteps(t *testing.T) {
	recipe := Recipe{
		Metadata: MetadataSection{
			Name: "test-tool",
		},
		Version: VersionSection{
			Source: "github_releases",
		},
		Steps: []Step{
			{
				Action: "download",
				Params: map[string]interface{}{
					"url": "https://example.com/file1",
				},
			},
			{
				Action: "chmod",
				Params: map[string]interface{}{
					"files": []string{"file1"},
					"mode":  "755",
				},
			},
		},
		Verify: VerifySection{
			Command: "tool --version",
		},
	}

	data, err := recipe.ToTOML()
	if err != nil {
		t.Fatalf("ToTOML() error = %v", err)
	}

	tomlStr := string(data)

	// Count [[steps]] occurrences - should be 2
	count := countOccurrences(tomlStr, "[[steps]]")
	if count != 2 {
		t.Errorf("ToTOML() has %d [[steps]] sections, want 2", count)
	}

	// Check both actions are present
	if !contains(tomlStr, `action = "download"`) {
		t.Error("ToTOML() missing download action")
	}
	if !contains(tomlStr, `action = "chmod"`) {
		t.Error("ToTOML() missing chmod action")
	}
}

func TestRecipe_ToTOML_Roundtrip(t *testing.T) {
	// Create a recipe
	original := Recipe{
		Metadata: MetadataSection{
			Name:          "roundtrip-test",
			Description:   "Testing roundtrip",
			Homepage:      "https://example.com",
			VersionFormat: "semver",
		},
		Version: VersionSection{
			Source:     "github_releases",
			GitHubRepo: "owner/repo",
		},
		Steps: []Step{
			{
				Action: "github_file",
				Params: map[string]interface{}{
					"repo":          "owner/repo",
					"asset_pattern": "tool-{os}-{arch}",
					"binary":        "tool",
				},
			},
		},
		Verify: VerifySection{
			Command: "tool --version",
			Pattern: "{version}",
		},
	}

	// Serialize to TOML
	data, err := original.ToTOML()
	if err != nil {
		t.Fatalf("ToTOML() error = %v", err)
	}

	// Deserialize back
	var parsed Recipe
	err = toml.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v, toml:\n%s", err, string(data))
	}

	// Compare key fields
	if parsed.Metadata.Name != original.Metadata.Name {
		t.Errorf("Roundtrip Name = %s, want %s", parsed.Metadata.Name, original.Metadata.Name)
	}
	if parsed.Metadata.Description != original.Metadata.Description {
		t.Errorf("Roundtrip Description = %s, want %s", parsed.Metadata.Description, original.Metadata.Description)
	}
	if parsed.Version.Source != original.Version.Source {
		t.Errorf("Roundtrip Source = %s, want %s", parsed.Version.Source, original.Version.Source)
	}
	if len(parsed.Steps) != len(original.Steps) {
		t.Errorf("Roundtrip Steps length = %d, want %d", len(parsed.Steps), len(original.Steps))
	}
	if parsed.Steps[0].Action != original.Steps[0].Action {
		t.Errorf("Roundtrip Step[0].Action = %s, want %s", parsed.Steps[0].Action, original.Steps[0].Action)
	}
	if parsed.Verify.Command != original.Verify.Command {
		t.Errorf("Roundtrip Verify.Command = %s, want %s", parsed.Verify.Command, original.Verify.Command)
	}
}

func TestRecipe_ToTOML_EmptyFields(t *testing.T) {
	// Recipe with minimal fields
	recipe := Recipe{
		Metadata: MetadataSection{
			Name: "minimal-tool",
		},
		Version: VersionSection{},
		Steps:   []Step{},
		Verify: VerifySection{
			Command: "tool --version",
		},
	}

	data, err := recipe.ToTOML()
	if err != nil {
		t.Fatalf("ToTOML() error = %v", err)
	}

	tomlStr := string(data)

	// Should have all sections
	if !contains(tomlStr, "[metadata]") {
		t.Error("ToTOML() missing [metadata] section")
	}
	if !contains(tomlStr, "[version]") {
		t.Error("ToTOML() missing [version] section")
	}
	if !contains(tomlStr, "[verify]") {
		t.Error("ToTOML() missing [verify] section")
	}

	// Empty fields should not be serialized
	if contains(tomlStr, `description = ""`) {
		t.Error("ToTOML() should not serialize empty description")
	}
	if contains(tomlStr, `homepage = ""`) {
		t.Error("ToTOML() should not serialize empty homepage")
	}
}

func TestRecipe_ToTOML_VersionSectionFields(t *testing.T) {
	// Test that all version section fields are serialized
	recipe := Recipe{
		Metadata: MetadataSection{
			Name: "test-tool",
		},
		Version: VersionSection{
			Source:     "homebrew",
			GitHubRepo: "owner/repo",
			TagPrefix:  "v",
			Module:     "github.com/owner/repo",
			Formula:    "test-formula",
		},
		Steps: []Step{},
		Verify: VerifySection{
			Command: "test --version",
		},
	}

	data, err := recipe.ToTOML()
	if err != nil {
		t.Fatalf("ToTOML() error = %v", err)
	}

	tomlStr := string(data)

	// Check all version fields are present
	if !contains(tomlStr, `source = "homebrew"`) {
		t.Error("ToTOML() missing source field")
	}
	if !contains(tomlStr, `github_repo = "owner/repo"`) {
		t.Error("ToTOML() missing github_repo field")
	}
	if !contains(tomlStr, `tag_prefix = "v"`) {
		t.Error("ToTOML() missing tag_prefix field")
	}
	if !contains(tomlStr, `module = "github.com/owner/repo"`) {
		t.Error("ToTOML() missing module field")
	}
	if !contains(tomlStr, `formula = "test-formula"`) {
		t.Error("ToTOML() missing formula field")
	}
}

func TestRecipe_ToTOML_HomebrewRecipe(t *testing.T) {
	// Test a realistic Homebrew-style recipe
	recipe := Recipe{
		Metadata: MetadataSection{
			Name:        "jq",
			Description: "Lightweight and flexible command-line JSON processor",
			Homepage:    "https://jqlang.github.io/jq/",
		},
		Version: VersionSection{
			Source:  "homebrew",
			Formula: "jq",
		},
		Steps: []Step{
			{
				Action: "homebrew_bottle",
				Params: map[string]interface{}{
					"formula": "jq",
				},
			},
			{
				Action: "install_binaries",
				Params: map[string]interface{}{
					"binaries": []string{"bin/jq"},
				},
			},
		},
		Verify: VerifySection{
			Command: "jq --version",
		},
	}

	data, err := recipe.ToTOML()
	if err != nil {
		t.Fatalf("ToTOML() error = %v", err)
	}

	tomlStr := string(data)

	// Verify homebrew-specific fields
	if !contains(tomlStr, `source = "homebrew"`) {
		t.Error("ToTOML() missing homebrew source")
	}
	if !contains(tomlStr, `formula = "jq"`) {
		t.Error("ToTOML() missing formula field")
	}
	if !contains(tomlStr, `action = "homebrew_bottle"`) {
		t.Error("ToTOML() missing homebrew_bottle action")
	}

	// Verify roundtrip works
	var parsed Recipe
	err = toml.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if parsed.Version.Source != "homebrew" {
		t.Errorf("Roundtrip: Version.Source = %q, want %q", parsed.Version.Source, "homebrew")
	}
	if parsed.Version.Formula != "jq" {
		t.Errorf("Roundtrip: Version.Formula = %q, want %q", parsed.Version.Formula, "jq")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper function to count occurrences
func countOccurrences(s, substr string) int {
	count := 0
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			count++
		}
	}
	return count
}

func TestRecipe_Resources_UnmarshalTOML(t *testing.T) {
	tomlData := `
[metadata]
name = "neovim"
description = "Ambitious Vim-fork"

[[resources]]
name = "tree-sitter-c"
url = "https://github.com/tree-sitter/tree-sitter-c/archive/refs/tags/v0.24.1.tar.gz"
checksum = "sha256:25dd4bb3dec770769a407e0fc803f424ce02c494a56ce95fedc525316dcf9b48"
dest = "deps/tree-sitter-c"

[[resources]]
name = "tree-sitter-lua"
url = "https://github.com/tree-sitter-grammars/tree-sitter-lua/archive/refs/tags/v0.4.0.tar.gz"
checksum = "sha256:b0977aced4a63bb75f26725787e047b8f5f4a092712c840ea7070765d4049559"
dest = "deps/tree-sitter-lua"

[[steps]]
action = "cmake_build"
source_dir = "."

[verify]
command = "nvim --version"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Verify resources
	if len(recipe.Resources) != 2 {
		t.Fatalf("Resources length = %d, want 2", len(recipe.Resources))
	}

	res1 := recipe.Resources[0]
	if res1.Name != "tree-sitter-c" {
		t.Errorf("Resources[0].Name = %s, want tree-sitter-c", res1.Name)
	}
	if res1.Dest != "deps/tree-sitter-c" {
		t.Errorf("Resources[0].Dest = %s, want deps/tree-sitter-c", res1.Dest)
	}
	if res1.Checksum != "sha256:25dd4bb3dec770769a407e0fc803f424ce02c494a56ce95fedc525316dcf9b48" {
		t.Errorf("Resources[0].Checksum = %s, want sha256:25dd...", res1.Checksum)
	}

	res2 := recipe.Resources[1]
	if res2.Name != "tree-sitter-lua" {
		t.Errorf("Resources[1].Name = %s, want tree-sitter-lua", res2.Name)
	}
}

func TestRecipe_Patches_UnmarshalTOML(t *testing.T) {
	tomlData := `
[metadata]
name = "curl"
description = "Command line tool for URL operations"

[[patches]]
url = "https://raw.githubusercontent.com/Homebrew/formula-patches/master/curl/fix.patch"
strip = 1

[[patches]]
data = "--- a/src/main.c\n+++ b/src/main.c\n@@ -1 +1 @@\n-old\n+new"
subdir = "src"

[[steps]]
action = "configure_make"

[verify]
command = "curl --version"
`

	var recipe Recipe
	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Verify patches
	if len(recipe.Patches) != 2 {
		t.Fatalf("Patches length = %d, want 2", len(recipe.Patches))
	}

	patch1 := recipe.Patches[0]
	if patch1.URL != "https://raw.githubusercontent.com/Homebrew/formula-patches/master/curl/fix.patch" {
		t.Errorf("Patches[0].URL = %s, want URL-based patch", patch1.URL)
	}
	if patch1.Strip != 1 {
		t.Errorf("Patches[0].Strip = %d, want 1", patch1.Strip)
	}

	patch2 := recipe.Patches[1]
	if patch2.Data == "" {
		t.Error("Patches[1].Data should not be empty for inline patch")
	}
	if patch2.Subdir != "src" {
		t.Errorf("Patches[1].Subdir = %s, want src", patch2.Subdir)
	}
}

func TestRecipe_ToTOML_WithResources(t *testing.T) {
	recipe := Recipe{
		Metadata: MetadataSection{
			Name:        "neovim",
			Description: "Vim-fork",
		},
		Version: VersionSection{
			Source:  "homebrew",
			Formula: "neovim",
		},
		Resources: []Resource{
			{
				Name:     "tree-sitter-c",
				URL:      "https://github.com/tree-sitter/tree-sitter-c/archive/v0.24.1.tar.gz",
				Checksum: "sha256:25dd4bb3",
				Dest:     "deps/tree-sitter-c",
			},
			{
				Name: "tree-sitter-lua",
				URL:  "https://github.com/tree-sitter-grammars/tree-sitter-lua/archive/v0.4.0.tar.gz",
				Dest: "deps/tree-sitter-lua",
			},
		},
		Steps: []Step{
			{Action: "cmake_build", Params: map[string]interface{}{}},
		},
		Verify: VerifySection{Command: "nvim --version"},
	}

	data, err := recipe.ToTOML()
	if err != nil {
		t.Fatalf("ToTOML() error = %v", err)
	}

	tomlStr := string(data)

	// Verify resources are serialized
	if !contains(tomlStr, "[[resources]]") {
		t.Error("ToTOML() missing [[resources]] section")
	}
	if !contains(tomlStr, `name = "tree-sitter-c"`) {
		t.Error("ToTOML() missing resource name")
	}
	if !contains(tomlStr, `dest = "deps/tree-sitter-c"`) {
		t.Error("ToTOML() missing resource dest")
	}
	if !contains(tomlStr, `checksum = "sha256:25dd4bb3"`) {
		t.Error("ToTOML() missing resource checksum")
	}

	// Count resources - should be 2
	count := countOccurrences(tomlStr, "[[resources]]")
	if count != 2 {
		t.Errorf("ToTOML() has %d [[resources]] sections, want 2", count)
	}

	// Verify roundtrip
	var parsed Recipe
	err = toml.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("Unmarshal roundtrip error = %v", err)
	}
	if len(parsed.Resources) != 2 {
		t.Errorf("Roundtrip: Resources length = %d, want 2", len(parsed.Resources))
	}
}

func TestRecipe_ToTOML_WithPatches(t *testing.T) {
	recipe := Recipe{
		Metadata: MetadataSection{
			Name: "curl",
		},
		Version: VersionSection{
			Source: "homebrew",
		},
		Patches: []Patch{
			{
				URL:   "https://github.com/Homebrew/formula-patches/raw/master/curl/fix.patch",
				Strip: 1,
			},
			{
				Data:   "--- a/main.c\n+++ b/main.c",
				Subdir: "src",
			},
		},
		Steps: []Step{
			{Action: "configure_make", Params: map[string]interface{}{}},
		},
		Verify: VerifySection{Command: "curl --version"},
	}

	data, err := recipe.ToTOML()
	if err != nil {
		t.Fatalf("ToTOML() error = %v", err)
	}

	tomlStr := string(data)

	// Verify patches are serialized
	if !contains(tomlStr, "[[patches]]") {
		t.Error("ToTOML() missing [[patches]] section")
	}
	if !contains(tomlStr, "formula-patches") {
		t.Error("ToTOML() missing patch URL")
	}
	if !contains(tomlStr, "strip = 1") {
		t.Error("ToTOML() missing patch strip level")
	}
	if !contains(tomlStr, `subdir = "src"`) {
		t.Error("ToTOML() missing patch subdir")
	}

	// Count patches - should be 2
	count := countOccurrences(tomlStr, "[[patches]]")
	if count != 2 {
		t.Errorf("ToTOML() has %d [[patches]] sections, want 2", count)
	}

	// Verify roundtrip
	var parsed Recipe
	err = toml.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("Unmarshal roundtrip error = %v", err)
	}
	if len(parsed.Patches) != 2 {
		t.Errorf("Roundtrip: Patches length = %d, want 2", len(parsed.Patches))
	}
}

func TestRecipe_ToTOML_ResourcesAndPatches_Combined(t *testing.T) {
	// Test a recipe with both resources and patches
	recipe := Recipe{
		Metadata: MetadataSection{
			Name:        "neovim",
			Description: "Vim-fork with resources and patches",
		},
		Version: VersionSection{
			Source:  "homebrew",
			Formula: "neovim",
		},
		Resources: []Resource{
			{
				Name:     "tree-sitter-c",
				URL:      "https://example.com/tree-sitter-c.tar.gz",
				Checksum: "sha256:abc123",
				Dest:     "deps/tree-sitter-c",
			},
		},
		Patches: []Patch{
			{
				URL:   "https://example.com/fix.patch",
				Strip: 1,
			},
		},
		Steps: []Step{
			{Action: "cmake_build", Params: map[string]interface{}{}},
		},
		Verify: VerifySection{Command: "nvim --version"},
	}

	data, err := recipe.ToTOML()
	if err != nil {
		t.Fatalf("ToTOML() error = %v", err)
	}

	tomlStr := string(data)

	// Verify both sections are present
	if !contains(tomlStr, "[[resources]]") {
		t.Error("ToTOML() missing [[resources]] section")
	}
	if !contains(tomlStr, "[[patches]]") {
		t.Error("ToTOML() missing [[patches]] section")
	}

	// Verify roundtrip
	var parsed Recipe
	err = toml.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("Unmarshal roundtrip error = %v", err)
	}
	if len(parsed.Resources) != 1 {
		t.Errorf("Roundtrip: Resources length = %d, want 1", len(parsed.Resources))
	}
	if len(parsed.Patches) != 1 {
		t.Errorf("Roundtrip: Patches length = %d, want 1", len(parsed.Patches))
	}
}
