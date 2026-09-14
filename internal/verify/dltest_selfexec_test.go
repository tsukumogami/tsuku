package verify

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestInstallDltestRefusesAndDoesNotFallBackToPath pins the constraint from
// tsuku#2582 that is easiest to get wrong while fixing it.
//
// installDltest used to resolve os.Executable() with no check and run it as
// `tsuku install tsuku-dltest`. Under `go test` that is this package's test
// binary, which discards its -test.* flags when handed a positional argument.
// The obvious repair is to ask selfexec.Binary() and, when it refuses, fall
// through to the exec.LookPath("tsuku") that already sits in the function --
// and that is worse than the bug: on a developer machine the fallback finds the
// real tsuku and performs a genuine install into the developer's actual
// $TSUKU_HOME, outside every temporary directory the test set up.
//
// So the refusal is terminal. This test puts a decoy named "tsuku" first on
// PATH, one that records having run, and requires that it is never invoked.
func TestInstallDltestRefusesAndDoesNotFallBackToPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("decoy relies on a shell script")
	}

	dir := t.TempDir()
	marker := filepath.Join(dir, "decoy-ran")
	decoy := filepath.Join(dir, "tsuku")

	script := "#!/bin/sh\necho ran > " + marker + "\nexit 0\n"
	if err := os.WriteFile(decoy, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := installDltest("")

	if !errors.Is(err, ErrHelperUnavailable) {
		t.Fatalf("installDltest() error = %v, want ErrHelperUnavailable.\n"+
			"Under a test binary there is no tsuku CLI to install the helper with, "+
			"and the caller is expected to skip level 3 verification.", err)
	}

	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("installDltest() fell back to the tsuku on PATH and ran it.\n" +
			"On a developer machine that is the real tsuku, and this would have " +
			"performed a genuine `install tsuku-dltest` against their own " +
			"$TSUKU_HOME rather than anything this test created.")
	}
}
