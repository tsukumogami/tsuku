package updates

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// recordSpawns replaces the detached spawn with a recorder for the duration of
// the test and returns a pointer to the commands it would have started.
//
// The recorder never starts a process. That matters for this file in
// particular: the behaviour under test is one that, unguarded, re-execs the
// test binary, and a test that verified it by really forking would be the
// fork bomb rather than a test for it.
func recordSpawns(t *testing.T) *[]string {
	t.Helper()

	var spawned []string
	original := spawnDetached
	spawnDetached = func(cmd *exec.Cmd) error {
		spawned = append(spawned, strings.Join(cmd.Args, " "))
		return nil
	}
	t.Cleanup(func() { spawnDetached = original })

	return &spawned
}

// TestTriggersDoNotSpawnUnderGoTest is the regression test for the update
// triggers re-execing their own test binary.
//
// Both triggers resolve what to spawn with os.Executable(). Under `go test`
// that is the package test binary, not tsuku, and a test binary handed a
// positional subcommand does not reject it -- flag.Parse stops at the first
// non-flag argument, so the child discards every -test.* flag and runs the
// whole package suite. A suite that reaches a trigger more than once then
// grows without bound, and the flock deduplication cannot prevent it because
// each test's temporary $TSUKU_HOME produces its own lock path.
//
// So the assertion is not "the right binary is spawned" but "nothing is
// spawned at all". Both triggers are driven past every early return that could
// make them return for some reason other than the guard: updates enabled, a
// stale sentinel, a pending cache entry, and locks nobody else holds.
func TestTriggersDoNotSpawnUnderGoTest(t *testing.T) {
	// The suppression env vars must be clear, or a pass would only prove that
	// the feature was switched off rather than that a test binary is refused.
	t.Setenv("TSUKU_NO_UPDATE_CHECK", "")
	t.Setenv("TSUKU_AUTO_UPDATE", "")
	t.Setenv("CI", "")

	dir := t.TempDir()
	cacheDir := CacheDir(dir)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// A sentinel old enough to be stale drives the check trigger to the spawn.
	sentinelPath := filepath.Join(cacheDir, SentinelFile)
	if err := os.WriteFile(sentinelPath, nil, 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(sentinelPath, old, old); err != nil {
		t.Fatal(err)
	}

	// A pending entry drives the auto-apply trigger to its spawn.
	if err := WriteEntry(cacheDir, &UpdateCheckEntry{
		Tool:            "some-tool",
		ActiveVersion:   "1.0.0",
		LatestWithinPin: "1.1.0",
	}); err != nil {
		t.Fatal(err)
	}

	spawned := recordSpawns(t)

	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()

	CheckAndSpawnUpdateCheck(cfg, userCfg)
	MaybeSpawnAutoApply(cfg, userCfg)

	if len(*spawned) != 0 {
		t.Fatalf("update triggers spawned %d process(es) under go test: %v.\n"+
			"os.Executable() is the test binary here, so each of these re-runs "+
			"this package's suite, which reaches the triggers again. Nothing may "+
			"be spawned while testing.Testing() reports true.",
			len(*spawned), *spawned)
	}
}

// TestSelfBinaryForSpawnRefusesTestBinary pins the guard itself, so that a
// refactor moving the spawn sites around cannot quietly drop it.
func TestSelfBinaryForSpawnRefusesTestBinary(t *testing.T) {
	binary, ok := selfBinaryForSpawn("update check")
	if ok {
		t.Fatalf("selfBinaryForSpawn returned %q under go test; it must refuse "+
			"to hand back a test binary for re-exec", binary)
	}
	if binary != "" {
		t.Errorf("selfBinaryForSpawn returned path %q alongside ok=false; want empty", binary)
	}
}
