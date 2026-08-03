package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

// precedenceEnv is a $TSUKU_HOME with a competing prefix beside it, both under
// one temp root so nothing outside t.TempDir() is written or named.
type precedenceEnv struct {
	home       string
	binDir     string
	currentDir string
	shadowDir  string
}

func newPrecedenceEnv(t *testing.T) precedenceEnv {
	t.Helper()

	root := t.TempDir()
	env := precedenceEnv{
		home:       filepath.Join(root, "tsuku"),
		binDir:     filepath.Join(root, "tsuku", "bin"),
		currentDir: filepath.Join(root, "tsuku", "tools", "current"),
		shadowDir:  filepath.Join(root, "other", "bin"),
	}
	for _, dir := range []string{env.binDir, env.currentDir, env.shadowDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(env.home, "env"), []byte(config.EnvFileContent), 0o644); err != nil {
		t.Fatalf("writing env file: %v", err)
	}
	return env
}

// installVisibleTool writes a state.json recording one visible tool and puts its
// binary in tools/current, the way a real install leaves things.
func (e precedenceEnv) installVisibleTool(t *testing.T, tool, binary string) {
	t.Helper()

	state := install.State{Installed: map[string]install.ToolState{
		tool: {
			ActiveVersion: "1.0.0",
			Versions:      map[string]install.VersionState{"1.0.0": {Binaries: []string{binary}}},
			IsExplicit:    true,
		},
	}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshaling state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(e.home, "state.json"), data, 0o644); err != nil {
		t.Fatalf("writing state.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(e.currentDir, binary), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("writing managed binary: %v", err)
	}
}

func (e precedenceEnv) plantShadow(t *testing.T, binary string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.shadowDir, binary), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("writing shadow binary: %v", err)
	}
}

func (e precedenceEnv) config() *config.Config {
	return &config.Config{
		HomeDir:  e.home,
		ToolsDir: filepath.Join(e.home, "tools"),
		ShareDir: filepath.Join(e.home, "share"),
	}
}

// precedenceLine returns doctor's "Tool precedence" status line.
func precedenceLine(t *testing.T, stdout string) string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "Tool precedence") {
			return line
		}
	}
	t.Fatalf("no 'Tool precedence' line in doctor output:\n%s", stdout)
	return ""
}

// TestDoctorReportsShadowedTool is the issue's reproduction, driven through the
// real check. A tool tsuku manages sits behind a competing copy on PATH; every
// other check is green, and before this check doctor said "Everything looks
// good".
func TestDoctorReportsShadowedTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute-bit resolution differs on Windows")
	}

	env := newPrecedenceEnv(t)
	env.installVisibleTool(t, "koto", "koto")
	env.plantShadow(t, "koto")

	t.Setenv("PATH", strings.Join([]string{env.shadowDir, env.binDir, env.currentDir}, string(os.PathListSeparator)))

	var failed bool
	stdout, stderr := captureDoctorOutput(func() {
		failed, _ = runDoctorChecks(env.config(), env.home)
	})

	line := precedenceLine(t, stdout)
	if !strings.Contains(line, "WARN") {
		t.Errorf("expected a WARN on the precedence line, got %q", line)
	}

	// Reporting the shadowing path is the point: it is the thing a user cannot
	// discover without reading `which -a` by hand.
	shadowPath := filepath.Join(env.shadowDir, "koto")
	if !strings.Contains(stderr, shadowPath) {
		t.Errorf("expected the shadowing path %q in stderr, got:\n%s", shadowPath, stderr)
	}
	tsukuPath := filepath.Join(env.currentDir, "koto")
	if !strings.Contains(stderr, tsukuPath) {
		t.Errorf("expected tsuku's own path %q in stderr, got:\n%s", tsukuPath, stderr)
	}

	// WARN, not FAIL: `tsuku doctor || exit 1` must not start failing for
	// someone who fronts a system tool deliberately.
	if failed {
		t.Error("a shadowed tool must not fail the environment check")
	}
}

// TestDoctorPrecedenceQuietWhenTsukuWins is the other half: the same setup with
// tsuku's directories ahead of the competing one has to stay silent, or the
// warning is noise everyone learns to skip.
func TestDoctorPrecedenceQuietWhenTsukuWins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute-bit resolution differs on Windows")
	}

	env := newPrecedenceEnv(t)
	env.installVisibleTool(t, "koto", "koto")
	env.plantShadow(t, "koto")

	t.Setenv("PATH", strings.Join([]string{env.binDir, env.currentDir, env.shadowDir}, string(os.PathListSeparator)))

	stdout, _ := captureDoctorOutput(func() {
		runDoctorChecks(env.config(), env.home)
	})

	line := precedenceLine(t, stdout)
	if !strings.Contains(line, "ok") || strings.Contains(line, "WARN") {
		t.Errorf("expected the precedence check to pass, got %q", line)
	}
}

// TestDoctorPrecedenceIgnoresHiddenTools pins the exclusion that keeps this
// check usable. Execution dependencies are installed with no symlink in
// tools/current, so their names resolve to a system copy on every healthy
// machine; warning about them would mean a warning per hidden dependency.
func TestDoctorPrecedenceIgnoresHiddenTools(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute-bit resolution differs on Windows")
	}

	env := newPrecedenceEnv(t)

	state := install.State{Installed: map[string]install.ToolState{
		"node": {
			ActiveVersion:         "22.0.0",
			IsHidden:              true,
			IsExecutionDependency: true,
			Versions:              map[string]install.VersionState{"22.0.0": {Binaries: []string{"node"}}},
		},
	}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshaling state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(env.home, "state.json"), data, 0o644); err != nil {
		t.Fatalf("writing state.json: %v", err)
	}
	// The system copy that a hidden tool always resolves to.
	env.plantShadow(t, "node")

	t.Setenv("PATH", strings.Join([]string{env.shadowDir, env.binDir, env.currentDir}, string(os.PathListSeparator)))

	stdout, _ := captureDoctorOutput(func() {
		runDoctorChecks(env.config(), env.home)
	})

	line := precedenceLine(t, stdout)
	if strings.Contains(line, "WARN") {
		t.Errorf("a hidden execution dependency must not be reported as shadowed, got %q", line)
	}
}

// TestDoctorNamesTheToolBehindABinary covers the case where acting on the
// warning means knowing which tsuku entry owns the shadowed name. "rg" alone
// doesn't tell the reader to look at ripgrep.
func TestDoctorNamesTheToolBehindABinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute-bit resolution differs on Windows")
	}

	env := newPrecedenceEnv(t)
	env.installVisibleTool(t, "ripgrep", "rg")
	env.plantShadow(t, "rg")

	t.Setenv("PATH", strings.Join([]string{env.shadowDir, env.binDir, env.currentDir}, string(os.PathListSeparator)))

	_, stderr := captureDoctorOutput(func() {
		runDoctorChecks(env.config(), env.home)
	})

	if !strings.Contains(stderr, "rg (ripgrep)") {
		t.Errorf("expected the owning tool named beside the binary, got:\n%s", stderr)
	}
}

// TestDoctorPrecedenceAllowsShim guards the other false positive. tsuku owns
// two PATH entries and the env file puts bin ahead of tools/current, so a shim
// winning over the version symlink is the design working.
func TestDoctorPrecedenceAllowsShim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute-bit resolution differs on Windows")
	}

	env := newPrecedenceEnv(t)
	env.installVisibleTool(t, "ripgrep", "rg")
	if err := os.WriteFile(filepath.Join(env.binDir, "rg"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("writing shim: %v", err)
	}

	t.Setenv("PATH", strings.Join([]string{env.binDir, env.currentDir, env.shadowDir}, string(os.PathListSeparator)))

	stdout, _ := captureDoctorOutput(func() {
		runDoctorChecks(env.config(), env.home)
	})

	line := precedenceLine(t, stdout)
	if strings.Contains(line, "WARN") {
		t.Errorf("a shim in $TSUKU_HOME/bin must not be reported as shadowing, got %q", line)
	}
}
