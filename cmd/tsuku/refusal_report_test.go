package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/project"
)

// refusedLayout builds a .tsuku.toml that discovery refuses, and points the
// package's discovery seam at metadata that makes it so.
//
// The layout is the reported attack reduced to what a test can build: a config
// owned by somebody who is neither the invoking user nor root, under a
// directory anyone can write to. The ownership is presented rather than
// created, because a test running as an unprivileged user with no second
// account cannot create a file owned by another user (R26).
func refusedLayout(t *testing.T) (workDir, configPath string) {
	t.Helper()

	root := t.TempDir()
	// Keep the fixture out from under any resolved home, so the rule applies.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TSUKU_CEILING_PATHS", root)

	shared := filepath.Join(root, "shared")
	work := filepath.Join(shared, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(shared, project.ConfigFileName)
	if err := os.WriteFile(cfgPath, []byte("[tools]\nnode = \"20.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	owners := map[string]uint32{cfgPath: 9999}
	modes := map[string]fs.FileMode{shared: 0o777}

	real := project.OSDiscoveryEnv()
	fake := real
	fake.Lstat = func(path string) (project.FileMeta, error) {
		m, err := real.Lstat(path)
		if err != nil {
			return m, err
		}
		p := filepath.Clean(path)
		if uid, ok := owners[p]; ok {
			m.UID = uid
		}
		if mode, ok := modes[p]; ok {
			m.Mode = (m.Mode &^ fs.ModePerm) | mode
		}
		return m, nil
	}
	// Bound the ancestor walk at the fixture so it does not climb into the real
	// filesystem, where the presented owner owns nothing.
	fake.Ancestors = func(dir string) []string {
		dir = filepath.Clean(dir)
		var out []string
		for {
			out = append(out, dir)
			if dir == filepath.Clean(root) {
				return out
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return out
			}
			dir = parent
		}
	}

	orig := discoveryEnv
	discoveryEnv = func() project.DiscoveryEnv { return fake }
	t.Cleanup(func() { discoveryEnv = orig })

	return work, cfgPath
}

// TestRefusalIsReportedOnceWithAReasonAndARemedy covers R6.
//
// One line, on stderr, naming the file, why it was not applied, and something
// the reader can do. A refusal that names only a reason leaves somebody staring
// at a repository that stopped working.
func TestRefusalIsReportedOnceWithAReasonAndARemedy(t *testing.T) {
	work, cfgPath := refusedLayout(t)

	outcome := runCommandOutcome(t, func() error {
		_, err := loadProjectConfigReporting(work)
		return err
	})

	lines := nonEmptyLines(outcome.stderr)
	if len(lines) != 1 {
		t.Fatalf("want exactly one stderr line, got %d:\n%s", len(lines), outcome.stderr)
	}
	line := lines[0]
	if !strings.Contains(line, filepath.Base(cfgPath)) {
		t.Errorf("the line does not name the file: %q", line)
	}
	if !strings.Contains(line, "anyone can write") {
		t.Errorf("the line does not state a reason: %q", line)
	}
	if !strings.Contains(strings.ToLower(line), "move the checkout") {
		t.Errorf("the line does not offer a remedy: %q", line)
	}
	if outcome.stdout != "" {
		t.Errorf("a refusal reached stdout: %q", outcome.stdout)
	}
}

// TestRefusalQuotesAHostilePath is the assertion that cannot pass by accident.
//
// The directory name comes from a repository the invoking user may not control,
// so the path is attacker-chosen. Rendering it unquoted puts a newline and an
// escape sequence straight into the terminal.
func TestRefusalQuotesAHostilePath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TSUKU_CEILING_PATHS", root)

	hostile := filepath.Join(root, "a\nb$(x)`y`\x1b[31m")
	if err := os.MkdirAll(hostile, 0o755); err != nil {
		t.Skipf("cannot create a directory with that name here: %v", err)
	}
	cfgPath := filepath.Join(hostile, project.ConfigFileName)
	if err := os.WriteFile(cfgPath, []byte("[tools]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	real := project.OSDiscoveryEnv()
	fake := real
	fake.Lstat = func(path string) (project.FileMeta, error) {
		m, err := real.Lstat(path)
		if err != nil {
			return m, err
		}
		if filepath.Clean(path) == filepath.Clean(hostile) {
			m.Mode = (m.Mode &^ fs.ModePerm) | 0o777
		}
		return m, nil
	}
	orig := discoveryEnv
	discoveryEnv = func() project.DiscoveryEnv { return fake }
	t.Cleanup(func() { discoveryEnv = orig })

	outcome := runCommandOutcome(t, func() error {
		_, err := loadProjectConfigReporting(hostile)
		return err
	})

	if strings.Contains(outcome.stderr, "\x1b") {
		t.Error("a raw escape sequence reached the terminal")
	}
	if n := len(nonEmptyLines(outcome.stderr)); n != 1 {
		t.Fatalf("the hostile path broke the line count: got %d lines:\n%q", n, outcome.stderr)
	}
	if outcome.stdout != "" {
		t.Errorf("a refusal reached stdout: %q", outcome.stdout)
	}
}

// TestProjectInstallExitsForbiddenOnARefusedConfig covers R10 for
// `tsuku install` with no arguments: a non-zero exit, the refusal, and no claim
// that no .tsuku.toml exists -- because one does, and saying otherwise sends
// the reader to `tsuku init` for a file that is already there.
func TestProjectInstallExitsForbiddenOnARefusedConfig(t *testing.T) {
	work, _ := refusedLayout(t)
	chdir(t, work)

	outcome := runCommandOutcome(t, func() error {
		runProjectInstall(installCmd)
		return nil
	})

	if !outcome.exited || outcome.exit != ExitForbidden {
		t.Fatalf("want exit %d, got exited=%v code=%d\nstderr:\n%s",
			ExitForbidden, outcome.exited, outcome.exit, outcome.stderr)
	}
	if strings.Contains(outcome.stderr, "run 'tsuku init'") {
		t.Errorf("reported a missing config for a file that exists:\n%s", outcome.stderr)
	}
	if n := len(nonEmptyLines(outcome.stderr)); n != 1 {
		t.Errorf("want exactly one stderr line, got %d:\n%s", n, outcome.stderr)
	}
}

// TestShimInstallExitsForbiddenOnARefusedConfig is R10's other half.
func TestShimInstallExitsForbiddenOnARefusedConfig(t *testing.T) {
	work, _ := refusedLayout(t)
	chdir(t, work)

	outcome := runCommandOutcome(t, func() error {
		shimInstallCmd.Run(shimInstallCmd, nil)
		return nil
	})

	if !outcome.exited || outcome.exit != ExitForbidden {
		t.Fatalf("want exit %d, got exited=%v code=%d\nstderr:\n%s",
			ExitForbidden, outcome.exited, outcome.exit, outcome.stderr)
	}
	if strings.Contains(outcome.stderr, "Specify a tool name") {
		t.Errorf("reported a missing config for a file that exists:\n%s", outcome.stderr)
	}
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
