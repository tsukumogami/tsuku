package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeProject creates dir/.tsuku.toml with the given contents and returns the
// directory. Callers pass the directory they want the file to live in, so the
// walk-up case can put it above the start directory.
func writeProject(t *testing.T, dir, contents string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A caller has to be able to recover the directory the bad file was in, because
// the activation path records it to avoid re-reporting on the next prompt.
func TestLoadProjectConfig_ParseErrorCarriesDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TSUKU_CEILING_PATHS", filepath.Dir(root))

	dir := writeProject(t, filepath.Join(root, "proj"), "this is not valid toml {{{\n")

	_, err := LoadProjectConfig(dir)
	if err == nil {
		t.Fatal("expected an error for an unparseable .tsuku.toml")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected a *ParseError, got %T: %v", err, err)
	}
	if pe.Dir != dir {
		t.Fatalf("ParseError.Dir = %q, want %q", pe.Dir, dir)
	}
	if pe.Path != filepath.Join(dir, ConfigFileName) {
		t.Fatalf("ParseError.Path = %q, want the config file", pe.Path)
	}
	if pe.Unwrap() == nil {
		t.Fatal("the wrapped cause must be reachable")
	}
}

// The same holds when the file is found by walking up rather than in the
// starting directory -- Dir is the file's directory, not where the walk began.
func TestLoadProjectConfig_ParseErrorDirWhenFoundByWalkingUp(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TSUKU_CEILING_PATHS", filepath.Dir(root))

	projDir := writeProject(t, filepath.Join(root, "proj"), "not toml {{{\n")
	deep := filepath.Join(projDir, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := LoadProjectConfig(deep)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected a *ParseError, got %T: %v", err, err)
	}
	if pe.Dir != projDir {
		t.Fatalf("ParseError.Dir = %q, want the directory holding the file, %q", pe.Dir, projDir)
	}
}

// Every failure that belongs to the file is a ParseError, not only the decode.
// A read failure and the MaxTools cap otherwise reach callers as bare errors and
// keep the behavior the reporting work exists to stop.
func TestLoadProjectConfig_ParseErrorCoversEveryFileFailure(t *testing.T) {
	t.Run("over the tool cap", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("TSUKU_CEILING_PATHS", filepath.Dir(root))

		var b strings.Builder
		b.WriteString("[tools]\n")
		for i := range MaxTools + 1 {
			b.WriteString("tool")
			b.WriteString(string(rune('a' + i%26)))
			b.WriteString(string(rune('a' + (i/26)%26)))
			b.WriteString(string(rune('a' + (i/676)%26)))
			b.WriteString(" = \"1.0.0\"\n")
		}
		dir := writeProject(t, filepath.Join(root, "proj"), b.String())

		_, err := LoadProjectConfig(dir)
		var pe *ParseError
		if !errors.As(err, &pe) {
			t.Fatalf("a file over the tool cap must be a *ParseError, got %T: %v", err, err)
		}
		if pe.Dir != dir {
			t.Fatalf("ParseError.Dir = %q, want %q", pe.Dir, dir)
		}
	})

	t.Run("unreadable file", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root; permission bits do not deny reads")
		}
		root := t.TempDir()
		t.Setenv("TSUKU_CEILING_PATHS", filepath.Dir(root))

		dir := writeProject(t, filepath.Join(root, "proj"), "[tools]\n")
		if err := os.Chmod(filepath.Join(dir, ConfigFileName), 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, ConfigFileName), 0o644) })

		_, err := LoadProjectConfig(dir)
		var pe *ParseError
		if !errors.As(err, &pe) {
			t.Fatalf("an unreadable file must be a *ParseError, got %T: %v", err, err)
		}
	})
}

// A valid file is unaffected, and callers that only test for a non-nil error
// keep working.
func TestLoadProjectConfig_ValidFileUnaffected(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TSUKU_CEILING_PATHS", filepath.Dir(root))

	dir := writeProject(t, filepath.Join(root, "proj"), "[tools]\njq = \"1.7\"\n")

	result, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("valid config returned an error: %v", err)
	}
	if result == nil || result.Dir != dir {
		t.Fatalf("valid config did not load: %+v", result)
	}
	if got := result.Config.Tools["jq"].Version; got != "1.7" {
		t.Fatalf("jq version = %q, want 1.7", got)
	}
}
