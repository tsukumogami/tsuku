package actions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallShellInitAction_Name(t *testing.T) {
	a := &InstallShellInitAction{}
	if a.Name() != "install_shell_init" {
		t.Errorf("expected name install_shell_init, got %s", a.Name())
	}
}

func TestInstallShellInitAction_IsDeterministic(t *testing.T) {
	a := InstallShellInitAction{}
	if !a.IsDeterministic() {
		t.Error("expected IsDeterministic to return true")
	}
}

func TestInstallShellInitAction_Preflight(t *testing.T) {
	a := &InstallShellInitAction{}

	t.Run("missing source_file", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{
			"target": "niwa",
		})
		if !result.HasErrors() {
			t.Error("expected error for missing source_file")
		}
	})

	t.Run("missing target", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{
			"source_file": "init.sh",
		})
		if !result.HasErrors() {
			t.Error("expected error for missing target")
		}
	})

	t.Run("invalid shell", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{
			"source_file": "init.sh",
			"target":      "niwa",
			"shells":      []interface{}{"bash", "powershell"},
		})
		if !result.HasErrors() {
			t.Error("expected error for invalid shell")
		}
	})

	t.Run("valid params", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{
			"source_file": "init.sh",
			"target":      "niwa",
			"shells":      []interface{}{"bash", "zsh"},
		})
		if result.HasErrors() {
			t.Errorf("unexpected errors: %v", result.Errors)
		}
	})
}

func TestInstallShellInitAction_Execute(t *testing.T) {
	a := &InstallShellInitAction{}

	t.Run("copies file to shell.d for each shell", func(t *testing.T) {
		// Set up temp directories
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		installDir := filepath.Join(toolsDir, "mytool-1.0")

		if err := os.MkdirAll(installDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create source file
		sourceContent := "export FOO=bar\n"
		if err := os.WriteFile(filepath.Join(installDir, "init.sh"), []byte(sourceContent), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir: installDir,
			ToolsDir:   toolsDir,
		}

		params := map[string]interface{}{
			"source_file": "init.sh",
			"target":      "mytool",
			"shells":      []interface{}{"bash", "zsh"},
		}

		if err := a.Execute(ctx, params); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Check files were created
		shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
		for _, shell := range []string{"bash", "zsh"} {
			path := filepath.Join(shellDDir, "mytool."+shell)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("expected file %s to exist: %v", path, err)
			}
			if string(content) != sourceContent {
				t.Errorf("expected content %q, got %q", sourceContent, string(content))
			}
		}
	})

	t.Run("uses default shells when not specified", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		installDir := filepath.Join(toolsDir, "mytool-1.0")

		if err := os.MkdirAll(installDir, 0755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(installDir, "init.sh"), []byte("# init\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir: installDir,
			ToolsDir:   toolsDir,
		}

		params := map[string]interface{}{
			"source_file": "init.sh",
			"target":      "mytool",
		}

		if err := a.Execute(ctx, params); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Default shells are bash and zsh
		shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
		for _, shell := range []string{"bash", "zsh"} {
			path := filepath.Join(shellDDir, "mytool."+shell)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("expected default shell file %s to exist", path)
			}
		}
	})

	t.Run("rejects invalid shell", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		installDir := filepath.Join(toolsDir, "mytool-1.0")

		if err := os.MkdirAll(installDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(installDir, "init.sh"), []byte("# init\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir: installDir,
			ToolsDir:   toolsDir,
		}

		params := map[string]interface{}{
			"source_file": "init.sh",
			"target":      "mytool",
			"shells":      []interface{}{"bash", "powershell"},
		}

		err := a.Execute(ctx, params)
		if err == nil {
			t.Fatal("expected error for invalid shell")
		}
	})

	t.Run("errors on missing source file", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		installDir := filepath.Join(toolsDir, "mytool-1.0")

		if err := os.MkdirAll(installDir, 0755); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir: installDir,
			ToolsDir:   toolsDir,
		}

		params := map[string]interface{}{
			"source_file": "nonexistent.sh",
			"target":      "mytool",
		}

		err := a.Execute(ctx, params)
		if err == nil {
			t.Fatal("expected error for missing source file")
		}
	})
}

func TestInstallShellInitAction_Registered(t *testing.T) {
	action := Get("install_shell_init")
	if action == nil {
		t.Fatal("install_shell_init action not registered")
	}
	if action.Name() != "install_shell_init" {
		t.Errorf("expected name install_shell_init, got %s", action.Name())
	}
}
