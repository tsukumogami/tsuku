package selfexec

import (
	"errors"
	"runtime/debug"
	"testing"
)

// withSeams substitutes the three inputs Binary reads and restores them after
// the test. The combinations that matter cannot be produced from inside one
// test binary: this suite is always a test binary, and it is always built from
// this package rather than from a sibling command.
func withSeams(t *testing.T, isTest bool, buildPath string, buildOK bool, exePath string, exeErr error) {
	t.Helper()

	origTest, origInfo, origExe := isTestBinary, readBuildInfo, executable
	t.Cleanup(func() { isTestBinary, readBuildInfo, executable = origTest, origInfo, origExe })

	isTestBinary = func() bool { return isTest }
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		if !buildOK {
			return nil, false
		}
		return &debug.BuildInfo{Path: buildPath}, true
	}
	executable = func() (string, error) { return exePath, exeErr }
}

// withBuildOnly seams the build info and the executable path but leaves
// isTestBinary alone, so the real testing.Testing() decides. Used where the
// point of the test is that the test-binary half is what refuses.
func withBuildOnly(t *testing.T, buildPath string, exePath string) {
	t.Helper()

	origInfo, origExe := readBuildInfo, executable
	t.Cleanup(func() { readBuildInfo, executable = origInfo, origExe })

	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Path: buildPath}, true
	}
	executable = func() (string, error) { return exePath, nil }
}

const (
	cli     = "github.com/tsukumogami/tsuku/cmd/tsuku"
	sibling = "github.com/tsukumogami/tsuku/cmd/seed-queue"
)

// TestBinaryRefusesATestBinary is the regression test for tsuku#2580, and it
// pins the test-binary half specifically.
//
// The build info is seamed to the CLI's own package so the identity half
// cannot do the work: without that, this test passes on its own because the
// suite's build path is internal/selfexec rather than cmd/tsuku, and a test
// named for the fork bomb would survive removal of the guard that prevents it.
// testing.Testing() is deliberately not seamed here -- the real one runs, as it
// would in any package's suite -- so this fails if that half is removed.
func TestBinaryRefusesATestBinary(t *testing.T) {
	withBuildOnly(t, cli, "/usr/local/bin/tsuku")

	if path, ok := Binary(); ok {
		t.Fatalf("Binary() approved %q from inside a test binary.\n"+
			"Running a test binary as tsuku is what took a host to 70,004 tasks: "+
			"a positional subcommand makes it discard its -test.* flags and run "+
			"the whole suite, which reaches the spawner again.", path)
	}
}

// TestBinaryRefusesASiblingCommand covers the second half of the guard.
//
// cmd/benchmark, cmd/seed-discovery and cmd/seed-queue all link
// internal/validate and internal/sandbox transitively. In one of those,
// testing.Testing() is false and os.Executable() is a real binary -- so the
// test-binary half of the guard approves it, and only the build info shows it
// is not the CLI. Mounting it at /usr/local/bin/tsuku and invoking `tsuku
// install --plan ...` would fail silently, with a seeding tool's exit code.
func TestBinaryRefusesASiblingCommand(t *testing.T) {
	withSeams(t, false, sibling, true, "/usr/local/bin/tsuku", nil)

	if path, ok := Binary(); ok {
		t.Fatalf("Binary() approved %q, built from %s rather than the CLI.\n"+
			"A sibling command does not understand `tsuku install`, and running "+
			"it as tsuku fails silently rather than loudly.", path, sibling)
	}
}

// TestBinaryAcceptsTheCLIWhateverItIsCalled pins the newly accepted direction.
//
// The QA binary is tsuku-test (Makefile: build-test), an un-renamed release
// artifact is tsuku-linux-amd64, and a user may rename the binary to anything.
// All are the CLI. internal/validate's old exact-match check rejected every one
// of them.
func TestBinaryAcceptsTheCLIWhateverItIsCalled(t *testing.T) {
	for _, name := range []string{
		"/usr/local/bin/tsuku",
		"/repo/tsuku-test",
		"/tmp/tsuku-linux-amd64",
		"/home/dev/bin/my-renamed-tsuku",
	} {
		withSeams(t, false, cli, true, name, nil)

		path, ok := Binary()
		if !ok {
			t.Errorf("Binary() refused %q, which is the CLI under another name", name)
			continue
		}
		if path != name {
			t.Errorf("Binary() = %q, want %q", path, name)
		}
	}
}

// TestBinaryIgnoresTheFilename pins the newly rejected direction, and the
// reason no name rule can work.
//
// `go test -c -o tsuku ./cmd/tsuku` produces a test binary called exactly
// "tsuku", and its build info names cmd/tsuku just as the real binary's does.
// Only testing.Testing() separates them. internal/sandbox's old prefix check
// accepted it, along with tsuku.test.
func TestBinaryIgnoresTheFilename(t *testing.T) {
	withSeams(t, true, cli, true, "/tmp/go-build42/b001/tsuku", nil)

	if _, ok := Binary(); ok {
		t.Fatal("Binary() approved a test binary because it was named \"tsuku\". " +
			"The name carries no information about how the binary was built.")
	}
}

// TestBinaryRefusesWhenBuildInfoIsMissing pins the fail-closed direction.
// Without build info there is no way to tell the CLI from a sibling, and
// guessing wrong fails silently.
func TestBinaryRefusesWhenBuildInfoIsMissing(t *testing.T) {
	withSeams(t, false, "", false, "/usr/local/bin/tsuku", nil)

	if _, ok := Binary(); ok {
		t.Fatal("Binary() approved a binary whose build info could not be read")
	}
}

func TestBinaryRefusesWhenThePathCannotBeResolved(t *testing.T) {
	withSeams(t, false, cli, true, "", errors.New("no /proc"))

	if _, ok := Binary(); ok {
		t.Fatal("Binary() approved a path it could not resolve")
	}
}
