// Package telemetry provides anonymous usage telemetry for tsuku.
package telemetry

import (
	"runtime"

	"github.com/tsukumogami/tsuku/internal/buildinfo"
)

// Event represents a telemetry event sent to the backend.
type Event struct {
	Action            string `json:"action"`             // "install", "update", or "remove"
	Recipe            string `json:"recipe"`             // Recipe name (e.g., "nodejs")
	VersionConstraint string `json:"version_constraint"` // User's original constraint (e.g., "@LTS", ">=1.0", or empty)
	VersionResolved   string `json:"version_resolved"`   // Actual version installed/updated to
	VersionPrevious   string `json:"version_previous"`   // Previous version (for update/remove)
	OS                string `json:"os"`                 // Operating system ("linux", "darwin")
	Arch              string `json:"arch"`               // CPU architecture ("amd64", "arm64")
	TsukuVersion      string `json:"tsuku_version"`      // Version of tsuku CLI
	IsDependency      bool   `json:"is_dependency"`      // True if installed as a transitive dependency
	SchemaVersion     string `json:"schema_version"`     // Event schema version
}

const schemaVersion = "1"

// newBaseEvent creates an event with common fields pre-filled.
func newBaseEvent() Event {
	return Event{
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		TsukuVersion:  buildinfo.Version(),
		SchemaVersion: schemaVersion,
	}
}

// NewInstallEvent creates a telemetry event for an install action.
func NewInstallEvent(recipe, versionConstraint, versionResolved string, isDependency bool) Event {
	e := newBaseEvent()
	e.Action = "install"
	e.Recipe = recipe
	e.VersionConstraint = versionConstraint
	e.VersionResolved = versionResolved
	e.IsDependency = isDependency
	return e
}

// NewUpdateEvent creates a telemetry event for an update action.
func NewUpdateEvent(recipe, versionPrevious, versionResolved string) Event {
	e := newBaseEvent()
	e.Action = "update"
	e.Recipe = recipe
	e.VersionPrevious = versionPrevious
	e.VersionResolved = versionResolved
	return e
}

// NewRemoveEvent creates a telemetry event for a remove action.
func NewRemoveEvent(recipe, versionPrevious string) Event {
	e := newBaseEvent()
	e.Action = "remove"
	e.Recipe = recipe
	e.VersionPrevious = versionPrevious
	return e
}
