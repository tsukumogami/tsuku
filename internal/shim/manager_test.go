package shim

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// testLoader returns a recipe.Loader that serves a single recipe with the
// given binaries list. The recipe is minimal -- just enough for
// ExtractBinaries to return the desired names.
func testLoader(t *testing.T, recipeName string, binaries []string) *recipe.Loader {
	t.Helper()

	dir := t.TempDir()
	toml := buildTestRecipe(recipeName, binaries)
	if err := os.WriteFile(filepath.Join(dir, recipeName+".toml"), []byte(toml), 0644); err != nil {
		t.Fatalf("writing test recipe: %v", err)
	}

	local := recipe.NewLocalProvider(dir)
	return recipe.NewLoader(local)
}

// buildTestRecipe creates a minimal TOML recipe that ExtractBinaries will
// parse. Uses install_binaries with outputs.
func buildTestRecipe(name string, binaries []string) string {
	var outputs string
	for i, b := range binaries {
		if i > 0 {
			outputs += ", "
		}
		outputs += `"` + b + `"`
	}

	return `[metadata]
name = "` + name + `"
description = "test"

[version]
source = "static"
version = "1.0.0"

[[steps]]
action = "install_binaries"
outputs = [` + outputs + `]

[verify]
command = "` + name + ` --version"
`
}

func testManager(t *testing.T, recipeName string, binaries []string) (*Manager, string) {
	t.Helper()

	home := t.TempDir()
	cfg := &config.Config{HomeDir: home}
	loader := testLoader(t, recipeName, binaries)
	mgr := NewManager(cfg, loader)
	return mgr, filepath.Join(home, "bin")
}

func TestInstall(t *testing.T) {
	mgr, binDir := testManager(t, "ripgrep", []string{"rg"})

	paths, err := mgr.Install("ripgrep")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("Install returned %d paths, want 1", len(paths))
	}

	want := filepath.Join(binDir, "rg")
	if paths[0] != want {
		t.Errorf("path = %q, want %q", paths[0], want)
	}

	// Verify file content.
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("reading shim: %v", err)
	}
	if string(data) != ShimContent {
		t.Errorf("shim content = %q, want %q", string(data), ShimContent)
	}

	// Verify permissions.
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat shim: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("shim permissions = %v, want 0755", info.Mode().Perm())
	}
}

func TestInstallMultipleBinaries(t *testing.T) {
	mgr, binDir := testManager(t, "age", []string{"age", "age-keygen"})

	paths, err := mgr.Install("age")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if len(paths) != 2 {
		t.Fatalf("Install returned %d paths, want 2", len(paths))
	}

	for _, name := range []string{"age", "age-keygen"} {
		p := filepath.Join(binDir, name)
		if !IsShim(p) {
			t.Errorf("%s is not a shim", p)
		}
	}
}

func TestInstallIdempotent(t *testing.T) {
	mgr, _ := testManager(t, "ripgrep", []string{"rg"})

	if _, err := mgr.Install("ripgrep"); err != nil {
		t.Fatalf("first Install: %v", err)
	}

	// Second install should succeed (overwriting existing shim).
	paths, err := mgr.Install("ripgrep")
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("second Install returned %d paths, want 1", len(paths))
	}
}

func TestInstallRefusesOverwriteNonShim(t *testing.T) {
	mgr, binDir := testManager(t, "ripgrep", []string{"rg"})

	// Create a non-shim file at the target path.
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "rg"), []byte("#!/bin/sh\nreal binary"), 0755); err != nil {
		t.Fatal(err)
	}

	_, err := mgr.Install("ripgrep")
	if err == nil {
		t.Fatal("expected error when overwriting non-shim file")
	}
	if got := err.Error(); got == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestUninstall(t *testing.T) {
	mgr, binDir := testManager(t, "ripgrep", []string{"rg"})

	if _, err := mgr.Install("ripgrep"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if err := mgr.Uninstall("ripgrep"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	// File should be gone.
	p := filepath.Join(binDir, "rg")
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("shim file still exists after Uninstall")
	}

	// Metadata should be empty.
	entries, err := mgr.List()
	if err != nil {
		t.Fatalf("List after Uninstall: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("List returned %d entries after Uninstall, want 0", len(entries))
	}
}

func TestUninstallNoShims(t *testing.T) {
	mgr, _ := testManager(t, "ripgrep", []string{"rg"})

	err := mgr.Uninstall("ripgrep")
	if err == nil {
		t.Fatal("expected error when uninstalling non-existent shims")
	}
}

func TestList(t *testing.T) {
	mgr, _ := testManager(t, "age", []string{"age", "age-keygen"})

	if _, err := mgr.Install("age"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	entries, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("List returned %d entries, want 2", len(entries))
	}

	// Entries should be sorted by name.
	if entries[0].Name != "age" {
		t.Errorf("entries[0].Name = %q, want %q", entries[0].Name, "age")
	}
	if entries[1].Name != "age-keygen" {
		t.Errorf("entries[1].Name = %q, want %q", entries[1].Name, "age-keygen")
	}

	for _, e := range entries {
		if e.Recipe != "age" {
			t.Errorf("entry %q Recipe = %q, want %q", e.Name, e.Recipe, "age")
		}
	}
}

func TestListEmpty(t *testing.T) {
	mgr, _ := testManager(t, "ripgrep", []string{"rg"})

	entries, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("List returned %d entries, want 0", len(entries))
	}
}

func TestIsShim(t *testing.T) {
	dir := t.TempDir()

	shimPath := filepath.Join(dir, "rg")
	if err := os.WriteFile(shimPath, []byte(ShimContent), 0755); err != nil {
		t.Fatal(err)
	}

	notShimPath := filepath.Join(dir, "real")
	if err := os.WriteFile(notShimPath, []byte("#!/bin/sh\necho hello\n"), 0755); err != nil {
		t.Fatal(err)
	}

	if !IsShim(shimPath) {
		t.Error("IsShim returned false for shim file")
	}
	if IsShim(notShimPath) {
		t.Error("IsShim returned true for non-shim file")
	}
	if IsShim(filepath.Join(dir, "nonexistent")) {
		t.Error("IsShim returned true for nonexistent file")
	}
}

func TestUninstallLeavesNonShimFiles(t *testing.T) {
	mgr, binDir := testManager(t, "ripgrep", []string{"rg"})

	// Install the shim first so metadata exists.
	if _, err := mgr.Install("ripgrep"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Replace the shim with a non-shim file.
	p := filepath.Join(binDir, "rg")
	if err := os.WriteFile(p, []byte("#!/bin/sh\nreal binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// Uninstall should not remove the non-shim file.
	if err := mgr.Uninstall("ripgrep"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	// File should still exist.
	if _, err := os.Stat(p); os.IsNotExist(err) {
		t.Error("non-shim file was removed by Uninstall")
	}
}

func TestExtractBinaryNames(t *testing.T) {
	r := &recipe.Recipe{
		Steps: []recipe.Step{
			{
				Action: "install_binaries",
				Params: map[string]interface{}{
					"outputs": []interface{}{"rg", "rga"},
				},
			},
		},
	}

	names := extractBinaryNames(r)
	if len(names) != 2 {
		t.Fatalf("extractBinaryNames returned %d names, want 2", len(names))
	}
	if names[0] != "rg" {
		t.Errorf("names[0] = %q, want %q", names[0], "rg")
	}
	if names[1] != "rga" {
		t.Errorf("names[1] = %q, want %q", names[1], "rga")
	}
}

func TestExtractBinaryNamesFromBinPrefixed(t *testing.T) {
	// ExtractBinaries returns "bin/rg" for simple string outputs.
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Binaries: []string{"bin/rg"},
		},
	}

	names := extractBinaryNames(r)
	if len(names) != 1 {
		t.Fatalf("extractBinaryNames returned %d names, want 1", len(names))
	}
	if names[0] != "rg" {
		t.Errorf("names[0] = %q, want %q", names[0], "rg")
	}
}
