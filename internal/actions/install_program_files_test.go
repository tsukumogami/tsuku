package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// programFilesCtx builds an execution context whose tool install directory is populated,
// the way the post-install phase runs it.
func programFilesCtx(t *testing.T, tool string) (*ExecutionContext, string, string) {
	t.Helper()
	home := t.TempDir()
	toolsDir := filepath.Join(home, "tools")
	installDir := filepath.Join(toolsDir, tool+"-1.0.0")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	ctx := &ExecutionContext{
		Context:        context.Background(),
		ToolsDir:       toolsDir,
		ToolInstallDir: installDir,
		Version:        "1.0.0",
		Recipe:         &recipe.Recipe{Metadata: recipe.MetadataSection{Name: tool}},
	}
	return ctx, home, installDir
}

func TestInstallProgramFilesExecute(t *testing.T) {
	tests := []struct {
		name string
		// seed populates the tool install directory and returns the files parameter.
		seed func(t *testing.T, installDir string) []interface{}
		// wantErr is a substring of the expected error, or "" when the call must succeed.
		wantErr string
		// check runs on success.
		check func(t *testing.T, home, installDir string, ctx *ExecutionContext)
	}{
		{
			name: "copies files and preserves executability",
			seed: func(t *testing.T, installDir string) []interface{} {
				if err := os.WriteFile(filepath.Join(installDir, "nvm.sh"), []byte("# program"), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				if err := os.WriteFile(filepath.Join(installDir, "nvm-exec"), []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return []interface{}{"nvm.sh", "nvm-exec"}
			},
			check: func(t *testing.T, home, installDir string, ctx *ExecutionContext) {
				dataDir := filepath.Join(home, "data", "nvm")
				sh, err := os.Stat(filepath.Join(dataDir, "nvm.sh"))
				if err != nil {
					t.Fatalf("nvm.sh was not placed: %v", err)
				}
				if sh.Mode().Perm()&0o111 != 0 {
					t.Errorf("nvm.sh mode = %v, want no execute bit", sh.Mode().Perm())
				}
				exec, err := os.Stat(filepath.Join(dataDir, "nvm-exec"))
				if err != nil {
					t.Fatalf("nvm-exec was not placed: %v", err)
				}
				// nvm invokes this file directly; losing the bit breaks `nvm exec`.
				if exec.Mode().Perm()&0o111 == 0 {
					t.Errorf("nvm-exec mode = %v, want it executable", exec.Mode().Perm())
				}
				if len(ctx.CleanupActions) != 2 {
					t.Errorf("CleanupActions = %d, want 2", len(ctx.CleanupActions))
				}
				for _, ca := range ctx.CleanupActions {
					if filepath.IsAbs(ca.Path) {
						t.Errorf("cleanup path %q is absolute; it is resolved against $TSUKU_HOME", ca.Path)
					}
				}
			},
		},
		{
			// The archive extractor's containment is lexical, so a release tarball can
			// carry a symlink pointing anywhere. Resolving before trusting is what stops
			// it becoming a read-any-file primitive.
			name: "refuses a file that resolves outside the tool directory",
			seed: func(t *testing.T, installDir string) []interface{} {
				outside := filepath.Join(t.TempDir(), "secret.txt")
				if err := os.WriteFile(outside, []byte("not yours"), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				if err := os.Symlink(outside, filepath.Join(installDir, "nvm.sh")); err != nil {
					t.Fatalf("Symlink: %v", err)
				}
				return []interface{}{"nvm.sh"}
			},
			wantErr: "outside the tool install directory",
		},
		{
			name: "refuses a source that is not a regular file",
			seed: func(t *testing.T, installDir string) []interface{} {
				if err := os.MkdirAll(filepath.Join(installDir, "nvm.sh"), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				return []interface{}{"nvm.sh"}
			},
			wantErr: "not a regular file",
		},
		{
			name: "errors when the tool install directory is not available",
			seed: func(t *testing.T, installDir string) []interface{} {
				return []interface{}{"nvm.sh"}
			},
			wantErr: "post-install",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, home, installDir := programFilesCtx(t, "nvm")
			files := tt.seed(t, installDir)
			if tt.wantErr == "post-install" {
				ctx.ToolInstallDir = ""
			}

			action := &InstallProgramFilesAction{}
			err := action.Execute(ctx, map[string]interface{}{"files": files})

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Execute() error = nil, want one containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Execute() error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			tt.check(t, home, installDir, ctx)
		})
	}
}

func TestInstallProgramFilesRefreshesOnReinstall(t *testing.T) {
	// Placement and repointing are the same operation, which is the whole reason this
	// copies rather than links: an upgrade must leave the data root holding the new
	// version's program files without any separate refresh step.
	ctx, home, installDir := programFilesCtx(t, "nvm")
	if err := os.WriteFile(filepath.Join(installDir, "nvm.sh"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	action := &InstallProgramFilesAction{}
	params := map[string]interface{}{"files": []interface{}{"nvm.sh"}}
	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	if err := os.WriteFile(filepath.Join(installDir, "nvm.sh"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := action.Execute(ctx, params); err != nil {
		t.Fatalf("second Execute: %v", err)
	}

	placed, err := os.ReadFile(filepath.Join(home, "data", "nvm", "nvm.sh"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(placed) != "v2" {
		t.Errorf("placed file = %q, want %q", placed, "v2")
	}
	if len(ctx.CleanupActions) != 1 {
		t.Errorf("CleanupActions = %d after two runs, want 1", len(ctx.CleanupActions))
	}
}

func TestInstallProgramFilesPreflight(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]interface{}
		wantErr string
	}{
		{
			name:    "accepts relative paths",
			params:  map[string]interface{}{"files": []interface{}{"nvm.sh", "nvm-exec"}},
			wantErr: "",
		},
		{
			name:    "requires files",
			params:  map[string]interface{}{},
			wantErr: "non-empty 'files'",
		},
		{
			name:    "rejects an absolute path",
			params:  map[string]interface{}{"files": []interface{}{"/etc/passwd"}},
			wantErr: "must be relative",
		},
		{
			name:    "rejects parent traversal",
			params:  map[string]interface{}{"files": []interface{}{"../../etc/passwd"}},
			wantErr: "..",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := &InstallProgramFilesAction{}
			result := action.Preflight(tt.params)

			if tt.wantErr == "" {
				if result.HasErrors() {
					t.Fatalf("Preflight() errors = %v, want none", result.Errors)
				}
				return
			}
			if !result.HasErrors() {
				t.Fatalf("Preflight() errors = none, want one containing %q", tt.wantErr)
			}
			if !strings.Contains(strings.Join(result.Errors, "; "), tt.wantErr) {
				t.Errorf("Preflight() errors = %v, want one containing %q", result.Errors, tt.wantErr)
			}
		})
	}
}

func TestInstallProgramFilesRunsAfterTheAtomicRename(t *testing.T) {
	// The source only exists once the tool's permanent directory does, so this action
	// has to be a post-install one.
	action := &InstallProgramFilesAction{}
	if got := action.DefaultPhase(); got != "post-install" {
		t.Errorf("DefaultPhase() = %q, want %q", got, "post-install")
	}
}
