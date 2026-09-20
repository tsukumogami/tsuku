package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
)

// AuditProbeResult is the audit-specific representation of a single
// ecosystem probe outcome. It captures the key fields needed for
// inspecting disambiguation decisions without coupling to the
// builders.ProbeResult type.
type AuditProbeResult struct {
	Source        string `json:"source"`
	Downloads     int    `json:"downloads"`
	VersionCount  int    `json:"version_count"`
	HasRepository bool   `json:"has_repository"`
}

// AuditEntry records a full disambiguation decision for a single package.
// It embeds the standard DisambiguationRecord and adds seeding-specific
// fields for probe details and freshness tracking.
type AuditEntry struct {
	batch.DisambiguationRecord
	ProbeResults    []AuditProbeResult `json:"probe_results"`
	PreviousSource  *string            `json:"previous_source"`
	DisambiguatedAt time.Time          `json:"disambiguated_at"`
	SeedingRun      time.Time          `json:"seeding_run"`
}

// WriteAuditEntry writes an audit entry to a per-package JSON file at
// <dir>/<tool>.json with indented JSON. The directory is created if it
// doesn't exist. Existing files are overwritten so the most recent
// disambiguation decision wins.
func WriteAuditEntry(dir string, entry AuditEntry) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create audit dir: %w", err)
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal audit entry: %w", err)
	}
	data = append(data, '\n')

	path := filepath.Join(dir, entry.Tool+".json")

	// A scoped package name such as "@scope/pkg" puts the entry in a
	// subdirectory, which MkdirAll above does not create -- it makes the audit
	// root, not the leaf. Confine the result to the audit root first: nothing
	// constrains entry.Tool, so a name containing ".." would otherwise write
	// outside it.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve audit dir: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve audit path: %w", err)
	}
	if absPath != absDir && !strings.HasPrefix(absPath, absDir+string(os.PathSeparator)) {
		return fmt.Errorf("audit entry for %q would write outside the audit directory", entry.Tool)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create audit entry dir: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write audit entry: %w", err)
	}
	return nil
}

// ReadAuditEntry reads and parses an audit file for the given tool name.
// Returns (nil, nil) if the file doesn't exist.
func ReadAuditEntry(dir, toolName string) (*AuditEntry, error) {
	path := filepath.Join(dir, toolName+".json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read audit entry: %w", err)
	}

	var entry AuditEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("parse audit entry: %w", err)
	}
	return &entry, nil
}

// HasSource checks whether a given source string exists in the audit
// entry's probe_results. Returns false if entry is nil.
func HasSource(entry *AuditEntry, source string) bool {
	if entry == nil {
		return false
	}
	for _, pr := range entry.ProbeResults {
		if pr.Source == source {
			return true
		}
	}
	return false
}
