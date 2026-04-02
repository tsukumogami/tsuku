package updates

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

func TestMaybeAutoApplyDisabled(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()
	f := false
	userCfg.Updates.AutoApply = &f

	called := false
	installFn := func(_, _, _ string) error {
		called = true
		return nil
	}

	MaybeAutoApply(cfg, userCfg, installFn, nil)
	if called {
		t.Error("installFn should not be called when auto-apply is disabled")
	}
}

func TestMaybeAutoApplyNilUserConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}

	// Should not panic
	MaybeAutoApply(cfg, nil, func(_, _, _ string) error { return nil }, nil)
}

func TestMaybeAutoApplyNoPendingEntries(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}
	t.Setenv("TSUKU_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	userCfg := userconfig.DefaultConfig()

	called := false
	MaybeAutoApply(cfg, userCfg, func(_, _, _ string) error {
		called = true
		return nil
	}, nil)
	if called {
		t.Error("installFn should not be called with no pending entries")
	}
}

func TestMaybeAutoApplySuccessfulUpdate(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a pending cache entry
	entry := &UpdateCheckEntry{
		Tool:            "test-tool",
		ActiveVersion:   "1.0.0",
		Requested:       "1",
		LatestWithinPin: "1.1.0",
		LatestOverall:   "2.0.0",
		CheckedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	}
	if err := WriteEntry(cacheDir, entry); err != nil {
		t.Fatal(err)
	}

	// Write state.json so the manager can load it
	stateDir := filepath.Join(dir, "tools")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"installed":{"test-tool":{"active_version":"1.0.0","versions":{"1.0.0":{"requested":"1"}}}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{HomeDir: dir, ToolsDir: stateDir}
	t.Setenv("TSUKU_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	userCfg := userconfig.DefaultConfig()

	var installedTool, installedVersion string
	MaybeAutoApply(cfg, userCfg, func(toolName, version, constraint string) error {
		installedTool = toolName
		installedVersion = version
		return nil
	}, nil)

	if installedTool != "test-tool" {
		t.Errorf("installed tool = %q, want %q", installedTool, "test-tool")
	}
	if installedVersion != "1.1.0" {
		t.Errorf("installed version = %q, want %q", installedVersion, "1.1.0")
	}

	// Cache entry should be removed
	remaining, _ := ReadEntry(cacheDir, "test-tool")
	if remaining != nil {
		t.Error("cache entry should be removed after successful apply")
	}
}

func TestMaybeAutoApplyFailureWritesNotice(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	noticesDir := filepath.Join(dir, "notices")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	entry := &UpdateCheckEntry{
		Tool:            "fail-tool",
		ActiveVersion:   "1.0.0",
		Requested:       "",
		LatestWithinPin: "2.0.0",
		LatestOverall:   "2.0.0",
		CheckedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	}
	if err := WriteEntry(cacheDir, entry); err != nil {
		t.Fatal(err)
	}

	stateDir := filepath.Join(dir, "tools")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"installed":{"fail-tool":{"active_version":"1.0.0","versions":{"1.0.0":{"requested":""}}}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{HomeDir: dir, ToolsDir: stateDir}
	t.Setenv("TSUKU_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	userCfg := userconfig.DefaultConfig()

	MaybeAutoApply(cfg, userCfg, func(_, _, _ string) error {
		return fmt.Errorf("download failed: network error")
	}, nil)

	// Notice should be written (and marked shown by displayUnshownNotices at the end of MaybeAutoApply)
	allNotices, err := notices.ReadAllNotices(noticesDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(allNotices) != 1 {
		t.Fatalf("expected 1 notice, got %d", len(allNotices))
	}
	if allNotices[0].Tool != "fail-tool" {
		t.Errorf("notice tool = %q, want %q", allNotices[0].Tool, "fail-tool")
	}
	// Notice is marked shown because MaybeAutoApply displays it on stderr
	if !allNotices[0].Shown {
		t.Error("notice should be shown after MaybeAutoApply displays it")
	}

	// Cache entry should be removed (prevents repeated attempts)
	remaining, _ := ReadEntry(cacheDir, "fail-tool")
	if remaining != nil {
		t.Error("cache entry should be removed after failed apply")
	}
}

func TestMaybeAutoApplySkipsErrorEntries(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Entry with Error field set (check failed)
	entry := &UpdateCheckEntry{
		Tool:            "error-tool",
		ActiveVersion:   "1.0.0",
		LatestWithinPin: "1.1.0",
		Error:           "network timeout",
		CheckedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	}
	if err := WriteEntry(cacheDir, entry); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{HomeDir: dir}
	t.Setenv("TSUKU_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	userCfg := userconfig.DefaultConfig()

	called := false
	MaybeAutoApply(cfg, userCfg, func(_, _, _ string) error {
		called = true
		return nil
	}, nil)

	if called {
		t.Error("installFn should not be called for entries with Error")
	}
}

func TestMaybeAutoApplySkipsAlreadyCurrent(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Entry where LatestWithinPin == ActiveVersion (already up to date)
	entry := &UpdateCheckEntry{
		Tool:            "current-tool",
		ActiveVersion:   "1.0.0",
		LatestWithinPin: "1.0.0",
		LatestOverall:   "2.0.0",
		CheckedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(24 * time.Hour),
	}
	if err := WriteEntry(cacheDir, entry); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{HomeDir: dir}
	t.Setenv("TSUKU_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	userCfg := userconfig.DefaultConfig()

	called := false
	MaybeAutoApply(cfg, userCfg, func(_, _, _ string) error {
		called = true
		return nil
	}, nil)

	if called {
		t.Error("installFn should not be called when already at latest within pin")
	}
}
