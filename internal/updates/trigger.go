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

// ApplyLockFile is the advisory lock file for apply-updates spawn deduplication.
const ApplyLockFile = ".apply-lock"

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

// MaybeSpawnAutoApply spawns a detached tsuku apply-updates process if auto-apply
// is enabled and pending cache entries exist. All errors are logged at debug level
// and swallowed -- trigger failures must never block command execution.
func MaybeSpawnAutoApply(cfg *config.Config, userCfg *userconfig.Config) error {
	if userCfg == nil || !userCfg.UpdatesAutoApplyEnabled() {
		return nil
	}

	cacheDir := CacheDir(cfg.HomeDir)

	// Check for pending entries before spawning
	entries, err := ReadAllEntries(cacheDir)
	if err != nil {
		log.Default().Debug("auto-apply trigger: read cache entries", "error", err)
		return nil
	}

	var hasPending bool
	for _, e := range entries {
		if IsSelfUpdate(&e) {
			continue
		}
		if e.LatestWithinPin != "" && e.Error == "" && e.LatestWithinPin != e.ActiveVersion {
			hasPending = true
			break
		}
	}
	if !hasPending {
		return nil
	}

	// Use a dedicated probe lock to deduplicate spawns
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Default().Debug("auto-apply trigger: create cache dir", "error", err)
		return nil
	}

	lockPath := filepath.Join(cacheDir, ApplyLockFile)
	lock := install.NewFileLock(lockPath)
	acquired, err := lock.TryLockExclusive()
	if err != nil {
		log.Default().Debug("auto-apply trigger: try lock", "error", err)
		return nil
	}
	if !acquired {
		// Another spawn is already running or was recently spawned
		return nil
	}
	// Release probe lock immediately -- the background process manages its own locking
	_ = lock.Unlock()

	binary, err := os.Executable()
	if err != nil {
		log.Default().Debug("auto-apply trigger: resolve binary path", "error", err)
		return nil
	}

	cmd := exec.Command(binary, "apply-updates")
	if err := spawnDetached(cmd); err != nil {
		log.Default().Debug("auto-apply trigger: spawn apply-updates", "error", err)
	}
	return nil
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
