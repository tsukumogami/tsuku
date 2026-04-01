package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCompletionsAction_Name(t *testing.T) {
	a := &InstallCompletionsAction{}
	if a.Name() != "install_completions" {
		t.Errorf("expected name install_completions, got %s", a.Name())
	}
}

func TestInstallCompletionsAction_IsDeterministic(t *testing.T) {
	a := InstallCompletionsAction{}
	if !a.IsDeterministic() {
		t.Error("expected IsDeterministic to return true")
	}
}

func TestInstallCompletionsAction_Preflight(t *testing.T) {
	a := &InstallCompletionsAction{}

	t.Run("missing both sources errors", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{"target": "gh"})
		if !result.HasErrors() {
			t.Error("expected error for missing source_file and source_command")
		}
	})

	t.Run("mutually exclusive sources", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{
			"source_file": "comp.sh", "source_command": "gh completion -s {shell}", "target": "gh",
		})
		if !result.HasErrors() {
			t.Error("expected error for mutually exclusive params")
		}
		if !containsSubstring(result.Errors, "mutually exclusive") {
			t.Errorf("expected 'mutually exclusive' error, got: %v", result.Errors)
		}
	})

	t.Run("missing target errors", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{"source_file": "comp.sh"})
		if !result.HasErrors() {
			t.Error("expected error for missing target")
		}
	})

	t.Run("rejects disallowed shell", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{
			"source_file": "comp.sh", "target": "gh",
			"shells": []interface{}{"bash", "powershell"},
		})
		if !result.HasErrors() {
			t.Error("expected error for invalid shell")
		}
	})

	t.Run("accepts source_file with valid shells", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{
			"source_file": "comp.sh", "target": "gh", "shells": []interface{}{"bash", "zsh"},
		})
		if result.HasErrors() {
			t.Errorf("unexpected errors: %v", result.Errors)
		}
	})

	t.Run("accepts source_command with all shells", func(t *testing.T) {
		result := a.Preflight(map[string]interface{}{
			"source_command": "gh completion -s {shell}", "target": "gh",
			"shells": []interface{}{"bash", "zsh", "fish"},
		})
		if result.HasErrors() {
			t.Errorf("unexpected errors: %v", result.Errors)
		}
	})
}

// containsSubstring checks if any string in the slice contains the given substring.
func containsSubstring(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestInstallCompletionsAction_Execute_SourceFile(t *testing.T) {
	a := &InstallCompletionsAction{}

	t.Run("copies file to completions dir for each shell", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		installDir := filepath.Join(toolsDir, "gh-2.0")

		if err := os.MkdirAll(installDir, 0755); err != nil {
			t.Fatal(err)
		}

		sourceContent := "complete -o default -C gh gh\n"
		if err := os.WriteFile(filepath.Join(installDir, "completions.sh"), []byte(sourceContent), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir: installDir,
			ToolsDir:   toolsDir,
		}

		params := map[string]interface{}{
			"source_file": "completions.sh",
			"target":      "gh",
			"shells":      []interface{}{"bash", "zsh"},
		}

		if err := a.Execute(ctx, params); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// bash: share/completions/bash/gh
		bashPath := filepath.Join(tsukuHome, "share", "completions", "bash", "gh")
		content, err := os.ReadFile(bashPath)
		if err != nil {
			t.Fatalf("expected file %s to exist: %v", bashPath, err)
		}
		if string(content) != sourceContent {
			t.Errorf("expected content %q, got %q", sourceContent, string(content))
		}

		// zsh: share/completions/zsh/_gh (prefixed with _)
		zshPath := filepath.Join(tsukuHome, "share", "completions", "zsh", "_gh")
		content, err = os.ReadFile(zshPath)
		if err != nil {
			t.Fatalf("expected file %s to exist: %v", zshPath, err)
		}
		if string(content) != sourceContent {
			t.Errorf("expected content %q, got %q", sourceContent, string(content))
		}
	})

	t.Run("zsh completion prefixed with underscore", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		installDir := filepath.Join(toolsDir, "gh-2.0")

		if err := os.MkdirAll(installDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(installDir, "comp.zsh"), []byte("#compdef gh\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir: installDir,
			ToolsDir:   toolsDir,
		}

		params := map[string]interface{}{
			"source_file": "comp.zsh",
			"target":      "gh",
			"shells":      []interface{}{"zsh"},
		}

		if err := a.Execute(ctx, params); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Should be _gh, not gh
		zshPath := filepath.Join(tsukuHome, "share", "completions", "zsh", "_gh")
		if _, err := os.Stat(zshPath); os.IsNotExist(err) {
			t.Errorf("expected zsh completion file %s to exist", zshPath)
		}

		// Should NOT have gh without underscore
		wrongPath := filepath.Join(tsukuHome, "share", "completions", "zsh", "gh")
		if _, err := os.Stat(wrongPath); err == nil {
			t.Errorf("did not expect file %s (zsh completions should be prefixed with _)", wrongPath)
		}
	})

	t.Run("uses default shells when not specified", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		installDir := filepath.Join(toolsDir, "gh-2.0")

		if err := os.MkdirAll(installDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(installDir, "comp.sh"), []byte("# comp\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir: installDir,
			ToolsDir:   toolsDir,
		}

		params := map[string]interface{}{
			"source_file": "comp.sh",
			"target":      "gh",
		}

		if err := a.Execute(ctx, params); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Default shells: bash and zsh
		bashPath := filepath.Join(tsukuHome, "share", "completions", "bash", "gh")
		if _, err := os.Stat(bashPath); os.IsNotExist(err) {
			t.Errorf("expected default shell file %s to exist", bashPath)
		}
		zshPath := filepath.Join(tsukuHome, "share", "completions", "zsh", "_gh")
		if _, err := os.Stat(zshPath); os.IsNotExist(err) {
			t.Errorf("expected default shell file %s to exist", zshPath)
		}
	})

	t.Run("rejects invalid shell", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		installDir := filepath.Join(toolsDir, "gh-2.0")

		if err := os.MkdirAll(installDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(installDir, "comp.sh"), []byte("# comp\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir: installDir,
			ToolsDir:   toolsDir,
		}

		params := map[string]interface{}{
			"source_file": "comp.sh",
			"target":      "gh",
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
		installDir := filepath.Join(toolsDir, "gh-2.0")

		if err := os.MkdirAll(installDir, 0755); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir: installDir,
			ToolsDir:   toolsDir,
		}

		params := map[string]interface{}{
			"source_file": "nonexistent.sh",
			"target":      "gh",
		}

		err := a.Execute(ctx, params)
		if err == nil {
			t.Fatal("expected error for missing source file")
		}
	})

	t.Run("records cleanup actions", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		installDir := filepath.Join(toolsDir, "gh-2.0")

		if err := os.MkdirAll(installDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(installDir, "comp.sh"), []byte("# comp\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir: installDir,
			ToolsDir:   toolsDir,
		}

		params := map[string]interface{}{
			"source_file": "comp.sh",
			"target":      "gh",
			"shells":      []interface{}{"bash", "zsh"},
		}

		if err := a.Execute(ctx, params); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if len(ctx.CleanupActions) != 2 {
			t.Fatalf("expected 2 cleanup actions, got %d", len(ctx.CleanupActions))
		}

		// Check bash cleanup
		if ctx.CleanupActions[0].Path != "share/completions/bash/gh" {
			t.Errorf("expected cleanup path share/completions/bash/gh, got %s", ctx.CleanupActions[0].Path)
		}
		if ctx.CleanupActions[0].Action != "delete_file" {
			t.Errorf("expected action delete_file, got %s", ctx.CleanupActions[0].Action)
		}
		if ctx.CleanupActions[0].ContentHash == "" {
			t.Error("expected non-empty content hash")
		}

		// Check zsh cleanup (should use _ prefix)
		if ctx.CleanupActions[1].Path != "share/completions/zsh/_gh" {
			t.Errorf("expected cleanup path share/completions/zsh/_gh, got %s", ctx.CleanupActions[1].Path)
		}
	})
}

func TestInstallCompletionsAction_Execute_SourceCommand(t *testing.T) {
	a := &InstallCompletionsAction{}

	t.Run("runs command and writes output to completions dir", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		toolInstallDir := filepath.Join(toolsDir, "gh-2.0")

		if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
			t.Fatal(err)
		}

		scriptContent := "#!/bin/sh\necho \"completion for $1\"\n"
		scriptPath := filepath.Join(toolInstallDir, "gh")
		if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir:     toolInstallDir,
			ToolInstallDir: toolInstallDir,
			ToolsDir:       toolsDir,
		}

		params := map[string]interface{}{
			"source_command": "{install_dir}/gh {shell}",
			"target":         "gh",
			"shells":         []interface{}{"bash", "zsh"},
		}

		if err := a.Execute(ctx, params); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// bash: share/completions/bash/gh
		bashPath := filepath.Join(tsukuHome, "share", "completions", "bash", "gh")
		content, err := os.ReadFile(bashPath)
		if err != nil {
			t.Fatalf("expected file %s to exist: %v", bashPath, err)
		}
		expected := "completion for bash\n"
		if string(content) != expected {
			t.Errorf("expected content %q, got %q", expected, string(content))
		}

		// zsh: share/completions/zsh/_gh (underscore prefix)
		zshPath := filepath.Join(tsukuHome, "share", "completions", "zsh", "_gh")
		content, err = os.ReadFile(zshPath)
		if err != nil {
			t.Fatalf("expected file %s to exist: %v", zshPath, err)
		}
		expected = "completion for zsh\n"
		if string(content) != expected {
			t.Errorf("expected content %q, got %q", expected, string(content))
		}
	})

	t.Run("skips shell when command fails", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		toolInstallDir := filepath.Join(toolsDir, "gh-2.0")

		if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Script that fails for fish
		scriptContent := "#!/bin/sh\nif [ \"$1\" = \"fish\" ]; then exit 1; fi\necho \"completion for $1\"\n"
		scriptPath := filepath.Join(toolInstallDir, "gh")
		if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir:     toolInstallDir,
			ToolInstallDir: toolInstallDir,
			ToolsDir:       toolsDir,
		}

		params := map[string]interface{}{
			"source_command": "{install_dir}/gh {shell}",
			"target":         "gh",
			"shells":         []interface{}{"bash", "fish"},
		}

		// Should not error -- fish failure is a warning, bash succeeds
		if err := a.Execute(ctx, params); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// bash should exist
		bashPath := filepath.Join(tsukuHome, "share", "completions", "bash", "gh")
		if _, err := os.Stat(bashPath); os.IsNotExist(err) {
			t.Errorf("expected bash completion to exist")
		}

		// fish should NOT exist
		fishPath := filepath.Join(tsukuHome, "share", "completions", "fish", "gh")
		if _, err := os.Stat(fishPath); err == nil {
			t.Errorf("did not expect fish completion to exist (command should have failed)")
		}
	})

	t.Run("rejects command binary outside tool install dir", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		toolInstallDir := filepath.Join(toolsDir, "gh-2.0")

		if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir:     toolInstallDir,
			ToolInstallDir: toolInstallDir,
			ToolsDir:       toolsDir,
		}

		params := map[string]interface{}{
			"source_command": "/usr/bin/echo completions",
			"target":         "gh",
			"shells":         []interface{}{"bash"},
		}

		err := a.Execute(ctx, params)
		if err == nil {
			t.Fatal("expected error for binary outside tool install dir")
		}
	})

	t.Run("records cleanup actions for source_command", func(t *testing.T) {
		tsukuHome := t.TempDir()
		toolsDir := filepath.Join(tsukuHome, "tools")
		toolInstallDir := filepath.Join(toolsDir, "gh-2.0")

		if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
			t.Fatal(err)
		}

		scriptContent := "#!/bin/sh\necho \"comp\"\n"
		scriptPath := filepath.Join(toolInstallDir, "gh")
		if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
			t.Fatal(err)
		}

		ctx := &ExecutionContext{
			InstallDir:     toolInstallDir,
			ToolInstallDir: toolInstallDir,
			ToolsDir:       toolsDir,
		}

		params := map[string]interface{}{
			"source_command": "{install_dir}/gh {shell}",
			"target":         "gh",
			"shells":         []interface{}{"bash"},
		}

		if err := a.Execute(ctx, params); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if len(ctx.CleanupActions) != 1 {
			t.Fatalf("expected 1 cleanup action, got %d", len(ctx.CleanupActions))
		}
		if ctx.CleanupActions[0].Path != "share/completions/bash/gh" {
			t.Errorf("expected path share/completions/bash/gh, got %s", ctx.CleanupActions[0].Path)
		}
	})
}

func TestCompletionFileName(t *testing.T) {
	tests := []struct {
		target, shell, expected string
	}{
		{"gh", "bash", "gh"},
		{"gh", "zsh", "_gh"},
		{"gh", "fish", "gh"},
		{"cargo-audit", "bash", "cargo-audit"},
		{"cargo-audit", "zsh", "_cargo-audit"},
	}

	for _, tt := range tests {
		t.Run(tt.target+"_"+tt.shell, func(t *testing.T) {
			got := completionFileName(tt.target, tt.shell)
			if got != tt.expected {
				t.Errorf("completionFileName(%q, %q) = %q, want %q", tt.target, tt.shell, got, tt.expected)
			}
		})
	}
}
