package notices

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/installevents"
)

func TestSubscriber_Installed(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	s.Handle(installevents.Installed{
		Tool: "niwa", Version: "0.11.0",
		Source: installevents.SourceManual, Timestamp: time.Now(),
	})

	got, err := ReadNotice(dir, "niwa")
	if err != nil || got == nil {
		t.Fatalf("expected notice, got err=%v notice=%v", err, got)
	}
	if got.Verb != VerbInstall {
		t.Errorf("Verb = %q, want %q", got.Verb, VerbInstall)
	}
	if got.AttemptedVersion != "0.11.0" {
		t.Errorf("AttemptedVersion = %q, want 0.11.0", got.AttemptedVersion)
	}
	if got.Error != "" {
		t.Errorf("Error must be empty for success, got %q", got.Error)
	}
	if got.Shown {
		t.Error("Shown must be false")
	}
}

func TestSubscriber_Updated(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	s.Handle(installevents.Updated{
		Tool: "niwa", FromVersion: "0.11.0", ToVersion: "0.11.1",
		Source: installevents.SourceManual,
	})

	got, _ := ReadNotice(dir, "niwa")
	if got == nil {
		t.Fatal("expected notice")
	}
	if got.Verb != VerbUpdate {
		t.Errorf("Verb = %q, want %q", got.Verb, VerbUpdate)
	}
	if got.AttemptedVersion != "0.11.1" {
		t.Errorf("AttemptedVersion = %q, want 0.11.1", got.AttemptedVersion)
	}
}

func TestSubscriber_RolledBack(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	s.Handle(installevents.RolledBack{
		Tool: "niwa", FromVersion: "0.11.1", ToVersion: "0.11.0",
		Source: installevents.SourceManual,
	})

	got, _ := ReadNotice(dir, "niwa")
	if got == nil {
		t.Fatal("expected notice")
	}
	if got.Verb != VerbRollback {
		t.Errorf("Verb = %q, want %q", got.Verb, VerbRollback)
	}
	if got.AttemptedVersion != "0.11.0" {
		t.Errorf("AttemptedVersion = %q, want 0.11.0", got.AttemptedVersion)
	}
}

func TestSubscriber_Removed(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	// Pre-seed a notice so the Removed event has something to remove.
	if err := WriteNotice(dir, &Notice{Tool: "niwa", AttemptedVersion: "0.11.0"}); err != nil {
		t.Fatal(err)
	}

	s.Handle(installevents.Removed{
		Tool: "niwa", Version: "", ActiveAfter: "",
		Source: installevents.SourceManual,
	})

	got, _ := ReadNotice(dir, "niwa")
	if got != nil {
		t.Errorf("expected notice to be removed, got %+v", got)
	}
}

func TestSubscriber_InstallFailed(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	s.Handle(installevents.InstallFailed{
		Tool: "niwa", AttemptedVersion: "0.11.0",
		Err:    errors.New("boom"),
		Source: installevents.SourceManual,
	})

	got, _ := ReadNotice(dir, "niwa")
	if got == nil {
		t.Fatal("expected notice")
	}
	if got.Verb != VerbInstall {
		t.Errorf("Verb = %q, want %q", got.Verb, VerbInstall)
	}
	if got.Error != "boom" {
		t.Errorf("Error = %q, want boom", got.Error)
	}
	if got.ConsecutiveFailures != 1 {
		t.Errorf("ConsecutiveFailures = %d, want 1", got.ConsecutiveFailures)
	}
}

// Consecutive failure events should increment ConsecutiveFailures.
func TestSubscriber_FailedConsecutiveCount(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	for i := 0; i < 3; i++ {
		s.Handle(installevents.UpdateFailed{
			Tool: "niwa", AttemptedVersion: "0.11.1", FromVersion: "0.11.0",
			ActiveAfter: "0.11.0",
			Err:         errors.New("flaky"),
			Source:      installevents.SourceAuto,
		})
	}
	got, _ := ReadNotice(dir, "niwa")
	if got == nil {
		t.Fatal("expected notice")
	}
	if got.ConsecutiveFailures != 3 {
		t.Errorf("ConsecutiveFailures = %d, want 3", got.ConsecutiveFailures)
	}
}

// A success event must clear any prior failure (the file is overwritten,
// ConsecutiveFailures resets via the default zero value).
func TestSubscriber_SuccessClearsConsecutiveFailures(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	// First, three failures.
	for i := 0; i < 3; i++ {
		s.Handle(installevents.UpdateFailed{
			Tool: "niwa", AttemptedVersion: "0.11.1", FromVersion: "0.11.0",
			ActiveAfter: "0.11.0", Err: errors.New("flaky"),
			Source: installevents.SourceAuto,
		})
	}
	// Then a success.
	s.Handle(installevents.Updated{
		Tool: "niwa", FromVersion: "0.11.0", ToVersion: "0.11.1",
		Source: installevents.SourceAuto,
	})

	got, _ := ReadNotice(dir, "niwa")
	if got == nil {
		t.Fatal("expected notice")
	}
	if got.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d after success, want 0", got.ConsecutiveFailures)
	}
	if got.Error != "" {
		t.Errorf("Error = %q after success, want empty", got.Error)
	}
}

// Subscriber-locality: a Handle call must touch only the file for the
// event's Tool, never any other file in the directory.
func TestSubscriber_LocalityContract(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	// Pre-seed an unrelated notice.
	if err := WriteNotice(dir, &Notice{Tool: "other", AttemptedVersion: "1.0"}); err != nil {
		t.Fatal(err)
	}

	s.Handle(installevents.Installed{
		Tool: "niwa", Version: "0.11.0", Source: installevents.SourceManual,
	})

	// "other" notice must remain untouched.
	other, _ := ReadNotice(dir, "other")
	if other == nil {
		t.Fatal("unrelated notice was removed")
	}
	if other.AttemptedVersion != "1.0" {
		t.Errorf("unrelated notice was mutated; AttemptedVersion = %q", other.AttemptedVersion)
	}
}

// sanitizeError must replace newlines with " / " (one-liner safe for renderer).
func TestSanitizeError_NewlinesReplaced(t *testing.T) {
	in := errors.New("first line\nsecond line\rthird line\r\nfourth line")
	got := sanitizeError(in)
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("sanitizeError must replace all newlines/CRs; got %q", got)
	}
}

// sanitizeError must cap output at MaxErrorBytes with an ellipsis suffix.
func TestSanitizeError_Truncated(t *testing.T) {
	long := strings.Repeat("a", MaxErrorBytes*2)
	got := sanitizeError(errors.New(long))
	if len(got) > MaxErrorBytes {
		t.Errorf("sanitizeError output too long: %d bytes (cap %d)", len(got), MaxErrorBytes)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated output should end with ellipsis; got %q", got[len(got)-10:])
	}
}

// sanitizeError on nil returns "".
func TestSanitizeError_Nil(t *testing.T) {
	if got := sanitizeError(nil); got != "" {
		t.Errorf("sanitizeError(nil) = %q, want \"\"", got)
	}
}

// Notice errors persisted by the subscriber must never contain newlines.
// This is the property that prevents multi-line error rendering issues
// and protects against log-spam-as-DoS from upstream services with
// newline-rich error messages.
func TestSubscriber_PersistedErrorNeverContainsNewline(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	s.Handle(installevents.UpdateFailed{
		Tool: "niwa", AttemptedVersion: "0.11.1", FromVersion: "0.11.0",
		ActiveAfter: "0.11.0",
		Err:         errors.New("upstream error\nresponse body:\n<html>\n  <body>HTTP 500</body>\n</html>"),
		Source:      installevents.SourceAuto,
	})

	got, _ := ReadNotice(dir, "niwa")
	if got == nil {
		t.Fatal("expected notice")
	}
	if strings.ContainsAny(got.Error, "\n\r") {
		t.Errorf("persisted Error must not contain newlines; got %q", got.Error)
	}
}

// kindFor must never map a Source to a single-view Kind. Otherwise
// publishers would control whether notices persist or auto-delete,
// which is an integrity surface the design forbids.
func TestKindFor_NeverReturnsSingleViewKind(t *testing.T) {
	sources := []installevents.Source{
		installevents.SourceManual,
		installevents.SourceAuto,
		installevents.SourceProjectAuto,
	}
	for _, src := range sources {
		got := kindFor(src)
		if got == KindVersionFallback || got == KindShellInitChange {
			t.Errorf("kindFor(%q) = %q, must not be a single-view kind", src, got)
		}
	}
}

// Source mapping: SourceAuto produces KindAutoApplyResult so the
// existing renderer logic that distinguishes auto-apply notices still
// works. Other sources use the empty/default Kind.
func TestKindFor_AutoMapsToKindAutoApplyResult(t *testing.T) {
	if got := kindFor(installevents.SourceAuto); got != KindAutoApplyResult {
		t.Errorf("kindFor(SourceAuto) = %q, want %q", got, KindAutoApplyResult)
	}
	if got := kindFor(installevents.SourceManual); got != KindUpdateResult {
		t.Errorf("kindFor(SourceManual) = %q, want %q", got, KindUpdateResult)
	}
	if got := kindFor(installevents.SourceProjectAuto); got != KindUpdateResult {
		t.Errorf("kindFor(SourceProjectAuto) = %q, want %q", got, KindUpdateResult)
	}
}

// LibraryInstalled writes a notice under the lib-- prefix with Verb=install.
func TestSubscriber_LibraryInstalled(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	s.Handle(installevents.LibraryInstalled{
		Library: "libyaml", Version: "0.2.5",
		Source: installevents.SourceManual, Timestamp: time.Now(),
	})

	got, err := ReadNotice(dir, LibraryNoticePrefix+"libyaml")
	if err != nil || got == nil {
		t.Fatalf("expected notice at %qlibyaml, got err=%v notice=%v", LibraryNoticePrefix, err, got)
	}
	if got.Verb != VerbInstall {
		t.Errorf("Verb = %q, want %q", got.Verb, VerbInstall)
	}
	if got.AttemptedVersion != "0.2.5" {
		t.Errorf("AttemptedVersion = %q, want 0.2.5", got.AttemptedVersion)
	}
	if got.Shown {
		t.Error("Shown must be false")
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty on success", got.Error)
	}
}

// LibraryInstallFailed writes a failure notice with sanitized error and
// the consecutive-failures counter starting at 1.
func TestSubscriber_LibraryInstallFailed(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	s.Handle(installevents.LibraryInstallFailed{
		Library: "libyaml", AttemptedVersion: "0.2.5",
		Err:    errors.New("download failed"),
		Source: installevents.SourceManual,
	})

	got, _ := ReadNotice(dir, LibraryNoticePrefix+"libyaml")
	if got == nil {
		t.Fatal("expected notice")
	}
	if got.Error != "download failed" {
		t.Errorf("Error = %q, want %q", got.Error, "download failed")
	}
	if got.ConsecutiveFailures != 1 {
		t.Errorf("ConsecutiveFailures = %d, want 1", got.ConsecutiveFailures)
	}
}

// LibraryRemoved removes any prior notice for the library.
func TestSubscriber_LibraryRemoved(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	if err := WriteNotice(dir, &Notice{
		Tool: LibraryNoticePrefix + "libyaml", AttemptedVersion: "0.2.5",
	}); err != nil {
		t.Fatal(err)
	}

	s.Handle(installevents.LibraryRemoved{
		Library: "libyaml", Version: "0.2.5",
		Source: installevents.SourceManual,
	})

	got, _ := ReadNotice(dir, LibraryNoticePrefix+"libyaml")
	if got != nil {
		t.Errorf("expected notice to be removed, got %+v", got)
	}
}

// LibraryRemoveFailed writes a failure notice with Verb=remove.
func TestSubscriber_LibraryRemoveFailed(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	s.Handle(installevents.LibraryRemoveFailed{
		Library: "libyaml", AttemptedVersion: "0.2.5",
		Err:    errors.New("permission denied"),
		Source: installevents.SourceManual,
	})

	got, _ := ReadNotice(dir, LibraryNoticePrefix+"libyaml")
	if got == nil {
		t.Fatal("expected notice")
	}
	if got.Verb != VerbRemove {
		t.Errorf("Verb = %q, want %q", got.Verb, VerbRemove)
	}
}

// Library events must not touch tool notices that happen to share a name.
func TestSubscriber_LibraryEventsDoNotCollideWithToolNotices(t *testing.T) {
	dir := t.TempDir()
	s := NewSubscriber(dir)

	// Pre-seed a tool notice named "libyaml" (unusual but possible).
	if err := WriteNotice(dir, &Notice{
		Tool: "libyaml", AttemptedVersion: "tool-1.0", Verb: VerbInstall,
	}); err != nil {
		t.Fatal(err)
	}

	s.Handle(installevents.LibraryInstalled{
		Library: "libyaml", Version: "0.2.5",
		Source: installevents.SourceManual,
	})

	tool, _ := ReadNotice(dir, "libyaml")
	if tool == nil {
		t.Fatal("tool notice 'libyaml' was removed by library event")
	}
	if tool.AttemptedVersion != "tool-1.0" {
		t.Errorf("tool notice mutated: AttemptedVersion = %q, want tool-1.0", tool.AttemptedVersion)
	}

	lib, _ := ReadNotice(dir, LibraryNoticePrefix+"libyaml")
	if lib == nil {
		t.Fatal("library notice 'lib--libyaml' was not written")
	}
	if lib.AttemptedVersion != "0.2.5" {
		t.Errorf("library notice AttemptedVersion = %q, want 0.2.5", lib.AttemptedVersion)
	}
}
