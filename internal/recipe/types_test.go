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
	// Test that singular 'binary' parameter gets "bin/" prefix (github_file, hashicorp_release)
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
