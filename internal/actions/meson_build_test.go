package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMesonBuildAction_Name(t *testing.T) {
	t.Parallel()
	action := &MesonBuildAction{}
	if action.Name() != "meson_build" {
		t.Errorf("Name() = %q, want %q", action.Name(), "meson_build")
	}
}

func TestMesonBuildAction_Execute_MissingSourceDir(t *testing.T) {
	t.Parallel()
	action := &MesonBuildAction{}
	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    t.TempDir(),
		InstallDir: t.TempDir(),
	}

	params := map[string]interface{}{
		"executables": []string{"test"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Expected error for missing source_dir")
	}
	if err.Error() != "meson_build action requires 'source_dir' parameter" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestMesonBuildAction_Execute_MissingExecutables(t *testing.T) {
	t.Parallel()
	action := &MesonBuildAction{}
	workDir := t.TempDir()

	// Create source dir with a meson.build
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "meson.build"), []byte("project('test', 'c')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: t.TempDir(),
	}

	params := map[string]interface{}{
		"source_dir": sourceDir,
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Expected error for missing executables")
	}
	if err.Error() != "meson_build action requires 'executables' parameter with at least one executable" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestMesonBuildAction_Execute_MesonBuildNotFound(t *testing.T) {
	t.Parallel()
	action := &MesonBuildAction{}
	workDir := t.TempDir()

	// Create empty source dir (no meson.build)
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: t.TempDir(),
	}

	params := map[string]interface{}{
		"source_dir":  sourceDir,
		"executables": []string{"test"},
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Expected error for missing meson.build")
	}
	if !strings.Contains(err.Error(), "meson.build not found") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestMesonBuildAction_Execute_InvalidExecutableName(t *testing.T) {
	t.Parallel()
	action := &MesonBuildAction{}
	workDir := t.TempDir()

	// Create source dir with a meson.build
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "meson.build"), []byte("project('test', 'c')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: t.TempDir(),
	}

	testCases := []string{
		"../evil",
		"/absolute/path",
		"with/slash",
		"..",
		".",
		"",
	}

	for _, tc := range testCases {
		params := map[string]interface{}{
			"source_dir":  sourceDir,
			"executables": []string{tc},
		}

		err := action.Execute(ctx, params)
		if err == nil {
			t.Errorf("Expected error for invalid executable name %q", tc)
		}
	}
}

func TestMesonBuildAction_Execute_InvalidMesonArg(t *testing.T) {
	t.Parallel()
	action := &MesonBuildAction{}
	workDir := t.TempDir()

	// Create source dir with a meson.build
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "meson.build"), []byte("project('test', 'c')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: t.TempDir(),
	}

	invalidArgs := []string{
		"-Dopt=val;rm -rf /",
		"-Dopt && evil",
		"-Dopt | cat /etc/passwd",
		"-Dopt `id`",
		"-Dopt $(whoami)",
		"-Dopt{evil}",
	}

	for _, arg := range invalidArgs {
		params := map[string]interface{}{
			"source_dir":  sourceDir,
			"executables": []string{"test"},
			"meson_args":  []string{arg},
		}

		err := action.Execute(ctx, params)
		if err == nil {
			t.Errorf("Expected error for invalid meson arg %q", arg)
		}
		if !strings.Contains(err.Error(), "invalid meson argument") {
			t.Errorf("Unexpected error for %q: %v", arg, err)
		}
	}
}

func TestMesonBuildAction_Execute_InvalidBuildtype(t *testing.T) {
	t.Parallel()
	action := &MesonBuildAction{}
	workDir := t.TempDir()

	// Create source dir with a meson.build
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "meson.build"), []byte("project('test', 'c')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: t.TempDir(),
	}

	params := map[string]interface{}{
		"source_dir":  sourceDir,
		"executables": []string{"test"},
		"buildtype":   "invalid_buildtype",
	}

	err := action.Execute(ctx, params)
	if err == nil {
		t.Error("Expected error for invalid buildtype")
	}
	if !strings.Contains(err.Error(), "invalid buildtype") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestMesonBuildAction_Execute_RelativeSourceDir(t *testing.T) {
	t.Parallel()
	action := &MesonBuildAction{}
	workDir := t.TempDir()

	// Create source dir with a meson.build
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "meson.build"), []byte("project('test', 'c')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: t.TempDir(),
	}

	// Use relative path
	params := map[string]interface{}{
		"source_dir":  "src",
		"executables": []string{"test"},
	}

	// This will fail at meson step (won't setup), but proves relative path resolved
	err := action.Execute(ctx, params)
	if err != nil && strings.Contains(err.Error(), "meson.build not found") {
		t.Error("Relative source_dir should have been resolved")
	}
}

func TestMesonBuildAction_Registered(t *testing.T) {
	t.Parallel()
	// Verify meson_build is registered as a primitive action
	if !IsPrimitive("meson_build") {
		t.Error("meson_build should be registered as a primitive action")
	}

	// Verify it's in the action registry
	action := Get("meson_build")
	if action == nil {
		t.Error("meson_build should be registered in the action registry")
	}
}

func TestMesonBuildAction_NotDeterministic(t *testing.T) {
	t.Parallel()
	// meson_build uses system compiler, so it's not deterministic
	if IsDeterministic("meson_build") {
		t.Error("meson_build should not be deterministic")
	}
}

func TestIsValidMesonArg(t *testing.T) {
	t.Parallel()
	validArgs := []string{
		"-Dbuildtype=release",
		"-Dfeature=enabled",
		"-Dlibdir=lib",
		"-Dprefix=/usr/local",
		"-Dopt=val with spaces",
	}

	for _, arg := range validArgs {
		if !isValidMesonArg(arg) {
			t.Errorf("isValidMesonArg(%q) = false, want true", arg)
		}
	}

	invalidArgs := []string{
		"",                        // empty
		"-Dopt;rm",                // shell metachar
		"-Dopt && echo",           // shell metachar
		"-Dopt | cat",             // shell metachar
		"-Dopt `id`",              // shell metachar
		"-Dopt$(whoami)",          // shell metachar
		"-Dopt{evil}",             // shell metachar
		"-Dopt\necho",             // newline
		string(make([]byte, 501)), // too long
	}

	for _, arg := range invalidArgs {
		if isValidMesonArg(arg) {
			if len(arg) <= 20 {
				t.Errorf("isValidMesonArg(%q) = true, want false", arg)
			} else {
				t.Errorf("isValidMesonArg(len=%d) = true, want false", len(arg))
			}
		}
	}
}

func TestIsValidBuildtype(t *testing.T) {
	t.Parallel()
	validTypes := []string{"release", "debug", "plain", "debugoptimized"}
	for _, buildtype := range validTypes {
		if !isValidBuildtype(buildtype) {
			t.Errorf("isValidBuildtype(%q) = false, want true", buildtype)
		}
	}

	invalidTypes := []string{"", "invalid", "Release", "DEBUG", "custom"}
	for _, buildtype := range invalidTypes {
		if isValidBuildtype(buildtype) {
			t.Errorf("isValidBuildtype(%q) = true, want false", buildtype)
		}
	}
}
