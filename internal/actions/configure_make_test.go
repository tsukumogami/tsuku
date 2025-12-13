package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureMakeAction_Name(t *testing.T) {
	action := &ConfigureMakeAction{}
	if action.Name() != "configure_make" {
		t.Errorf("Name() = %q, want %q", action.Name(), "configure_make")
	}
}

func TestConfigureMakeAction_Execute_MissingSourceDir(t *testing.T) {
	action := &ConfigureMakeAction{}
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
	if err.Error() != "configure_make action requires 'source_dir' parameter" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestConfigureMakeAction_Execute_MissingExecutables(t *testing.T) {
	action := &ConfigureMakeAction{}
	workDir := t.TempDir()

	// Create source dir with a configure script
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "configure"), []byte("#!/bin/sh\n"), 0755); err != nil {
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
	if err.Error() != "configure_make action requires 'executables' parameter with at least one executable" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestConfigureMakeAction_Execute_ConfigureNotFound(t *testing.T) {
	action := &ConfigureMakeAction{}
	workDir := t.TempDir()

	// Create empty source dir (no configure script)
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
		t.Error("Expected error for missing configure script")
	}
	if !strings.Contains(err.Error(), "configure script not found") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestConfigureMakeAction_Execute_InvalidExecutableName(t *testing.T) {
	action := &ConfigureMakeAction{}
	workDir := t.TempDir()

	// Create source dir with a configure script
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "configure"), []byte("#!/bin/sh\n"), 0755); err != nil {
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

func TestConfigureMakeAction_Execute_InvalidConfigureArg(t *testing.T) {
	action := &ConfigureMakeAction{}
	workDir := t.TempDir()

	// Create source dir with a configure script
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "configure"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    workDir,
		InstallDir: t.TempDir(),
	}

	invalidArgs := []string{
		"--opt;rm -rf /",
		"--opt && evil",
		"--opt | cat /etc/passwd",
		"--opt `id`",
		"$(whoami)",
	}

	for _, arg := range invalidArgs {
		params := map[string]interface{}{
			"source_dir":     sourceDir,
			"executables":    []string{"test"},
			"configure_args": []string{arg},
		}

		err := action.Execute(ctx, params)
		if err == nil {
			t.Errorf("Expected error for invalid configure arg %q", arg)
		}
		if !strings.Contains(err.Error(), "invalid configure argument") {
			t.Errorf("Unexpected error for %q: %v", arg, err)
		}
	}
}

func TestConfigureMakeAction_Execute_RelativeSourceDir(t *testing.T) {
	action := &ConfigureMakeAction{}
	workDir := t.TempDir()

	// Create source dir with a configure script
	sourceDir := filepath.Join(workDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "configure"), []byte("#!/bin/sh\n"), 0755); err != nil {
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

	// This will fail at configure step (fake script), but proves relative path resolved
	err := action.Execute(ctx, params)
	if err != nil && strings.Contains(err.Error(), "configure script not found") {
		t.Error("Relative source_dir should have been resolved")
	}
}

func TestConfigureMakeAction_Registered(t *testing.T) {
	// Verify configure_make is registered as a primitive action
	if !IsPrimitive("configure_make") {
		t.Error("configure_make should be registered as a primitive action")
	}

	// Verify it's in the action registry
	action := Get("configure_make")
	if action == nil {
		t.Error("configure_make should be registered in the action registry")
	}
}

func TestConfigureMakeAction_NotDeterministic(t *testing.T) {
	// configure_make uses system compiler, so it's not deterministic
	if IsDeterministic("configure_make") {
		t.Error("configure_make should not be deterministic")
	}
}

func TestIsValidConfigureArg(t *testing.T) {
	validArgs := []string{
		"--prefix=/usr/local",
		"--enable-shared",
		"--disable-static",
		"--with-ssl",
		"--without-debug",
		"CFLAGS=-O2",
		"--host=x86_64-linux-gnu",
	}

	for _, arg := range validArgs {
		if !isValidConfigureArg(arg) {
			t.Errorf("isValidConfigureArg(%q) = false, want true", arg)
		}
	}

	invalidArgs := []string{
		"",                        // empty
		"--opt;rm",                // shell metachar
		"--opt && echo",           // shell metachar
		"--opt | cat",             // shell metachar
		"--opt `id`",              // shell metachar
		"$(whoami)",               // shell metachar
		"--opt\necho",             // newline
		string(make([]byte, 501)), // too long
	}

	for _, arg := range invalidArgs {
		if isValidConfigureArg(arg) {
			if len(arg) <= 20 {
				t.Errorf("isValidConfigureArg(%q) = true, want false", arg)
			} else {
				t.Errorf("isValidConfigureArg(len=%d) = true, want false", len(arg))
			}
		}
	}
}
