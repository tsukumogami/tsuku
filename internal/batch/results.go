package batch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// EcosystemResult holds per-ecosystem success/failure breakdown within a batch.
type EcosystemResult struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// BatchResult holds the outcome of a batch generation run.
type BatchResult struct {
	BatchID      string                     `json:"batch_id"`
	Ecosystems   map[string]int             `json:"ecosystems"`
	PerEcosystem map[string]EcosystemResult `json:"per_ecosystem"`
	Total        int                        `json:"total"`
	Succeeded    int                        `json:"succeeded"`
	Failed       int                        `json:"failed"`
	Blocked      int                        `json:"blocked"`
	Timestamp    time.Time                  `json:"timestamp"`

	// Recipes is the list of generated recipe file paths.
	Recipes []string `json:"-"`

	// Failures is the list of failure records for this run.
	Failures []FailureRecord `json:"-"`

	// Disambiguations is the list of disambiguation records for this run.
	Disambiguations []DisambiguationRecord `json:"-"`
}

// Summary returns a markdown summary of the batch run for use in PR descriptions.
func (r *BatchResult) Summary() string {
	// Build ecosystem label from the breakdown map.
	ecoLabel := "mixed"
	if len(r.Ecosystems) == 1 {
		for eco := range r.Ecosystems {
			ecoLabel = eco
		}
	}

	s := fmt.Sprintf("Batch run for **%s** on %s.\n\n", ecoLabel, r.Timestamp.Format("2006-01-02"))

	// Show per-ecosystem breakdown when multiple ecosystems are present.
	if len(r.PerEcosystem) > 1 {
		s += "### Ecosystem breakdown\n\n"
		s += "| Ecosystem | Succeeded | Failed | Total |\n|-----------|-----------|--------|-------|\n"
		for eco, er := range r.PerEcosystem {
			s += fmt.Sprintf("| %s | %d | %d | %d |\n", eco, er.Succeeded, er.Failed, er.Total)
		}
		s += "\n"
	}

	s += "| Metric | Count |\n|--------|-------|\n"
	s += fmt.Sprintf("| Succeeded | %d |\n", r.Succeeded)
	s += fmt.Sprintf("| Failed | %d |\n", r.Failed)
	s += fmt.Sprintf("| Blocked | %d |\n", r.Blocked)
	s += fmt.Sprintf("| **Total** | **%d** |\n", r.Total)

	if len(r.Recipes) > 0 {
		s += "\n### Recipes added\n\n"
		for _, p := range r.Recipes {
			s += fmt.Sprintf("- `%s`\n", filepath.Base(p))
		}
	}

	if len(r.Failures) > 0 {
		s += "\n### Failures\n\n"
		for _, f := range r.Failures {
			if f.Category == "missing_dep" && len(f.BlockedBy) > 0 {
				s += fmt.Sprintf("- **%s**: blocked by %v\n", f.PackageID, f.BlockedBy)
			} else {
				s += fmt.Sprintf("- **%s**: %s\n", f.PackageID, f.Category)
			}
		}
	}

	if len(r.Disambiguations) > 0 {
		highRisk := 0
		for _, d := range r.Disambiguations {
			if d.HighRisk {
				highRisk++
			}
		}
		s += fmt.Sprintf("\n### Disambiguations: %d", len(r.Disambiguations))
		if highRisk > 0 {
			s += fmt.Sprintf(" (%d high-risk)", highRisk)
		}
		s += "\n\n"
		for _, d := range r.Disambiguations {
			risk := ""
			if d.HighRisk {
				risk = " ⚠️"
			}
			s += fmt.Sprintf("- **%s**: %s (%s)%s\n", d.Tool, d.Selected, d.SelectionReason, risk)
		}
	}

	return s
}

// Selection reasons for disambiguation records.
const (
	SelectionSingleMatch      = "single_match"
	Selection10xPopularityGap = "10x_popularity_gap"
	SelectionPriorityFallback = "priority_fallback"
	SelectionCurated          = "curated" // Manual curation from seed files
)

// DisambiguationRecord represents a disambiguation decision made during batch processing.
// This tracks when multiple ecosystem matches exist for a tool and records which
// source was selected, enabling later human review of potentially incorrect selections.
type DisambiguationRecord struct {
	Tool            string   `json:"tool"`
	Selected        string   `json:"selected"`
	Alternatives    []string `json:"alternatives"`
	SelectionReason string   `json:"selection_reason"`
	DownloadsRatio  float64  `json:"downloads_ratio,omitempty"`
	HighRisk        bool     `json:"high_risk"`
}

// FailureRecord represents a single package generation failure.
type FailureRecord struct {
	PackageID   string    `json:"package_id"`
	Category    string    `json:"category"`
	Subcategory string    `json:"subcategory,omitempty"`
	BlockedBy   []string  `json:"blocked_by,omitempty"`
	Message     string    `json:"message"`
	Timestamp   time.Time `json:"timestamp"`
}

// FailureFile is the top-level structure for failure JSONL entries, matching
// data/schemas/failure-record.schema.json.
type FailureFile struct {
	SchemaVersion int             `json:"schema_version"`
	Ecosystem     string          `json:"ecosystem"`
	Environment   string          `json:"environment"`
	UpdatedAt     string          `json:"updated_at"`
	Failures      []FailureRecord `json:"failures"`
}

// WriteFailures writes failure records to data/failures/<ecosystem>-<timestamp>.jsonl.
// Each call writes one JSON line containing all failures from a single batch
// run for one environment. Uses timestamped filenames to eliminate append-only
// conflicts when parallel workflows run simultaneously.
func WriteFailures(dir, ecosystem string, failures []FailureRecord) error {
	if len(failures) == 0 {
		return nil
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create failures dir: %w", err)
	}

	// Use hyphens instead of colons in timestamp for artifact upload compatibility
	// GitHub Actions artifacts reject filenames containing colons
	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	record := FailureFile{
		SchemaVersion: 1,
		Ecosystem:     ecosystem,
		Environment:   "linux-glibc-x86_64",
		UpdatedAt:     timestamp,
		Failures:      failures,
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal failures: %w", err)
	}
	data = append(data, '\n')

	// Write to timestamped file instead of appending to shared file
	filename := fmt.Sprintf("%s-%s.jsonl", ecosystem, timestamp)
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write failures: %w", err)
	}

	return nil
}

// DisambiguationFile is the top-level structure for disambiguation JSONL entries.
type DisambiguationFile struct {
	SchemaVersion   int                    `json:"schema_version"`
	Ecosystem       string                 `json:"ecosystem"`
	Environment     string                 `json:"environment"`
	UpdatedAt       string                 `json:"updated_at"`
	Disambiguations []DisambiguationRecord `json:"disambiguations"`
}

// WriteDisambiguations writes disambiguation records to data/disambiguations/<ecosystem>-<timestamp>.jsonl.
// Each call writes one JSON line containing all disambiguation decisions from a single batch
// run for one environment. Uses timestamped filenames to eliminate append-only conflicts.
func WriteDisambiguations(dir, ecosystem string, disambiguations []DisambiguationRecord) error {
	if len(disambiguations) == 0 {
		return nil
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create disambiguations dir: %w", err)
	}

	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	record := DisambiguationFile{
		SchemaVersion:   1,
		Ecosystem:       ecosystem,
		Environment:     "linux-glibc-x86_64",
		UpdatedAt:       timestamp,
		Disambiguations: disambiguations,
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal disambiguations: %w", err)
	}
	data = append(data, '\n')

	filename := fmt.Sprintf("%s-%s.jsonl", ecosystem, timestamp)
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write disambiguations: %w", err)
	}

	return nil
}
