package batch

import (
	"encoding/json"
	"testing"
	"time"
)

func TestQueueEntry_JSONRoundTrip(t *testing.T) {
	disambiguated := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	entry := QueueEntry{
		Name:            "ripgrep",
		Source:          "cargo:ripgrep",
		Priority:        1,
		Status:          StatusPending,
		Confidence:      ConfidenceAuto,
		DisambiguatedAt: &disambiguated,
		FailureCount:    0,
		NextRetryAt:     nil,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded QueueEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Name != entry.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, entry.Name)
	}
	if decoded.Source != entry.Source {
		t.Errorf("Source = %q, want %q", decoded.Source, entry.Source)
	}
	if decoded.Priority != entry.Priority {
		t.Errorf("Priority = %d, want %d", decoded.Priority, entry.Priority)
	}
	if decoded.Status != entry.Status {
		t.Errorf("Status = %q, want %q", decoded.Status, entry.Status)
	}
	if decoded.Confidence != entry.Confidence {
		t.Errorf("Confidence = %q, want %q", decoded.Confidence, entry.Confidence)
	}
	if decoded.DisambiguatedAt == nil {
		t.Fatal("DisambiguatedAt should not be nil")
	}
	if !decoded.DisambiguatedAt.Equal(disambiguated) {
		t.Errorf("DisambiguatedAt = %v, want %v", decoded.DisambiguatedAt, disambiguated)
	}
	if decoded.FailureCount != entry.FailureCount {
		t.Errorf("FailureCount = %d, want %d", decoded.FailureCount, entry.FailureCount)
	}
	if decoded.NextRetryAt != nil {
		t.Errorf("NextRetryAt should be nil, got %v", decoded.NextRetryAt)
	}
}

func TestQueueEntry_JSONRoundTrip_WithNextRetryAt(t *testing.T) {
	disambiguated := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	nextRetry := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	entry := QueueEntry{
		Name:            "bat",
		Source:          "github:sharkdp/bat",
		Priority:        1,
		Status:          StatusFailed,
		Confidence:      ConfidenceCurated,
		DisambiguatedAt: &disambiguated,
		FailureCount:    2,
		NextRetryAt:     &nextRetry,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded QueueEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.NextRetryAt == nil {
		t.Fatal("NextRetryAt should not be nil")
	}
	if !decoded.NextRetryAt.Equal(nextRetry) {
		t.Errorf("NextRetryAt = %v, want %v", decoded.NextRetryAt, nextRetry)
	}
	if decoded.FailureCount != 2 {
		t.Errorf("FailureCount = %d, want 2", decoded.FailureCount)
	}
	if decoded.Confidence != ConfidenceCurated {
		t.Errorf("Confidence = %q, want %q", decoded.Confidence, ConfidenceCurated)
	}
}

func TestQueueEntry_JSONRoundTrip_NullTimeFields(t *testing.T) {
	// Verify that null/missing time fields deserialize correctly from raw JSON.
	raw := `{
		"name": "fd",
		"source": "cargo:fd-find",
		"priority": 2,
		"status": "pending",
		"confidence": "auto",
		"disambiguated_at": null,
		"failure_count": 0,
		"next_retry_at": null
	}`

	var entry QueueEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if entry.Name != "fd" {
		t.Errorf("Name = %q, want %q", entry.Name, "fd")
	}
	if entry.DisambiguatedAt != nil {
		t.Errorf("DisambiguatedAt should be nil, got %v", entry.DisambiguatedAt)
	}
	if entry.NextRetryAt != nil {
		t.Errorf("NextRetryAt should be nil, got %v", entry.NextRetryAt)
	}
}

func TestQueueEntry_Validate_Valid(t *testing.T) {
	tests := []struct {
		name  string
		entry QueueEntry
	}{
		{
			name: "pending auto entry",
			entry: QueueEntry{
				Name:       "ripgrep",
				Source:     "cargo:ripgrep",
				Priority:   1,
				Status:     StatusPending,
				Confidence: ConfidenceAuto,
			},
		},
		{
			name: "success curated entry",
			entry: QueueEntry{
				Name:       "bat",
				Source:     "github:sharkdp/bat",
				Priority:   1,
				Status:     StatusSuccess,
				Confidence: ConfidenceCurated,
			},
		},
		{
			name: "failed entry with retries",
			entry: QueueEntry{
				Name:         "fzf",
				Source:       "homebrew:fzf",
				Priority:     2,
				Status:       StatusFailed,
				Confidence:   ConfidenceAuto,
				FailureCount: 3,
			},
		},
		{
			name: "blocked entry",
			entry: QueueEntry{
				Name:       "imagemagick",
				Source:     "homebrew:imagemagick",
				Priority:   3,
				Status:     StatusBlocked,
				Confidence: ConfidenceAuto,
			},
		},
		{
			name: "requires_manual entry",
			entry: QueueEntry{
				Name:       "terraform",
				Source:     "homebrew:terraform",
				Priority:   2,
				Status:     StatusRequiresManual,
				Confidence: ConfidenceCurated,
			},
		},
		{
			name: "excluded entry",
			entry: QueueEntry{
				Name:       "node",
				Source:     "homebrew:node",
				Priority:   3,
				Status:     StatusExcluded,
				Confidence: ConfidenceAuto,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.entry.Validate(); err != nil {
				t.Errorf("Validate() returned unexpected error: %v", err)
			}
		})
	}
}

func TestQueueEntry_Validate_EmptyName(t *testing.T) {
	entry := QueueEntry{
		Name:       "",
		Source:     "cargo:ripgrep",
		Priority:   1,
		Status:     StatusPending,
		Confidence: ConfidenceAuto,
	}

	err := entry.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for empty name")
	}
	if got := err.Error(); !contains(got, "name must not be empty") {
		t.Errorf("error = %q, should mention name", got)
	}
}

func TestQueueEntry_Validate_EmptySource(t *testing.T) {
	entry := QueueEntry{
		Name:       "ripgrep",
		Source:     "",
		Priority:   1,
		Status:     StatusPending,
		Confidence: ConfidenceAuto,
	}

	err := entry.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for empty source")
	}
	if got := err.Error(); !contains(got, "source must not be empty") {
		t.Errorf("error = %q, should mention source", got)
	}
}

func TestQueueEntry_Validate_InvalidPriority(t *testing.T) {
	tests := []struct {
		name     string
		priority int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too high", 4},
		{"way too high", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := QueueEntry{
				Name:       "ripgrep",
				Source:     "cargo:ripgrep",
				Priority:   tt.priority,
				Status:     StatusPending,
				Confidence: ConfidenceAuto,
			}
			err := entry.Validate()
			if err == nil {
				t.Fatalf("Validate() should return error for priority %d", tt.priority)
			}
			if got := err.Error(); !contains(got, "priority must be 1, 2, or 3") {
				t.Errorf("error = %q, should mention priority", got)
			}
		})
	}
}

func TestQueueEntry_Validate_InvalidStatus(t *testing.T) {
	entry := QueueEntry{
		Name:       "ripgrep",
		Source:     "cargo:ripgrep",
		Priority:   1,
		Status:     "in_progress",
		Confidence: ConfidenceAuto,
	}

	err := entry.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for invalid status")
	}
	if got := err.Error(); !contains(got, "invalid status") {
		t.Errorf("error = %q, should mention invalid status", got)
	}
}

func TestQueueEntry_Validate_InvalidConfidence(t *testing.T) {
	entry := QueueEntry{
		Name:       "ripgrep",
		Source:     "cargo:ripgrep",
		Priority:   1,
		Status:     StatusPending,
		Confidence: "manual",
	}

	err := entry.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for invalid confidence")
	}
	if got := err.Error(); !contains(got, "invalid confidence") {
		t.Errorf("error = %q, should mention invalid confidence", got)
	}
}

func TestQueueEntry_Validate_NegativeFailureCount(t *testing.T) {
	entry := QueueEntry{
		Name:         "ripgrep",
		Source:       "cargo:ripgrep",
		Priority:     1,
		Status:       StatusPending,
		Confidence:   ConfidenceAuto,
		FailureCount: -1,
	}

	err := entry.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for negative failure_count")
	}
	if got := err.Error(); !contains(got, "failure_count must not be negative") {
		t.Errorf("error = %q, should mention failure_count", got)
	}
}

func TestQueueEntry_Validate_MultipleErrors(t *testing.T) {
	entry := QueueEntry{
		Name:         "",
		Source:       "",
		Priority:     0,
		Status:       "bogus",
		Confidence:   "unknown",
		FailureCount: -5,
	}

	err := entry.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for multiple violations")
	}

	got := err.Error()
	expected := []string{
		"name must not be empty",
		"source must not be empty",
		"priority must be 1, 2, or 3",
		"invalid status",
		"invalid confidence",
		"failure_count must not be negative",
	}
	for _, want := range expected {
		if !contains(got, want) {
			t.Errorf("error = %q, should contain %q", got, want)
		}
	}
}

func TestQueueEntry_Validate_WhitespaceOnlyName(t *testing.T) {
	entry := QueueEntry{
		Name:       "   ",
		Source:     "cargo:ripgrep",
		Priority:   1,
		Status:     StatusPending,
		Confidence: ConfidenceAuto,
	}

	err := entry.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for whitespace-only name")
	}
}

func TestQueueEntry_StatusConstants(t *testing.T) {
	// Verify all documented status values exist as constants.
	statuses := []string{
		StatusPending,
		StatusSuccess,
		StatusFailed,
		StatusBlocked,
		StatusRequiresManual,
		StatusExcluded,
	}

	for _, s := range statuses {
		if !validStatuses[s] {
			t.Errorf("status %q not in validStatuses map", s)
		}
	}

	if len(validStatuses) != len(statuses) {
		t.Errorf("validStatuses has %d entries, expected %d", len(validStatuses), len(statuses))
	}
}

func TestQueueEntry_ConfidenceConstants(t *testing.T) {
	// Verify all documented confidence values exist as constants.
	confidences := []string{
		ConfidenceAuto,
		ConfidenceCurated,
	}

	for _, c := range confidences {
		if !validConfidences[c] {
			t.Errorf("confidence %q not in validConfidences map", c)
		}
	}

	if len(validConfidences) != len(confidences) {
		t.Errorf("validConfidences has %d entries, expected %d", len(validConfidences), len(confidences))
	}
}

func TestQueueEntry_JSONFieldNames(t *testing.T) {
	// Verify JSON field names match the expected schema from the design doc.
	disambiguated := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	nextRetry := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	entry := QueueEntry{
		Name:            "ripgrep",
		Source:          "cargo:ripgrep",
		Priority:        1,
		Status:          "pending",
		Confidence:      "auto",
		DisambiguatedAt: &disambiguated,
		FailureCount:    2,
		NextRetryAt:     &nextRetry,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}

	expectedFields := []string{
		"name",
		"source",
		"priority",
		"status",
		"confidence",
		"disambiguated_at",
		"failure_count",
		"next_retry_at",
	}

	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("expected JSON field %q not found in marshaled output", field)
		}
	}

	if len(raw) != len(expectedFields) {
		t.Errorf("marshaled JSON has %d fields, expected %d", len(raw), len(expectedFields))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
