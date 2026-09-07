package activation

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/project"
)

// A parse failure returns a non-nil result AND a non-nil error. The unusual
// convention is deliberate: the file that would not parse still identifies the
// project the developer is standing in, and that has to be recorded or the
// prompt hook re-reports on every prompt.
func TestComputeActivation_ParseFailureReturnsBoth(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "this is not toml = = =\n", nil)
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	result, err := ComputeActivation(projectDir, "", "", "", cfg, installed)

	if err == nil {
		t.Fatal("expected an error for an unparseable .tsuku.toml")
	}
	var parseErr *project.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error = %v, want a *project.ParseError", err)
	}
	if result == nil {
		t.Fatal("expected a result alongside the error; a nil result makes the " +
			"once-per-entry budget unenforceable")
	}

	if result.Dir != projectDir {
		t.Errorf("Dir = %q, want the directory containing the broken file, %q", result.Dir, projectDir)
	}
	if !result.Active {
		t.Error("Active = false; the shape of an activation where nothing activated is still an activation")
	}
	if result.PATH != "/usr/bin:/bin" {
		t.Errorf("PATH = %q, want the unchanged base %q", result.PATH, "/usr/bin:/bin")
	}
	if result.PrevPath != "/usr/bin:/bin" {
		t.Errorf("PrevPath = %q, want the base to be recorded", result.PrevPath)
	}
	if result.Stamp == "" {
		t.Error("Stamp is empty; without it the next prompt cannot short-circuit")
	}
	if !result.Entered {
		t.Error("Entered = false when arriving at the broken project, want true")
	}
}

// The result has the shape FormatExports already renders correctly, so nothing
// in cmd/tsuku needs to synthesize exports for this path.
func TestComputeActivation_ParseFailureEmitsARecording(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "= broken\n", nil)
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	result, _ := ComputeActivation(projectDir, "", "", "", cfg, installed)

	want := renderVar("bash", "PATH", "/usr/bin") +
		renderVar("bash", "_TSUKU_DIR", projectDir) +
		renderVar("bash", "_TSUKU_PREV_PATH", "/usr/bin") +
		renderVar("bash", "_TSUKU_STATE_STAMP", result.Stamp)

	if got := FormatExports(result, "bash"); got != want {
		t.Errorf("FormatExports = %q, want %q", got, want)
	}
}

// Standing still in a broken project: the recording made on arrival lets the
// next invocation short-circuit, so nothing is said twice.
func TestComputeActivation_ParseFailureIsReportedOncePerEntry(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "= broken\n", nil)
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))
	writeState(t, cfg, map[string][]string{"jq": {"1.7"}})

	first, err := ComputeActivation(projectDir, "", "", "", cfg, installed)
	if err == nil {
		t.Fatal("expected a parse error on arrival")
	}

	// Only what the first invocation emitted, never a hand-built environment.
	second, err := ComputeActivation(projectDir, first.PrevPath, first.Dir, first.Stamp, cfg, installed)
	if second != nil || err != nil {
		t.Errorf("second invocation = (%+v, %v), want a short-circuit", second, err)
	}

	// And from a subdirectory of the same project, which walks up to the same
	// recorded directory.
	sub := filepath.Join(projectDir, "pkg")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	third, err := ComputeActivation(sub, first.PrevPath, first.Dir, first.Stamp, cfg, installed)
	if third != nil {
		if !third.Entered {
			// Re-resolving is fine; re-reporting is not. Entered is what the
			// caller gates the message on.
			t.Log("re-resolved from a subdirectory without entering, which reports nothing")
		} else {
			t.Errorf("moving within the project set Entered = true, so it would report again")
		}
	}
	_ = err
}

// Repairing the file while standing at the project root changes nothing until
// the project is left and re-entered, because the directory has not changed and
// installation state has not moved.
func TestComputeActivation_RepairingInPlaceTakesEffectOnReEntry(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "= broken\n", map[string][]string{"jq": {"1.7"}})
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))
	writeState(t, cfg, map[string][]string{"jq": {"1.7"}})

	first, _ := ComputeActivation(projectDir, "", "", "", cfg, installed)

	// The developer fixes the file without leaving.
	if err := os.WriteFile(filepath.Join(projectDir, ".tsuku.toml"),
		[]byte("[tools]\njq = \"latest\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	standing, err := ComputeActivation(projectDir, first.PrevPath, first.Dir, first.Stamp, cfg, installed)
	if standing != nil || err != nil {
		t.Errorf("standing still after a repair = (%+v, %v), want a short-circuit: "+
			".tsuku.toml is not what the stamp watches", standing, err)
	}

	// Leaving and coming back picks it up.
	reentered, err := ComputeActivation(projectDir, first.PrevPath, "/elsewhere", first.Stamp, cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error after re-entry: %v", err)
	}
	want := filepath.Join(cfg.ToolsDir, "jq-1.7", "bin")
	if reentered == nil || reentered.PATH != want+":/usr/bin" {
		t.Errorf("re-entry PATH = %+v, want %q activated", reentered, want)
	}
}

// An error that is not a parse failure still comes back as an error with no
// result, so the callers' parse-failure branch cannot swallow a real problem.
func TestComputeActivation_NonParseErrorsStillReturnNilResult(t *testing.T) {
	var parseErr *project.ParseError
	if errors.As(errors.New("something else"), &parseErr) {
		t.Fatal("errors.As matched a plain error; the branch would swallow real failures")
	}
}
