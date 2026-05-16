package install

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/installevents"
	"github.com/tsukumogami/tsuku/internal/log"
)

// seedInstalledTool writes a tool directory containing a bin/<tool>
// stub so Activate's directory-exists check passes.
func seedInstalledTool(t *testing.T, cfg *config.Config, name, version string) {
	t.Helper()
	dir := cfg.ToolDir(name, version)
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, name), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

// recordingSub captures every event in the order received. Used to
// assert which events Manager publishes and in what order.
type recordingSub struct {
	events []installevents.Event
}

func (r *recordingSub) Handle(event installevents.Event) {
	r.events = append(r.events, event)
}

// stateReadingSub reads state.json inside Handle to verify the
// publish-after-state invariant: when the subscriber observes the
// event, state must already reflect the post-mutation value.
type stateReadingSub struct {
	mgr     *Manager
	tool    string
	results []string // ActiveVersion observed at event time, per event
}

func (s *stateReadingSub) Handle(_ installevents.Event) {
	ts, _ := s.mgr.GetState().GetToolState(s.tool)
	if ts == nil {
		s.results = append(s.results, "<not-in-state>")
		return
	}
	s.results = append(s.results, ts.ActiveVersion)
}

// newBusWithSubscriber constructs a bus wired with the given subscriber.
func newBusWithSubscriber(name string, sub installevents.Subscriber) *installevents.Bus {
	bus := installevents.NewBusForTest(log.NewNoop())
	bus.Subscribe(name, sub)
	return bus
}

// TestManager_Rollback_PublishesRolledBack verifies that the new
// Manager.Rollback method publishes RolledBack on success and
// RollbackFailed on failure, with FromVersion / ToVersion derived from
// state before the activate call.
func TestManager_Rollback_PublishesRolledBack(t *testing.T) {
	cfg, cleanup := newTestConfig(t)
	defer cleanup()

	// Pre-seed two installed versions of "rb-tool" with active 2.0.0.
	seedInstalledTool(t, cfg, "rb-tool", "1.0.0")
	seedInstalledTool(t, cfg, "rb-tool", "2.0.0")
	tmpMgr := New(cfg)
	if err := tmpMgr.GetState().UpdateTool("rb-tool", func(ts *ToolState) {
		ts.ActiveVersion = "2.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Binaries: []string{"bin/rb-tool"}, InstalledAt: time.Now().Add(-time.Hour)},
			"2.0.0": {Binaries: []string{"bin/rb-tool"}, InstalledAt: time.Now()},
		}
	}); err != nil {
		t.Fatal(err)
	}

	rec := &recordingSub{}
	bus := newBusWithSubscriber("rec", rec)
	mgr := New(cfg, WithEventBus(bus))

	if err := mgr.Rollback("rb-tool", "1.0.0", installevents.SourceManual); err != nil {
		t.Fatalf("Rollback() = %v, want nil", err)
	}

	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rec.events))
	}
	e, ok := rec.events[0].(installevents.RolledBack)
	if !ok {
		t.Fatalf("event[0] type = %T, want RolledBack", rec.events[0])
	}
	if e.Tool != "rb-tool" {
		t.Errorf("Tool = %q, want rb-tool", e.Tool)
	}
	if e.FromVersion != "2.0.0" {
		t.Errorf("FromVersion = %q, want 2.0.0", e.FromVersion)
	}
	if e.ToVersion != "1.0.0" {
		t.Errorf("ToVersion = %q, want 1.0.0", e.ToVersion)
	}
	if e.Source != installevents.SourceManual {
		t.Errorf("Source = %q, want manual", e.Source)
	}
}

// TestManager_Rollback_PublishesRollbackFailed verifies the failure
// branch of Manager.Rollback: rolling back to a non-installed version
// publishes RollbackFailed and returns the error.
func TestManager_Rollback_PublishesRollbackFailed(t *testing.T) {
	cfg, cleanup := newTestConfig(t)
	defer cleanup()

	tmpMgr := New(cfg)
	if err := tmpMgr.GetState().UpdateTool("rb-tool", func(ts *ToolState) {
		ts.ActiveVersion = "2.0.0"
		ts.Versions = map[string]VersionState{
			"2.0.0": {InstalledAt: time.Now()},
		}
	}); err != nil {
		t.Fatal(err)
	}

	rec := &recordingSub{}
	bus := newBusWithSubscriber("rec", rec)
	mgr := New(cfg, WithEventBus(bus))

	// 0.9.0 is not installed; Activate should fail.
	err := mgr.Rollback("rb-tool", "0.9.0", installevents.SourceManual)
	if err == nil {
		t.Fatal("Rollback() to non-installed version = nil, want error")
	}

	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rec.events))
	}
	if _, ok := rec.events[0].(installevents.RollbackFailed); !ok {
		t.Errorf("event[0] type = %T, want RollbackFailed", rec.events[0])
	}
}

// TestManager_RemoveAllVersions_PublishesRemoved verifies that
// RemoveAllVersions publishes a Removed event with the right fields.
func TestManager_RemoveAllVersions_PublishesRemoved(t *testing.T) {
	cfg, cleanup := newTestConfig(t)
	defer cleanup()

	tmpMgr := New(cfg)
	if err := tmpMgr.GetState().UpdateTool("gone-tool", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {InstalledAt: time.Now()},
		}
	}); err != nil {
		t.Fatal(err)
	}

	rec := &recordingSub{}
	bus := newBusWithSubscriber("rec", rec)
	mgr := New(cfg, WithEventBus(bus))

	if err := mgr.RemoveAllVersions("gone-tool", installevents.SourceManual); err != nil {
		t.Fatalf("RemoveAllVersions() = %v, want nil", err)
	}

	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rec.events))
	}
	e, ok := rec.events[0].(installevents.Removed)
	if !ok {
		t.Fatalf("event[0] type = %T, want Removed", rec.events[0])
	}
	if e.Tool != "gone-tool" {
		t.Errorf("Tool = %q, want gone-tool", e.Tool)
	}
	if e.Source != installevents.SourceManual {
		t.Errorf("Source = %q, want manual", e.Source)
	}
}

// TestManager_PublishAfterStateInvariant verifies that the publish
// fires AFTER the state write, so a subscriber reading state inside
// Handle sees the post-write value. This is the load-bearing
// invariant the design depends on.
func TestManager_PublishAfterStateInvariant_Rollback(t *testing.T) {
	cfg, cleanup := newTestConfig(t)
	defer cleanup()

	seedInstalledTool(t, cfg, "inv-tool", "1.0.0")
	seedInstalledTool(t, cfg, "inv-tool", "2.0.0")
	tmpMgr := New(cfg)
	if err := tmpMgr.GetState().UpdateTool("inv-tool", func(ts *ToolState) {
		ts.ActiveVersion = "2.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Binaries: []string{"bin/inv-tool"}, InstalledAt: time.Now().Add(-time.Hour)},
			"2.0.0": {Binaries: []string{"bin/inv-tool"}, InstalledAt: time.Now()},
		}
	}); err != nil {
		t.Fatal(err)
	}

	// Manager and subscriber refer to each other; construct subscriber first
	// with a placeholder Manager, then patch after constructing the bus.
	probe := &stateReadingSub{tool: "inv-tool"}
	bus := newBusWithSubscriber("probe", probe)
	mgr := New(cfg, WithEventBus(bus))
	probe.mgr = mgr

	if err := mgr.Rollback("inv-tool", "1.0.0", installevents.SourceManual); err != nil {
		t.Fatal(err)
	}

	if len(probe.results) != 1 {
		t.Fatalf("expected 1 state observation, got %d", len(probe.results))
	}
	if probe.results[0] != "1.0.0" {
		t.Errorf("subscriber observed ActiveVersion = %q at event time, want %q (publish-after-state invariant violated)", probe.results[0], "1.0.0")
	}
}

// TestManager_NilBusIsSafe verifies a Manager constructed without
// WithEventBus does not crash when its lifecycle methods would
// otherwise publish. Lets unit tests construct lightweight Managers.
func TestManager_NilBusIsSafe(t *testing.T) {
	cfg, cleanup := newTestConfig(t)
	defer cleanup()

	tmpMgr := New(cfg)
	if err := tmpMgr.GetState().UpdateTool("nil-bus-tool", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {InstalledAt: time.Now()},
		}
	}); err != nil {
		t.Fatal(err)
	}

	mgr := New(cfg) // no WithEventBus
	if err := mgr.RemoveAllVersions("nil-bus-tool", installevents.SourceManual); err != nil {
		t.Fatalf("RemoveAllVersions on nil-bus Manager = %v, want nil", err)
	}
}

// TestPublishInstallOutcome_DirectlyExercised verifies the
// publishInstallOutcome helper picks the right event type based on
// priorActiveVersion and err. The function is tested in isolation
// because the full InstallWithOptions path requires a real workDir
// that the install test harness creates only for specific scenarios.
func TestPublishInstallOutcome_PicksRightEventType(t *testing.T) {
	cfg, cleanup := newTestConfig(t)
	defer cleanup()

	cases := []struct {
		name               string
		priorActiveVersion string
		err                error
		wantType           string
	}{
		{"fresh install success", "", nil, "installevents.Installed"},
		{"update success", "1.0.0", nil, "installevents.Updated"},
		{"fresh install failure", "", errors.New("boom"), "installevents.InstallFailed"},
		{"update failure (recovery)", "1.0.0", errors.New("boom"), "installevents.UpdateFailed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingSub{}
			bus := newBusWithSubscriber("rec", rec)
			mgr := New(cfg, WithEventBus(bus))

			mgr.publishInstallOutcome("t", "2.0.0", tc.priorActiveVersion, installevents.SourceManual, tc.err)

			if len(rec.events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(rec.events))
			}
			gotType := typeName(rec.events[0])
			if gotType != tc.wantType {
				t.Errorf("event type = %s, want %s", gotType, tc.wantType)
			}

			// Verify ActiveAfter for UpdateFailed: must equal FromVersion
			// (atomic rename leaves state untouched on failure).
			if e, ok := rec.events[0].(installevents.UpdateFailed); ok {
				if e.ActiveAfter != tc.priorActiveVersion {
					t.Errorf("UpdateFailed.ActiveAfter = %q, want %q", e.ActiveAfter, tc.priorActiveVersion)
				}
				if e.FromVersion != tc.priorActiveVersion {
					t.Errorf("UpdateFailed.FromVersion = %q, want %q", e.FromVersion, tc.priorActiveVersion)
				}
			}
		})
	}
}

// typeName returns a stable string representation of an event's type.
func typeName(e installevents.Event) string {
	switch e.(type) {
	case installevents.Installed:
		return "installevents.Installed"
	case installevents.Updated:
		return "installevents.Updated"
	case installevents.RolledBack:
		return "installevents.RolledBack"
	case installevents.Removed:
		return "installevents.Removed"
	case installevents.InstallFailed:
		return "installevents.InstallFailed"
	case installevents.UpdateFailed:
		return "installevents.UpdateFailed"
	case installevents.RollbackFailed:
		return "installevents.RollbackFailed"
	case installevents.RemoveFailed:
		return "installevents.RemoveFailed"
	}
	return "unknown"
}

// newTestConfig is a local helper to build a minimal test config.
// install/testutil provides a richer NewTestConfig but takes a heavier
// dependency; for these focused tests, the lightweight version is enough.
// CurrentDir is set explicitly so Activate's symlink writes land in the
// per-test temp directory, never in the package working tree.
func newTestConfig(t *testing.T) (*config.Config, func()) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		HomeDir:    dir,
		ToolsDir:   dir + "/tools",
		CurrentDir: dir + "/bin",
	}
	if err := os.MkdirAll(cfg.CurrentDir, 0755); err != nil {
		t.Fatal(err)
	}
	return cfg, func() {}
}
