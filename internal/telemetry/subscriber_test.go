package telemetry

import (
	"errors"
	"testing"

	"github.com/tsukumogami/tsuku/internal/installevents"
)

// recordingSender captures every UpdateOutcomeEvent passed to it.
type recordingSender struct {
	events []UpdateOutcomeEvent
}

func (r *recordingSender) SendUpdateOutcome(event UpdateOutcomeEvent) {
	r.events = append(r.events, event)
}

// subscriberFromSender constructs a Subscriber wired to a custom
// outcomeSender for testing. Production wiring uses NewSubscriber(client).
func subscriberFromSender(s outcomeSender) *Subscriber {
	return &Subscriber{client: s}
}

// nil client must be a no-op (telemetry-disabled path).
func TestSubscriber_NilClient(t *testing.T) {
	sub := NewSubscriber(nil)
	// Should not panic.
	sub.Handle(installevents.Updated{Tool: "x", Source: installevents.SourceManual})
}

// Updated -> UpdateOutcomeSuccess
func TestSubscriber_Updated(t *testing.T) {
	rec := &recordingSender{}
	sub := subscriberFromSender(rec)

	sub.Handle(installevents.Updated{
		Tool: "niwa", FromVersion: "0.11.0", ToVersion: "0.11.1",
		Source: installevents.SourceManual,
	})

	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rec.events))
	}
	e := rec.events[0]
	if e.Action != "update_outcome_success" {
		t.Errorf("Action = %q, want update_outcome_success", e.Action)
	}
	if e.Recipe != "niwa" {
		t.Errorf("Recipe = %q, want niwa", e.Recipe)
	}
	if e.VersionPrevious != "0.11.0" {
		t.Errorf("VersionPrevious = %q, want 0.11.0", e.VersionPrevious)
	}
	if e.VersionTarget != "0.11.1" {
		t.Errorf("VersionTarget = %q, want 0.11.1", e.VersionTarget)
	}
	if e.Trigger != "manual" {
		t.Errorf("Trigger = %q, want manual", e.Trigger)
	}
}

// RolledBack -> UpdateOutcomeRollback
func TestSubscriber_RolledBack(t *testing.T) {
	rec := &recordingSender{}
	sub := subscriberFromSender(rec)

	sub.Handle(installevents.RolledBack{
		Tool: "niwa", FromVersion: "0.11.1", ToVersion: "0.11.0",
		Source: installevents.SourceManual,
	})

	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rec.events))
	}
	e := rec.events[0]
	if e.Action != "update_outcome_rollback" {
		t.Errorf("Action = %q, want update_outcome_rollback", e.Action)
	}
	if e.Recipe != "niwa" {
		t.Errorf("Recipe = %q, want niwa", e.Recipe)
	}
	if e.Trigger != "manual" {
		t.Errorf("Trigger = %q, want manual", e.Trigger)
	}
}

// UpdateFailed with auto-recovery -> Failure + Rollback (matches today's apply.go).
func TestSubscriber_UpdateFailedWithRecovery(t *testing.T) {
	rec := &recordingSender{}
	sub := subscriberFromSender(rec)

	sub.Handle(installevents.UpdateFailed{
		Tool: "niwa", AttemptedVersion: "0.11.1",
		FromVersion: "0.11.0", ActiveAfter: "0.11.0", // recovery succeeded
		Err:    errors.New("download failed"),
		Source: installevents.SourceAuto,
	})

	if len(rec.events) != 2 {
		t.Fatalf("expected 2 events (failure + rollback), got %d", len(rec.events))
	}
	if rec.events[0].Action != "update_outcome_failure" {
		t.Errorf("first event Action = %q, want update_outcome_failure", rec.events[0].Action)
	}
	if rec.events[1].Action != "update_outcome_rollback" {
		t.Errorf("second event Action = %q, want update_outcome_rollback", rec.events[1].Action)
	}
	if rec.events[0].Trigger != "auto" {
		t.Errorf("Trigger = %q, want auto", rec.events[0].Trigger)
	}
	// Error classification: "download failed" should map to download error type.
	if rec.events[0].ErrorType != ErrorTypeDownloadFailed {
		t.Errorf("ErrorType = %q, want %q", rec.events[0].ErrorType, ErrorTypeDownloadFailed)
	}
}

// UpdateFailed without auto-recovery (rollback also failed or no prior
// version) -> Failure only.
func TestSubscriber_UpdateFailedNoRecovery(t *testing.T) {
	rec := &recordingSender{}
	sub := subscriberFromSender(rec)

	// Case 1: recovery also failed (ActiveAfter != FromVersion).
	sub.Handle(installevents.UpdateFailed{
		Tool: "niwa", AttemptedVersion: "0.11.1",
		FromVersion: "0.11.0", ActiveAfter: "0.11.1", // recovery did NOT happen
		Err:    errors.New("download failed"),
		Source: installevents.SourceAuto,
	})

	if len(rec.events) != 1 {
		t.Fatalf("recovery-failed: expected 1 event, got %d", len(rec.events))
	}
	if rec.events[0].Action != "update_outcome_failure" {
		t.Errorf("Action = %q, want update_outcome_failure", rec.events[0].Action)
	}

	// Case 2: no prior version existed (FromVersion == "" should not
	// happen for UpdateFailed since by definition update means a prior
	// version existed, but defensive coverage).
	rec.events = nil
	sub.Handle(installevents.UpdateFailed{
		Tool: "niwa", AttemptedVersion: "0.11.1",
		FromVersion: "", ActiveAfter: "",
		Err:    errors.New("download failed"),
		Source: installevents.SourceManual,
	})
	if len(rec.events) != 1 {
		t.Errorf("empty-FromVersion: expected 1 event, got %d", len(rec.events))
	}
}

// Source -> trigger string mapping.
func TestSubscriber_TriggerStringMapping(t *testing.T) {
	cases := []struct {
		source installevents.Source
		want   string
	}{
		{installevents.SourceManual, "manual"},
		{installevents.SourceAuto, "auto"},
		{installevents.SourceProjectAuto, "project-auto"},
	}
	for _, tc := range cases {
		t.Run(string(tc.source), func(t *testing.T) {
			rec := &recordingSender{}
			sub := subscriberFromSender(rec)
			sub.Handle(installevents.Updated{
				Tool: "t", FromVersion: "1", ToVersion: "2",
				Source: tc.source,
			})
			if len(rec.events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(rec.events))
			}
			if rec.events[0].Trigger != tc.want {
				t.Errorf("Trigger = %q, want %q", rec.events[0].Trigger, tc.want)
			}
		})
	}
}

// Events without an outcome mapping in today's schema should produce
// no telemetry emission from the subscriber.
func TestSubscriber_NoOutcomeMappingEventsAreIgnored(t *testing.T) {
	cases := []struct {
		name  string
		event installevents.Event
	}{
		{"Installed", installevents.Installed{Tool: "t", Version: "1", Source: installevents.SourceManual}},
		{"Removed", installevents.Removed{Tool: "t", Source: installevents.SourceManual}},
		{"InstallFailed", installevents.InstallFailed{Tool: "t", AttemptedVersion: "1", Err: errors.New("x"), Source: installevents.SourceManual}},
		{"RemoveFailed", installevents.RemoveFailed{Tool: "t", AttemptedVersion: "1", Err: errors.New("x"), Source: installevents.SourceManual}},
		{"RollbackFailed", installevents.RollbackFailed{Tool: "t", AttemptedVersion: "1", Err: errors.New("x"), Source: installevents.SourceManual}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingSender{}
			sub := subscriberFromSender(rec)
			sub.Handle(tc.event)
			if len(rec.events) != 0 {
				t.Errorf("%s should not emit telemetry, got %d events", tc.name, len(rec.events))
			}
		})
	}
}

// *Client must satisfy the outcomeSender interface at compile time so
// production wiring through NewSubscriber(c *Client) works.
func TestClientImplementsOutcomeSender(t *testing.T) {
	var _ outcomeSender = (*Client)(nil)
}
