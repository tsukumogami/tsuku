// Package installevents defines the install lifecycle event vocabulary
// published by install.Manager and consumed by internal/notices and
// internal/telemetry. Events are typed per verb (Installed, Updated,
// RolledBack, Removed) and outcome (success or *Failed). The Source enum
// is an orthogonal trigger tag.
//
// See docs/designs/current/DESIGN-notices-install-event-bus.md for the full
// rationale and reaction tables.
package installevents

import (
	"context"
	"time"
)

// srcKey is the unexported context-key type used by WithSource and
// SourceFromContext. Declaring it as an unexported struct{} prevents
// collisions with keys defined in other packages: only this package can
// produce a value of this type, so context.WithValue lookups using this
// key are guaranteed not to clash with anything outside installevents.
type srcKey struct{}

// WithSource returns a new context carrying src so it can be retrieved
// downstream via SourceFromContext. The install pipeline reads Source
// from ctx at publish callsites rather than threading it as a positional
// parameter through every method.
//
// Context is normally reserved for request-scoped cancellation and
// deadlines, not for "configuration". Source is an exception: it is
// request-scoped metadata that travels with the request (a single CLI
// invocation or background trigger) and is consumed by code several
// layers below the entry point. Threading it as a parameter at every
// layer added churn without improving clarity; carrying it on ctx
// matches the way Source is logically scoped.
func WithSource(ctx context.Context, src Source) context.Context {
	return context.WithValue(ctx, srcKey{}, src)
}

// SourceFromContext returns the Source previously stored on ctx by
// WithSource. If ctx carries no Source (or is nil), it returns the
// empty Source value. Callers that publish events MUST set Source on
// the context before invoking the pipeline; the bus drops empty-Source
// events with a diagnostic log line so missing WithSource calls surface
// quickly.
func SourceFromContext(ctx context.Context) Source {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(srcKey{}).(Source); ok {
		return v
	}
	return ""
}

// Source identifies what triggered a lifecycle operation. Values are
// first-party identifiers chosen by tsuku code; they must remain
// non-PII and non-attacker-influenced strings. They surface in
// telemetry as the trigger tag and may eventually be rendered in
// user-facing output.
type Source string

const (
	// SourceManual: user-invoked CLI command (tsuku install/update/remove/rollback/self-update).
	SourceManual Source = "manual"
	// SourceAuto: background auto-apply (tsuku's update loop).
	SourceAuto Source = "auto"
	// SourceProjectAuto: .tsuku.toml auto-approval (autoinstall path).
	SourceProjectAuto Source = "project-auto"
)

// Event is the sealed marker interface implemented by every install
// lifecycle event type. The unexported method prevents foreign types
// from masquerading as events at the bus boundary, giving subscribers
// an exhaustive type switch.
type Event interface {
	isInstallEvent()
	// GetSource returns the event's Source for uniform subscriber tagging.
	GetSource() Source
}

// Installed reports a successful fresh install of a tool that had no
// prior ActiveVersion.
type Installed struct {
	Tool      string
	Version   string
	Source    Source
	Timestamp time.Time
}

func (Installed) isInstallEvent()     {}
func (e Installed) GetSource() Source { return e.Source }

// Updated reports a successful version transition for a tool that
// already had a prior ActiveVersion. Self-update emits Updated with
// Tool == "tsuku".
type Updated struct {
	Tool        string
	FromVersion string
	ToVersion   string
	Source      Source
	Timestamp   time.Time
}

func (Updated) isInstallEvent()     {}
func (e Updated) GetSource() Source { return e.Source }

// RolledBack reports a successful explicit rollback (tsuku rollback).
// Automatic rollback inside a failed update is not a RolledBack event;
// it is reflected in UpdateFailed.ActiveAfter instead.
type RolledBack struct {
	Tool        string
	FromVersion string // version active before the rollback
	ToVersion   string // version active after the rollback
	Source      Source
	Timestamp   time.Time
}

func (RolledBack) isInstallEvent()     {}
func (e RolledBack) GetSource() Source { return e.Source }

// Removed reports a successful removal. Version is the specific version
// removed; "" if all versions were removed. ActiveAfter is the new
// active version after removal; "" if the tool was fully removed.
type Removed struct {
	Tool        string
	Version     string
	ActiveAfter string
	Source      Source
	Timestamp   time.Time
}

func (Removed) isInstallEvent()     {}
func (e Removed) GetSource() Source { return e.Source }

// InstallFailed reports a failed fresh install (no prior ActiveVersion
// existed). Update failures use UpdateFailed instead so subscribers can
// distinguish "user tried to install a new tool" from "user tried to
// move an existing tool to a new version".
type InstallFailed struct {
	Tool             string
	AttemptedVersion string
	Err              error
	Source           Source
	Timestamp        time.Time
}

func (InstallFailed) isInstallEvent()     {}
func (e InstallFailed) GetSource() Source { return e.Source }

// UpdateFailed reports a failed update attempt. FromVersion is what
// was active before the attempt. ActiveAfter is what is active after
// the attempt AND any automatic recovery the manager performed:
//   - == FromVersion: rollback to prior version succeeded
//   - == "" if FromVersion == "": should not happen (use InstallFailed)
//   - == AttemptedVersion: rollback also failed; state is broken
//
// Telemetry derives "automatic rollback happened" from
// ActiveAfter == FromVersion && FromVersion != "".
type UpdateFailed struct {
	Tool             string
	AttemptedVersion string
	FromVersion      string
	ActiveAfter      string
	Err              error
	Source           Source
	Timestamp        time.Time
}

func (UpdateFailed) isInstallEvent()     {}
func (e UpdateFailed) GetSource() Source { return e.Source }

// RollbackFailed reports a failed explicit rollback. FromVersion is
// what was active before the attempt; AttemptedVersion is what we
// tried to roll to.
type RollbackFailed struct {
	Tool             string
	AttemptedVersion string
	FromVersion      string
	Err              error
	Source           Source
	Timestamp        time.Time
}

func (RollbackFailed) isInstallEvent()     {}
func (e RollbackFailed) GetSource() Source { return e.Source }

// RemoveFailed reports a failed remove attempt. AttemptedVersion is
// the version that was requested for removal; "" for a full-tool remove.
type RemoveFailed struct {
	Tool             string
	AttemptedVersion string
	Err              error
	Source           Source
	Timestamp        time.Time
}

func (RemoveFailed) isInstallEvent()     {}
func (e RemoveFailed) GetSource() Source { return e.Source }
