package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- meson_build.go: findLibraryDirectories, buildRpathFromLibDirs --

func TestFindLibraryDirectories(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create directory structure with .so and .dylib files
	libDir := filepath.Join(tmpDir, "lib")
	subDir := filepath.Join(libDir, "pkgconfig")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a .so file in lib/
	if err := os.WriteFile(filepath.Join(libDir, "libfoo.so"), []byte("lib"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create a .dylib in lib/
	if err := os.WriteFile(filepath.Join(libDir, "libbar.dylib"), []byte("lib"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create non-library file
	if err := os.WriteFile(filepath.Join(libDir, "libfoo.a"), []byte("lib"), 0644); err != nil {
		t.Fatal(err)
	}

	dirs := findLibraryDirectories(tmpDir)
	if len(dirs) == 0 {
		t.Error("findLibraryDirectories() returned empty")
	}
	// Should contain the lib dir (only once, despite two library files)
	found := false
	for _, d := range dirs {
		if d == libDir {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("findLibraryDirectories() = %v, expected to contain %s", dirs, libDir)
	}
}

func TestFindLibraryDirectories_Empty(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	dirs := findLibraryDirectories(tmpDir)
	if len(dirs) != 0 {
		t.Errorf("findLibraryDirectories() on empty dir = %v, want empty", dirs)
	}
}

func TestBuildRpathFromLibDirs(t *testing.T) {
	t.Parallel()
	installDir := "/home/user/.tsuku/tools/mylib-1.0"

	t.Run("single lib dir", func(t *testing.T) {
		libPaths := []string{filepath.Join(installDir, "lib")}
		rpath := buildRpathFromLibDirs(libPaths, installDir)
		if !strings.HasPrefix(rpath, "$ORIGIN/") {
			t.Errorf("buildRpathFromLibDirs() = %q, want $ORIGIN/ prefix", rpath)
		}
		if !strings.Contains(rpath, "../lib") {
			t.Errorf("buildRpathFromLibDirs() = %q, want to contain ../lib", rpath)
		}
	})

	t.Run("empty", func(t *testing.T) {
		rpath := buildRpathFromLibDirs(nil, installDir)
		if rpath != "" {
			t.Errorf("buildRpathFromLibDirs(nil) = %q, want empty", rpath)
		}
	})
}

// -- configure_make.go: buildAutotoolsEnv with dependencies --

func TestBuildAutotoolsEnv_WithDeps(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create mock dependency directories
	toolsDir := filepath.Join(tmpDir, "tools")
	libsDir := filepath.Join(tmpDir, "libs")

	// Create a dependency with bin, lib, include, and pkgconfig
	depDir := filepath.Join(libsDir, "openssl-3.0.0")
	for _, sub := range []string{"bin", "lib/pkgconfig", "include"} {
		if err := os.MkdirAll(filepath.Join(depDir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		ToolsDir:   toolsDir,
		LibsDir:    libsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"openssl": "3.0.0"},
		},
	}

	env := buildAutotoolsEnv(ctx)

	// Should contain PKG_CONFIG_PATH
	hasPkgConfig := false
	hasCppFlags := false
	hasLdFlags := false
	hasSourceDateEpoch := false
	for _, e := range env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") {
			hasPkgConfig = true
		}
		if strings.HasPrefix(e, "CPPFLAGS=") && strings.Contains(e, "openssl") {
			hasCppFlags = true
		}
		if strings.HasPrefix(e, "LDFLAGS=") && strings.Contains(e, "openssl") {
			hasLdFlags = true
		}
		if e == "SOURCE_DATE_EPOCH=0" {
			hasSourceDateEpoch = true
		}
	}
	if !hasPkgConfig {
		t.Error("Expected PKG_CONFIG_PATH in env")
	}
	if !hasCppFlags {
		t.Error("Expected CPPFLAGS with openssl path")
	}
	if !hasLdFlags {
		t.Error("Expected LDFLAGS with openssl path")
	}
	if !hasSourceDateEpoch {
		t.Error("Expected SOURCE_DATE_EPOCH=0")
	}
}

func TestBuildAutotoolsEnv_NoDeps(t *testing.T) {
	t.Parallel()
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
		Version:    "1.0.0",
	}
	env := buildAutotoolsEnv(ctx)

	// Should still contain SOURCE_DATE_EPOCH
	hasSDEpoch := false
	for _, e := range env {
		if e == "SOURCE_DATE_EPOCH=0" {
			hasSDEpoch = true
		}
	}
	if !hasSDEpoch {
		t.Error("Expected SOURCE_DATE_EPOCH=0 even without deps")
	}
}

func TestBuildAutotoolsEnv_WithExecPaths(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "mybin")
	if err := os.MkdirAll(execPath, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		ExecPaths:  []string{execPath},
	}
	env := buildAutotoolsEnv(ctx)

	hasExecInPath := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") && strings.Contains(e, execPath) {
			hasExecInPath = true
		}
	}
	if !hasExecInPath {
		t.Error("Expected ExecPaths in PATH env var")
	}
}

// -- configure_make.go: ConfigureMakeAction Dependencies --

func TestConfigureMakeAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := ConfigureMakeAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 3 {
		t.Errorf("Dependencies().InstallTime has %d entries, want 3", len(deps.InstallTime))
	}
	expected := map[string]bool{"make": true, "zig": true, "pkg-config": true}
	for _, dep := range deps.InstallTime {
		if !expected[dep] {
			t.Errorf("Unexpected dependency: %s", dep)
		}
	}
}

// -- meson_build.go: MesonBuildAction Dependencies --

func TestMesonBuildAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := MesonBuildAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) == 0 {
		t.Error("Dependencies().InstallTime is empty")
	}
}

// -- nix_install.go: NixInstallAction Dependencies --

func TestNixInstallAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := NixInstallAction{}
	deps := action.Dependencies()
	if len(deps.InstallTime) != 1 || deps.InstallTime[0] != "nix-portable" {
		t.Errorf("Dependencies().InstallTime = %v, want [nix-portable]", deps.InstallTime)
	}
}

func TestNixInstallAction_RequiresNetwork(t *testing.T) {
	t.Parallel()
	action := NixInstallAction{}
	if !action.RequiresNetwork() {
		t.Error("RequiresNetwork() = false, want true")
	}
}

// -- cargo_build.go: buildDeterministicCargoEnv --

func TestBuildDeterministicCargoEnv_Basic(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "bin", "cargo")
	if err := os.MkdirAll(filepath.Dir(cargoPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cargoPath, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	env := buildDeterministicCargoEnv(cargoPath, tmpDir, nil)

	hasCargoHome := false
	hasCargoIncremental := false
	hasSourceDateEpoch := false
	for _, e := range env {
		if strings.HasPrefix(e, "CARGO_HOME=") {
			hasCargoHome = true
		}
		if e == "CARGO_INCREMENTAL=0" {
			hasCargoIncremental = true
		}
		if e == "SOURCE_DATE_EPOCH=0" {
			hasSourceDateEpoch = true
		}
	}
	if !hasCargoHome {
		t.Error("Expected CARGO_HOME in env")
	}
	if !hasCargoIncremental {
		t.Error("Expected CARGO_INCREMENTAL=0 in env")
	}
	if !hasSourceDateEpoch {
		t.Error("Expected SOURCE_DATE_EPOCH=0 in env")
	}
}

func TestBuildDeterministicCargoEnv_WithContext(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "bin", "cargo")
	if err := os.MkdirAll(filepath.Dir(cargoPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cargoPath, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	execPath := filepath.Join(tmpDir, "extra-bin")
	if err := os.MkdirAll(execPath, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:   context.Background(),
		WorkDir:   tmpDir,
		ExecPaths: []string{execPath},
	}

	env := buildDeterministicCargoEnv(cargoPath, tmpDir, ctx)

	hasExecInPath := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") && strings.Contains(e, execPath) {
			hasExecInPath = true
		}
	}
	if !hasExecInPath {
		t.Error("Expected ExecPaths in PATH")
	}
}

func TestBuildDeterministicCargoEnv_WithLibDeps(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cargoPath := filepath.Join(tmpDir, "bin", "cargo")
	if err := os.MkdirAll(filepath.Dir(cargoPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cargoPath, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a library dep with lib, include, lib/pkgconfig, and bin
	libsDir := filepath.Join(tmpDir, "libs")
	depDir := filepath.Join(libsDir, "openssl-3.0.0")
	for _, sub := range []string{"bin", "lib/pkgconfig", "include"} {
		if err := os.MkdirAll(filepath.Join(depDir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: tmpDir,
		LibsDir: libsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"openssl": "3.0.0"},
		},
	}

	env := buildDeterministicCargoEnv(cargoPath, tmpDir, ctx)

	hasPkgConfig := false
	hasCInclude := false
	hasLibrary := false
	for _, e := range env {
		if strings.HasPrefix(e, "PKG_CONFIG_PATH=") && strings.Contains(e, "openssl") {
			hasPkgConfig = true
		}
		if strings.HasPrefix(e, "C_INCLUDE_PATH=") && strings.Contains(e, "openssl") {
			hasCInclude = true
		}
		if strings.HasPrefix(e, "LIBRARY_PATH=") && strings.Contains(e, "openssl") {
			hasLibrary = true
		}
	}
	if !hasPkgConfig {
		t.Error("Expected PKG_CONFIG_PATH with openssl")
	}
	if !hasCInclude {
		t.Error("Expected C_INCLUDE_PATH with openssl")
	}
	if !hasLibrary {
		t.Error("Expected LIBRARY_PATH with openssl")
	}
}

// -- configure_make.go: touchAutogeneratedFiles --

func TestTouchAutogeneratedFiles(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create some autotools-style files
	files := []string{"configure.ac", "Makefile.in", "aclocal.m4", "configure"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Should not panic
	touchAutogeneratedFiles(tmpDir)

	// Verify files still exist
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(tmpDir, f)); os.IsNotExist(err) {
			t.Errorf("File %s disappeared after touchAutogeneratedFiles", f)
		}
	}
}

// -- composites.go: CopyDirectory --

func TestCopyDirectory(t *testing.T) {
	t.Parallel()
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create some files in src
	if err := os.MkdirAll(filepath.Join(srcDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "bin", "tool"), []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "README"), []byte("readme"), 0644); err != nil {
		t.Fatal(err)
	}

	err := CopyDirectory(srcDir, dstDir)
	if err != nil {
		t.Fatalf("CopyDirectory() error: %v", err)
	}

	// Verify files were copied
	if _, err := os.Stat(filepath.Join(dstDir, "bin", "tool")); os.IsNotExist(err) {
		t.Error("Expected bin/tool to be copied")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "README")); os.IsNotExist(err) {
		t.Error("Expected README to be copied")
	}
}

// -- download.go: DownloadAction.Decompose --

func TestDownloadAction_Decompose_Basic(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	steps, err := action.Decompose(ctx, map[string]any{
		"url": "https://example.com/{version}/tool-{os}-{arch}.tar.gz",
	})
	if err != nil {
		t.Fatalf("Decompose() error: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("Decompose() returned %d steps, want 1", len(steps))
	}
	if steps[0].Action != "download_file" {
		t.Errorf("step action = %q, want download_file", steps[0].Action)
	}
	url, _ := GetString(steps[0].Params, "url")
	if !strings.Contains(url, "1.0.0") {
		t.Errorf("URL %q should contain version 1.0.0", url)
	}
}

func TestDownloadAction_Decompose_WithMappings(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "2.0.0",
		VersionTag: "v2.0.0",
		OS:         "darwin",
		Arch:       "amd64",
	}

	steps, err := action.Decompose(ctx, map[string]any{
		"url":          "https://example.com/tool-{os}-{arch}.tar.gz",
		"os_mapping":   map[string]any{"darwin": "macos"},
		"arch_mapping": map[string]any{"amd64": "x64"},
	})
	if err != nil {
		t.Fatalf("Decompose() error: %v", err)
	}
	url, _ := GetString(steps[0].Params, "url")
	if !strings.Contains(url, "macos") {
		t.Errorf("URL %q should contain mapped OS 'macos'", url)
	}
	if !strings.Contains(url, "x64") {
		t.Errorf("URL %q should contain mapped arch 'x64'", url)
	}
}

func TestDownloadAction_Decompose_WithDest(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	steps, err := action.Decompose(ctx, map[string]any{
		"url":  "https://example.com/tool.tar.gz",
		"dest": "custom-name-{version}.tar.gz",
	})
	if err != nil {
		t.Fatalf("Decompose() error: %v", err)
	}
	dest, _ := GetString(steps[0].Params, "dest")
	if dest != "custom-name-1.0.0.tar.gz" {
		t.Errorf("dest = %q, want custom-name-1.0.0.tar.gz", dest)
	}
}

func TestDownloadAction_Decompose_MissingURL(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
	ctx := &EvalContext{Context: context.Background(), Version: "1.0.0", OS: "linux", Arch: "amd64"}
	_, err := action.Decompose(ctx, map[string]any{})
	if err == nil {
		t.Error("Expected error for missing URL")
	}
}

// -- composites.go: GitHubArchiveAction.resolveAssetName --

func TestGitHubArchiveAction_ResolveAssetName(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	name, err := action.resolveAssetName(ctx, map[string]any{}, "tool-{version}-{os}-{arch}.tar.gz", "owner/repo")
	if err != nil {
		t.Fatalf("resolveAssetName() error: %v", err)
	}
	if name != "tool-1.0.0-linux-amd64.tar.gz" {
		t.Errorf("resolveAssetName() = %q, want tool-1.0.0-linux-amd64.tar.gz", name)
	}
}

func TestGitHubArchiveAction_ResolveAssetName_WithMappings(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}

	ctx := &EvalContext{
		Context:    context.Background(),
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "darwin",
		Arch:       "arm64",
	}

	params := map[string]any{
		"os_mapping":   map[string]any{"darwin": "macOS"},
		"arch_mapping": map[string]any{"arm64": "aarch64"},
	}

	name, err := action.resolveAssetName(ctx, params, "tool-{os}-{arch}.tar.gz", "owner/repo")
	if err != nil {
		t.Fatalf("resolveAssetName() error: %v", err)
	}
	if name != "tool-macOS-aarch64.tar.gz" {
		t.Errorf("resolveAssetName() = %q, want tool-macOS-aarch64.tar.gz", name)
	}
}

// -- download.go: Preflight additional warnings --

func TestDownloadAction_Preflight_UnusedOSMapping(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
	result := action.Preflight(map[string]any{
		"url":        "https://example.com/tool-{version}.tar.gz",
		"os_mapping": map[string]any{"darwin": "macos"},
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for unused os_mapping")
	}
}

func TestDownloadAction_Preflight_UnusedArchMapping(t *testing.T) {
	t.Parallel()
	action := &DownloadAction{}
	result := action.Preflight(map[string]any{
		"url":          "https://example.com/tool-{version}.tar.gz",
		"arch_mapping": map[string]any{"amd64": "x64"},
	})
	if len(result.Warnings) == 0 {
		t.Error("Expected warning for unused arch_mapping")
	}
}

// -- download_cache.go: writeMeta roundtrip --

func TestDownloadCache_WriteMeta_ReadMeta(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cache := NewDownloadCache(tmpDir)

	meta := &downloadCacheEntry{
		URL:      "https://example.com/test.tar.gz",
		Checksum: "abc123",
	}

	metaPath := filepath.Join(tmpDir, "test.meta")
	if err := cache.writeMeta(metaPath, meta); err != nil {
		t.Fatalf("writeMeta() error: %v", err)
	}

	readBack, err := cache.readMeta(metaPath)
	if err != nil {
		t.Fatalf("readMeta() error: %v", err)
	}
	if readBack.URL != meta.URL {
		t.Errorf("readMeta().URL = %q, want %q", readBack.URL, meta.URL)
	}
	if readBack.Checksum != meta.Checksum {
		t.Errorf("readMeta().Checksum = %q, want %q", readBack.Checksum, meta.Checksum)
	}
}

// -- configure_make.go: buildAutotoolsEnv with libcurl dependency --

func TestBuildAutotoolsEnv_WithLibcurl(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	libsDir := filepath.Join(tmpDir, "libs")

	// Create libcurl dependency with bin/curl-config
	depDir := filepath.Join(libsDir, "libcurl-8.0.0")
	if err := os.MkdirAll(filepath.Join(depDir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "bin", "curl-config"), []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(depDir, "lib"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(depDir, "include"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		ToolsDir:   filepath.Join(tmpDir, "tools"),
		LibsDir:    libsDir,
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"libcurl": "8.0.0"},
		},
	}

	env := buildAutotoolsEnv(ctx)

	hasCurlConfig := false
	hasCurlDir := false
	hasNoCurl := false
	for _, e := range env {
		if strings.HasPrefix(e, "CURL_CONFIG=") {
			hasCurlConfig = true
		}
		if strings.HasPrefix(e, "CURLDIR=") {
			hasCurlDir = true
		}
		if e == "NO_CURL=" {
			hasNoCurl = true
		}
	}
	if !hasCurlConfig {
		t.Error("Expected CURL_CONFIG in env")
	}
	if !hasCurlDir {
		t.Error("Expected CURLDIR in env")
	}
	if !hasNoCurl {
		t.Error("Expected NO_CURL= in env")
	}
}
