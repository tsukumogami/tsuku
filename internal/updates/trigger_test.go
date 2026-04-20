package updates

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

func TestCheckAndSpawnFreshSentinel(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := TouchSentinel(cacheDir); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()

	// Should return immediately without spawning (sentinel is fresh)
	CheckAndSpawnUpdateCheck(cfg, userCfg)
}

func TestCheckAndSpawnDisabledViaConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()
	f := false
	userCfg.Updates.Enabled = &f

	CheckAndSpawnUpdateCheck(cfg, userCfg)
}

func TestCheckAndSpawnDisabledViaEnv(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()
	t.Setenv("TSUKU_NO_UPDATE_CHECK", "1")

	CheckAndSpawnUpdateCheck(cfg, userCfg)
}

func TestCheckAndSpawnNilUserConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}

	CheckAndSpawnUpdateCheck(cfg, nil)
}

func TestCheckAndSpawnStaleSentinel(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	sentinelPath := filepath.Join(cacheDir, SentinelFile)
	if err := os.WriteFile(sentinelPath, nil, 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(sentinelPath, old, old); err != nil {
		t.Fatal(err)
	}

	// Hold the dedup lock so spawning is attempted but blocked. Without this,
	// os.Executable() returns the test binary and spawning it recursively
	// creates a fork bomb in the test environment.
	lockPath := filepath.Join(cacheDir, LockFile)
	lock := install.NewFileLock(lockPath)
	if err := lock.LockExclusive(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()

	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()

	// Detects stale sentinel and reaches the spawn path, blocked by held lock.
	CheckAndSpawnUpdateCheck(cfg, userCfg)
}

func TestCheckAndSpawnMissingSentinel(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Hold the dedup lock so spawning is attempted but blocked. Without this,
	// os.Executable() returns the test binary and spawning it recursively
	// creates a fork bomb in the test environment.
	lockPath := filepath.Join(cacheDir, LockFile)
	lock := install.NewFileLock(lockPath)
	if err := lock.LockExclusive(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()

	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()

	// Detects missing sentinel and reaches the spawn path, blocked by held lock.
	CheckAndSpawnUpdateCheck(cfg, userCfg)
}

func TestCheckAndSpawnLockHeld(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Hold the lock using FileLock
	lockPath := filepath.Join(cacheDir, LockFile)
	lock := install.NewFileLock(lockPath)
	if err := lock.LockExclusive(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()

	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()

	// Should detect lock is held and return without spawning
	CheckAndSpawnUpdateCheck(cfg, userCfg)
}

func TestMaybeSpawnAutoApplyNilUserConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}

	// Should return immediately without panicking
	MaybeSpawnAutoApply(cfg, nil)
}

func TestMaybeSpawnAutoApplyDisabled(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()
	f := false
	userCfg.Updates.AutoApply = &f

	MaybeSpawnAutoApply(cfg, userCfg)
}

func TestMaybeSpawnAutoApplyNoPendingEntries(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}
	t.Setenv("TSUKU_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	userCfg := userconfig.DefaultConfig()

	// Empty cache -- nothing to apply
	MaybeSpawnAutoApply(cfg, userCfg)
}

func TestMaybeSpawnAutoApplyLockHeld(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a pending entry so the function reaches the lock check
	entry := &UpdateCheckEntry{
		Tool:            "some-tool",
		ActiveVersion:   "1.0.0",
		LatestWithinPin: "1.1.0",
	}
	if err := WriteEntry(cacheDir, entry); err != nil {
		t.Fatal(err)
	}

	// Hold the apply lock
	lockPath := filepath.Join(cacheDir, ApplyLockFile)
	lock := install.NewFileLock(lockPath)
	if err := lock.LockExclusive(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()

	cfg := &config.Config{HomeDir: dir}
	t.Setenv("TSUKU_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	userCfg := userconfig.DefaultConfig()

	// Should detect lock is held and return without spawning
	MaybeSpawnAutoApply(cfg, userCfg)
}
