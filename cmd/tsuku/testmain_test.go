package main

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

// TestMain refuses to run the suite when this test binary is handed a
// positional argument.
//
// A Go test binary accepts only -test.* flags, but it does not reject anything
// else: flag.Parse stops at the first non-flag argument, so an argument like
// "check-updates" silently discards every -test.* flag that follows it and the
// binary runs the whole package suite unfiltered. Anything that re-execs
// os.Executable() with a subcommand therefore turns one test process into a
// full suite run, and because this package's tests reach the update triggers,
// that run spawns more of them.
//
// internal/updates refuses to re-exec a test binary at all, which is what
// closes the hole. This is the second line of defense: if some future path
// spawns one anyway, it dies here with a diagnostic instead of quietly
// becoming another generation.
func TestMain(m *testing.M) {
	flag.Parse()

	if args := flag.Args(); len(args) > 0 {
		fmt.Fprintf(os.Stderr,
			"tsuku test binary invoked with positional argument %q. This binary "+
				"accepts only -test.* flags; refusing to run the test suite.\n",
			args[0])
		os.Exit(2)
	}

	os.Exit(m.Run())
}
