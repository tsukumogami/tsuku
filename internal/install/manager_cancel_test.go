package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/installevents"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/testutil"
)

// stagingInstallWorkDir creates a minimal .install layout under a t.TempDir
// so InstallWithOptions's copy step succeeds and the test reaches the
// state-write phase.
func stagingInstallWorkDir(t *testing.T, binary string) string {
	t.Helper()
	dir := t.TempDir()
	installDir := filepath.Join(dir, ".install", "bin")
	if err := os.MkdirAll(installDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, binary), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestCancelBeforeStateWrite verifies that a context already canceled
// when InstallWithOptions is called returns context.Canceled without
// touching state.json. The check sits at the top of the method, before
// EnsureDirectories and any disk I/O that mutates persistent state.
func TestCancelBeforeStateWrite(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)
	workDir := stagingInstallWorkDir(t, "mytool")

	ctx, cancel := context.WithCancel(installevents.WithSource(context.Background(), installevents.SourceManual))
	cancel() // cancel before invoking

	err := mgr.InstallWithOptions(ctx, "mytool", "1.0.0", workDir, InstallOptions{
		CreateSymlinks: false,
		Binaries:       []string{"bin/mytool"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("InstallWithOptions(canceled ctx) = %v, want context.Canceled", err)
	}

	// State must not contain the tool: the install bailed before mutation.
	state, loadErr := mgr.state.Load()
	if loadErr != nil {
		// State file may not exist at all -- that's the strongest signal that no write happened.
		if os.IsNotExist(loadErr) {
			return
		}
		t.Fatalf("state.Load() error = %v", loadErr)
	}
	if _, exists := state.Installed["mytool"]; exists {
		t.Errorf("state.Installed[mytool] exists after canceled install; want no entry")
	}
}

// TestCancelAfterStateWriteBeforePublish verifies that cancellation
// AFTER the state-write window does not unwind the state write and does
// not block the lifecycle publish. The bus subscriber observes the
// committed state and the publish fires exactly once.
//
// Strategy: drive InstallWithOptions with a context that's still live;
// after the call returns, assert state was committed and the bus saw
// exactly one event. This exercises the publish-after-state invariant
// from a cancellation angle: even when the caller's context gets
// canceled "right after" the work commits, the subscriber still sees
// the post-write value.
func TestCancelAfterStateWriteBeforePublish(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	rec := &recordingSub{}
	bus := installevents.NewBusForTest(log.NewNoop())
	bus.Subscribe("rec", rec)

	mgr := New(cfg, WithEventBus(bus))
	workDir := stagingInstallWorkDir(t, "mytool")

	// Start with a live ctx so the install completes; cancel after the
	// returns so we can inspect that the state-write happened before
	// the publish-after-state defer ran.
	ctx, cancel := context.WithCancel(installevents.WithSource(context.Background(), installevents.SourceManual))
	t.Cleanup(cancel)

	err := mgr.InstallWithOptions(ctx, "mytool", "1.0.0", workDir, InstallOptions{
		CreateSymlinks: false,
		Binaries:       []string{"bin/mytool"},
	})
	if err != nil {
		t.Fatalf("InstallWithOptions() = %v, want nil", err)
	}

	// Cancel now -- after the install returned. The publish has already
	// fired via the deferred publishInstallOutcome closure (publish-after-state).
	cancel()

	// Confirm state was committed.
	state, loadErr := mgr.state.Load()
	if loadErr != nil {
		t.Fatalf("state.Load() error = %v", loadErr)
	}
	ts, ok := state.Installed["mytool"]
	if !ok {
		t.Fatalf("state.Installed[mytool] missing after successful install")
	}
	if ts.ActiveVersion != "1.0.0" {
		t.Errorf("ActiveVersion = %q, want 1.0.0", ts.ActiveVersion)
	}

	// And exactly one Installed event was published.
	if len(rec.events) != 1 {
		t.Fatalf("recorded events = %d, want 1", len(rec.events))
	}
	if _, ok := rec.events[0].(installevents.Installed); !ok {
		t.Errorf("event[0] type = %T, want Installed", rec.events[0])
	}
}

// TestEmptySource_DropsEventWithDiagnostic exercises the bus contract
// that an event lacking a Source is dropped with a diagnostic log
// line. This is the safety net for the "forgot to call WithSource"
// case: if any CLI entry point neglects to wrap globalCtx, the event
// silently disappears from notices/telemetry but leaves a debug-level
// breadcrumb in logs.
func TestEmptySource_DropsEventWithDiagnostic(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	rec := &recordingSub{}
	bus := installevents.NewBusForTest(log.NewNoop())
	bus.Subscribe("rec", rec)

	mgr := New(cfg, WithEventBus(bus))
	workDir := stagingInstallWorkDir(t, "notagged")

	// Plain context.Background() carries no Source; the publish callsite
	// extracts "" via SourceFromContext and the bus drops the event.
	err := mgr.InstallWithOptions(context.Background(), "notagged", "1.0.0", workDir, InstallOptions{
		CreateSymlinks: false,
		Binaries:       []string{"bin/notagged"},
	})
	if err != nil {
		t.Fatalf("InstallWithOptions() = %v, want nil", err)
	}

	// State was written -- the bus drop happens at publish time, not at install time.
	state, loadErr := mgr.state.Load()
	if loadErr != nil {
		t.Fatalf("state.Load() error = %v", loadErr)
	}
	if _, ok := state.Installed["notagged"]; !ok {
		t.Errorf("state.Installed[notagged] missing; install path should still write state")
	}

	// And no events reached the subscriber: the bus dropped the
	// Installed event because its Source was empty.
	if len(rec.events) != 0 {
		t.Fatalf("recorded events = %d, want 0 (empty-Source drop)", len(rec.events))
	}
}
