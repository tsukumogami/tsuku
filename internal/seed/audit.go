package seed

import (
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
	"github.com/tsukumogami/tsuku/internal/builders"
)

// AuditEntry records a full disambiguation decision for a single package.
// It embeds the standard DisambiguationRecord and adds seeding-specific
// fields for probe details and freshness tracking.
type AuditEntry struct {
	batch.DisambiguationRecord
	ProbeResults    []builders.ProbeResult `json:"probe_results"`
	PreviousSource  *string                `json:"previous_source"`
	DisambiguatedAt time.Time              `json:"disambiguated_at"`
	SeedingRun      time.Time              `json:"seeding_run"`
}

// WriteAuditEntry writes an audit entry to a per-package JSON file.
// This is a stub; the full implementation is in #1725.
func WriteAuditEntry(_ string, _ AuditEntry) error {
	return nil
}
