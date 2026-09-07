package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// chdir moves into dir for the duration of the test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

// runCommand runs fn with both standard streams captured and exitWithCode
// intercepted, and returns what each stream received.
//
// Intercepting the exit is what lets a test assert "this path does not exit
// non-zero" as an ordinary failure. Without it a regression would call os.Exit
// and take the test binary down mid-run, reporting nothing about which
// assertion was violated.
//
// It swaps process-global state -- both standard streams and the exit seam --
// so callers must not be parallel. That is why no test in this file calls
// t.Parallel.
func runCommand(t *testing.T, fn func() error) (stdout, stderr string, err error) {
	t.Helper()

	got := runCommandOutcome(t, fn)
	return got.stdout, got.stderr, got.err
}

// commandOutcome is everything a driven command produced, including the exit
// code as a number.
//
// runCommand flattens the exit into an error, which is enough for a test
// asserting that a path does not exit at all. A test whose subject is *which*
// code a path exits with has to compare numbers, and reading them back out of
// an error string would pass for a command that exited with a different code
// and a matching message.
type commandOutcome struct {
	stdout string
	stderr string

	// exit is the code passed to exitWithCode, and exited says whether one was.
	// A command that returned normally leaves exit at zero, which is why the
	// two are separate: zero is also a code a command can exit with.
	exit   int
	exited bool

	// err is what fn returned, or the exit rendered as an error for
	// runCommand's callers.
	err error
}

// runCommandOutcome is runCommand keeping the exit code. See commandOutcome.
func runCommandOutcome(t *testing.T, fn func() error) (outcome commandOutcome) {
	t.Helper()

	outR, outW, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	errR, errW, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}

	origOut, origErr, origExit := os.Stdout, os.Stderr, exitFunc
	os.Stdout, os.Stderr = outW, errW
	exitFunc = func(int) {} // exitWithCode panics with the sentinel after this

	func() {
		defer func() {
			// exitWithCode never returns, in tests either: the override above
			// falls through to its panic, which is recovered here. Anything
			// else is a real panic and is re-raised.
			if r := recover(); r != nil {
				sentinel, ok := r.(exitSentinel)
				if !ok {
					panic(r)
				}
				outcome.exit, outcome.exited = sentinel.code, true
				outcome.err = fmt.Errorf("command exited with code %d", sentinel.code)
			}
		}()
		outcome.err = fn()
	}()

	os.Stdout, os.Stderr, exitFunc = origOut, origErr, origExit
	_ = outW.Close()
	_ = errW.Close()

	outcome.stdout, outcome.stderr = readAll(t, outR), readAll(t, errR)
	return outcome
}

func runHookEnv(t *testing.T, shell string) (stdout, stderr string, err error) {
	t.Helper()
	return runCommand(t, func() error {
		return hookEnvCmd.RunE(hookEnvCmd, []string{shell})
	})
}

func runShellCmd(t *testing.T) (stdout, stderr string, err error) {
	t.Helper()
	orig := shellFlag
	shellFlag = "bash"
	t.Cleanup(func() { shellFlag = orig })

	return runCommand(t, func() error {
		return shellCmd.RunE(shellCmd, nil)
	})
}

// parseUnsets reads the variable names out of emitted bash unset statements.
func parseUnsets(script string) []string {
	var names []string
	for _, line := range strings.Split(script, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "unset ")
		if !ok {
			continue
		}
		names = append(names, strings.Fields(rest)...)
	}
	return names
}

// parseExports reads variable assignments back out of emitted bash export
// statements.
//
// Tests build the next invocation's environment from this rather than by hand.
// A hand-built environment manufactures exactly the state a broken
// implementation failed to emit, so a build that should fail passes.
func parseExports(script string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(script, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "export ")
		if !ok {
			continue
		}
		name, quoted, ok := strings.Cut(rest, "=")
		if !ok {
			continue
		}
		value, err := strconv.Unquote(quoted)
		if err != nil {
			// The emitted quoting is a Go string literal today. If that changes
			// -- and it should, since %q is not a shell quoter -- this falls
			// back to the raw text rather than silently dropping the variable,
			// which would make a "second invocation stays silent" test pass for
			// the wrong reason.
			value = strings.Trim(quoted, `"'`)
		}
		out[name] = value
	}
	return out
}
