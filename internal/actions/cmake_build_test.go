package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCMakeBuildAction_Name(t *testing.T) {
	action := &CMakeBuildAction{}
	if action.Name() != "cmake_build" {
		t.Errorf("Name() = %q, want %q", action.Name(), "cmake_build")
	}
}

func TestCMakeBuildAction_Execute_MissingSourceDir(t *testing.T) {
	action := &CMakeBuildAction{}
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
	if err.Error() != "cmake_build action requires 'source_dir' parameter" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCMakeBuildAction_Execute_MissingExecutables(t *testing.T) {
	action := &CMakeBuildAction{}
	workDir := t.TempDir()

	// Create source dir with a CMakeLists.txt
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.10)\n"), 0644); err != nil {
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
	if err.Error() != "cmake_build action requires 'executables' parameter with at least one executable" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCMakeBuildAction_Execute_CMakeListsNotFound(t *testing.T) {
	action := &CMakeBuildAction{}
	workDir := t.TempDir()

	// Create empty source dir (no CMakeLists.txt)
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
		t.Error("Expected error for missing CMakeLists.txt")
	}
	if !strings.Contains(err.Error(), "CMakeLists.txt not found") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestCMakeBuildAction_Execute_InvalidExecutableName(t *testing.T) {
	action := &CMakeBuildAction{}
	workDir := t.TempDir()

	// Create source dir with a CMakeLists.txt
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.10)\n"), 0644); err != nil {
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

func TestCMakeBuildAction_Execute_InvalidCMakeArg(t *testing.T) {
	action := &CMakeBuildAction{}
	workDir := t.TempDir()

	// Create source dir with a CMakeLists.txt
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.10)\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: t.TempDir(),
	}

	invalidArgs := []string{
		"-DOPT=val;rm -rf /",
		"-DOPT && evil",
		"-DOPT | cat /etc/passwd",
		"-DOPT `id`",
	}

	for _, arg := range invalidArgs {
		params := map[string]interface{}{
			"source_dir":  sourceDir,
			"executables": []string{"test"},
			"cmake_args":  []string{arg},
		}

		err := action.Execute(ctx, params)
		if err == nil {
			t.Errorf("Expected error for invalid cmake arg %q", arg)
		}
		if !strings.Contains(err.Error(), "invalid cmake argument") {
			t.Errorf("Unexpected error for %q: %v", arg, err)
		}
	}
}

func TestCMakeBuildAction_Execute_RelativeSourceDir(t *testing.T) {
	action := &CMakeBuildAction{}
	workDir := t.TempDir()

	// Create source dir with a CMakeLists.txt
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.10)\n"), 0644); err != nil {
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

	// This will fail at cmake step (won't configure), but proves relative path resolved
	err := action.Execute(ctx, params)
	if err != nil && strings.Contains(err.Error(), "CMakeLists.txt not found") {
		t.Error("Relative source_dir should have been resolved")
	}
}

func TestCMakeBuildAction_Registered(t *testing.T) {
	// Verify cmake_build is registered as a primitive action
	if !IsPrimitive("cmake_build") {
		t.Error("cmake_build should be registered as a primitive action")
	}

	// Verify it's in the action registry
	action := Get("cmake_build")
	if action == nil {
		t.Error("cmake_build should be registered in the action registry")
	}
}

func TestCMakeBuildAction_NotDeterministic(t *testing.T) {
	// cmake_build uses system compiler, so it's not deterministic
	if IsDeterministic("cmake_build") {
		t.Error("cmake_build should not be deterministic")
	}
}

func TestIsValidCMakeArg(t *testing.T) {
	validArgs := []string{
		"-DCMAKE_BUILD_TYPE=Release",
		"-DBUILD_SHARED_LIBS=ON",
		"-DCMAKE_C_COMPILER=/usr/bin/gcc",
		"-G Ninja",
		"-DOPT=val with spaces",
	}

	for _, arg := range validArgs {
		if !isValidCMakeArg(arg) {
			t.Errorf("isValidCMakeArg(%q) = false, want true", arg)
		}
	}

	invalidArgs := []string{
		"",                        // empty
		"-DOPT;rm",                // shell metachar
		"-DOPT && echo",           // shell metachar
		"-DOPT | cat",             // shell metachar
		"-DOPT `id`",              // shell metachar
		"-DOPT\necho",             // newline
		string(make([]byte, 501)), // too long
	}

	for _, arg := range invalidArgs {
		if isValidCMakeArg(arg) {
			if len(arg) <= 20 {
				t.Errorf("isValidCMakeArg(%q) = true, want false", arg)
			} else {
				t.Errorf("isValidCMakeArg(len=%d) = true, want false", len(arg))
			}
		}
	}
}
