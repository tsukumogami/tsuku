package actions

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/shellenv"
)

// setEnvHome builds a $TSUKU_HOME skeleton and returns the home directory plus
// an ExecutionContext wired to it, with the tool installed at
// $TSUKU_HOME/tools/<name>-<version>.
func setEnvHome(t *testing.T, name, version string) (string, *ExecutionContext) {
	t.Helper()

	home := t.TempDir()
	toolsDir := filepath.Join(home, "tools")
	toolInstallDir := filepath.Join(toolsDir, name+"-"+version)
	if err := os.MkdirAll(toolInstallDir, 0755); err != nil {
		t.Fatalf("failed to create tool install dir: %v", err)
	}

	ctx := &ExecutionContext{
		Context:        context.Background(),
		WorkDir:        t.TempDir(),
		InstallDir:     filepath.Join(toolsDir, ".install"),
		ToolInstallDir: toolInstallDir,
		ToolsDir:       toolsDir,
		Version:        version,
		Recipe:         &recipe.Recipe{Metadata: recipe.MetadataSection{Name: name}},
	}
	return home, ctx
}

func TestSetEnvAction_Name(t *testing.T) {
	t.Parallel()
	action := &SetEnvAction{}
	if action.Name() != "set_env" {
		t.Errorf("Name() = %q, want %q", action.Name(), "set_env")
	}
}

// TestSetEnvAction_ReachesUserShell is the end-to-end check the action was
// missing: it installs exports plus a tool init script that consumes them,
// rebuilds the shell cache, then sources $TSUKU_HOME/env in a real bash
// subshell and reads the variable back. Asserting on the generated file alone
// is what let the "set_env does nothing" bug survive.
func TestSetEnvAction_ReachesUserShell(t *testing.T) {
	t.Parallel()

	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	home, ctx := setEnvHome(t, "nvm", "0.40.6")

	if err := os.WriteFile(filepath.Join(home, "env"), []byte(config.EnvFileContent), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	// The recipe exports NVM_DIR pointing at the tool install directory.
	action := &SetEnvAction{}
	if err := action.Execute(ctx, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "NVM_DIR", "value": "{install_dir}"},
			map[string]interface{}{"name": "NVM_VERSION", "value": "{version}"},
		},
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// The tool ships an init script that self-locates only when the variable is
	// unset -- the nvm.sh pattern. It records what it saw at source time, so the
	// assertion below fails if the export is concatenated after it rather than
	// before. Checking only the final value would pass either way, since a later
	// export simply overwrites.
	initScript := filepath.Join(ctx.ToolInstallDir, "nvm.sh")
	selfLocated := filepath.Join(home, "share", "shell.d")
	if err := os.WriteFile(initScript, []byte(
		"export NVM_DIR_AT_INIT=\"${NVM_DIR-<unset>}\"\n"+
			"if [ -z \"${NVM_DIR-}\" ]; then\n  NVM_DIR="+selfLocated+"\n  export NVM_DIR\nfi\n"), 0644); err != nil {
		t.Fatalf("failed to write init script: %v", err)
	}
	shellInit := &InstallShellInitAction{}
	// install_shell_init copies source_file from InstallDir, so point it at the
	// directory holding the script for this test.
	shellCtx := *ctx
	shellCtx.InstallDir = ctx.ToolInstallDir
	if err := shellInit.Execute(&shellCtx, map[string]interface{}{
		"source_file": "nvm.sh",
		"target":      "nvm",
		"shells":      []interface{}{"bash"},
	}); err != nil {
		t.Fatalf("install_shell_init Execute() error = %v", err)
	}

	if err := shellenv.RebuildShellCache(home, "bash"); err != nil {
		t.Fatalf("RebuildShellCache() error = %v", err)
	}

	read := func(varName string) string {
		t.Helper()
		cmd := exec.Command(bashPath, "--norc", "--noprofile", "-c",
			`. "$TSUKU_HOME/env"; printf %s "${`+varName+`-}"`)
		cmd.Env = []string{"TSUKU_HOME=" + home, "HOME=" + home, "PATH=/usr/bin:/bin"}
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("sourcing env for %s failed: %v", varName, err)
		}
		return string(out)
	}

	if got := read("NVM_DIR"); got != ctx.ToolInstallDir {
		t.Errorf("NVM_DIR = %q, want %q (the tool install dir, not the self-located %q)",
			got, ctx.ToolInstallDir, selfLocated)
	}
	if got := read("NVM_DIR_AT_INIT"); got != ctx.ToolInstallDir {
		t.Errorf("the tool init script saw NVM_DIR = %q, want %q; the exports must be "+
			"concatenated into the shell cache before the tool's own init script", got, ctx.ToolInstallDir)
	}
	if got := read("NVM_VERSION"); got != "0.40.6" {
		t.Errorf("NVM_VERSION = %q, want %q", got, "0.40.6")
	}
}

// The "00-env-" prefix is what keeps exports ahead of a tool's init script in
// the shell cache, so no other action may claim it.
func TestInstallShellInit_RejectsReservedEnvPrefix(t *testing.T) {
	t.Parallel()

	action := &InstallShellInitAction{}
	result := action.Preflight(map[string]interface{}{
		"source_file": "init.sh",
		"target":      EnvFilePrefix + "nvm",
	})
	if !result.HasErrors() {
		t.Errorf("Preflight() accepted a target claiming the %q prefix reserved for set_env", EnvFilePrefix)
	}

	result = action.Preflight(map[string]interface{}{
		"source_file": "init.sh",
		"target":      "nvm",
	})
	if result.HasErrors() {
		t.Errorf("Preflight() rejected an ordinary target: %v", result.Errors)
	}
}

func TestSetEnvAction_WritesShellDFiles(t *testing.T) {
	t.Parallel()

	home, ctx := setEnvHome(t, "demo", "2.1.0")

	action := &SetEnvAction{}
	if err := action.Execute(ctx, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "DEMO_HOME", "value": "{install_dir}"},
		},
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	shellDDir := filepath.Join(home, "share", "shell.d")
	for _, shell := range []string{"bash", "zsh"} {
		path := filepath.Join(shellDDir, "00-env-demo."+shell)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		want := "export DEMO_HOME='" + ctx.ToolInstallDir + "'\n"
		if string(content) != want {
			t.Errorf("%s content = %q, want %q", path, content, want)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("%s permissions = %o, want 0600", path, perm)
		}
	}

	// The staging directory must never appear in what the shell reads.
	if strings.Contains(ctx.ToolInstallDir, ".install") {
		t.Fatal("test setup error: tool install dir contains the staging marker")
	}
	if _, err := os.Stat(filepath.Join(ctx.ToolInstallDir, "env.sh")); !os.IsNotExist(err) {
		t.Error("set_env should no longer write env.sh into the tool directory")
	}
}

func TestSetEnvAction_RecordsCleanupActions(t *testing.T) {
	t.Parallel()

	home, ctx := setEnvHome(t, "demo", "2.1.0")

	action := &SetEnvAction{}
	if err := action.Execute(ctx, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "DEMO_HOME", "value": "{install_dir}"},
		},
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(ctx.CleanupActions) != 2 {
		t.Fatalf("recorded %d cleanup actions, want 2 (bash and zsh)", len(ctx.CleanupActions))
	}
	for _, ca := range ctx.CleanupActions {
		if ca.Action != "delete_file" {
			t.Errorf("cleanup action = %q, want delete_file", ca.Action)
		}
		if !strings.HasPrefix(ca.Path, "share/shell.d/00-env-demo.") {
			t.Errorf("cleanup path = %q, want a share/shell.d/00-env-demo.* entry", ca.Path)
		}
		content, err := os.ReadFile(filepath.Join(home, ca.Path))
		if err != nil {
			t.Fatalf("reading recorded path %s: %v", ca.Path, err)
		}
		if got := contentHash(content); got != ca.ContentHash {
			t.Errorf("recorded hash for %s = %q, want %q", ca.Path, ca.ContentHash, got)
		}
	}
}

// A recipe may carry several set_env steps; they all target one file, so a
// later step must not silently drop the earlier one's exports.
func TestSetEnvAction_MultipleStepsAppend(t *testing.T) {
	t.Parallel()

	home, ctx := setEnvHome(t, "demo", "2.1.0")
	action := &SetEnvAction{}

	for _, name := range []string{"FIRST", "SECOND"} {
		if err := action.Execute(ctx, map[string]interface{}{
			"vars": []interface{}{
				map[string]interface{}{"name": name, "value": "{install_dir}"},
			},
		}); err != nil {
			t.Fatalf("Execute() for %s error = %v", name, err)
		}
	}

	path := filepath.Join(home, "share", "shell.d", "00-env-demo.bash")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	want := "export FIRST='" + ctx.ToolInstallDir + "'\nexport SECOND='" + ctx.ToolInstallDir + "'\n"
	if string(content) != want {
		t.Errorf("content = %q, want %q", content, want)
	}

	// One cleanup action per shell, and its hash must cover the merged file.
	if len(ctx.CleanupActions) != 2 {
		t.Fatalf("recorded %d cleanup actions, want 2", len(ctx.CleanupActions))
	}
	for _, ca := range ctx.CleanupActions {
		merged, err := os.ReadFile(filepath.Join(home, ca.Path))
		if err != nil {
			t.Fatalf("reading %s: %v", ca.Path, err)
		}
		if got := contentHash(merged); got != ca.ContentHash {
			t.Errorf("hash for %s = %q, want %q (must cover the merged file)", ca.Path, ca.ContentHash, got)
		}
	}
}

func TestSetEnvAction_NoShellInit(t *testing.T) {
	t.Parallel()

	home, ctx := setEnvHome(t, "demo", "2.1.0")
	ctx.NoShellInit = true

	action := &SetEnvAction{}
	if err := action.Execute(ctx, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "DEMO_HOME", "value": "{install_dir}"},
		},
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, "share", "shell.d")); !os.IsNotExist(err) {
		t.Error("--no-shell-init should not create shell.d")
	}
	if len(ctx.CleanupActions) != 0 {
		t.Errorf("--no-shell-init recorded %d cleanup actions, want 0", len(ctx.CleanupActions))
	}
}

func TestSetEnvAction_RequiresToolInstallDir(t *testing.T) {
	t.Parallel()

	_, ctx := setEnvHome(t, "demo", "2.1.0")
	ctx.ToolInstallDir = ""

	action := &SetEnvAction{}
	err := action.Execute(ctx, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "DEMO_HOME", "value": "{install_dir}"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "post-install") {
		t.Errorf("Execute() error = %v, want an error naming the post-install phase", err)
	}
}

func TestSetEnvAction_QuotesValues(t *testing.T) {
	t.Parallel()

	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	home, ctx := setEnvHome(t, "demo", "2.1.0")

	action := &SetEnvAction{}
	if err := action.Execute(ctx, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "WITH_SPACES", "value": "a b c"},
			map[string]interface{}{"name": "WITH_QUOTE", "value": "it's"},
			map[string]interface{}{"name": "WITH_META", "value": "$(touch pwned);`id`"},
		},
	}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	script := filepath.Join(home, "share", "shell.d", "00-env-demo.bash")
	cmd := exec.Command(bashPath, "--norc", "--noprofile", "-c",
		`. "`+script+`"; printf '%s|%s|%s' "$WITH_SPACES" "$WITH_QUOTE" "$WITH_META"`)
	cmd.Dir = home
	cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sourcing generated script failed: %v", err)
	}

	want := "a b c|it's|$(touch pwned);`id`"
	if string(out) != want {
		t.Errorf("sourced values = %q, want %q", out, want)
	}
	if _, err := os.Stat(filepath.Join(home, "pwned")); !os.IsNotExist(err) {
		t.Error("command substitution in a value was executed by the shell")
	}
}

func TestSetEnvAction_RejectsUnsafeVars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		varName     string
		varValue    string
		errContains string
	}{
		{"name with semicolon", "FOO;rm -rf /", "bar", "invalid variable name"},
		{"name with space", "FOO BAR", "bar", "invalid variable name"},
		{"name starting with digit", "1FOO", "bar", "invalid variable name"},
		{"empty name", "", "bar", "invalid variable name"},
		{"value with newline", "FOO", "bar\nexport EVIL=1", "newlines"},
		{"value with carriage return", "FOO", "bar\rbaz", "newlines"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			home, ctx := setEnvHome(t, "demo", "2.1.0")
			params := map[string]interface{}{
				"vars": []interface{}{
					map[string]interface{}{"name": tt.varName, "value": tt.varValue},
				},
			}

			action := &SetEnvAction{}
			err := action.Execute(ctx, params)
			if err == nil || !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Execute() error = %v, want an error containing %q", err, tt.errContains)
			}
			if _, statErr := os.Stat(filepath.Join(home, "share", "shell.d", "00-env-demo.bash")); statErr == nil {
				t.Error("a rejected variable should not have been written")
			}

			if result := action.Preflight(params); !result.HasErrors() {
				t.Errorf("Preflight() accepted %s", tt.name)
			}
		})
	}
}

func TestSetEnvAction_Preflight(t *testing.T) {
	t.Parallel()

	action := &SetEnvAction{}

	if result := action.Preflight(map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "JAVA_HOME", "value": "{install_dir}"},
		},
	}); result.HasErrors() {
		t.Errorf("Preflight() rejected a valid recipe: %v", result.Errors)
	}

	if result := action.Preflight(map[string]interface{}{}); !result.HasErrors() {
		t.Error("Preflight() should reject a missing 'vars' parameter")
	}

	if result := action.Preflight(map[string]interface{}{"vars": []interface{}{}}); !result.HasErrors() {
		t.Error("Preflight() should reject an empty 'vars' list")
	}
}

func TestSetEnvAction_RequiresRecipeName(t *testing.T) {
	t.Parallel()

	_, ctx := setEnvHome(t, "demo", "2.1.0")
	ctx.Recipe = nil

	action := &SetEnvAction{}
	err := action.Execute(ctx, map[string]interface{}{
		"vars": []interface{}{
			map[string]interface{}{"name": "DEMO_HOME", "value": "{install_dir}"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "recipe name") {
		t.Errorf("Execute() error = %v, want an error about the missing recipe name", err)
	}
}

func TestEnvTargetName_RejectsPathSegments(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"../escape", "a/b", ".", ".."} {
		ctx := &ExecutionContext{Recipe: &recipe.Recipe{Metadata: recipe.MetadataSection{Name: name}}}
		if _, err := envTargetName(ctx); err == nil {
			t.Errorf("envTargetName() accepted recipe name %q", name)
		}
	}
}

func TestSetEnvAction_Execute_MissingVars(t *testing.T) {
	t.Parallel()

	_, ctx := setEnvHome(t, "demo", "1.0.0")

	action := &SetEnvAction{}
	err := action.Execute(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("Execute() should fail when 'vars' parameter is missing")
	}
}

func TestSetEnvAction_parseVars(t *testing.T) {
	t.Parallel()
	action := &SetEnvAction{}

	// Valid input
	vars := []interface{}{
		map[string]interface{}{"name": "FOO", "value": "bar"},
		map[string]interface{}{"name": "BAZ", "value": "qux"},
	}

	result, err := action.parseVars(vars)
	if err != nil {
		t.Fatalf("parseVars() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("parseVars() returned %d vars, want 2", len(result))
	}
	if result[0].Name != "FOO" || result[0].Value != "bar" {
		t.Errorf("parseVars()[0] = {%q, %q}, want {FOO, bar}", result[0].Name, result[0].Value)
	}
}

// Malformed vars must be rejected by parseVars itself and by Execute, which
// wraps it -- a recipe reaching Execute with bad vars must not write a file.
func TestSetEnvAction_parseVars_InvalidFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       interface{}
		errContains string
	}{
		{"not an array", "string", "array"},
		{"array item not a map", []interface{}{"string"}, "map"},
		{"missing name", []interface{}{map[string]interface{}{"value": "bar"}}, "name"},
		{"missing value", []interface{}{map[string]interface{}{"name": "FOO"}}, "value"},
		{"name not string", []interface{}{map[string]interface{}{"name": 123, "value": "bar"}}, "name"},
		{"value not string", []interface{}{map[string]interface{}{"name": "FOO", "value": 123}}, "value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			action := &SetEnvAction{}

			if _, err := action.parseVars(tt.input); err == nil {
				t.Errorf("parseVars(%v) should fail", tt.input)
			}

			home, ctx := setEnvHome(t, "demo", "1.0.0")
			err := action.Execute(ctx, map[string]interface{}{"vars": tt.input})
			if err == nil || !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Execute() error = %v, want an error containing %q", err, tt.errContains)
			}
			if _, statErr := os.Stat(filepath.Join(home, "share", "shell.d")); !os.IsNotExist(statErr) {
				t.Error("malformed vars should not have produced any shell.d output")
			}
		})
	}
}
