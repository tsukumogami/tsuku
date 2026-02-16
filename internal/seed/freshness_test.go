package seed

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
)

func TestIsStale_NilDisambiguatedAt(t *testing.T) {
	entry := batch.QueueEntry{
		Name:            "tool",
		Source:          "homebrew:tool",
		Priority:        3,
		Status:          batch.StatusPending,
		Confidence:      batch.ConfidenceAuto,
		DisambiguatedAt: nil,
	}
	cfg := FreshnessConfig{ThresholdDays: 30, Now: time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)}

	if !IsStale(entry, cfg) {
		t.Error("entry with nil disambiguated_at should be stale")
	}
}

func TestIsStale_OlderThanThreshold(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -31) // 31 days ago
	entry := batch.QueueEntry{
		Name:            "tool",
		Source:          "homebrew:tool",
		Priority:        3,
		Status:          batch.StatusPending,
		Confidence:      batch.ConfidenceAuto,
		DisambiguatedAt: &old,
	}
	cfg := FreshnessConfig{ThresholdDays: 30, Now: now}

	if !IsStale(entry, cfg) {
		t.Error("entry older than threshold should be stale")
	}
}

func TestIsStale_FreshEntry(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	recent := now.AddDate(0, 0, -10) // 10 days ago
	entry := batch.QueueEntry{
		Name:            "tool",
		Source:          "homebrew:tool",
		Priority:        3,
		Status:          batch.StatusPending,
		Confidence:      batch.ConfidenceAuto,
		DisambiguatedAt: &recent,
	}
	cfg := FreshnessConfig{ThresholdDays: 30, Now: now}

	if IsStale(entry, cfg) {
		t.Error("entry within threshold should not be stale")
	}
}

func TestIsFailuresStale_HighFailuresAndStale(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -45) // 45 days ago
	entry := batch.QueueEntry{
		Name:            "broken-tool",
		Source:          "homebrew:broken-tool",
		Priority:        3,
		Status:          batch.StatusFailed,
		Confidence:      batch.ConfidenceAuto,
		DisambiguatedAt: &old,
		FailureCount:    3,
	}
	cfg := FreshnessConfig{ThresholdDays: 30, Now: now}

	if !IsFailuresStale(entry, cfg) {
		t.Error("entry with failure_count >= 3 AND stale should trigger")
	}
}

func TestIsFailuresStale_HighFailuresButFresh(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	recent := now.AddDate(0, 0, -5) // 5 days ago
	entry := batch.QueueEntry{
		Name:            "recently-failing",
		Source:          "homebrew:recently-failing",
		Priority:        3,
		Status:          batch.StatusFailed,
		Confidence:      batch.ConfidenceAuto,
		DisambiguatedAt: &recent,
		FailureCount:    5,
	}
	cfg := FreshnessConfig{ThresholdDays: 30, Now: now}

	if IsFailuresStale(entry, cfg) {
		t.Error("entry with failure_count >= 3 but fresh disambiguated_at should NOT trigger")
	}
}

func TestIsNewAuditCandidate_NewEcosystem(t *testing.T) {
	entry := batch.QueueEntry{
		Name:       "ripgrep",
		Source:     "homebrew:ripgrep",
		Priority:   1,
		Status:     batch.StatusPending,
		Confidence: batch.ConfidenceAuto,
	}

	// Audit entry only has homebrew probe results.
	auditEntry := &AuditEntry{
		ProbeResults: []AuditProbeResult{
			{Source: "homebrew:ripgrep", Downloads: 89000},
		},
	}

	// A cargo source was discovered that's not in the audit.
	if !IsNewAuditCandidate(entry, auditEntry, "cargo:ripgrep") {
		t.Error("new ecosystem not in audit should be flagged as new candidate")
	}
}

func TestIsNewAuditCandidate_ExistingEcosystem(t *testing.T) {
	entry := batch.QueueEntry{
		Name:       "ripgrep",
		Source:     "homebrew:ripgrep",
		Priority:   1,
		Status:     batch.StatusPending,
		Confidence: batch.ConfidenceAuto,
	}

	auditEntry := &AuditEntry{
		ProbeResults: []AuditProbeResult{
			{Source: "homebrew:ripgrep", Downloads: 89000},
			{Source: "cargo:ripgrep", Downloads: 1250000},
		},
	}

	if IsNewAuditCandidate(entry, auditEntry, "cargo:ripgrep") {
		t.Error("ecosystem already in audit should not be flagged")
	}
}

func TestIsNewAuditCandidate_NilAudit(t *testing.T) {
	entry := batch.QueueEntry{
		Name:       "new-tool",
		Source:     "homebrew:new-tool",
		Priority:   3,
		Status:     batch.StatusPending,
		Confidence: batch.ConfidenceAuto,
	}

	// No audit entry exists yet -- any source is new.
	if !IsNewAuditCandidate(entry, nil, "cargo:new-tool") {
		t.Error("nil audit entry should make any source a new candidate")
	}
}

func TestShouldSkip_SuccessEntry(t *testing.T) {
	entry := batch.QueueEntry{
		Name:       "done-tool",
		Source:     "cargo:done-tool",
		Priority:   3,
		Status:     batch.StatusSuccess,
		Confidence: batch.ConfidenceAuto,
	}

	if !ShouldSkip(entry) {
		t.Error("entries with status success should be skipped")
	}
}

func TestShouldSkip_PendingEntry(t *testing.T) {
	entry := batch.QueueEntry{
		Name:       "tool",
		Source:     "cargo:tool",
		Priority:   3,
		Status:     batch.StatusPending,
		Confidence: batch.ConfidenceAuto,
	}

	if ShouldSkip(entry) {
		t.Error("pending entries should not be skipped")
	}
}

func TestIsCurated(t *testing.T) {
	curated := batch.QueueEntry{
		Name:       "jq",
		Source:     "github:jqlang/jq",
		Priority:   1,
		Status:     batch.StatusPending,
		Confidence: batch.ConfidenceCurated,
	}
	auto := batch.QueueEntry{
		Name:       "tool",
		Source:     "cargo:tool",
		Priority:   3,
		Status:     batch.StatusPending,
		Confidence: batch.ConfidenceAuto,
	}

	if !IsCurated(curated) {
		t.Error("curated entry should be identified as curated")
	}
	if IsCurated(auto) {
		t.Error("auto entry should not be identified as curated")
	}
}

func TestNeedsRedisambiguation_CuratedSkipped(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	entry := batch.QueueEntry{
		Name:            "jq",
		Source:          "github:jqlang/jq",
		Priority:        1,
		Status:          batch.StatusPending,
		Confidence:      batch.ConfidenceCurated,
		DisambiguatedAt: nil, // Would be stale, but curated skips
	}
	cfg := FreshnessConfig{ThresholdDays: 30, Now: now}

	if NeedsRedisambiguation(entry, cfg, nil, "") {
		t.Error("curated entries should be skipped from re-disambiguation")
	}
}

func TestNeedsRedisambiguation_SuccessSkipped(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	entry := batch.QueueEntry{
		Name:            "done",
		Source:          "cargo:done",
		Priority:        3,
		Status:          batch.StatusSuccess,
		Confidence:      batch.ConfidenceAuto,
		DisambiguatedAt: nil, // Would be stale, but success skips
	}
	cfg := FreshnessConfig{ThresholdDays: 30, Now: now}

	if NeedsRedisambiguation(entry, cfg, nil, "") {
		t.Error("success entries should be skipped from re-disambiguation")
	}
}

func TestApplySourceChange_Priority1KeepsOldSource(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	entry := batch.QueueEntry{
		Name:         "ripgrep",
		Source:       "homebrew:ripgrep",
		Priority:     1,
		Status:       batch.StatusPending,
		Confidence:   batch.ConfidenceAuto,
		FailureCount: 2,
	}

	change, modified := ApplySourceChange(&entry, "cargo:ripgrep", now)

	if modified {
		t.Error("priority 1 source change should NOT modify the entry")
	}
	if entry.Source != "homebrew:ripgrep" {
		t.Errorf("Source should remain homebrew:ripgrep, got %q", entry.Source)
	}
	if change.AutoAccepted {
		t.Error("priority 1 changes should not be auto-accepted")
	}
	if change.Old != "homebrew:ripgrep" {
		t.Errorf("Old = %q, want homebrew:ripgrep", change.Old)
	}
	if change.New != "cargo:ripgrep" {
		t.Errorf("New = %q, want cargo:ripgrep", change.New)
	}
	if change.Priority != 1 {
		t.Errorf("Priority = %d, want 1", change.Priority)
	}
}

func TestApplySourceChange_Priority2KeepsOldSource(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	entry := batch.QueueEntry{
		Name:         "bat",
		Source:       "homebrew:bat",
		Priority:     2,
		Status:       batch.StatusPending,
		Confidence:   batch.ConfidenceAuto,
		FailureCount: 1,
	}

	change, modified := ApplySourceChange(&entry, "cargo:bat", now)

	if modified {
		t.Error("priority 2 source change should NOT modify the entry")
	}
	if entry.Source != "homebrew:bat" {
		t.Errorf("Source should remain homebrew:bat, got %q", entry.Source)
	}
	if change.AutoAccepted {
		t.Error("priority 2 changes should not be auto-accepted")
	}
}

func TestApplySourceChange_Priority3UpdatesSource(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	retryAt := now.Add(24 * time.Hour)
	entry := batch.QueueEntry{
		Name:         "tokei",
		Source:       "homebrew:tokei",
		Priority:     3,
		Status:       batch.StatusFailed,
		Confidence:   batch.ConfidenceAuto,
		FailureCount: 4,
		NextRetryAt:  &retryAt,
	}

	change, modified := ApplySourceChange(&entry, "cargo:tokei", now)

	if !modified {
		t.Error("priority 3 source change should modify the entry")
	}
	if entry.Source != "cargo:tokei" {
		t.Errorf("Source = %q, want cargo:tokei", entry.Source)
	}
	if entry.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0 (reset on source change)", entry.FailureCount)
	}
	if entry.NextRetryAt != nil {
		t.Errorf("NextRetryAt = %v, want nil (cleared on source change)", entry.NextRetryAt)
	}
	if !change.AutoAccepted {
		t.Error("priority 3 changes should be auto-accepted")
	}
	if change.Old != "homebrew:tokei" {
		t.Errorf("Old = %q, want homebrew:tokei", change.Old)
	}
	if change.New != "cargo:tokei" {
		t.Errorf("New = %q, want cargo:tokei", change.New)
	}
}

func TestApplySelectionResult_PriorityFallback(t *testing.T) {
	entry := batch.QueueEntry{
		Name:       "ambiguous-tool",
		Source:     "npm:ambiguous-tool",
		Priority:   3,
		Status:     batch.StatusPending,
		Confidence: batch.ConfidenceAuto,
	}

	ApplySelectionResult(&entry, batch.SelectionPriorityFallback)

	if entry.Status != batch.StatusRequiresManual {
		t.Errorf("Status = %q, want requires_manual for priority_fallback", entry.Status)
	}
}

func TestApplySelectionResult_10xPopularityGap(t *testing.T) {
	entry := batch.QueueEntry{
		Name:       "clear-winner",
		Source:     "cargo:clear-winner",
		Priority:   3,
		Status:     batch.StatusRequiresManual, // was requires_manual before
		Confidence: batch.ConfidenceAuto,
	}

	ApplySelectionResult(&entry, batch.Selection10xPopularityGap)

	if entry.Status != batch.StatusPending {
		t.Errorf("Status = %q, want pending for 10x_popularity_gap", entry.Status)
	}
}

func TestApplySelectionResult_SingleMatch(t *testing.T) {
	entry := batch.QueueEntry{
		Name:       "unique-tool",
		Source:     "pypi:unique-tool",
		Priority:   3,
		Status:     batch.StatusRequiresManual,
		Confidence: batch.ConfidenceAuto,
	}

	ApplySelectionResult(&entry, batch.SelectionSingleMatch)

	if entry.Status != batch.StatusPending {
		t.Errorf("Status = %q, want pending for single_match", entry.Status)
	}
}

func TestUpdateDisambiguatedAt(t *testing.T) {
	now := time.Date(2026, 2, 16, 6, 0, 0, 0, time.UTC)
	entry := batch.QueueEntry{
		Name:            "tool",
		Source:          "cargo:tool",
		Priority:        3,
		Status:          batch.StatusPending,
		Confidence:      batch.ConfidenceAuto,
		DisambiguatedAt: nil,
	}

	UpdateDisambiguatedAt(&entry, now)

	if entry.DisambiguatedAt == nil {
		t.Fatal("DisambiguatedAt should be set")
	}
	if !entry.DisambiguatedAt.Equal(now) {
		t.Errorf("DisambiguatedAt = %v, want %v", entry.DisambiguatedAt, now)
	}
}

func TestCuratedSourceValidator_HTTP404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	// Test the underlying validateURL directly since the Validate method
	// resolves real ecosystem URLs that we can't control in tests.
	_ = &CuratedSourceValidator{Client: ts.Client()}
	err := validateURL(ts.Client(), ts.URL)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if err.Error() != "404 Not Found" {
		t.Errorf("error = %q, want '404 Not Found'", err.Error())
	}
}

func TestCuratedSourceValidator_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	err := validateURL(ts.Client(), ts.URL)
	if err != nil {
		t.Errorf("unexpected error for 200 response: %v", err)
	}
}

func TestCuratedSourceValidator_ConnectionError(t *testing.T) {
	// Use a client that talks to nothing.
	err := validateURL(http.DefaultClient, "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error for connection failure")
	}
}

func TestEcosystemAPIURL(t *testing.T) {
	tests := []struct {
		source string
		want   string
		err    bool
	}{
		{"homebrew:ripgrep", "https://formulae.brew.sh/api/formula/ripgrep.json", false},
		{"cargo:tokei", "https://crates.io/api/v1/crates/tokei", false},
		{"npm:serve", "https://registry.npmjs.org/serve", false},
		{"pypi:httpie", "https://pypi.org/pypi/httpie/json", false},
		{"rubygems:fpm", "https://rubygems.org/api/v1/gems/fpm.json", false},
		{"github:BurntSushi/ripgrep", "https://api.github.com/repos/BurntSushi/ripgrep", false},
		{"unknown:tool", "", true},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		got, err := ecosystemAPIURL(tt.source)
		if tt.err && err == nil {
			t.Errorf("ecosystemAPIURL(%q): expected error", tt.source)
			continue
		}
		if !tt.err && err != nil {
			t.Errorf("ecosystemAPIURL(%q): unexpected error: %v", tt.source, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ecosystemAPIURL(%q) = %q, want %q", tt.source, got, tt.want)
		}
	}
}

// Integration-style tests combining multiple freshness checks.

func TestFreshnessFlow_StaleEntryFlagged(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -45)
	cfg := FreshnessConfig{ThresholdDays: 30, Now: now}

	entry := batch.QueueEntry{
		Name:            "stale-tool",
		Source:          "homebrew:stale-tool",
		Priority:        3,
		Status:          batch.StatusPending,
		Confidence:      batch.ConfidenceAuto,
		DisambiguatedAt: &old,
	}

	if !NeedsRedisambiguation(entry, cfg, nil, "") {
		t.Error("stale entry should need re-disambiguation")
	}
}

func TestFreshnessFlow_FreshEntryNotFlagged(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	recent := now.AddDate(0, 0, -5)
	cfg := FreshnessConfig{ThresholdDays: 30, Now: now}

	entry := batch.QueueEntry{
		Name:            "fresh-tool",
		Source:          "homebrew:fresh-tool",
		Priority:        3,
		Status:          batch.StatusPending,
		Confidence:      batch.ConfidenceAuto,
		DisambiguatedAt: &recent,
	}

	if NeedsRedisambiguation(entry, cfg, nil, "") {
		t.Error("fresh entry should not need re-disambiguation")
	}
}

func TestFreshnessFlow_FailuresAndStaleTriggered(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -31)
	cfg := FreshnessConfig{ThresholdDays: 30, Now: now}

	entry := batch.QueueEntry{
		Name:            "failing-tool",
		Source:          "homebrew:failing-tool",
		Priority:        3,
		Status:          batch.StatusFailed,
		Confidence:      batch.ConfidenceAuto,
		DisambiguatedAt: &old,
		FailureCount:    5,
	}

	if !NeedsRedisambiguation(entry, cfg, nil, "") {
		t.Error("entry with failures+stale should need re-disambiguation")
	}
}

func TestFreshnessFlow_NewAuditCandidateTriggered(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	recent := now.AddDate(0, 0, -5) // Fresh, so only trigger 3 applies
	cfg := FreshnessConfig{ThresholdDays: 30, Now: now}

	entry := batch.QueueEntry{
		Name:            "ripgrep",
		Source:          "homebrew:ripgrep",
		Priority:        1,
		Status:          batch.StatusPending,
		Confidence:      batch.ConfidenceAuto,
		DisambiguatedAt: &recent,
	}

	auditEntry := &AuditEntry{
		ProbeResults: []AuditProbeResult{
			{Source: "homebrew:ripgrep", Downloads: 89000},
		},
	}

	// New cargo source not in audit.
	if !NeedsRedisambiguation(entry, cfg, auditEntry, "cargo:ripgrep") {
		t.Error("entry with new audit candidate should need re-disambiguation")
	}
}

func TestCuratedSourceValidator_ValidateWithTestServer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/valid", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	tests := []struct {
		path    string
		wantErr string
	}{
		{"/valid", ""},
		{"/missing", "404 Not Found"},
		{"/error", "HTTP 500"},
	}

	for _, tt := range tests {
		err := validateURL(ts.Client(), ts.URL+tt.path)
		if tt.wantErr == "" && err != nil {
			t.Errorf("path %s: unexpected error: %v", tt.path, err)
		}
		if tt.wantErr != "" {
			if err == nil {
				t.Errorf("path %s: expected error %q", tt.path, tt.wantErr)
			} else if err.Error() != tt.wantErr {
				t.Errorf("path %s: error = %q, want %q", tt.path, err.Error(), tt.wantErr)
			}
		}
	}
}

func TestCuratedSourceValidator_CratesIOAlias(t *testing.T) {
	// Verify crates.io is also accepted as an ecosystem alias.
	url, err := ecosystemAPIURL("crates.io:ripgrep")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://crates.io/api/v1/crates/ripgrep" {
		t.Errorf("URL = %q, want crates.io URL", url)
	}
}

func TestApplySourceChange_NoChange(t *testing.T) {
	now := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	entry := batch.QueueEntry{
		Name:         "tool",
		Source:       "cargo:tool",
		Priority:     3,
		Status:       batch.StatusPending,
		Confidence:   batch.ConfidenceAuto,
		FailureCount: 2,
	}

	// When the source doesn't change, the caller shouldn't call ApplySourceChange.
	// But verify that applying the same source still works correctly for priority 3.
	change, modified := ApplySourceChange(&entry, "cargo:tool", now)
	if !modified {
		t.Error("priority 3 accepts any source change call")
	}
	if change.Old != change.New {
		t.Error("when source is the same, old and new should match")
	}
	// Failure count still gets reset
	if entry.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0", entry.FailureCount)
	}
}
