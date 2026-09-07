package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestShellenv_HostileHomeDoesNotExecute covers the emitter that quoted
// nothing at all: it interpolated into hand-written double quotes under a
// command its own help documents as `eval $(tsuku shellenv)`.
//
// The precondition is load-bearing and is the whole reason this test
// discriminates. Both interpolated components derive from the home directory,
// so with a benign home they round-trip untouched and the assertions below
// pass against completely unmodified code. TSUKU_HOME must contain a
// metacharacter.
func TestShellenv_HostileHomeDoesNotExecute(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	scratch := t.TempDir()
	marker := filepath.Join(scratch, "pwned")

	for _, hostile := range []string{
		"$(touch " + marker + ")",
		"`touch " + marker + "`",
		"$HOME",
		"pro'j",
	} {
		t.Run(hostile, func(t *testing.T) {
			home := filepath.Join(scratch, hostile)
			output := shellenvScript(home, "")

			// PATH must survive: the trailing $PATH is the one expansion in
			// this emission that has to stay live. An implementation that
			// quotes the whole statement passes every "nothing executed"
			// assertion while discarding the user's PATH.
			script := "export PATH=/sentinel/before\n" + output + "\nprintf '%s' \"$PATH\"\n"
			out, err := exec.Command(bash, "--norc", "--noprofile", "-c", script).Output()
			if err != nil {
				t.Fatalf("bash rejected the emitted output: %v\n%s", err, output)
			}

			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("home %q executed a command:\n%s", hostile, output)
			}
			got := string(out)
			if !strings.HasSuffix(got, "/sentinel/before") {
				t.Errorf("pre-existing PATH was discarded\n got: %q\nwant suffix: %q\noutput:\n%s",
					got, "/sentinel/before", output)
			}
			if !strings.Contains(got, filepath.Join(home, "bin")) {
				t.Errorf("bin directory missing from PATH\n got: %q\noutput:\n%s", got, output)
			}
		})
	}
}

// TestShellenv_CacheSourceLineIsQuoted covers the second emission separately.
//
// It sits behind an os.Stat in the command, so the precondition here is that
// the cache file exists -- without it the line never runs and an assertion on
// it passes vacuously.
func TestShellenv_CacheSourceLineIsQuoted(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	scratch := t.TempDir()
	marker := filepath.Join(scratch, "pwned")
	shellDir := filepath.Join(scratch, "$(touch "+marker+")", "share", "shell.d")
	if err := os.MkdirAll(shellDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(shellDir, ".init-cache.bash")
	// The precondition: the file must exist, or the emission is skipped.
	if err := os.WriteFile(cachePath, []byte("SOURCED=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output := shellenvScript(filepath.Dir(filepath.Dir(shellDir)), cachePath)
	script := output + "\nprintf '%s' \"$SOURCED\"\n"
	out, err := exec.Command(bash, "--norc", "--noprofile", "-c", script).Output()
	if err != nil {
		t.Fatalf("bash rejected the emitted output: %v\n%s", err, output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("cache path executed a command:\n%s", output)
	}
	if string(out) != "1" {
		t.Errorf("cache file was not sourced: SOURCED=%q\noutput:\n%s", string(out), output)
	}
}

// TestShellenvCmd_HostileTsukuHome drives the actual cobra command with
// TSUKU_HOME set to a path containing a command substitution, and evaluates
// what it wrote to stdout.
//
// The test above covers the emitter; this one covers the wiring to it. Between
// the environment variable and the quoted output sit config.DefaultConfig and
// filepath.Abs, and a RunE that stopped calling shellenvScript -- or quoted
// something itself on the way past -- would leave the emitter's own test green.
// The acceptance criterion asks for TSUKU_HOME specifically, and the reason is
// that both interpolated components derive from it: with a benign home they
// round-trip untouched and every assertion here passes against unmodified,
// vulnerable code.
func TestShellenvCmd_HostileTsukuHome(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	scratch := t.TempDir()
	marker := filepath.Join(scratch, "pwned-by-cmd")
	t.Setenv("TSUKU_HOME", filepath.Join(scratch, "$(touch "+marker+")"))

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	runErr := shellenvCmd.RunE(shellenvCmd, nil)
	os.Stdout = orig
	w.Close()
	out, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("shellenv returned %v", runErr)
	}

	script := string(out)
	if script == "" {
		t.Fatal("shellenv wrote nothing to stdout")
	}

	evaluated, err := exec.Command(bash, "--norc", "--noprofile", "-c",
		script+"\nprintf '%s' \"$PATH\"").Output()
	if err != nil {
		t.Fatalf("bash rejected the emitted script: %v\nscript:\n%s", err, script)
	}

	if _, err := os.Stat(marker); err == nil {
		t.Errorf("evaluating shellenv output ran the substitution in TSUKU_HOME.\n\nscript:\n%s", script)
	}
	if !strings.Contains(string(evaluated), "$(touch") {
		t.Errorf("the hostile home did not survive into PATH literally, so this "+
			"fixture is not exercising the quoting.\nPATH = %s", evaluated)
	}
	// The trailing $PATH must still expand: over-quoting passes every
	// "nothing executed" assertion while discarding the user's PATH.
	if !strings.Contains(script, `:"$PATH"`) {
		t.Errorf("emitted script does not leave the trailing $PATH live:\n%s", script)
	}
}
