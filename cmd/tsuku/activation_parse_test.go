package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

// brokenProject writes a .tsuku.toml that will not parse, over a fake
// $TSUKU_HOME with real installation state.
func brokenProject(t *testing.T) (projectDir string, cfg *config.Config) {
	t.Helper()

	projectDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, ".tsuku.toml"),
		[]byte("[tools\njq = \"latest\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tsukuHome := t.TempDir()
	cfg = &config.Config{HomeDir: tsukuHome, ToolsDir: filepath.Join(tsukuHome, "tools")}
	if err := install.NewStateManager(cfg).Save(&install.State{
		Installed: map[string]install.ToolState{},
	}); err != nil {
		t.Fatal(err)
	}

	return projectDir, cfg
}

// tsuku hook-env on an unparseable file: one diagnostic line, no usage block,
// exit 0, and stdout carrying the recording so the next prompt says nothing.
//
// Exit status is asserted directly. Checking only for the absence of usage text
// would pass an implementation that returns the error, because cobra's usage
// block and main's non-zero exit are separate behaviors.
func TestHookEnv_ParseFailure(t *testing.T) {
	projectDir, cfg := brokenProject(t)

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))
	t.Setenv("TSUKU_HOME", cfg.HomeDir)
	chdir(t, projectDir)

	stdout, stderr, err := runHookEnv(t, "bash")

	if err != nil {
		t.Fatalf("hook-env returned an error for an unparseable file: %v. "+
			"main exits non-zero on a returned error, and a prompt hook that exits "+
			"non-zero gets wrapped in \"|| true\", discarding the diagnostic.", err)
	}
	if strings.Contains(stderr, "Usage:") || strings.Contains(stderr, "Flags:") {
		t.Errorf("stderr carries a usage block:\n%s", stderr)
	}
	if n := strings.Count(strings.TrimSpace(stderr), "\n"); n != 0 {
		t.Errorf("stderr should be exactly one line, got %d:\n%s", n+1, stderr)
	}
	if !strings.Contains(stderr, ".tsuku.toml") {
		t.Errorf("stderr should name the file, got:\n%s", stderr)
	}

	// The broken project is recorded, or the hook re-reports on every prompt.
	if !strings.Contains(stdout, "_TSUKU_DIR=") || !strings.Contains(stdout, projectDir) {
		t.Errorf("stdout should record the project directory, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "_TSUKU_STATE_STAMP=") {
		t.Errorf("stdout should record the stamp, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, `export PATH="/usr/bin"`) {
		t.Errorf("stdout should leave PATH unchanged, got:\n%s", stdout)
	}
}

// Feeding back only what that invocation emitted, a second run from the same
// directory says nothing.
func TestHookEnv_ParseFailureIsSilentOnTheSecondPrompt(t *testing.T) {
	projectDir, cfg := brokenProject(t)

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))
	t.Setenv("TSUKU_HOME", cfg.HomeDir)
	chdir(t, projectDir)

	stdout, stderr, err := runHookEnv(t, "bash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr == "" {
		t.Fatal("expected a diagnostic on arrival")
	}

	// The environment the emitted code would have produced, read back out of
	// that code rather than assembled by hand -- a hand-built environment
	// manufactures the state a broken implementation failed to emit.
	for name, value := range parseExports(stdout) {
		t.Setenv(name, value)
	}

	_, stderr2, err := runHookEnv(t, "bash")
	if err != nil {
		t.Fatalf("unexpected error on the second prompt: %v", err)
	}
	if stderr2 != "" {
		t.Errorf("the second prompt in the same broken project should say nothing, got:\n%s", stderr2)
	}
}

// tsuku shell on an unparseable file: the diagnostic on stderr, nothing on
// stdout, no recording, and exit 0 -- with and without _TSUKU_PREV_PATH set.
//
// Both halves matter. Leaving the status unstated lets an implementation return
// the parse error and exit non-zero, or emit empty output and fall into the
// "no .tsuku.toml found" branch, which also exits non-zero. Both would pass a
// criterion that only checked the streams.
func TestShell_ParseFailure(t *testing.T) {
	for _, prev := range []string{"", "/original/bin:/usr/bin"} {
		name := "without prevPath"
		if prev != "" {
			name = "with prevPath"
		}
		t.Run(name, func(t *testing.T) {
			projectDir, cfg := brokenProject(t)

			t.Setenv("PATH", "/usr/bin")
			t.Setenv("HOME", filepath.Dir(projectDir))
			t.Setenv("TSUKU_HOME", cfg.HomeDir)
			t.Setenv("_TSUKU_PREV_PATH", prev)
			chdir(t, projectDir)

			stdout, stderr, err := runShellCmd(t)

			if err != nil {
				t.Fatalf("tsuku shell returned an error for an unparseable file: %v", err)
			}
			if stdout != "" {
				t.Errorf("stdout should be empty; tsuku shell records nothing on this path, got:\n%s", stdout)
			}
			if !strings.Contains(stderr, ".tsuku.toml") {
				t.Errorf("stderr should carry the diagnostic, got:\n%s", stderr)
			}
			if strings.Contains(stderr, "no .tsuku.toml found") {
				t.Error("fell through to the no-project branch, which exits non-zero")
			}
		})
	}
}

// tsuku shell with no .tsuku.toml anywhere above keeps today's behavior: the
// "no project file" message and a non-zero exit.
//
// This is the no-regression criterion, and it doubles as the check that the
// exit seam works. If exitWithCode ever stopped halting control flow, this test
// would see no error while the parse-failure tests kept passing, and the
// difference between "does not exit" and "exits" would stop being observable.
func TestShell_NoProjectExitsNonZero(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", dir)
	t.Setenv("TSUKU_HOME", home)
	t.Setenv("_TSUKU_PREV_PATH", "")
	chdir(t, dir)

	stdout, stderr, err := runShellCmd(t)

	if err == nil {
		t.Fatal("expected a non-zero exit when no .tsuku.toml is found")
	}
	if !strings.Contains(err.Error(), "exited with code") {
		t.Errorf("err = %v, want an exit rather than a returned error", err)
	}
	if !strings.Contains(stderr, "no .tsuku.toml found") {
		t.Errorf("stderr = %q, want the no-project message", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

// Leaving a project through tsuku shell: PATH restored, exit 0. The deactivation
// branch produces output, so it must not reach the no-project exit above.
func TestShell_LeavingAProjectExitsZero(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()

	t.Setenv("PATH", "/tools/jq-1.7/bin:/usr/bin")
	t.Setenv("HOME", dir)
	t.Setenv("TSUKU_HOME", home)
	t.Setenv("_TSUKU_PREV_PATH", "/usr/bin")
	chdir(t, dir)

	stdout, stderr, err := runShellCmd(t)

	if err != nil {
		t.Fatalf("leaving a project should exit 0, got %v", err)
	}
	if stderr != "" {
		t.Errorf("leaving a project should print nothing, got:\n%s", stderr)
	}
	if !strings.Contains(stdout, `export PATH="/usr/bin"`) {
		t.Errorf("stdout should restore the pre-activation PATH, got:\n%s", stdout)
	}
}

// --quiet suppresses the parse diagnostic, exactly as it suppresses the five
// reasons.
func TestHookEnv_ParseFailureRespectsQuiet(t *testing.T) {
	projectDir, cfg := brokenProject(t)

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))
	t.Setenv("TSUKU_HOME", cfg.HomeDir)
	chdir(t, projectDir)

	orig := quietFlag
	t.Cleanup(func() { quietFlag = orig })
	quietFlag = true

	stdout, stderr, err := runHookEnv(t, "bash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Errorf("--quiet should suppress the parse diagnostic, got:\n%s", stderr)
	}
	// PATH behavior is unchanged: the recording still happens.
	if !strings.Contains(stdout, "_TSUKU_DIR=") {
		t.Errorf("--quiet must not change what is emitted, got:\n%s", stdout)
	}
}

// No project file anywhere and nothing previously activated: exit 0, both
// streams empty.
func TestHookEnv_NoProjectIsSilent(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", dir)
	t.Setenv("TSUKU_HOME", home)
	t.Setenv("_TSUKU_PREV_PATH", "")
	t.Setenv("_TSUKU_DIR", "")
	t.Setenv("_TSUKU_STATE_STAMP", "")
	chdir(t, dir)

	stdout, stderr, err := runHookEnv(t, "bash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("expected both streams empty, got stdout %q stderr %q", stdout, stderr)
	}
}

// Leaving a project restores the pre-activation PATH, prints nothing, exits 0.
func TestHookEnv_LeavingAProjectIsSilent(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()

	t.Setenv("PATH", "/tools/jq-1.7/bin:/usr/bin")
	t.Setenv("HOME", dir)
	t.Setenv("TSUKU_HOME", home)
	t.Setenv("_TSUKU_PREV_PATH", "/usr/bin")
	t.Setenv("_TSUKU_DIR", "/some/project")
	t.Setenv("_TSUKU_STATE_STAMP", "123-4")
	chdir(t, dir)

	stdout, stderr, err := runHookEnv(t, "bash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "" {
		t.Errorf("leaving a project should print nothing, got:\n%s", stderr)
	}
	if !strings.Contains(stdout, `export PATH="/usr/bin"`) {
		t.Errorf("stdout should restore the pre-activation PATH, got:\n%s", stdout)
	}
	for _, name := range []string{"_TSUKU_DIR", "_TSUKU_PREV_PATH", "_TSUKU_STATE_STAMP"} {
		if !strings.Contains(stdout, name) {
			t.Errorf("leaving should unset %s, got:\n%s", name, stdout)
		}
	}
}
