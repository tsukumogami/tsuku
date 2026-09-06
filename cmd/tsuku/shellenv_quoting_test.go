package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/shellquote"
)

// emitShellenv reproduces what the shellenv command writes to stdout, for a
// given home directory. It mirrors the command body rather than invoking the
// binary so the test can run without a build step; the two must be kept in
// step, which the round-trip assertions below would catch if they drifted.
func emitShellenv(homeDir string, cachePath string) string {
	var b strings.Builder
	binDir := filepath.Join(homeDir, "bin")
	currentDir := filepath.Join(homeDir, "tools", "current")
	b.WriteString("export PATH=" + shellquote.POSIX(binDir) + ":" +
		shellquote.POSIX(currentDir) + ":\"$PATH\"\n")
	if cachePath != "" {
		b.WriteString(". " + shellquote.POSIX(cachePath) + "\n")
	}
	return b.String()
}

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
			output := emitShellenv(home, "")

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

	output := emitShellenv(filepath.Dir(filepath.Dir(shellDir)), cachePath)
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
