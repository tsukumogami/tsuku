package seed

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
)

// A scoped npm name puts the audit entry in a subdirectory the audit root does
// not contain. Before the fix every scoped package failed with
// "no such file or directory" (tsukumogami/tsuku#2609).
func TestWriteAuditEntryCreatesScopeDirectory(t *testing.T) {
	dir := t.TempDir()
	entry := AuditEntry{
		DisambiguationRecord: batch.DisambiguationRecord{Tool: "@inquirer/core"},
		DisambiguatedAt:      time.Now(),
		SeedingRun:           time.Now(),
	}

	if err := WriteAuditEntry(dir, entry); err != nil {
		t.Fatalf("WriteAuditEntry for a scoped name: %v", err)
	}
	want := filepath.Join(dir, "@inquirer", "core.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected audit entry at %s: %v", want, err)
	}
}

// Nothing constrains the tool name, so a traversal attempt must be refused
// rather than writing outside the audit root.
func TestWriteAuditEntryRefusesEscapingName(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "audit")
	entry := AuditEntry{
		DisambiguationRecord: batch.DisambiguationRecord{Tool: "../escaped"},
		DisambiguatedAt:      time.Now(),
		SeedingRun:           time.Now(),
	}

	if err := WriteAuditEntry(dir, entry); err == nil {
		t.Fatal("expected an error for a name escaping the audit directory")
	}
	if _, err := os.Stat(filepath.Join(root, "escaped.json")); !os.IsNotExist(err) {
		t.Fatal("a traversing name wrote outside the audit directory")
	}
}
