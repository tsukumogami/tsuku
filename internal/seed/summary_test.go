package seed

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewSeedingSummary_DefaultArraysNonNil(t *testing.T) {
	s := NewSeedingSummary()

	if s.SourcesProcessed == nil {
		t.Error("SourcesProcessed should be non-nil")
	}
	if s.SourcesFailed == nil {
		t.Error("SourcesFailed should be non-nil")
	}
	if s.SourceChanges == nil {
		t.Error("SourceChanges should be non-nil")
	}
	if s.CuratedInvalid == nil {
		t.Error("CuratedInvalid should be non-nil")
	}
	if s.Errors == nil {
		t.Error("Errors should be non-nil")
	}
	if s.NewPackages != 0 {
		t.Errorf("NewPackages = %d, want 0", s.NewPackages)
	}
	if s.StaleRefreshed != 0 {
		t.Errorf("StaleRefreshed = %d, want 0", s.StaleRefreshed)
	}
	if s.CuratedSkipped != 0 {
		t.Errorf("CuratedSkipped = %d, want 0", s.CuratedSkipped)
	}
}

func TestSeedingSummary_JSONArraysNeverNull(t *testing.T) {
	// Even with zero-value struct, JSON should produce [] not null.
	s := NewSeedingSummary()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	arrayFields := []string{
		"sources_processed", "sources_failed", "source_changes",
		"curated_invalid", "errors",
	}
	for _, field := range arrayFields {
		val, ok := raw[field]
		if !ok {
			t.Errorf("field %q missing from JSON output", field)
			continue
		}
		arr, isArr := val.([]interface{})
		if !isArr {
			t.Errorf("field %q is not an array, got %T", field, val)
			continue
		}
		if len(arr) != 0 {
			t.Errorf("field %q should be empty array, got %v", field, arr)
		}
	}

	intFields := []string{"new_packages", "stale_refreshed", "curated_skipped"}
	for _, field := range intFields {
		val, ok := raw[field]
		if !ok {
			t.Errorf("field %q missing from JSON output", field)
			continue
		}
		num, isNum := val.(float64) // JSON numbers are float64
		if !isNum {
			t.Errorf("field %q is not a number, got %T", field, val)
			continue
		}
		if num != 0 {
			t.Errorf("field %q = %v, want 0", field, num)
		}
	}
}

func TestSeedingSummary_JSONAllFieldsPresent(t *testing.T) {
	s := &SeedingSummary{
		SourcesProcessed: []string{"homebrew", "cargo"},
		SourcesFailed:    []string{"npm"},
		NewPackages:      52,
		StaleRefreshed:   198,
		SourceChanges: []SourceChange{
			{Package: "tokei", Old: "homebrew:tokei", New: "cargo:tokei", Priority: 3, AutoAccepted: true},
		},
		CuratedSkipped: 277,
		CuratedInvalid: []CuratedInvalid{
			{Package: "example-tool", Source: "homebrew:example-tool", Error: "404 Not Found"},
		},
		Errors: []string{"npm: connection timeout"},
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Check all expected keys exist.
	expectedKeys := []string{
		"sources_processed", "sources_failed", "new_packages",
		"stale_refreshed", "source_changes", "curated_skipped",
		"curated_invalid", "errors",
	}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing key %q in JSON output", key)
		}
	}

	// Verify specific values.
	if raw["new_packages"].(float64) != 52 {
		t.Errorf("new_packages = %v, want 52", raw["new_packages"])
	}
	if raw["stale_refreshed"].(float64) != 198 {
		t.Errorf("stale_refreshed = %v, want 198", raw["stale_refreshed"])
	}
	if raw["curated_skipped"].(float64) != 277 {
		t.Errorf("curated_skipped = %v, want 277", raw["curated_skipped"])
	}
}

func TestSeedingSummary_MarshalNilSlicesBecomesEmptyArrays(t *testing.T) {
	// Explicitly set nil slices to verify MarshalJSON handles them.
	s := &SeedingSummary{
		SourcesProcessed: nil,
		SourcesFailed:    nil,
		SourceChanges:    nil,
		CuratedInvalid:   nil,
		Errors:           nil,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	arrayFields := []string{
		"sources_processed", "sources_failed", "source_changes",
		"curated_invalid", "errors",
	}
	for _, field := range arrayFields {
		val, ok := raw[field]
		if !ok {
			t.Errorf("field %q missing from JSON output", field)
			continue
		}
		if _, isArr := val.([]interface{}); !isArr {
			t.Errorf("field %q should be an array (not null), got %T", field, val)
		}
	}
}

func TestSeedingSummary_ToRunEntry(t *testing.T) {
	runAt := time.Date(2026, 2, 16, 6, 0, 0, 0, time.UTC)
	s := &SeedingSummary{
		SourcesProcessed: []string{"cargo"},
		SourcesFailed:    []string{},
		NewPackages:      10,
		StaleRefreshed:   5,
		SourceChanges:    []SourceChange{},
		CuratedSkipped:   3,
		CuratedInvalid:   []CuratedInvalid{},
		Errors:           []string{},
	}

	entry := s.ToRunEntry(runAt)

	if !entry.RunAt.Equal(runAt) {
		t.Errorf("RunAt = %v, want %v", entry.RunAt, runAt)
	}
	if entry.NewPackages != 10 {
		t.Errorf("NewPackages = %d, want 10", entry.NewPackages)
	}
	if entry.StaleRefreshed != 5 {
		t.Errorf("StaleRefreshed = %d, want 5", entry.StaleRefreshed)
	}
	if entry.CuratedSkipped != 3 {
		t.Errorf("CuratedSkipped = %d, want 3", entry.CuratedSkipped)
	}
	if len(entry.SourcesProcessed) != 1 || entry.SourcesProcessed[0] != "cargo" {
		t.Errorf("SourcesProcessed = %v, want [cargo]", entry.SourcesProcessed)
	}
}

func TestSeedingSummary_ToRunEntryNilSlices(t *testing.T) {
	runAt := time.Date(2026, 2, 16, 6, 0, 0, 0, time.UTC)
	s := &SeedingSummary{}

	entry := s.ToRunEntry(runAt)

	// All array fields should be non-nil even when source is nil.
	if entry.SourcesProcessed == nil {
		t.Error("SourcesProcessed should be non-nil")
	}
	if entry.SourcesFailed == nil {
		t.Error("SourcesFailed should be non-nil")
	}
	if entry.SourceChanges == nil {
		t.Error("SourceChanges should be non-nil")
	}
	if entry.CuratedInvalid == nil {
		t.Error("CuratedInvalid should be non-nil")
	}
	if entry.Errors == nil {
		t.Error("Errors should be non-nil")
	}
}

func TestSeedingRunEntry_JSONContainsRunAt(t *testing.T) {
	runAt := time.Date(2026, 2, 16, 6, 0, 0, 0, time.UTC)
	entry := SeedingRunEntry{
		RunAt:            runAt,
		SourcesProcessed: []string{"cargo"},
		SourcesFailed:    []string{},
		NewPackages:      5,
		StaleRefreshed:   0,
		SourceChanges:    []SourceChange{},
		CuratedSkipped:   0,
		CuratedInvalid:   []CuratedInvalid{},
		Errors:           []string{},
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	runAtStr, ok := raw["run_at"].(string)
	if !ok {
		t.Fatal("run_at field missing or not a string")
	}
	parsed, err := time.Parse(time.RFC3339, runAtStr)
	if err != nil {
		t.Fatalf("run_at is not valid RFC3339: %v", err)
	}
	if !parsed.Equal(runAt) {
		t.Errorf("run_at = %v, want %v", parsed, runAt)
	}
}
