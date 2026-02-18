package seed

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/batch"
	"github.com/tsukumogami/tsuku/internal/discover"
)

func TestAssignTier(t *testing.T) {
	tests := []struct {
		name      string
		downloads int
		ecosystem string
		want      int
	}{
		// Tier 1: curated tools.
		{"ripgrep", 0, "cargo", 1},
		{"jq", 0, "npm", 1},
		{"bat", 999999, "cargo", 1}, // tier1 takes priority over downloads

		// Tier 2: cargo threshold.
		{"tokei-clone", 200000, "cargo", 2},
		{"at-boundary", 100001, "cargo", 2},
		{"below-cargo", 100000, "cargo", 3}, // exactly at threshold is not >

		// Tier 2: npm threshold.
		{"popular-cli", 600000, "npm", 2},
		{"below-npm", 500000, "npm", 3},

		// Tier 2: rubygems threshold.
		{"popular-gem", 1500000, "rubygems", 2},
		{"below-gems", 1000000, "rubygems", 3},

		// Tier 3: pypi always (no download data).
		{"httpie", 0, "pypi", 3},
		{"httpie", 999999, "pypi", 3},

		// Tier 3: unknown ecosystem.
		{"tool", 999999, "unknown", 3},

		// Tier 3: zero downloads.
		{"tool", 0, "cargo", 3},
	}
	for _, tt := range tests {
		got := AssignTier(tt.name, tt.downloads, tt.ecosystem)
		if got != tt.want {
			t.Errorf("AssignTier(%q, %d, %q) = %d, want %d",
				tt.name, tt.downloads, tt.ecosystem, got, tt.want)
		}
	}
}

func TestToQueueEntry_WithDisambiguation(t *testing.T) {
	pkg := Package{
		ID:     "cargo:ripgrep",
		Source: "cargo",
		Name:   "ripgrep",
		Tier:   1,
		Status: "pending",
	}

	disambiguated := &discover.DiscoveryResult{
		Builder: "cargo",
		Source:  "ripgrep",
		Metadata: discover.Metadata{
			SelectionReason: "10x_popularity_gap",
		},
	}

	entry := ToQueueEntry(pkg, disambiguated)
	if entry.Name != "ripgrep" {
		t.Errorf("Name = %q, want ripgrep", entry.Name)
	}
	if entry.Source != "cargo:ripgrep" {
		t.Errorf("Source = %q, want cargo:ripgrep", entry.Source)
	}
	if entry.Priority != 1 {
		t.Errorf("Priority = %d, want 1", entry.Priority)
	}
	if entry.Status != batch.StatusPending {
		t.Errorf("Status = %q, want pending", entry.Status)
	}
	if entry.Confidence != batch.ConfidenceAuto {
		t.Errorf("Confidence = %q, want auto", entry.Confidence)
	}
	if entry.DisambiguatedAt == nil {
		t.Error("DisambiguatedAt should be set")
	}
	if entry.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0", entry.FailureCount)
	}
}

func TestToQueueEntry_PriorityFallback(t *testing.T) {
	pkg := Package{
		ID:     "npm:serve",
		Source: "npm",
		Name:   "serve",
		Tier:   3,
		Status: "pending",
	}

	disambiguated := &discover.DiscoveryResult{
		Builder: "npm",
		Source:  "serve",
		Metadata: discover.Metadata{
			SelectionReason: "priority_fallback",
		},
	}

	entry := ToQueueEntry(pkg, disambiguated)
	if entry.Status != batch.StatusRequiresManual {
		t.Errorf("Status = %q, want requires_manual for priority_fallback", entry.Status)
	}
}

func TestToQueueEntry_NilDisambiguation(t *testing.T) {
	pkg := Package{
		ID:     "cargo:some-tool",
		Source: "cargo:some-tool",
		Name:   "some-tool",
		Tier:   3,
		Status: "pending",
	}

	entry := ToQueueEntry(pkg, nil)
	if entry.Source != "cargo:some-tool" {
		t.Errorf("Source = %q, want cargo:some-tool (fallback to pkg.Source)", entry.Source)
	}
	if entry.Status != batch.StatusPending {
		t.Errorf("Status = %q, want pending", entry.Status)
	}
}

func TestIsTier1(t *testing.T) {
	if !IsTier1("ripgrep") {
		t.Error("ripgrep should be tier 1")
	}
	if !IsTier1("jq") {
		t.Error("jq should be tier 1")
	}
	if IsTier1("nonexistent-tool") {
		t.Error("nonexistent-tool should not be tier 1")
	}
}
