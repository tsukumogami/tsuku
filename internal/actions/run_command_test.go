package actions

import (
	"context"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestRunCommandAction_Name(t *testing.T) {
	t.Parallel()
	action := &RunCommandAction{}
	if action.Name() != "run_command" {
		t.Errorf("Name() = %q, want %q", action.Name(), "run_command")
	}
}

func TestRunCommandAction_Execute(t *testing.T) {
	t.Parallel()
	action := &RunCommandAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name: "test-tool",
			},
		},
	}

	err := action.Execute(ctx, map[string]interface{}{
		"command": "echo hello",
	})
	if err != nil {
		t.Errorf("Execute() with simple echo command failed: %v", err)
	}
}

func TestRunCommandAction_Execute_MissingCommand(t *testing.T) {
	t.Parallel()
	action := &RunCommandAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name: "test-tool",
			},
		},
	}

	err := action.Execute(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("Execute() should fail when 'command' parameter is missing")
	}
}

func TestRunCommandAction_Execute_RequiresSudo(t *testing.T) {
	t.Parallel()
	action := &RunCommandAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name: "test-tool",
			},
		},
	}

	// Should skip (not fail) when requires_sudo is true
	err := action.Execute(ctx, map[string]interface{}{
		"command":       "apt-get install something",
		"requires_sudo": true,
	})
	if err != nil {
		t.Errorf("Execute() with requires_sudo should not error: %v", err)
	}
}

func TestRunCommandAction_Execute_WithDescription(t *testing.T) {
	t.Parallel()
	action := &RunCommandAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name: "test-tool",
			},
		},
	}

	err := action.Execute(ctx, map[string]interface{}{
		"command":     "echo test",
		"description": "Test description",
	})
	if err != nil {
		t.Errorf("Execute() with description failed: %v", err)
	}
}

func TestRunCommandAction_Execute_VariableExpansion(t *testing.T) {
	t.Parallel()
	action := &RunCommandAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.2.3",
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name: "test-tool",
			},
		},
	}

	err := action.Execute(ctx, map[string]interface{}{
		"command": "echo {version}",
	})
	if err != nil {
		t.Errorf("Execute() with variable expansion failed: %v", err)
	}
}

func TestRunCommandAction_Execute_FailingCommand(t *testing.T) {
	t.Parallel()
	action := &RunCommandAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name: "test-tool",
			},
		},
	}

	err := action.Execute(ctx, map[string]interface{}{
		"command": "exit 1",
	})
	if err == nil {
		t.Error("Execute() should fail when command returns non-zero exit code")
	}
}

func TestRunCommandAction_Execute_CustomWorkDir(t *testing.T) {
	t.Parallel()
	action := &RunCommandAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		Context:    context.Background(),
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name: "test-tool",
			},
		},
	}

	err := action.Execute(ctx, map[string]interface{}{
		"command":     "pwd",
		"working_dir": "/tmp",
	})
	if err != nil {
		t.Errorf("Execute() with custom working_dir failed: %v", err)
	}
}

func TestRunCommandAction_Execute_ContextCancellation(t *testing.T) {
	t.Parallel()
	action := &RunCommandAction{}
	tmpDir := t.TempDir()

	// Create a context that is already canceled
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	ctx := &ExecutionContext{
		Context:    cancelledCtx,
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
		Recipe: &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name: "test-tool",
			},
		},
	}

	// Command should fail due to canceled context
	err := action.Execute(ctx, map[string]interface{}{
		"command": "sleep 10",
	})
	if err == nil {
		t.Error("Execute() should fail when context is canceled")
	}
}
