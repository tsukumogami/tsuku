package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/testutil"
)

func TestManager_InstallLibrary(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create work directory with .install subdirectory
	workDir := t.TempDir()
	installDir := filepath.Join(workDir, ".install")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		t.Fatalf("failed to create .install dir: %v", err)
	}

	// Create test files in .install
	testFile := filepath.Join(installDir, "lib", "libyaml.so")
	if err := os.MkdirAll(filepath.Dir(testFile), 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}
	if err := os.WriteFile(testFile, []byte("test library content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Install library
	opts := LibraryInstallOptions{}
	err := mgr.InstallLibrary("libyaml", "0.2.5", workDir, opts)
	if err != nil {
		t.Fatalf("InstallLibrary() error = %v", err)
	}

	// Verify library installed to libs directory
	libDir := cfg.LibDir("libyaml", "0.2.5")
	if _, err := os.Stat(libDir); os.IsNotExist(err) {
		t.Errorf("library directory not created: %s", libDir)
	}

	// Verify file copied
	copiedFile := filepath.Join(libDir, "lib", "libyaml.so")
	if _, err := os.Stat(copiedFile); os.IsNotExist(err) {
		t.Errorf("library file not copied: %s", copiedFile)
	}
}

func TestManager_InstallLibrary_WithUsedBy(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create work directory with .install subdirectory
	workDir := t.TempDir()
	installDir := filepath.Join(workDir, ".install")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		t.Fatalf("failed to create .install dir: %v", err)
	}

	// Create test file
	testFile := filepath.Join(installDir, "lib", "libyaml.so")
	if err := os.MkdirAll(filepath.Dir(testFile), 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Install with used_by tracking
	opts := LibraryInstallOptions{
		ToolNameVersion: "ruby-3.4.0",
	}
	err := mgr.InstallLibrary("libyaml", "0.2.5", workDir, opts)
	if err != nil {
		t.Fatalf("InstallLibrary() error = %v", err)
	}

	// Verify state updated
	state, _ := mgr.state.Load()
	libState := state.Libs["libyaml"]["0.2.5"]

	if len(libState.UsedBy) != 1 {
		t.Fatalf("UsedBy length = %d, want 1", len(libState.UsedBy))
	}

	if libState.UsedBy[0] != "ruby-3.4.0" {
		t.Errorf("UsedBy[0] = %s, want ruby-3.4.0", libState.UsedBy[0])
	}
}

func TestManager_IsLibraryInstalled(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Not installed initially
	if mgr.IsLibraryInstalled("libyaml", "0.2.5") {
		t.Error("IsLibraryInstalled() should return false for non-existent library")
	}

	// Create library directory
	libDir := cfg.LibDir("libyaml", "0.2.5")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	// Now installed
	if !mgr.IsLibraryInstalled("libyaml", "0.2.5") {
		t.Error("IsLibraryInstalled() should return true for existing library")
	}
}

func TestManager_GetInstalledLibraryVersion(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Not installed initially
	if ver := mgr.GetInstalledLibraryVersion("libyaml"); ver != "" {
		t.Errorf("GetInstalledLibraryVersion() = %q, want empty string", ver)
	}

	// Create library directory
	libDir := cfg.LibDir("libyaml", "0.2.5")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	// Now returns version
	if ver := mgr.GetInstalledLibraryVersion("libyaml"); ver != "0.2.5" {
		t.Errorf("GetInstalledLibraryVersion() = %q, want %q", ver, "0.2.5")
	}
}

func TestManager_GetInstalledLibraryVersion_MultipleVersions(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create multiple versions
	for _, ver := range []string{"0.2.4", "0.2.5"} {
		libDir := cfg.LibDir("libyaml", ver)
		if err := os.MkdirAll(libDir, 0755); err != nil {
			t.Fatalf("failed to create lib dir: %v", err)
		}
	}

	// Returns any version (implementation returns first match)
	ver := mgr.GetInstalledLibraryVersion("libyaml")
	if ver != "0.2.4" && ver != "0.2.5" {
		t.Errorf("GetInstalledLibraryVersion() = %q, want 0.2.4 or 0.2.5", ver)
	}
}

func TestManager_AddLibraryUsedBy(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	err := mgr.AddLibraryUsedBy("libyaml", "0.2.5", "ruby-3.4.0")
	if err != nil {
		t.Fatalf("AddLibraryUsedBy() error = %v", err)
	}

	// Verify through state
	state, _ := mgr.state.Load()
	libState := state.Libs["libyaml"]["0.2.5"]

	if len(libState.UsedBy) != 1 {
		t.Fatalf("UsedBy length = %d, want 1", len(libState.UsedBy))
	}

	if libState.UsedBy[0] != "ruby-3.4.0" {
		t.Errorf("UsedBy[0] = %s, want ruby-3.4.0", libState.UsedBy[0])
	}
}

func TestManager_LibDir(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	libDir := mgr.LibDir("libyaml", "0.2.5")
	expected := filepath.Join(cfg.LibsDir, "libyaml-0.2.5")

	if libDir != expected {
		t.Errorf("LibDir() = %q, want %q", libDir, expected)
	}
}

func TestCheckLibraryInstalled(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	// Not installed
	path := CheckLibraryInstalled(cfg, "libyaml", "0.2.5")
	if path != "" {
		t.Errorf("CheckLibraryInstalled() = %q, want empty string", path)
	}

	// Create library directory
	libDir := cfg.LibDir("libyaml", "0.2.5")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	// Now returns path
	path = CheckLibraryInstalled(cfg, "libyaml", "0.2.5")
	if path != libDir {
		t.Errorf("CheckLibraryInstalled() = %q, want %q", path, libDir)
	}
}

func TestManager_GetInstalledLibraryVersion_PartialMatch(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create library with similar name prefix
	libDir1 := cfg.LibDir("lib", "1.0.0")     // lib-1.0.0
	libDir2 := cfg.LibDir("libyaml", "0.2.5") // libyaml-0.2.5

	for _, dir := range []string{libDir1, libDir2} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create lib dir: %v", err)
		}
	}

	// Should only match exact name prefix
	ver := mgr.GetInstalledLibraryVersion("lib")
	if ver != "1.0.0" {
		t.Errorf("GetInstalledLibraryVersion(lib) = %q, want 1.0.0", ver)
	}

	ver = mgr.GetInstalledLibraryVersion("libyaml")
	if ver != "0.2.5" {
		t.Errorf("GetInstalledLibraryVersion(libyaml) = %q, want 0.2.5", ver)
	}
}

func TestManager_ListLibraries_Empty(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// libs directory doesn't exist yet
	libs, err := mgr.ListLibraries()
	if err != nil {
		t.Fatalf("ListLibraries() error = %v", err)
	}
	if len(libs) != 0 {
		t.Errorf("ListLibraries() returned %d libraries, want 0", len(libs))
	}
}

func TestManager_ListLibraries(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create some library directories
	libs := []struct {
		name    string
		version string
	}{
		{"libyaml", "0.2.5"},
		{"libffi", "3.4.4"},
	}

	for _, lib := range libs {
		libDir := cfg.LibDir(lib.name, lib.version)
		if err := os.MkdirAll(libDir, 0755); err != nil {
			t.Fatalf("failed to create lib dir: %v", err)
		}
	}

	// List libraries
	result, err := mgr.ListLibraries()
	if err != nil {
		t.Fatalf("ListLibraries() error = %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("ListLibraries() returned %d libraries, want 2", len(result))
	}

	// Check that both are present (order may vary)
	found := make(map[string]bool)
	for _, lib := range result {
		found[lib.Name+"-"+lib.Version] = true
	}

	for _, lib := range libs {
		key := lib.name + "-" + lib.version
		if !found[key] {
			t.Errorf("ListLibraries() missing %s", key)
		}
	}
}

func TestManager_ListLibraries_IgnoresFiles(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create libs directory with a file (not directory)
	if err := os.MkdirAll(cfg.LibsDir, 0755); err != nil {
		t.Fatalf("failed to create libs dir: %v", err)
	}

	// Create a regular file (should be ignored)
	filePath := filepath.Join(cfg.LibsDir, "some-file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Create a valid library directory
	libDir := cfg.LibDir("libyaml", "0.2.5")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	result, err := mgr.ListLibraries()
	if err != nil {
		t.Fatalf("ListLibraries() error = %v", err)
	}

	// Should only return the directory, not the file
	if len(result) != 1 {
		t.Fatalf("ListLibraries() returned %d libraries, want 1", len(result))
	}

	if result[0].Name != "libyaml" || result[0].Version != "0.2.5" {
		t.Errorf("ListLibraries()[0] = {%s, %s}, want {libyaml, 0.2.5}", result[0].Name, result[0].Version)
	}
}
