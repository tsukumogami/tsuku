package install

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/installevents"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/notices"
)

// TestE2E_UpdateFailedFlowsToNoticeAndTelemetry exercises the full
// chain Manager publish -> bus -> notices subscriber -> notice file
// AND -> telemetry subscriber -> outcome event. Models the design's
// acceptance scenario: a failed update should produce exactly one
// notice on disk (Verb: update, error string sanitized) AND exactly
// the right telemetry pair when ActiveAfter == FromVersion.
//
// This test exercises publishInstallOutcome directly because building
// a real InstallWithOptions failure requires recipe machinery beyond
// the install package's testable surface. The publish point is the
// same one InstallWithOptions's defer reaches; both subscribers see
// identical input.
func TestE2E_UpdateFailedFlowsToNoticeAndTelemetry(t *testing.T) {
	cfg, cleanup := newTestConfig(t)
	defer cleanup()

	noticesDir := notices.NoticesDir(cfg.HomeDir)
	noticesSub := notices.NewSubscriber(noticesDir)

	// Stub telemetry: capture every UpdateOutcomeEvent the subscriber
	// would send, without touching the network.
	telemetrySub := &capturingTelemetrySub{}

	bus := installevents.NewBusForTest(log.NewNoop())
	bus.Subscribe("notices", noticesSub)
	bus.Subscribe("telemetry", telemetrySub)

	mgr := New(cfg, WithEventBus(bus))

	// Drive the publish point with an UpdateFailed-shape outcome:
	// prior 0.11.0, attempted 0.11.1, failure with auto-recovery
	// (ActiveAfter would equal FromVersion under atomic-rename semantics).
	err := errors.New("download failed: HTTP 404")
	mgr.publishInstallOutcome("niwa", "0.11.1", "0.11.0", installevents.SourceAuto, err)

	// Notice store check.
	n, readErr := notices.ReadNotice(noticesDir, "niwa")
	if readErr != nil {
		t.Fatalf("ReadNotice error: %v", readErr)
	}
	if n == nil {
		t.Fatal("expected a notice on disk after UpdateFailed publish, got nil")
	}
	if n.Verb != notices.VerbUpdate {
		t.Errorf("notice Verb = %q, want %q", n.Verb, notices.VerbUpdate)
	}
	if n.AttemptedVersion != "0.11.1" {
		t.Errorf("notice AttemptedVersion = %q, want 0.11.1", n.AttemptedVersion)
	}
	if !strings.Contains(n.Error, "download failed: HTTP 404") {
		t.Errorf("notice Error should contain the source error; got %q", n.Error)
	}
	if n.ConsecutiveFailures != 1 {
		t.Errorf("notice ConsecutiveFailures = %d, want 1", n.ConsecutiveFailures)
	}
	if n.Shown {
		t.Error("notice should be Shown=false")
	}
	// Kind must be the auto-apply kind for SourceAuto (renderer keys
	// off this for the existing rendering path).
	if n.Kind != notices.KindAutoApplyResult {
		t.Errorf("notice Kind = %q, want %q", n.Kind, notices.KindAutoApplyResult)
	}

	// Telemetry check: failure + rollback (because ActiveAfter == FromVersion).
	if len(telemetrySub.received) != 2 {
		t.Fatalf("expected 2 telemetry events (failure + rollback), got %d", len(telemetrySub.received))
	}
	if telemetrySub.received[0].action != "update_outcome_failure" {
		t.Errorf("first telemetry event Action = %q, want update_outcome_failure", telemetrySub.received[0].action)
	}
	if telemetrySub.received[1].action != "update_outcome_rollback" {
		t.Errorf("second telemetry event Action = %q, want update_outcome_rollback", telemetrySub.received[1].action)
	}
	for _, e := range telemetrySub.received {
		if e.trigger != "auto" {
			t.Errorf("telemetry trigger = %q, want auto", e.trigger)
		}
	}
}

// TestE2E_InstalledFlowsToNoticeStore exercises a fresh-install
// success: Manager publishes Installed, the notices subscriber writes
// a Verb:install notice. Verifies the install path matches the
// expected Verb classification.
func TestE2E_InstalledFlowsToNoticeStore(t *testing.T) {
	cfg, cleanup := newTestConfig(t)
	defer cleanup()

	noticesDir := notices.NoticesDir(cfg.HomeDir)
	bus := installevents.NewBusForTest(log.NewNoop())
	bus.Subscribe("notices", notices.NewSubscriber(noticesDir))

	mgr := New(cfg, WithEventBus(bus))

	// No prior version -> Installed.
	mgr.publishInstallOutcome("gh", "2.47.0", "", installevents.SourceProjectAuto, nil)

	n, _ := notices.ReadNotice(noticesDir, "gh")
	if n == nil {
		t.Fatal("expected a notice on disk after Installed publish")
	}
	if n.Verb != notices.VerbInstall {
		t.Errorf("notice Verb = %q, want %q", n.Verb, notices.VerbInstall)
	}
	if n.AttemptedVersion != "2.47.0" {
		t.Errorf("notice AttemptedVersion = %q, want 2.47.0", n.AttemptedVersion)
	}
	if n.Error != "" {
		t.Errorf("notice Error = %q, want empty (success)", n.Error)
	}
}

// TestE2E_RemovedFlowsToNoticeStore verifies that a Removed event
// removes the notice file from disk via the subscriber.
func TestE2E_RemovedFlowsToNoticeStore(t *testing.T) {
	cfg, cleanup := newTestConfig(t)
	defer cleanup()

	noticesDir := notices.NoticesDir(cfg.HomeDir)

	// Pre-seed a notice for the tool.
	if err := notices.WriteNotice(noticesDir, &notices.Notice{
		Tool: "gh", AttemptedVersion: "2.47.0", Verb: notices.VerbUpdate,
	}); err != nil {
		t.Fatal(err)
	}

	bus := installevents.NewBusForTest(log.NewNoop())
	bus.Subscribe("notices", notices.NewSubscriber(noticesDir))

	mgr := New(cfg, WithEventBus(bus))

	if err := mgr.GetState().UpdateTool("gh", func(ts *ToolState) {
		ts.ActiveVersion = "2.47.0"
		ts.Versions = map[string]VersionState{
			"2.47.0": {Binaries: []string{"bin/gh"}},
		}
	}); err != nil {
		t.Fatal(err)
	}

	if err := mgr.RemoveAllVersions(installevents.WithSource(context.Background(), installevents.SourceManual), "gh"); err != nil {
		t.Fatalf("RemoveAllVersions: %v", err)
	}

	if n, _ := notices.ReadNotice(noticesDir, "gh"); n != nil {
		t.Errorf("notice should be removed after Removed event; got %+v", n)
	}
}

// capturingTelemetrySub implements installevents.Subscriber and captures
// the events the production telemetry subscriber would emit. Avoids
// pulling in internal/telemetry (which would create a test cycle) by
// duplicating only the subset of the mapping rules this test exercises.
type capturingTelemetrySub struct {
	received []capturedOutcome
}

type capturedOutcome struct {
	action  string
	tool    string
	trigger string
}

func (c *capturingTelemetrySub) Handle(event installevents.Event) {
	switch e := event.(type) {
	case installevents.Updated:
		c.received = append(c.received, capturedOutcome{
			action: "update_outcome_success", tool: e.Tool, trigger: string(e.Source),
		})
	case installevents.RolledBack:
		c.received = append(c.received, capturedOutcome{
			action: "update_outcome_rollback", tool: e.Tool, trigger: string(e.Source),
		})
	case installevents.UpdateFailed:
		c.received = append(c.received, capturedOutcome{
			action: "update_outcome_failure", tool: e.Tool, trigger: string(e.Source),
		})
		if e.ActiveAfter == e.FromVersion && e.FromVersion != "" {
			c.received = append(c.received, capturedOutcome{
				action: "update_outcome_rollback", tool: e.Tool, trigger: string(e.Source),
			})
		}
	case installevents.LibraryInstalled:
		c.received = append(c.received, capturedOutcome{
			action: "library_install_outcome_success", tool: e.Library, trigger: string(e.Source),
		})
	case installevents.LibraryInstallFailed:
		c.received = append(c.received, capturedOutcome{
			action: "library_install_outcome_failure", tool: e.Library, trigger: string(e.Source),
		})
	case installevents.LibraryRemoved:
		c.received = append(c.received, capturedOutcome{
			action: "library_remove_outcome_success", tool: e.Library, trigger: string(e.Source),
		})
	case installevents.LibraryRemoveFailed:
		c.received = append(c.received, capturedOutcome{
			action: "library_remove_outcome_failure", tool: e.Library, trigger: string(e.Source),
		})
	}
}

// TestE2E_LibraryInstalledFlowsToNoticeAndTelemetry exercises the full
// chain Manager.InstallLibrary -> bus -> notices subscriber -> notice
// file AND -> telemetry subscriber -> outcome event. Acceptance for
// Issue 4: a library install produces (a) a notice at lib--<library>.json
// with Verb: install, the right AttemptedVersion, and Shown: false, and
// (b) a library install outcome telemetry event with trigger: <source>.
func TestE2E_LibraryInstalledFlowsToNoticeAndTelemetry(t *testing.T) {
	cfg, cleanup := newTestConfig(t)
	defer cleanup()
	// InstallLibrary calls EnsureDirectories which MkdirAll's every
	// configured directory. Fill in the fields newTestConfig leaves
	// empty so EnsureDirectories doesn't choke on empty paths. Only
	// LibsDir is semantically required by InstallLibrary itself.
	cfg.LibsDir = cfg.HomeDir + "/libs"
	cfg.RecipesDir = cfg.HomeDir + "/recipes"
	cfg.RegistryDir = cfg.HomeDir + "/registry"
	cfg.AppsDir = cfg.HomeDir + "/apps"
	cfg.CacheDir = cfg.HomeDir + "/cache"
	cfg.VersionCacheDir = cfg.HomeDir + "/cache/versions"
	cfg.DownloadCacheDir = cfg.HomeDir + "/cache/downloads"
	cfg.CargoRegistryCacheDir = cfg.HomeDir + "/cache/cargo-registry"
	cfg.KeyCacheDir = cfg.HomeDir + "/cache/keys"
	cfg.TapCacheDir = cfg.HomeDir + "/cache/taps"

	// Build a fake workDir containing an .install directory; that's the
	// only filesystem layout InstallLibrary requires.
	workDir := t.TempDir()
	installDir := workDir + "/.install"
	if err := os.MkdirAll(installDir+"/lib", 0755); err != nil {
		t.Fatal(err)
	}
	// Place a small file so copyDir has something to do.
	if err := os.WriteFile(installDir+"/lib/libyaml.so.0.2.5", []byte("not a real library"), 0644); err != nil {
		t.Fatal(err)
	}

	noticesDir := notices.NoticesDir(cfg.HomeDir)
	telemetrySub := &capturingTelemetrySub{}

	bus := installevents.NewBusForTest(log.NewNoop())
	bus.Subscribe("notices", notices.NewSubscriber(noticesDir))
	bus.Subscribe("telemetry", telemetrySub)

	mgr := New(cfg, WithEventBus(bus))

	ctx := installevents.WithSource(context.Background(), installevents.SourceManual)
	if err := mgr.InstallLibrary(ctx, "libyaml", "0.2.5", workDir, LibraryInstallOptions{}); err != nil {
		t.Fatalf("InstallLibrary: %v", err)
	}

	// Notice store check at the lib-- prefix.
	n, readErr := notices.ReadNotice(noticesDir, notices.LibraryNoticePrefix+"libyaml")
	if readErr != nil {
		t.Fatalf("ReadNotice error: %v", readErr)
	}
	if n == nil {
		t.Fatal("expected library notice on disk after LibraryInstalled publish, got nil")
	}
	if n.Verb != notices.VerbInstall {
		t.Errorf("notice Verb = %q, want %q", n.Verb, notices.VerbInstall)
	}
	if n.AttemptedVersion != "0.2.5" {
		t.Errorf("notice AttemptedVersion = %q, want 0.2.5", n.AttemptedVersion)
	}
	// Foreground (SourceManual) library install is reported inline, so its
	// success notice is written Shown=true: it must not re-banner on the next
	// command, while `tsuku notices` (ReadAllNotices) still surfaces it once.
	if !n.Shown {
		t.Error("foreground library success should be written Shown=true")
	}

	// Telemetry check.
	if len(telemetrySub.received) != 1 {
		t.Fatalf("expected 1 library telemetry event, got %d (%+v)", len(telemetrySub.received), telemetrySub.received)
	}
	got := telemetrySub.received[0]
	if got.action != "library_install_outcome_success" {
		t.Errorf("telemetry action = %q, want library_install_outcome_success", got.action)
	}
	if got.tool != "libyaml" {
		t.Errorf("telemetry library = %q, want libyaml", got.tool)
	}
	if got.trigger != string(installevents.SourceManual) {
		t.Errorf("telemetry trigger = %q, want %q", got.trigger, installevents.SourceManual)
	}
}

// TestE2E_LibraryInstallFailedFlowsToNoticeAndTelemetry exercises the
// failure path: when copyDir or state update fails, the deferred
// publish closure fires LibraryInstallFailed instead of LibraryInstalled.
// Drives the publish point directly because constructing a real failure
// in InstallLibrary needs filesystem orchestration beyond this test's
// surface.
func TestE2E_LibraryInstallFailedFlowsToNoticeAndTelemetry(t *testing.T) {
	cfg, cleanup := newTestConfig(t)
	defer cleanup()

	noticesDir := notices.NoticesDir(cfg.HomeDir)
	telemetrySub := &capturingTelemetrySub{}

	bus := installevents.NewBusForTest(log.NewNoop())
	bus.Subscribe("notices", notices.NewSubscriber(noticesDir))
	bus.Subscribe("telemetry", telemetrySub)

	mgr := New(cfg, WithEventBus(bus))

	mgr.publishLibraryInstallOutcome("libyaml", "0.2.5", installevents.SourceAuto,
		errors.New("download failed: HTTP 503"))

	n, _ := notices.ReadNotice(noticesDir, notices.LibraryNoticePrefix+"libyaml")
	if n == nil {
		t.Fatal("expected library notice on disk after LibraryInstallFailed")
	}
	if n.Verb != notices.VerbInstall {
		t.Errorf("notice Verb = %q, want %q", n.Verb, notices.VerbInstall)
	}
	if !strings.Contains(n.Error, "download failed: HTTP 503") {
		t.Errorf("notice Error should contain source error; got %q", n.Error)
	}
	if n.ConsecutiveFailures != 1 {
		t.Errorf("notice ConsecutiveFailures = %d, want 1", n.ConsecutiveFailures)
	}

	if len(telemetrySub.received) != 1 {
		t.Fatalf("expected 1 library telemetry event, got %d", len(telemetrySub.received))
	}
	if telemetrySub.received[0].action != "library_install_outcome_failure" {
		t.Errorf("telemetry action = %q, want library_install_outcome_failure", telemetrySub.received[0].action)
	}
	if telemetrySub.received[0].trigger != string(installevents.SourceAuto) {
		t.Errorf("telemetry trigger = %q, want %q", telemetrySub.received[0].trigger, installevents.SourceAuto)
	}
}
