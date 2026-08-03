package actions

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	t.Run("missing both source_file and source_command", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{
			"target": "niwa",
		})
		if !result.HasErrors() {
			t.Error("expected error for missing source_file and source_command")
		}
	})

	t.Run("both source_file and source_command", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{
			"source_file":    "init.sh",
			"source_command": "niwa shell-init {shell}",
			"target":         "niwa",
		})
		if !result.HasErrors() {
			t.Error("expected error for mutually exclusive params")
		}
		found := false
		for _, e := range result.Errors {
			if strings.Contains(e, "mutually exclusive") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected 'mutually exclusive' error, got: %v", result.Errors)
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

	t.Run("valid params with source_file", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{
			"source_file": "init.sh",
			"target":      "niwa",
			"shells":      []interface{}{"bash", "zsh"},
		})
		if result.HasErrors() {
			t.Errorf("unexpected errors: %v", result.Errors)
		}
	})

	t.Run("valid params with source_command", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{
			"source_command": "niwa shell-init {shell}",
			"target":         "niwa",
			"shells":         []interface{}{"bash", "zsh"},
		})
		if result.HasErrors() {
			t.Errorf("unexpected errors: %v", result.Errors)
		}
	})
}

func TestInstallShellInitAction_Execute_SourceFile(t *testing.T) {
	a := &InstallShellInitAction{}

	t.Run("copies file to shell.d for each shell", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		installDir := filepath.Join(toolsDir, "mytool-1.0")

		if err := os.MkdirAll(installDir, 0755); err != nil {
			t.Fatal(err)
		}

		sourceContent := "export FOO=bar\n"
		if err := os.WriteFile(filepath.Join(installDir, "init.sh"), []byte(sourceContent), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			Version:    "1.2.3",
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

		shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
		for _, shell := range []string{"bash", "zsh"} {
			path := filepath.Join(shellDDir, "mytool@1.2.3."+shell)
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
			Version:    "1.2.3",
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

		shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
		for _, shell := range []string{"bash", "zsh"} {
			path := filepath.Join(shellDDir, "mytool@1.2.3."+shell)
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
			Version:    "1.2.3",
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
			Version:    "1.2.3",
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

func TestInstallShellInitAction_Execute_SourceCommand(t *testing.T) {
	a := &InstallShellInitAction{}

	t.Run("runs command and writes output to shell.d", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")

		if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create a fake binary in the tool install dir
		scriptContent := "#!/bin/sh\necho \"init for $1\"\n"
		scriptPath := filepath.Join(toolInstallDir, "mytool")
		if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			Version:        "1.2.3",
			InstallDir:     toolInstallDir,
			ToolInstallDir: toolInstallDir,
			ToolsDir:       toolsDir,
		}

		params := map[string]interface{}{
			"source_command": "{install_dir}/mytool {shell}",
			"target":         "mytool",
			"shells":         []interface{}{"bash", "zsh"},
		}

		if err := a.Execute(ctx, params); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
		for _, shell := range []string{"bash", "zsh"} {
			path := filepath.Join(shellDDir, "mytool@1.2.3."+shell)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("expected file %s to exist: %v", path, err)
			}
			expected := "init for " + shell + "\n"
			if string(content) != expected {
				t.Errorf("expected content %q, got %q", expected, string(content))
			}
		}
	})

	t.Run("substitutes shell and install_dir placeholders", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")

		if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Script that prints both its args
		scriptContent := "#!/bin/sh\necho \"shell=$1 dir=$2\"\n"
		scriptPath := filepath.Join(toolInstallDir, "mytool")
		if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			Version:        "1.2.3",
			InstallDir:     toolInstallDir,
			ToolInstallDir: toolInstallDir,
			ToolsDir:       toolsDir,
		}

		params := map[string]interface{}{
			"source_command": "{install_dir}/mytool {shell} {install_dir}",
			"target":         "mytool",
			"shells":         []interface{}{"zsh"},
		}

		if err := a.Execute(ctx, params); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
		content, err := os.ReadFile(filepath.Join(shellDDir, "mytool@1.2.3.zsh"))
		if err != nil {
			t.Fatal(err)
		}
		expected := "shell=zsh dir=" + toolInstallDir + "\n"
		if string(content) != expected {
			t.Errorf("expected %q, got %q", expected, string(content))
		}
	})

	t.Run("non-zero exit logs warning and skips shell", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")

		if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Script that exits 1
		scriptContent := "#!/bin/sh\nexit 1\n"
		scriptPath := filepath.Join(toolInstallDir, "mytool")
		if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			Version:        "1.2.3",
			InstallDir:     toolInstallDir,
			ToolInstallDir: toolInstallDir,
			ToolsDir:       toolsDir,
		}

		params := map[string]interface{}{
			"source_command": "{install_dir}/mytool {shell}",
			"target":         "mytool",
			"shells":         []interface{}{"bash"},
		}

		// Should NOT return an error -- graceful failure
		if err := a.Execute(ctx, params); err != nil {
			t.Fatalf("expected no error for non-zero exit, got: %v", err)
		}

		// File should not be created
		shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
		path := filepath.Join(shellDDir, "mytool@1.2.3.bash")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected file %s to NOT exist after non-zero exit", path)
		}
	})

	t.Run("empty output skips file creation", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")

		if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Script that outputs nothing
		scriptContent := "#!/bin/sh\n"
		scriptPath := filepath.Join(toolInstallDir, "mytool")
		if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			Version:        "1.2.3",
			InstallDir:     toolInstallDir,
			ToolInstallDir: toolInstallDir,
			ToolsDir:       toolsDir,
		}

		params := map[string]interface{}{
			"source_command": "{install_dir}/mytool {shell}",
			"target":         "mytool",
			"shells":         []interface{}{"bash"},
		}

		if err := a.Execute(ctx, params); err != nil {
			t.Fatalf("expected no error for empty output, got: %v", err)
		}

		shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
		path := filepath.Join(shellDDir, "mytool@1.2.3.bash")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected file %s to NOT exist for empty output", path)
		}
	})

	t.Run("rejects binary outside ToolInstallDir", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")

		if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			Version:        "1.2.3",
			InstallDir:     toolInstallDir,
			ToolInstallDir: toolInstallDir,
			ToolsDir:       toolsDir,
		}

		params := map[string]interface{}{
			"source_command": "/usr/bin/echo hello",
			"target":         "mytool",
			"shells":         []interface{}{"bash"},
		}

		err := a.Execute(ctx, params)
		if err == nil {
			t.Fatal("expected error for binary outside ToolInstallDir")
		}
		if !strings.Contains(err.Error(), "outside ToolInstallDir") {
			t.Errorf("expected 'outside ToolInstallDir' in error, got: %v", err)
		}
	})

	t.Run("rejects symlink pointing outside ToolInstallDir", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")

		if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create a symlink inside toolInstallDir that points outside
		symlinkPath := filepath.Join(toolInstallDir, "sneaky")
		if err := os.Symlink("/usr/bin/echo", symlinkPath); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			Version:        "1.2.3",
			InstallDir:     toolInstallDir,
			ToolInstallDir: toolInstallDir,
			ToolsDir:       toolsDir,
		}

		params := map[string]interface{}{
			"source_command": "{install_dir}/sneaky hello",
			"target":         "mytool",
			"shells":         []interface{}{"bash"},
		}

		err := a.Execute(ctx, params)
		if err == nil {
			t.Fatal("expected error for symlink pointing outside ToolInstallDir")
		}
		if !strings.Contains(err.Error(), "outside ToolInstallDir") {
			t.Errorf("expected 'outside ToolInstallDir' in error, got: %v", err)
		}
	})

	t.Run("uses exec.Command not shell", func(t *testing.T) {
		// Verify that shell metacharacters are NOT interpreted.
		// A command like "mytool; rm -rf /" should be treated as a single
		// binary name "mytool;" which won't exist.
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")

		if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			Version:        "1.2.3",
			InstallDir:     toolInstallDir,
			ToolInstallDir: toolInstallDir,
			ToolsDir:       toolsDir,
		}

		// The semicolon is part of the binary name, not a shell separator
		params := map[string]interface{}{
			"source_command": "{install_dir}/mytool; echo pwned",
			"target":         "mytool",
			"shells":         []interface{}{"bash"},
		}

		err := a.Execute(ctx, params)
		// Should fail because "mytool;" doesn't exist, not because it ran "echo pwned"
		if err == nil {
			t.Fatal("expected error for nonexistent binary with shell metacharacters")
		}
	})
}

func TestValidateCommandBinary(t *testing.T) {
	t.Run("accepts binary inside ToolInstallDir", func(t *testing.T) {
		dir := t.TempDir()
		binPath := filepath.Join(dir, "mytool")
		if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}

		err := validateCommandBinary("{install_dir}/mytool {shell}", dir)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("accepts relative binary path", func(t *testing.T) {
		dir := t.TempDir()
		binPath := filepath.Join(dir, "bin", "mytool")
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}

		err := validateCommandBinary("bin/mytool {shell}", dir)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("rejects absolute path outside dir", func(t *testing.T) {
		dir := t.TempDir()
		err := validateCommandBinary("/usr/bin/echo hello", dir)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rejects empty command", func(t *testing.T) {
		dir := t.TempDir()
		err := validateCommandBinary("", dir)
		if err == nil {
			t.Fatal("expected error for empty command")
		}
	})

	t.Run("rejects when ToolInstallDir is empty", func(t *testing.T) {
		err := validateCommandBinary("mytool {shell}", "")
		if err == nil {
			t.Fatal("expected error for empty ToolInstallDir")
		}
	})

	t.Run("resolves symlinks inside dir", func(t *testing.T) {
		dir := t.TempDir()
		realBin := filepath.Join(dir, "real-mytool")
		if err := os.WriteFile(realBin, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatal(err)
		}
		symlinkPath := filepath.Join(dir, "mytool")
		if err := os.Symlink(realBin, symlinkPath); err != nil {
			t.Fatal(err)
		}

		// Symlink inside dir pointing to file inside dir: OK
		err := validateCommandBinary("{install_dir}/mytool {shell}", dir)
		if err != nil {
			t.Errorf("expected no error for symlink within dir, got: %v", err)
		}
	})

	t.Run("rejects symlink pointing outside dir", func(t *testing.T) {
		dir := t.TempDir()
		symlinkPath := filepath.Join(dir, "mytool")
		// Point to something outside the dir (like /usr/bin/echo which should exist)
		target, err := exec.LookPath("echo")
		if err != nil {
			t.Skip("echo not found in PATH")
		}
		if err := os.Symlink(target, symlinkPath); err != nil {
			t.Fatal(err)
		}

		err = validateCommandBinary("{install_dir}/mytool {shell}", dir)
		if err == nil {
			t.Fatal("expected error for symlink pointing outside dir")
		}
		if !strings.Contains(err.Error(), "outside ToolInstallDir") {
			t.Errorf("expected 'outside ToolInstallDir' in error, got: %v", err)
		}
	})
}

func TestInstallShellInitAction_RecordsCleanupActions_SourceFile(t *testing.T) {
	a := &InstallShellInitAction{}

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
		Version:    "1.2.3",
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

	if len(ctx.CleanupActions) != 2 {
		t.Fatalf("expected 2 cleanup actions, got %d", len(ctx.CleanupActions))
	}

	expected := []CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/mytool@1.2.3.bash"},
		{Action: "delete_file", Path: "share/shell.d/mytool@1.2.3.zsh"},
	}

	for i, ca := range ctx.CleanupActions {
		if ca.Action != expected[i].Action || ca.Path != expected[i].Path {
			t.Errorf("CleanupActions[%d] = %+v, want %+v", i, ca, expected[i])
		}
	}
}

func TestInstallShellInitAction_RecordsCleanupActions_SourceCommand(t *testing.T) {
	a := &InstallShellInitAction{}

	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")

	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatal(err)
	}

	scriptContent := "#!/bin/sh\necho \"init for $1\"\n"
	scriptPath := filepath.Join(toolInstallDir, "mytool")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Version:        "1.2.3",
		InstallDir:     toolInstallDir,
		ToolInstallDir: toolInstallDir,
		ToolsDir:       toolsDir,
	}

	params := map[string]interface{}{
		"source_command": "{install_dir}/mytool {shell}",
		"target":         "mytool",
		"shells":         []interface{}{"bash", "zsh"},
	}

	if err := a.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(ctx.CleanupActions) != 2 {
		t.Fatalf("expected 2 cleanup actions, got %d", len(ctx.CleanupActions))
	}

	expected := []CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/mytool@1.2.3.bash"},
		{Action: "delete_file", Path: "share/shell.d/mytool@1.2.3.zsh"},
	}

	for i, ca := range ctx.CleanupActions {
		if ca.Action != expected[i].Action || ca.Path != expected[i].Path {
			t.Errorf("CleanupActions[%d] = %+v, want %+v", i, ca, expected[i])
		}
	}
}

func TestInstallShellInitAction_NoCleanupOnSkippedShell(t *testing.T) {
	a := &InstallShellInitAction{}

	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")

	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Script that exits 1 -- should not record cleanup
	scriptContent := "#!/bin/sh\nexit 1\n"
	scriptPath := filepath.Join(toolInstallDir, "mytool")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Version:        "1.2.3",
		InstallDir:     toolInstallDir,
		ToolInstallDir: toolInstallDir,
		ToolsDir:       toolsDir,
	}

	params := map[string]interface{}{
		"source_command": "{install_dir}/mytool {shell}",
		"target":         "mytool",
		"shells":         []interface{}{"bash"},
	}

	if err := a.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(ctx.CleanupActions) != 0 {
		t.Errorf("expected 0 cleanup actions for failed command, got %d", len(ctx.CleanupActions))
	}
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

func TestInstallShellInitAction_ContentHash_SourceFile(t *testing.T) {
	a := &InstallShellInitAction{}

	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	installDir := filepath.Join(toolsDir, "mytool-1.0")

	if err := os.MkdirAll(installDir, 0755); err != nil {
		t.Fatal(err)
	}

	sourceContent := "export FOO=bar\n"
	if err := os.WriteFile(filepath.Join(installDir, "init.sh"), []byte(sourceContent), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Version:    "1.2.3",
		InstallDir: installDir,
		ToolsDir:   toolsDir,
	}

	params := map[string]interface{}{
		"source_file": "init.sh",
		"target":      "mytool",
		"shells":      []interface{}{"bash"},
	}

	if err := a.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(ctx.CleanupActions) != 1 {
		t.Fatalf("expected 1 cleanup action, got %d", len(ctx.CleanupActions))
	}

	ca := ctx.CleanupActions[0]
	if ca.ContentHash == "" {
		t.Fatal("expected ContentHash to be set")
	}

	// Verify the hash matches the actual content
	expectedHash := contentHash([]byte(sourceContent))
	if ca.ContentHash != expectedHash {
		t.Errorf("ContentHash = %q, want %q", ca.ContentHash, expectedHash)
	}
}

func TestInstallShellInitAction_ContentHash_SourceCommand(t *testing.T) {
	a := &InstallShellInitAction{}

	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")

	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Script that outputs deterministic content
	scriptContent := "#!/bin/sh\necho \"init for $1\"\n"
	scriptPath := filepath.Join(toolInstallDir, "mytool")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Version:        "1.2.3",
		InstallDir:     toolInstallDir,
		ToolInstallDir: toolInstallDir,
		ToolsDir:       toolsDir,
	}

	params := map[string]interface{}{
		"source_command": "{install_dir}/mytool {shell}",
		"target":         "mytool",
		"shells":         []interface{}{"bash"},
	}

	if err := a.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(ctx.CleanupActions) != 1 {
		t.Fatalf("expected 1 cleanup action, got %d", len(ctx.CleanupActions))
	}

	ca := ctx.CleanupActions[0]
	if ca.ContentHash == "" {
		t.Fatal("expected ContentHash to be set")
	}

	// Verify the hash matches the command output
	expectedHash := contentHash([]byte("init for bash\n"))
	if ca.ContentHash != expectedHash {
		t.Errorf("ContentHash = %q, want %q", ca.ContentHash, expectedHash)
	}
}

func TestInstallShellInitAction_FilePermissions(t *testing.T) {
	a := &InstallShellInitAction{}

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
		Version:    "1.2.3",
		InstallDir: installDir,
		ToolsDir:   toolsDir,
	}

	params := map[string]interface{}{
		"source_file": "init.sh",
		"target":      "mytool",
		"shells":      []interface{}{"bash"},
	}

	if err := a.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Check shell.d directory permissions
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	dirInfo, err := os.Stat(shellDDir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0700 {
		t.Errorf("expected shell.d directory permissions 0700, got %04o", perm)
	}

	// Check file permissions
	filePath := filepath.Join(shellDDir, "mytool@1.2.3.bash")
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("expected file permissions 0600, got %04o", perm)
	}
}

func TestInstallShellInitAction_FilePermissions_SourceCommand(t *testing.T) {
	a := &InstallShellInitAction{}

	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")

	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatal(err)
	}

	scriptContent := "#!/bin/sh\necho \"init for $1\"\n"
	scriptPath := filepath.Join(toolInstallDir, "mytool")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Version:        "1.2.3",
		InstallDir:     toolInstallDir,
		ToolInstallDir: toolInstallDir,
		ToolsDir:       toolsDir,
	}

	params := map[string]interface{}{
		"source_command": "{install_dir}/mytool {shell}",
		"target":         "mytool",
		"shells":         []interface{}{"bash"},
	}

	if err := a.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	filePath := filepath.Join(shellDDir, "mytool@1.2.3.bash")
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("expected file permissions 0600, got %04o", perm)
	}
}

func TestInstallShellInitAction_NoShellInit_SourceFile(t *testing.T) {
	a := &InstallShellInitAction{}

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
		Version:     "1.2.3",
		InstallDir:  installDir,
		ToolsDir:    toolsDir,
		NoShellInit: true,
	}

	params := map[string]interface{}{
		"source_file": "init.sh",
		"target":      "mytool",
		"shells":      []interface{}{"bash", "zsh"},
	}

	if err := a.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// No files should be created
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	for _, shell := range []string{"bash", "zsh"} {
		path := filepath.Join(shellDDir, "mytool@1.2.3."+shell)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected file %s to NOT exist when NoShellInit is set", path)
		}
	}

	// No cleanup actions should be recorded
	if len(ctx.CleanupActions) != 0 {
		t.Errorf("expected 0 cleanup actions when NoShellInit is set, got %d", len(ctx.CleanupActions))
	}
}

func TestInstallShellInitAction_NoShellInit_SourceCommand(t *testing.T) {
	a := &InstallShellInitAction{}

	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")

	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatal(err)
	}

	scriptContent := "#!/bin/sh\necho \"init for $1\"\n"
	scriptPath := filepath.Join(toolInstallDir, "mytool")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Version:        "1.2.3",
		InstallDir:     toolInstallDir,
		ToolInstallDir: toolInstallDir,
		ToolsDir:       toolsDir,
		NoShellInit:    true,
	}

	params := map[string]interface{}{
		"source_command": "{install_dir}/mytool {shell}",
		"target":         "mytool",
		"shells":         []interface{}{"bash"},
	}

	if err := a.Execute(ctx, params); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// No files should be created
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	path := filepath.Join(shellDDir, "mytool@1.2.3.bash")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file %s to NOT exist when NoShellInit is set", path)
	}

	// No cleanup actions should be recorded
	if len(ctx.CleanupActions) != 0 {
		t.Errorf("expected 0 cleanup actions when NoShellInit is set, got %d", len(ctx.CleanupActions))
	}
}

// TestInstallShellInitAction_ShellAllowlist pins which shells the action
// accepts. fish is rejected because a fish fragment would be written and never
// loaded: the init cache reaches a shell only through $TSUKU_HOME/env, which is
// POSIX shell fish cannot parse. See issue #2471.
func TestInstallShellInitAction_ShellAllowlist(t *testing.T) {
	a := &InstallShellInitAction{}

	tests := []struct {
		name        string
		shell       string
		wantErr     bool
		wantMessage []string
	}{
		{name: "bash accepted", shell: "bash", wantErr: false},
		{name: "zsh accepted", shell: "zsh", wantErr: false},
		{
			name:    "fish rejected with a pointer to what works",
			shell:   "fish",
			wantErr: true,
			// The message has to name the alternative, not just say no.
			wantMessage: []string{"fish", "bash, zsh", "tsuku shellenv", "install_completions"},
		},
		{
			name:        "unknown shell rejected with the allowed set",
			shell:       "powershell",
			wantErr:     true,
			wantMessage: []string{"powershell", "bash, zsh"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := a.Preflight(map[string]interface{}{
				"source_file": "init.sh",
				"target":      "mytool",
				"shells":      []interface{}{tt.shell},
			})

			if !tt.wantErr {
				if result.HasErrors() {
					t.Fatalf("shell %q should be accepted, got errors: %v", tt.shell, result.Errors)
				}
				return
			}

			if !result.HasErrors() {
				t.Fatalf("shell %q should be rejected, got no errors", tt.shell)
			}
			joined := strings.Join(result.Errors, "\n")
			for _, want := range tt.wantMessage {
				if !strings.Contains(joined, want) {
					t.Errorf("error message should mention %q, got: %s", want, joined)
				}
			}
		})
	}
}

// TestInstallShellInitAction_Execute_RejectsFish drives Execute rather than
// Preflight, and asserts the outcome a recipe author would see: no fragment on
// disk and no cleanup action recorded. Checking only the returned error would
// not catch a guard that rejects after already writing the file.
func TestInstallShellInitAction_Execute_RejectsFish(t *testing.T) {
	a := &InstallShellInitAction{}

	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")
	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatal(err)
	}
	srcPath := filepath.Join(toolInstallDir, "init.sh")
	if err := os.WriteFile(srcPath, []byte("# init\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Version:        "1.2.3",
		InstallDir:     toolInstallDir,
		ToolInstallDir: toolInstallDir,
		ToolsDir:       toolsDir,
	}

	err := a.Execute(ctx, map[string]interface{}{
		"source_file": "init.sh",
		"target":      "mytool",
		"shells":      []interface{}{"fish"},
	})
	if err == nil {
		t.Fatal("Execute should reject shells = [fish]")
	}
	if !strings.Contains(err.Error(), "tsuku shellenv") {
		t.Errorf("error should point at the supported fish path, got: %v", err)
	}

	fragment := filepath.Join(tsukuHome, "share", "shell.d", "mytool@1.2.3.fish")
	if _, statErr := os.Stat(fragment); !os.IsNotExist(statErr) {
		t.Errorf("no fish fragment should be written, but %s exists", fragment)
	}
	if len(ctx.CleanupActions) != 0 {
		t.Errorf("no cleanup action should be recorded, got %d: %v", len(ctx.CleanupActions), ctx.CleanupActions)
	}
}

// TestInstallShellInitAction_Execute_RejectsFishAlongsideValidShell covers the
// mixed list. A recipe asking for bash and fish must be rejected outright
// rather than silently installing the bash half.
func TestInstallShellInitAction_Execute_RejectsFishAlongsideValidShell(t *testing.T) {
	a := &InstallShellInitAction{}

	tsukuHome := t.TempDir()
	toolsDir := filepath.Join(tsukuHome, "tools")
	toolInstallDir := filepath.Join(toolsDir, "mytool-1.0")
	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolInstallDir, "init.sh"), []byte("# init\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		Version:        "1.2.3",
		InstallDir:     toolInstallDir,
		ToolInstallDir: toolInstallDir,
		ToolsDir:       toolsDir,
	}

	if err := a.Execute(ctx, map[string]interface{}{
		"source_file": "init.sh",
		"target":      "mytool",
		"shells":      []interface{}{"bash", "fish"},
	}); err == nil {
		t.Fatal("Execute should reject a shells list containing fish")
	}

	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	for _, name := range []string{"mytool@1.2.3.bash", "mytool@1.2.3.fish"} {
		if _, err := os.Stat(filepath.Join(shellDDir, name)); !os.IsNotExist(err) {
			t.Errorf("nothing should be written when the shells list is invalid, but %s exists", name)
		}
	}
	if len(ctx.CleanupActions) != 0 {
		t.Errorf("no cleanup action should be recorded, got %d", len(ctx.CleanupActions))
	}
}
