//go:build e2e

package updates_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/updates"
)

// tsukuBinary returns the path to the tsuku binary for e2e tests.
// Requires TSUKU_TEST_BINARY to be set; calls t.Skip otherwise.
// PATH fallback is intentionally omitted: a system-installed tsuku may be
// an older release that lacks the features under test.
func tsukuBinary(t *testing.T) string {
	t.Helper()
	b := os.Getenv("TSUKU_TEST_BINARY")
	if b == "" {
		t.Skip("tsuku binary not found; set TSUKU_TEST_BINARY to run e2e tests")
	}
	if _, err := os.Stat(b); err != nil {
		t.Skipf("TSUKU_TEST_BINARY=%q not found: %v", b, err)
	}
	return b
}

// runTsuku spawns the tsuku binary with an isolated TSUKU_HOME and captures
// combined stdout+stderr. Returns the output and exit code.
func runTsuku(t *testing.T, tsukuHome string, args ...string) (string, int) {
	t.Helper()
	binary := tsukuBinary(t)
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(),
		"TSUKU_HOME="+tsukuHome,
		"TSUKU_TELEMETRY=0",
		"TSUKU_NO_SELF_UPDATE=1",
		// Force auto-apply and notifications regardless of CI/TTY detection.
		"TSUKU_AUTO_UPDATE=1",
		"CI=",
	)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		}
	}
	return string(out), code
}

// injectCacheEntry writes a pending update check entry into the cache directory.
func injectCacheEntry(t *testing.T, cacheDir string, entry updates.UpdateCheckEntry) {
	t.Helper()
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("create cache dir: %v", err)
	}
	if err := updates.WriteEntry(cacheDir, &entry); err != nil {
		t.Fatalf("write cache entry: %v", err)
	}
}

// waitForNotice polls for a notice file at 50ms intervals until timeout.
func waitForNotice(t *testing.T, noticesDir, toolName string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	path := filepath.Join(noticesDir, toolName+".json")
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("notice file for %q did not appear within %s", toolName, timeout)
}

// TestE2E_BackgroundApplyReturnsImmediately verifies that a foreground tsuku command
// returns without waiting for the background apply subprocess to finish.
func TestE2E_BackgroundApplyReturnsImmediately(t *testing.T) {
	tsukuHome := t.TempDir()
	cacheDir := updates.CacheDir(tsukuHome)

	injectCacheEntry(t, cacheDir, updates.UpdateCheckEntry{
		Tool:            "jq",
		ActiveVersion:   "1.0.0",
		LatestWithinPin: "99999.0.0-e2e",
		CheckedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	})

	// 5-second deadline accounts for process startup + registry init overhead.
	// Background apply must not block the foreground command perceptibly.
	deadline := time.Now().Add(5 * time.Second)
	runTsuku(t, tsukuHome, "list")
	if time.Now().After(deadline) {
		t.Error("tsuku list blocked longer than 5 seconds; foreground command should return immediately")
	}
}

// TestE2E_BackgroundApplyWritesNotices verifies that after triggering MaybeSpawnAutoApply,
// the background subprocess writes a notice file with Kind == auto_apply_result.
func TestE2E_BackgroundApplyWritesNotices(t *testing.T) {
	tsukuHome := t.TempDir()
	cacheDir := updates.CacheDir(tsukuHome)
	noticesDir := notices.NoticesDir(tsukuHome)
	toolName := "jq"

	injectCacheEntry(t, cacheDir, updates.UpdateCheckEntry{
		Tool:            toolName,
		ActiveVersion:   "1.0.0",
		LatestWithinPin: "99999.0.0-e2e",
		CheckedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	})

	runTsuku(t, tsukuHome, "list")

	waitForNotice(t, noticesDir, toolName, 10*time.Second)

	data, err := os.ReadFile(filepath.Join(noticesDir, toolName+".json"))
	if err != nil {
		t.Fatalf("read notice file: %v", err)
	}
	var n notices.Notice
	if err := json.Unmarshal(data, &n); err != nil {
		t.Fatalf("unmarshal notice: %v", err)
	}
	if n.Kind != notices.KindAutoApplyResult {
		t.Errorf("notice.Kind = %q, want %q", n.Kind, notices.KindAutoApplyResult)
	}
	if n.Tool != toolName {
		t.Errorf("notice.Tool = %q, want %q", n.Tool, toolName)
	}
}

// TestE2E_BackgroundApplyNoticesDisplayed verifies the cross-process notice handoff:
// a notice written by the background subprocess appears in the next command's output.
func TestE2E_BackgroundApplyNoticesDisplayed(t *testing.T) {
	tsukuHome := t.TempDir()
	cacheDir := updates.CacheDir(tsukuHome)
	noticesDir := notices.NoticesDir(tsukuHome)
	toolName := "jq"

	injectCacheEntry(t, cacheDir, updates.UpdateCheckEntry{
		Tool:            toolName,
		ActiveVersion:   "1.0.0",
		LatestWithinPin: "99999.0.0-e2e",
		CheckedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	})

	// First command triggers background apply.
	runTsuku(t, tsukuHome, "list")
	waitForNotice(t, noticesDir, toolName, 10*time.Second)

	// Second command should render the pending notice.
	out, _ := runTsuku(t, tsukuHome, "list")
	if !strings.Contains(out, toolName) {
		t.Errorf("second tsuku list output does not contain %q; got:\n%s", toolName, out)
	}

	// After display, notice should be marked shown.
	data, err := os.ReadFile(filepath.Join(noticesDir, toolName+".json"))
	if err != nil {
		t.Fatalf("read notice file after second command: %v", err)
	}
	var n notices.Notice
	if err := json.Unmarshal(data, &n); err != nil {
		t.Fatalf("unmarshal notice: %v", err)
	}
	if !n.Shown {
		t.Error("notice.Shown should be true after DisplayNotifications rendered it")
	}
}

// TestE2E_BackgroundApplyWritesNoticeOnInstallFailure verifies that a failure notice
// is written even when the install fails, and the cache entry is consumed.
func TestE2E_BackgroundApplyWritesNoticeOnInstallFailure(t *testing.T) {
	tsukuHome := t.TempDir()
	cacheDir := updates.CacheDir(tsukuHome)
	noticesDir := notices.NoticesDir(tsukuHome)
	toolName := "jq"

	injectCacheEntry(t, cacheDir, updates.UpdateCheckEntry{
		Tool:            toolName,
		ActiveVersion:   "1.0.0",
		LatestWithinPin: "99999.0.0-nonexistent",
		CheckedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	})

	runTsuku(t, tsukuHome, "list")
	waitForNotice(t, noticesDir, toolName, 10*time.Second)

	data, err := os.ReadFile(filepath.Join(noticesDir, toolName+".json"))
	if err != nil {
		t.Fatalf("read notice file: %v", err)
	}
	var n notices.Notice
	if err := json.Unmarshal(data, &n); err != nil {
		t.Fatalf("unmarshal notice: %v", err)
	}
	if n.Kind != notices.KindAutoApplyResult {
		t.Errorf("notice.Kind = %q, want %q", n.Kind, notices.KindAutoApplyResult)
	}

	// Cache entry must be consumed regardless of failure.
	entryPath := filepath.Join(cacheDir, toolName+".json")
	if _, err := os.Stat(entryPath); err == nil {
		t.Error("cache entry file should be absent after apply (RemoveEntry not called)")
	}
}
