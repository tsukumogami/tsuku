package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDiagnosticsAreBounded pins the cap on what one config file can print.
//
// The attack this closes is cheap and does not need a single valid declaration:
// unrecognized keys are unbounded where tools are not, and `tsuku hook-env`
// runs from the shell prompt, so an uncapped diagnostic list is a flood on
// every prompt in any directory at or below the one holding the file. The
// diagnostics were introduced by this change, so the flood was introduced with
// them -- the cap is what makes reporting safe to keep.
func TestDiagnosticsAreBounded(t *testing.T) {
	const hostileKeys = 5000

	dir := t.TempDir()
	var b strings.Builder
	for i := 0; i < hostileKeys; i++ {
		fmt.Fprintf(&b, "unknown_key_%d = \"x\"\n", i)
	}
	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	_, diags, err := parseConfigFile(path)
	if err != nil {
		t.Fatalf("parseConfigFile: %v", err)
	}

	// The cap plus the one "and N more" summary line.
	if len(diags) > maxDiagnostics+1 {
		t.Errorf("a file with %d unrecognized keys produced %d diagnostics; the cap is %d.\n\n"+
			"hook-env runs from the shell prompt, so this is that many lines on every "+
			"prompt beneath the directory holding the file.", hostileKeys, len(diags), maxDiagnostics)
	}

	// A truncated list that does not say it was truncated is worse than either
	// a full list or a short one, because the reader cannot tell which they have.
	last := diags[len(diags)-1]
	if !strings.Contains(last, "more") {
		t.Errorf("diagnostics were truncated but the last line does not say so: %q", last)
	}
}

// TestDiagnosticsBoundLeavesShortListsAlone guards the other direction: a cap
// that rewrote every list would hide the ordinary one-or-two-diagnostic case
// behind a summary line, and the bound test above would still pass.
func TestDiagnosticsBoundLeavesShortListsAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, []byte("typo_key = \"x\"\n[tools]\njq = \"1.7\"\n"), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	_, diags, err := parseConfigFile(path)
	if err != nil {
		t.Fatalf("parseConfigFile: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("want exactly the one unrecognized-key diagnostic, got %d: %q", len(diags), diags)
	}
	if strings.Contains(diags[0], "suppressed") {
		t.Errorf("a one-diagnostic file was summarized as if truncated: %q", diags[0])
	}
}
