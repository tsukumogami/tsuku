package seed

import (
	"encoding/json"
	"time"
)

// SeedingSummary is the JSON structure written to stdout after a seeding run.
// The workflow parses this with jq.
type SeedingSummary struct {
	SourcesProcessed []string         `json:"sources_processed"`
	SourcesFailed    []string         `json:"sources_failed"`
	NewPackages      int              `json:"new_packages"`
	StaleRefreshed   int              `json:"stale_refreshed"`
	SourceChanges    []SourceChange   `json:"source_changes"`
	CuratedSkipped   int              `json:"curated_skipped"`
	CuratedInvalid   []CuratedInvalid `json:"curated_invalid"`
	Errors           []string         `json:"errors"`
}

// NewSeedingSummary returns a summary with all array fields initialized to
// empty slices so they serialize as [] rather than null.
func NewSeedingSummary() *SeedingSummary {
	return &SeedingSummary{
		SourcesProcessed: []string{},
		SourcesFailed:    []string{},
		SourceChanges:    []SourceChange{},
		CuratedInvalid:   []CuratedInvalid{},
		Errors:           []string{},
	}
}

// MarshalJSON produces the summary as a JSON byte slice.
func (s *SeedingSummary) MarshalJSON() ([]byte, error) {
	// Ensure slices are never null in JSON output.
	if s.SourcesProcessed == nil {
		s.SourcesProcessed = []string{}
	}
	if s.SourcesFailed == nil {
		s.SourcesFailed = []string{}
	}
	if s.SourceChanges == nil {
		s.SourceChanges = []SourceChange{}
	}
	if s.CuratedInvalid == nil {
		s.CuratedInvalid = []CuratedInvalid{}
	}
	if s.Errors == nil {
		s.Errors = []string{}
	}

	// Use an alias to avoid infinite recursion.
	type Alias SeedingSummary
	return json.Marshal((*Alias)(s))
}

// SeedingRunEntry is the JSONL record appended to seeding-runs.jsonl.
// It contains the full summary plus a run timestamp.
type SeedingRunEntry struct {
	RunAt            time.Time        `json:"run_at"`
	SourcesProcessed []string         `json:"sources_processed"`
	SourcesFailed    []string         `json:"sources_failed"`
	NewPackages      int              `json:"new_packages"`
	StaleRefreshed   int              `json:"stale_refreshed"`
	SourceChanges    []SourceChange   `json:"source_changes"`
	CuratedSkipped   int              `json:"curated_skipped"`
	CuratedInvalid   []CuratedInvalid `json:"curated_invalid"`
	Errors           []string         `json:"errors"`
}

// ToRunEntry converts a SeedingSummary to a SeedingRunEntry with the given
// timestamp.
func (s *SeedingSummary) ToRunEntry(runAt time.Time) SeedingRunEntry {
	sourcesProcessed := s.SourcesProcessed
	if sourcesProcessed == nil {
		sourcesProcessed = []string{}
	}
	sourcesFailed := s.SourcesFailed
	if sourcesFailed == nil {
		sourcesFailed = []string{}
	}
	sourceChanges := s.SourceChanges
	if sourceChanges == nil {
		sourceChanges = []SourceChange{}
	}
	curatedInvalid := s.CuratedInvalid
	if curatedInvalid == nil {
		curatedInvalid = []CuratedInvalid{}
	}
	errors := s.Errors
	if errors == nil {
		errors = []string{}
	}

	return SeedingRunEntry{
		RunAt:            runAt,
		SourcesProcessed: sourcesProcessed,
		SourcesFailed:    sourcesFailed,
		NewPackages:      s.NewPackages,
		StaleRefreshed:   s.StaleRefreshed,
		SourceChanges:    sourceChanges,
		CuratedSkipped:   s.CuratedSkipped,
		CuratedInvalid:   curatedInvalid,
		Errors:           errors,
	}
}
