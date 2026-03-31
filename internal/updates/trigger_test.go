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
	os.MkdirAll(cacheDir, 0755)
	TouchSentinel(cacheDir)

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
	os.MkdirAll(cacheDir, 0755)

	sentinelPath := filepath.Join(cacheDir, SentinelFile)
	os.WriteFile(sentinelPath, nil, 0644)
	old := time.Now().Add(-25 * time.Hour)
	os.Chtimes(sentinelPath, old, old)

	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()

	// Will try to spawn (and fail in test env), but shouldn't panic
	CheckAndSpawnUpdateCheck(cfg, userCfg)
}

func TestCheckAndSpawnMissingSentinel(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()

	CheckAndSpawnUpdateCheck(cfg, userCfg)
}

func TestCheckAndSpawnLockHeld(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	os.MkdirAll(cacheDir, 0755)

	// Hold the lock using FileLock
	lockPath := filepath.Join(cacheDir, LockFile)
	lock := install.NewFileLock(lockPath)
	if err := lock.LockExclusive(); err != nil {
		t.Fatal(err)
	}
	defer lock.Unlock()

	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()

	// Should detect lock is held and return without spawning
	CheckAndSpawnUpdateCheck(cfg, userCfg)
}
