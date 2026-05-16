package telemetry

import (
	"github.com/tsukumogami/tsuku/internal/installevents"
)

// Subscriber translates installevents into telemetry outcome events.
//
// Today's telemetry pipeline only defines outcome events for the update
// flow (UpdateOutcomeSuccess / Failure / Rollback). The subscriber maps
// install-bus events to those outcome types:
//
//   - Updated         -> UpdateOutcomeSuccess
//   - RolledBack      -> UpdateOutcomeRollback (explicit rollback by user)
//   - UpdateFailed    -> UpdateOutcomeFailure; AND UpdateOutcomeRollback when
//     ActiveAfter == FromVersion && FromVersion != ""
//     (automatic recovery succeeded)
//
// Other lifecycle events (Installed, Removed, InstallFailed, RemoveFailed,
// RollbackFailed) do not have parallel outcome events in today's
// telemetry schema; the subscriber leaves them untouched. CLI commands
// continue to emit their existing action events (NewInstallEvent,
// NewRemoveEvent) directly via Client.Send. A future telemetry expansion
// can introduce per-verb outcome events and extend this subscriber
// without changing the bus contract.
type Subscriber struct {
	client outcomeSender
}

// outcomeSender is the narrow surface the subscriber needs from a
// telemetry client. *Client implements this; tests can implement it
// with a recording mock without spinning up an HTTP server.
type outcomeSender interface {
	SendUpdateOutcome(event UpdateOutcomeEvent)
}

// NewSubscriber returns a Subscriber that emits via client. A nil
// client makes Handle a no-op (telemetry is opt-out and the client is
// constructed as nil when disabled).
func NewSubscriber(client *Client) *Subscriber {
	if client == nil {
		return &Subscriber{client: nil}
	}
	return &Subscriber{client: client}
}

// Handle reacts to one event. Mappings match today's emission pattern
// in apply.go and cmd/tsuku/update.go so the subscriber-driven flow is
// behavior-equivalent to the direct calls it replaces.
func (s *Subscriber) Handle(event installevents.Event) {
	if s == nil || s.client == nil {
		return
	}
	trigger := string(event.GetSource())

	switch e := event.(type) {
	case installevents.Updated:
		s.client.SendUpdateOutcome(NewUpdateOutcomeSuccessEvent(
			e.Tool, e.FromVersion, e.ToVersion, trigger))

	case installevents.RolledBack:
		// Explicit rollback by the user. The previous version is what
		// was active before the rollback (FromVersion); the version we
		// rolled to is ToVersion. Match the NewUpdateOutcomeRollbackEvent
		// argument naming: (recipe, versionPrevious, versionTarget).
		// "versionTarget" here is the version we rolled AWAY from
		// (matching how auto-rollback uses the field).
		s.client.SendUpdateOutcome(NewUpdateOutcomeRollbackEvent(
			e.Tool, e.ToVersion, e.FromVersion, trigger))

	case installevents.UpdateFailed:
		s.client.SendUpdateOutcome(NewUpdateOutcomeFailureEvent(
			e.Tool, e.AttemptedVersion, ClassifyError(e.Err), trigger))
		// Automatic recovery succeeded if state moved back to the prior
		// version. Emit the rollback outcome too, matching today's
		// apply.go behavior.
		if e.ActiveAfter == e.FromVersion && e.FromVersion != "" {
			s.client.SendUpdateOutcome(NewUpdateOutcomeRollbackEvent(
				e.Tool, e.FromVersion, e.AttemptedVersion, trigger))
		}

	case installevents.Installed,
		installevents.Removed,
		installevents.InstallFailed,
		installevents.RemoveFailed,
		installevents.RollbackFailed:
		// No corresponding outcome event in today's telemetry schema.
		// CLI commands emit their own action events (NewInstallEvent,
		// NewRemoveEvent) independently. Future opportunity: extend
		// the telemetry schema with per-verb outcome events and react
		// here.
	}
}
