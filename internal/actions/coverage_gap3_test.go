package actions

import (
	"os"
	"path/filepath"
	"testing"
)

// -- cargo_build.go: Dependencies, RequiresNetwork --

func TestCargoBuildAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := CargoBuildAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "rust" {
		t.Errorf("Dependencies().InstallTime = %v, want [rust]", deps.InstallTime)
	}
}

func TestCargoBuildAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := CargoBuildAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() = false, want true")
	}
}

// -- cargo_build.go: linkCargoRegistryCache --

func TestLinkCargoRegistryCache_NoEnvVars(t *testing.T) {
	t.Parallel()
	err := linkCargoRegistryCache([]string{"PATH=/usr/bin"})
	if err != nil {
		t.Errorf("linkCargoRegistryCache() with no env vars = %v, want nil", err)
	}
}

func TestLinkCargoRegistryCache_MissingMount(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cargoHome := filepath.Join(tmpDir, "cargo-home")

	env := []string{
		"CARGO_HOME=" + cargoHome,
		"TSUKU_CARGO_REGISTRY_CACHE=/nonexistent/path",
	}
	err := linkCargoRegistryCache(env)
	if err != nil {
		t.Errorf("linkCargoRegistryCache() with missing mount = %v, want nil", err)
	}
}

func TestLinkCargoRegistryCache_Success(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cargoHome := filepath.Join(tmpDir, "cargo-home")
	registryCache := filepath.Join(tmpDir, "registry-cache")
	if err := os.MkdirAll(registryCache, 0755); err != nil {
		t.Fatal(err)
	}

	env := []string{
		"CARGO_HOME=" + cargoHome,
		"TSUKU_CARGO_REGISTRY_CACHE=" + registryCache,
	}
	err := linkCargoRegistryCache(env)
	if err != nil {
		t.Fatalf("linkCargoRegistryCache() = %v", err)
	}

	registryPath := filepath.Join(cargoHome, "registry")
	target, err := os.Readlink(registryPath)
	if err != nil {
		t.Fatalf("Readlink(%s) error: %v", registryPath, err)
	}
	if target != registryCache {
		t.Errorf("symlink target = %s, want %s", target, registryCache)
	}
}

// -- go_build.go: Dependencies, RequiresNetwork --

func TestGoBuildAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := GoBuildAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "go" {
		t.Errorf("Dependencies().InstallTime = %v, want [go]", deps.InstallTime)
	}
}

func TestGoBuildAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := GoBuildAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() = false, want true")
	}
}

// -- gem_exec.go: Dependencies --

func TestGemExecAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := GemExecAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "ruby" {
		t.Errorf("Dependencies().InstallTime = %v, want [ruby]", deps.InstallTime)
	}
	if len(deps.Runtime) != 1 || deps.Runtime[0] != "ruby" {
		t.Errorf("Dependencies().Runtime = %v, want [ruby]", deps.Runtime)
	}
}

// -- install_gem_direct.go: Dependencies, IsDeterministic, RequiresNetwork --

func TestInstallGemDirectAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := InstallGemDirectAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "ruby" {
		t.Errorf("Dependencies().InstallTime = %v, want [ruby]", deps.InstallTime)
	}
	if len(deps.Runtime) != 1 || deps.Runtime[0] != "ruby" {
		t.Errorf("Dependencies().Runtime = %v, want [ruby]", deps.Runtime)
	}
}

func TestInstallGemDirectAction_IsDeterministic(t *testing.T) {
	t.Parallel()
	action := InstallGemDirectAction{}
	if action.IsDeterministic() {
		t.Error("IsDeterministic() = true, want false")
	}
}

func TestInstallGemDirectAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := InstallGemDirectAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() = false, want true")
	}
}

// -- homebrew.go: Preflight, Dependencies, IsDeterministic, RequiresNetwork, formulaToGHCRPath --

func TestHomebrewAction_Preflight(t *testing.T) {
	t.Parallel()
	action := &HomebrewAction{}

	t.Run("valid", func(t *testing.T) {
		result := action.Preflight(map[string]any{"formula": "libyaml"})
		if len(result.Errors) != 0 {
			t.Errorf("Preflight() errors = %v, want 0", result.Errors)
		}
	})

	t.Run("missing formula", func(t *testing.T) {
		result := action.Preflight(map[string]any{})
		if len(result.Errors) == 0 {
			t.Error("Expected error for missing formula")
		}
	})
}

func TestHomebrewAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := HomebrewAction{}
	deps := action.Dependencies()
	if len(deps.LinuxInstallTime) != 1 || deps.LinuxInstallTime[0] != "patchelf" {
		t.Errorf("Dependencies().LinuxInstallTime = %v, want [patchelf]", deps.LinuxInstallTime)
	}
}

func TestHomebrewAction_IsDeterministic_Direct(t *testing.T) {
	t.Parallel()
	action := HomebrewAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestHomebrewAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := HomebrewAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() = false, want true")
	}
}

func TestFormulaToGHCRPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"libyaml", "libyaml"},
		{"openssl@3", "openssl/3"},
		{"python@3.12", "python/3.12"},
	}
	for _, tt := range tests {
		if got := formulaToGHCRPath(tt.input); got != tt.want {
			t.Errorf("formulaToGHCRPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGhcrHTTPClient(t *testing.T) {
	t.Parallel()
	client := ghcrHTTPClient()
	if client == nil {
		t.Fatal("ghcrHTTPClient() returned nil")
	}
	if client.Timeout == 0 {
		t.Error("ghcrHTTPClient() returned client with zero timeout")
	}
}

// -- homebrew_relocate.go: IsDeterministic, Dependencies, extractBottlePrefixes --

func TestHomebrewRelocateAction_IsDeterministic(t *testing.T) {
	t.Parallel()
	action := HomebrewRelocateAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}

func TestHomebrewRelocateAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := HomebrewRelocateAction{}
	deps := action.Dependencies()
	if len(deps.LinuxInstallTime) != 1 || deps.LinuxInstallTime[0] != "patchelf" {
		t.Errorf("Dependencies().LinuxInstallTime = %v, want [patchelf]", deps.LinuxInstallTime)
	}
}

func TestHomebrewRelocateAction_ExtractBottlePrefixes(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	content := []byte(`some text /tmp/action-validator-abc12345/.install/libyaml/0.2.5/lib/libyaml.so more text
another line /tmp/action-validator-abc12345/.install/libyaml/0.2.5/include/yaml.h end`)

	prefixMap := make(map[string]string)
	action.extractBottlePrefixes(content, prefixMap)

	if len(prefixMap) != 2 {
		t.Errorf("extractBottlePrefixes() found %d entries, want 2", len(prefixMap))
	}

	expectedPrefix := "/tmp/action-validator-abc12345/.install/libyaml/0.2.5"
	for fullPath, prefix := range prefixMap {
		if prefix != expectedPrefix {
			t.Errorf("prefix for %q = %q, want %q", fullPath, prefix, expectedPrefix)
		}
	}
}

func TestHomebrewRelocateAction_ExtractBottlePrefixes_NoMatch(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}
	prefixMap := make(map[string]string)
	action.extractBottlePrefixes([]byte("no bottle paths here"), prefixMap)
	if len(prefixMap) != 0 {
		t.Errorf("extractBottlePrefixes() found %d entries for no-match content, want 0", len(prefixMap))
	}
}

func TestHomebrewRelocateAction_ExtractBottlePrefixes_NoInstallSegment(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}
	// Has the marker but no /.install/ segment
	content := []byte("/tmp/action-validator-abc12345/other/path")
	prefixMap := make(map[string]string)
	action.extractBottlePrefixes(content, prefixMap)
	if len(prefixMap) != 0 {
		t.Errorf("extractBottlePrefixes() found %d entries for no-install content, want 0", len(prefixMap))
	}
}

// -- eval_deps.go: checkEvalDepsInDir additional paths --

func TestCheckEvalDepsInDir_MixedInstalled(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, "go-1.21.0"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "rust-1.76.0"), 0755); err != nil {
		t.Fatal(err)
	}

	missing := checkEvalDepsInDir([]string{"go", "python", "rust"}, tmpDir)
	if len(missing) != 1 || missing[0] != "python" {
		t.Errorf("checkEvalDepsInDir() = %v, want [python]", missing)
	}
}

func TestCheckEvalDepsInDir_EmptyDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	missing := checkEvalDepsInDir([]string{"go"}, tmpDir)
	if len(missing) != 1 {
		t.Errorf("checkEvalDepsInDir() = %v, want [go]", missing)
	}
}

// -- set_rpath.go: SetRpathAction.IsDeterministic --

func TestSetRpathAction_IsDeterministic_Direct(t *testing.T) {
	t.Parallel()
	action := SetRpathAction{}
	if !action.IsDeterministic() {
		t.Error("IsDeterministic() = false, want true")
	}
}
