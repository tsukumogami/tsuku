package project

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDiagConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestParseConfigFile_ReportsSilentDrops covers the two ways a config can be
// wrong today without anyone being told. Both load cleanly and both leave the
// user with fewer tools than they wrote down.
func TestParseConfigFile_ReportsSilentDrops(t *testing.T) {
	t.Run("unrecognized key", func(t *testing.T) {
		p := writeDiagConfig(t, "[tools]\nnode = \"20\"\n\n[tolos]\njq = \"1\"\n")
		cfg, diags, err := parseConfigFile(p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.Tools) != 1 {
			t.Errorf("expected the one recognized tool, got %d", len(cfg.Tools))
		}
		if !containsSub(diags, "tolos") {
			t.Errorf("a mistyped section was dropped without a word: %v", diags)
		}
	})

	t.Run("tools as a scalar", func(t *testing.T) {
		// This parses cleanly and yields no tools at all, so the file looks
		// fine and activates nothing.
		p := writeDiagConfig(t, "tools = \"not-a-table\"\n")
		_, diags, err := parseConfigFile(p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !containsSub(diags, "no tools declared") {
			t.Errorf("a config that declares nothing said nothing: %v", diags)
		}
	})

	t.Run("a good config is quiet", func(t *testing.T) {
		// The other direction: an over-eager diagnostic is its own defect, and
		// nothing here should nag about a correct file.
		p := writeDiagConfig(t, "[tools]\nnode = \"20.16.0\"\n")
		_, diags, err := parseConfigFile(p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(diags) != 0 {
			t.Errorf("a valid config produced diagnostics: %v", diags)
		}
	})
}

// TestParseConfigFile_UnreadableFileIsAnError pins the split the design turns
// on: a file whose contents are unknown is refused whole, because there is no
// per-entry judgement available. Everything else is a diagnostic.
func TestParseConfigFile_UnreadableFileIsAnError(t *testing.T) {
	p := writeDiagConfig(t, "this is not = = toml\n")
	if _, _, err := parseConfigFile(p); err == nil {
		t.Fatal("a file that is not TOML must be an error, not a diagnostic")
	}
}

// TestFprintDiagnostics_WritesWhereItIsTold is the security-relevant assertion.
//
// Diagnostics quote the offending key, which is attacker-controlled, and
// tsuku hook-env writes activation output to stdout where the shell hook
// evaluates it. A diagnostic on stdout would therefore be executed rather than
// read -- a fresh injection vector introduced by the fix for one. The helper
// exists so that rule has one implementation; this test pins that it writes
// only where it is told.
func TestFprintDiagnostics_WritesWhereItIsTold(t *testing.T) {
	r := &ConfigResult{
		Path:        "/tmp/proj/.tsuku.toml",
		Diagnostics: []string{`refused tool name "x$(id)y"`},
	}

	// The real os.Stdout, not a second buffer. This assertion used to compare a
	// bytes.Buffer that was never passed to anything, so it could not fail --
	// and the property it was reaching for is the load-bearing one here, since
	// hook-env's stdout is evaluated by the shell.
	realStdout := os.Stdout
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = pw

	var stderr bytes.Buffer
	r.FprintDiagnostics(&stderr)

	os.Stdout = realStdout
	pw.Close()
	leaked, _ := io.ReadAll(pr)

	if !strings.Contains(stderr.String(), "x$(id)y") {
		t.Errorf("diagnostic did not name the offending key: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "/tmp/proj/.tsuku.toml") {
		t.Errorf("diagnostic did not say which file to edit: %q", stderr.String())
	}
	if len(leaked) != 0 {
		t.Errorf("diagnostics reached os.Stdout: %q.\n\n"+
			"hook-env writes shell text there for the shell to evaluate, and these "+
			"messages quote a key that came from the config file.", leaked)
	}
}

func TestFprintDiagnostics_NilAndEmptyAreQuiet(t *testing.T) {
	var buf bytes.Buffer
	var nilResult *ConfigResult
	nilResult.FprintDiagnostics(&buf)
	(&ConfigResult{Path: "/x"}).FprintDiagnostics(&buf)
	if buf.Len() != 0 {
		t.Errorf("expected silence, got %q", buf.String())
	}
}

func containsSub(hay []string, needle string) bool {
	for _, h := range hay {
		if strings.Contains(h, needle) {
			return true
		}
	}
	return false
}
