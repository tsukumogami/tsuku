package updates

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/project"
	"github.com/tsukumogami/tsuku/internal/telemetry"
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

	MaybeAutoApply(cfg, userCfg, nil, installFn, nil)
	if called {
		t.Error("installFn should not be called when auto-apply is disabled")
	}
}

func TestMaybeAutoApplyNilUserConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}

	// Should not panic
	MaybeAutoApply(cfg, nil, nil, func(_, _, _ string) error { return nil }, nil)
}

func TestMaybeAutoApplyNoPendingEntries(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{HomeDir: dir}
	t.Setenv("TSUKU_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	userCfg := userconfig.DefaultConfig()

	called := false
	MaybeAutoApply(cfg, userCfg, nil, func(_, _, _ string) error {
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

	tc := telemetry.NewClientWithOptions("", 0, true, false) // disabled client for test coverage
	var installedTool, installedVersion string
	results := MaybeAutoApply(cfg, userCfg, nil, func(toolName, version, constraint string) error {
		installedTool = toolName
		installedVersion = version
		return nil
	}, tc)

	if installedTool != "test-tool" {
		t.Errorf("installed tool = %q, want %q", installedTool, "test-tool")
	}
	if installedVersion != "1.1.0" {
		t.Errorf("installed version = %q, want %q", installedVersion, "1.1.0")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("expected nil error, got %v", results[0].Err)
	}
	if results[0].NewVersion != "1.1.0" {
		t.Errorf("result version = %q, want %q", results[0].NewVersion, "1.1.0")
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

	tc := telemetry.NewClientWithOptions("", 0, true, false) // disabled client for test coverage
	MaybeAutoApply(cfg, userCfg, nil, func(_, _, _ string) error {
		return fmt.Errorf("download failed: network error")
	}, tc)

	// Notice should be written
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
	// First failure is suppressed (consecutive count = 1, below threshold of 3).
	// Notice is marked Shown=true to suppress display.
	if !allNotices[0].Shown {
		t.Error("first failure should be marked shown (suppressed, count < 3)")
	}
	if allNotices[0].ConsecutiveFailures != 1 {
		t.Errorf("consecutive failures = %d, want 1", allNotices[0].ConsecutiveFailures)
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
	MaybeAutoApply(cfg, userCfg, nil, func(_, _, _ string) error {
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
	MaybeAutoApply(cfg, userCfg, nil, func(_, _, _ string) error {
		called = true
		return nil
	}, nil)

	if called {
		t.Error("installFn should not be called when already at latest within pin")
	}
}

// setupProjectAutoApplyTest creates a temp directory with cache entry and state for project pin tests.
func setupProjectAutoApplyTest(t *testing.T, tool, activeVersion, requested, latestWithinPin, latestOverall, stateJSON string) (*config.Config, *userconfig.Config) {
	t.Helper()
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	entry := &UpdateCheckEntry{
		Tool: tool, ActiveVersion: activeVersion, Requested: requested,
		LatestWithinPin: latestWithinPin, LatestOverall: latestOverall,
		CheckedAt: time.Now(), ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := WriteEntry(cacheDir, entry); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(dir, "tools")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(stateJSON), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{HomeDir: dir, ToolsDir: stateDir}
	t.Setenv("TSUKU_AUTO_UPDATE", "")
	t.Setenv("CI", "")
	return cfg, userconfig.DefaultConfig()
}

func TestMaybeAutoApplyProjectExactPinSuppresses(t *testing.T) {
	cfg, userCfg := setupProjectAutoApplyTest(t, "node", "20.16.0", "20", "20.17.0", "22.0.0",
		`{"installed":{"node":{"active_version":"20.16.0","versions":{"20.16.0":{"requested":"20"}}}}}`)

	projCfg := &project.ConfigResult{
		Config: &project.ProjectConfig{
			Tools: map[string]project.ToolRequirement{"node": {Version: "20.16.0"}},
		},
	}

	installed := false
	MaybeAutoApply(cfg, userCfg, projCfg, func(_, _, _ string) error {
		installed = true
		return nil
	}, nil)

	if installed {
		t.Error("exact project pin should suppress auto-update, but install was called")
	}
}

func TestMaybeAutoApplyProjectPrefixPinNarrows(t *testing.T) {
	cfg, userCfg := setupProjectAutoApplyTest(t, "node", "20.16.0", "", "22.0.0", "22.0.0",
		`{"installed":{"node":{"active_version":"20.16.0","versions":{"20.16.0":{"requested":""}}}}}`)

	projCfg := &project.ConfigResult{
		Config: &project.ProjectConfig{
			Tools: map[string]project.ToolRequirement{"node": {Version: "20"}},
		},
	}

	installed := false
	MaybeAutoApply(cfg, userCfg, projCfg, func(_, _, _ string) error {
		installed = true
		return nil
	}, nil)

	if installed {
		t.Error("project pin '20' should block node 22.0.0 update, but install was called")
	}
}

func TestMaybeAutoApplyProjectPrefixPinAllows(t *testing.T) {
	cfg, userCfg := setupProjectAutoApplyTest(t, "node", "20.16.0", "20", "20.17.0", "22.0.0",
		`{"installed":{"node":{"active_version":"20.16.0","versions":{"20.16.0":{"requested":"20"}}}}}`)

	projCfg := &project.ConfigResult{
		Config: &project.ProjectConfig{
			Tools: map[string]project.ToolRequirement{"node": {Version: "20"}},
		},
	}

	var installedVersion string
	tc := telemetry.NewClientWithOptions("", 0, true, false)
	MaybeAutoApply(cfg, userCfg, projCfg, func(_, version, _ string) error {
		installedVersion = version
		return nil
	}, tc)

	if installedVersion != "20.17.0" {
		t.Errorf("project pin '20' should allow 20.17.0 update, got installed=%q", installedVersion)
	}
}

func TestMaybeAutoApplyProjectNilConfigUnchanged(t *testing.T) {
	cfg, userCfg := setupProjectAutoApplyTest(t, "ripgrep", "13.0.0", "", "14.0.0", "14.0.0",
		`{"installed":{"ripgrep":{"active_version":"13.0.0","versions":{"13.0.0":{"requested":""}}}}}`)

	var installedVersion string
	tc := telemetry.NewClientWithOptions("", 0, true, false)
	MaybeAutoApply(cfg, userCfg, nil, func(_, version, _ string) error {
		installedVersion = version
		return nil
	}, tc)

	if installedVersion != "14.0.0" {
		t.Errorf("nil project config should allow update, got installed=%q", installedVersion)
	}
}

func TestMaybeAutoApplyProjectUndeclaredToolUnchanged(t *testing.T) {
	cfg, userCfg := setupProjectAutoApplyTest(t, "ripgrep", "13.0.0", "", "14.0.0", "14.0.0",
		`{"installed":{"ripgrep":{"active_version":"13.0.0","versions":{"13.0.0":{"requested":""}}}}}`)

	projCfg := &project.ConfigResult{
		Config: &project.ProjectConfig{
			Tools: map[string]project.ToolRequirement{"python": {Version: "3.12"}},
		},
	}

	var installedVersion string
	tc := telemetry.NewClientWithOptions("", 0, true, false)
	MaybeAutoApply(cfg, userCfg, projCfg, func(_, version, _ string) error {
		installedVersion = version
		return nil
	}, tc)

	if installedVersion != "14.0.0" {
		t.Errorf("undeclared tool should use global pin, got installed=%q", installedVersion)
	}
}
