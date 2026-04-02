package updates

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

func TestDisplayNotifications_Suppressed(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()

	// Force suppression via quiet
	clearSuppressEnv(t)
	mockTTY(t, true)

	results := []ApplyResult{{Tool: "test", OldVersion: "1.0", NewVersion: "2.0"}}

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	DisplayNotifications(cfg, userCfg, true, results)

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = old

	if buf.Len() > 0 {
		t.Errorf("expected no output when suppressed, got %q", buf.String())
	}
}

func TestDisplayNotifications_ApplyResults(t *testing.T) {
	dir := t.TempDir()
	setupCacheDirs(t, dir)
	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()

	clearSuppressEnv(t)
	mockTTY(t, true)

	results := []ApplyResult{
		{Tool: "node", OldVersion: "20.14.0", NewVersion: "20.15.0"},
		{Tool: "broken", OldVersion: "1.0.0", NewVersion: "2.0.0", Err: fmt.Errorf("download failed")},
	}

	output := captureStderr(t, func() {
		DisplayNotifications(cfg, userCfg, false, results)
	})

	if !bytes.Contains(output, []byte("Updated node 20.14.0 -> 20.15.0")) {
		t.Errorf("expected success line, got %q", output)
	}
	if !bytes.Contains(output, []byte("Update failed: broken -> 2.0.0")) {
		t.Errorf("expected failure line, got %q", output)
	}
}

func TestDisplayNotifications_UnshownNotices(t *testing.T) {
	dir := t.TempDir()
	noticesDir := notices.NoticesDir(dir)
	setupCacheDirs(t, dir)
	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()

	clearSuppressEnv(t)
	mockTTY(t, true)

	// Write an unshown notice
	notice := &notices.Notice{
		Tool:             "ripgrep",
		AttemptedVersion: "14.0.0",
		Error:            "checksum mismatch",
		Timestamp:        time.Now(),
		Shown:            false,
	}
	_ = notices.WriteNotice(noticesDir, notice)

	output := captureStderr(t, func() {
		DisplayNotifications(cfg, userCfg, false, nil)
	})

	if !bytes.Contains(output, []byte("Update failed: ripgrep -> 14.0.0")) {
		t.Errorf("expected notice output, got %q", output)
	}

	// Notice should be marked shown
	after, _ := notices.ReadUnshownNotices(noticesDir)
	if len(after) != 0 {
		t.Error("notice should be marked shown after display")
	}
}

func TestDisplayNotifications_SuccessNotice(t *testing.T) {
	dir := t.TempDir()
	noticesDir := notices.NoticesDir(dir)
	setupCacheDirs(t, dir)
	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()

	clearSuppressEnv(t)
	mockTTY(t, true)

	// Write a success notice (empty Error = success)
	notice := &notices.Notice{
		Tool:             "tsuku",
		AttemptedVersion: "0.8.0",
		Error:            "",
		Timestamp:        time.Now(),
		Shown:            false,
	}
	_ = notices.WriteNotice(noticesDir, notice)

	output := captureStderr(t, func() {
		DisplayNotifications(cfg, userCfg, false, nil)
	})

	if !bytes.Contains(output, []byte("tsuku has been updated to 0.8.0")) {
		t.Errorf("expected success notice, got %q", output)
	}
}

func TestDisplayNotifications_AvailableSummary(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write cache entries with available updates
	for _, name := range []string{"node", "ripgrep", "jq"} {
		entry := &UpdateCheckEntry{
			Tool:            name,
			ActiveVersion:   "1.0.0",
			LatestWithinPin: "1.1.0",
			CheckedAt:       time.Now(),
			ExpiresAt:       time.Now().Add(24 * time.Hour),
		}
		_ = WriteEntry(cacheDir, entry)
	}

	cfg := &config.Config{HomeDir: dir}
	// Disable auto-apply so available summary shows
	f := false
	userCfg := userconfig.DefaultConfig()
	userCfg.Updates.AutoApply = &f
	clearSuppressEnv(t)
	mockTTY(t, true)

	output := captureStderr(t, func() {
		DisplayNotifications(cfg, userCfg, false, nil)
	})

	if !bytes.Contains(output, []byte("3 updates available")) {
		t.Errorf("expected '3 updates available', got %q", output)
	}
}

func TestDisplayNotifications_AvailableSummary_SentinelDedup(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	entry := &UpdateCheckEntry{
		Tool:            "node",
		ActiveVersion:   "1.0.0",
		LatestWithinPin: "1.1.0",
		CheckedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	}
	_ = WriteEntry(cacheDir, entry)

	cfg := &config.Config{HomeDir: dir}
	f := false
	userCfg := userconfig.DefaultConfig()
	userCfg.Updates.AutoApply = &f
	clearSuppressEnv(t)
	mockTTY(t, true)

	// First call should show summary
	output1 := captureStderr(t, func() {
		DisplayNotifications(cfg, userCfg, false, nil)
	})
	if !bytes.Contains(output1, []byte("1 update available")) {
		t.Errorf("first call should show summary, got %q", output1)
	}

	// Second call should be deduped (sentinel is fresh)
	output2 := captureStderr(t, func() {
		DisplayNotifications(cfg, userCfg, false, nil)
	})
	if bytes.Contains(output2, []byte("update available")) {
		t.Errorf("second call should be deduped, got %q", output2)
	}
}

// setupCacheDirs creates the cache/updates directory.
func setupCacheDirs(t *testing.T, homeDir string) {
	t.Helper()
	cacheDir := filepath.Join(homeDir, "cache", "updates")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
}

// captureStderr captures stderr output during fn execution.
func captureStderr(t *testing.T, fn func()) []byte {
	t.Helper()
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	fn()
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = old
	return buf.Bytes()
}
