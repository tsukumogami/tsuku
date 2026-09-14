// Package selfexec answers one question: may this process run its own
// executable as the tsuku command line, and where is it?
//
// Several places resolve os.Executable() and then run the result as tsuku --
// the update triggers spawn it as `tsuku check-updates`, the sandbox and
// validate executors mount it into a container and invoke `tsuku install
// --plan ...`, and the self-update path overwrites it in place. Each of those
// is correct only if the running executable really is the tsuku CLI. Two
// distinct things make it not so, and each is invisible to the check for the
// other.
//
// The first is a Go test binary. Under `go test`, os.Executable() is the
// package test binary, and a test binary handed a positional subcommand does
// not reject it: flag.Parse stops at the first non-flag argument, so the child
// discards every -test.* flag and runs the whole package suite. When that suite
// reaches a spawner again the process count grows by a factor per generation,
// which is how tsuku#2580 took a machine to 70,004 tasks.
//
// The second is a sibling binary. This module builds several other commands --
// cmd/benchmark, cmd/seed-discovery and cmd/seed-queue among them -- and they
// link internal/validate and internal/sandbox transitively. In one of those,
// os.Executable() is a perfectly real, non-test binary that knows nothing about
// `tsuku install`. Mounting it at /usr/local/bin/tsuku and invoking it would
// fail silently: the container would get a seeding tool's exit code where an
// install's was expected.
//
// Neither question can be answered from the file's name, which is why this
// package does not look at it. `go test -c -o tsuku ./cmd/tsuku` produces a
// test binary called exactly "tsuku", so no name rule can reject test
// binaries; and the QA binary "tsuku-test" (Makefile: build-test), an
// un-renamed release artifact such as "tsuku-linux-amd64", and a binary a user
// simply renamed are all real tsuku, so no name rule can accept them all. The
// two existing name checks in this tree disagree in opposite directions --
// internal/sandbox accepted "tsuku.test", internal/validate rejected
// "tsuku-test" -- and neither could be made right.
//
// So identity comes from the build rather than the filename. The embedded
// build info records which main package produced the binary, survives the
// release build's -s -w -trimpath, and is unaffected by renaming. Test-ness
// comes from testing.Testing(), which cmd/go sets for every test build and
// nothing else sets. Both halves are needed: build info alone cannot see that
// tsuku.test is a test binary, because its main package is cmd/tsuku too, and
// testing.Testing() alone cannot see that seed-queue is not the CLI.
//
// It is a leaf package rather than a function in one of its callers.
// internal/verify importing internal/updates to reach the guard would drag a
// SQLite driver, a GitHub API client and oauth2 into a package that verifies
// installed libraries. A predicate several callers need and none owns belongs
// beside none of them.
//
// What a refusal means is the caller's decision, not this package's. The
// update triggers must stop -- a system tsuku may be an older release, and
// running it would be a different program writing this $TSUKU_HOME. The
// container executors degrade to a tsuku on PATH on purpose, because for a
// containerised install a slightly different tsuku is an acceptable stand-in.
package selfexec

import (
	"os"
	"runtime/debug"
	"testing"
)

// cliPackage is the import path of tsuku's command line entry point. A binary
// built from any other main package in this module is not the CLI, however it
// is named.
const cliPackage = "github.com/tsukumogami/tsuku/cmd/tsuku"

// Seams. Tests substitute these to drive combinations that cannot be produced
// from inside a single test binary -- a non-test build, or a sibling command's
// build info. Nothing in production reassigns them.
var (
	isTestBinary  = testing.Testing
	readBuildInfo = debug.ReadBuildInfo
	executable    = os.Executable
)

// Binary returns the path of this process's executable when it is safe to run
// it as the tsuku CLI, and reports false when it is not.
//
// False means: do not exec this path, do not mount it as tsuku, do not
// overwrite it with a downloaded release. It is returned for a Go test binary,
// for a binary built from any main package other than tsuku's own, when the
// build info needed to tell those apart is missing, and when the path cannot
// be resolved at all. The caller decides what to do instead.
func Binary() (string, bool) {
	if isTestBinary() {
		return "", false
	}

	info, ok := readBuildInfo()
	if !ok || info == nil || info.Path != cliPackage {
		// Missing build info is a refusal rather than a shrug: without it there
		// is no way to tell the CLI from a sibling command, and the failure
		// mode of guessing wrong is silent.
		return "", false
	}

	path, err := executable()
	if err != nil {
		return "", false
	}
	return path, true
}
