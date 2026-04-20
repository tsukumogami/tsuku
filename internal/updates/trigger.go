package updates

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// CheckAndSpawnUpdateCheck is the single entry point for all update check triggers.
// It stats the sentinel file for staleness, checks config, attempts a non-blocking
// flock to deduplicate concurrent spawns, and launches a detached tsuku check-updates
// process if needed. All errors are logged at debug level and swallowed -- trigger
// failures must never block command execution.
func CheckAndSpawnUpdateCheck(cfg *config.Config, userCfg *userconfig.Config) {
	if userCfg == nil || !userCfg.UpdatesEnabled() {
		return
	}

	cacheDir := CacheDir(cfg.HomeDir)
	interval := userCfg.UpdatesCheckInterval()

	// Check staleness via sentinel mtime (<0.5ms)
	if !IsCheckStale(cacheDir, interval) {
		return
	}

	// Attempt non-blocking flock to deduplicate spawns
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Default().Debug("update check: create cache dir", "error", err)
		return
	}

	lockPath := filepath.Join(cacheDir, LockFile)
	lock := install.NewFileLock(lockPath)
	acquired, err := lock.TryLockExclusive()
	if err != nil {
		log.Default().Debug("update check: try lock", "error", err)
		return
	}
	if !acquired {
		// Another check is already running
		return
	}
	// Release the probe lock immediately -- the background process will acquire its own
	_ = lock.Unlock()

	// Spawn detached check-updates process
	spawnChecker()
}

// spawnDetached configures cmd for detached execution and starts it.
// It sets process group isolation via setSysProcAttr (Unix only), redirects
// all stdio to nil so the subprocess has no connection to the parent's
// terminal, then starts the process. The error from cmd.Start() is returned
// without swallowing -- callers decide how to handle it.
// Do not call cmd.Wait() after spawnDetached; the process runs independently.
func spawnDetached(cmd *exec.Cmd) error {
	setSysProcAttr(cmd)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

// spawnChecker launches a detached tsuku check-updates process.
// The process survives parent exit and runs independently.
func spawnChecker() {
	binary, err := os.Executable()
	if err != nil {
		log.Default().Debug("update check: resolve binary path", "error", err)
		return
	}

	cmd := exec.Command(binary, "check-updates")
	if err := spawnDetached(cmd); err != nil {
		log.Default().Debug("update check: spawn checker", "error", err)
		return
	}
	// Don't Wait() -- the process runs independently
}
