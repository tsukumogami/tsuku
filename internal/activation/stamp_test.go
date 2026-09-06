package activation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
)

func stampConfig(t *testing.T) *config.Config {
	t.Helper()
	home := t.TempDir()
	return &config.Config{HomeDir: home, ToolsDir: filepath.Join(home, "tools")}
}

// The stamp carries size as well as mtime. A stamp built from mtime alone loses
// on a filesystem with one-second granularity: activate at T, install at T, and
// the next prompt sees an unchanged stamp -- which is the exact case the stamp
// exists to catch. The size is forced to differ here with the mtime held equal,
// so an mtime-only implementation cannot pass.
func TestStateStamp_ChangesWithSizeAtEqualMtime(t *testing.T) {
	cfg := stampConfig(t)
	path := filepath.Join(cfg.HomeDir, "state.json")
	fixed := time.Unix(1_700_000_000, 0)

	if err := os.WriteFile(path, []byte(`{"installed":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, fixed, fixed); err != nil {
		t.Fatal(err)
	}
	small := StateStamp(cfg)

	if err := os.WriteFile(path, []byte(`{"installed":{"jq":{"versions":{"1.7":{}}}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, fixed, fixed); err != nil {
		t.Fatal(err)
	}
	large := StateStamp(cfg)

	if small == large {
		t.Errorf("stamp did not change when only the size did: both %q", small)
	}
}

// And it carries mtime as well as size, so a same-size rewrite still counts.
func TestStateStamp_ChangesWithMtimeAtEqualSize(t *testing.T) {
	cfg := stampConfig(t)
	path := filepath.Join(cfg.HomeDir, "state.json")

	if err := os.WriteFile(path, []byte(`{"a":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	before := StateStamp(cfg)

	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	after := StateStamp(cfg)

	if before == after {
		t.Errorf("stamp did not change when only the mtime did: both %q", before)
	}
}

// A stat failure yields a fixed, non-empty literal carrying no error text and
// no path.
//
// Non-empty is the load-bearing part: the short-circuit tests stamp != "", so
// an empty token would mean the early exit never fires on a machine with no
// state.json -- a fresh install, or TSUKU_HOME pointed somewhere new -- and
// every prompt would re-parse and re-resolve, forever, on exactly the machines
// with nothing installed.
func TestStateStamp_MissingStateIsAFixedNonEmptyToken(t *testing.T) {
	cfg := stampConfig(t)

	got := StateStamp(cfg)
	if got == "" {
		t.Fatal("stamp is empty for missing state; the short-circuit would never fire")
	}
	if got != NoStateToken {
		t.Errorf("stamp = %q, want the fixed token %q", got, NoStateToken)
	}
	// Stable across calls, or a machine with no state re-resolves forever.
	if again := StateStamp(cfg); again != got {
		t.Errorf("stamp for missing state is not stable: %q then %q", got, again)
	}
	// No path, no error text: this value is exported into the environment.
	if strings.Contains(got, cfg.HomeDir) || strings.Contains(got, "/") {
		t.Errorf("stamp %q leaks a path", got)
	}
	if strings.Contains(strings.ToLower(got), "no such file") {
		t.Errorf("stamp %q carries error text", got)
	}
}

// A machine with no installation state short-circuits on the second prompt,
// rather than re-resolving forever.
func TestComputeActivation_NoStateStillShortCircuits(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "[tools]\njq = \"latest\"\n", nil)
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	first, err := ComputeActivation(projectDir, "", "", "", cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first == nil {
		t.Fatal("expected a result on first entry")
	}

	second, err := ComputeActivation(projectDir, first.PrevPath, first.Dir, first.Stamp, cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second != nil {
		t.Errorf("a machine with no state should short-circuit on the second prompt, got %+v", second)
	}
}

// The comparison is inequality, not ordering. A state file restored from backup
// is older than the recorded stamp, and an ordering test would read that as
// "not newer" and never re-resolve.
func TestComputeActivation_StampMovingBackwardsStillReResolves(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "[tools]\njq = \"latest\"\n",
		map[string][]string{"jq": {"1.7"}})
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))
	writeState(t, cfg, map[string][]string{"jq": {"1.7"}})

	first, err := ComputeActivation(projectDir, "", "", "", cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// State is restored from a backup: same size, older mtime.
	older := time.Now().Add(-24 * time.Hour)
	path := filepath.Join(cfg.HomeDir, "state.json")
	if err := os.Chtimes(path, older, older); err != nil {
		t.Fatal(err)
	}

	got, err := ComputeActivation(projectDir, first.PrevPath, first.Dir, first.Stamp, cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Error("a state file moving backwards must still re-resolve, got a short-circuit")
	}
}

// The emitted stamp is always freshly computed; a value from the environment is
// never echoed back into the output.
func TestComputeActivation_StampIsNeverEchoedFromTheEnvironment(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "[tools]\njq = \"latest\"\n",
		map[string][]string{"jq": {"1.7"}})
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))

	// A stamp that matches nothing, from a directory that is not the current
	// one, so the short-circuit cannot fire and the value must be recomputed.
	result, err := ComputeActivation(projectDir, "", "/elsewhere", "9999-9999", cfg, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stamp == "9999-9999" {
		t.Error("the incoming stamp was echoed back instead of being recomputed")
	}
	if result.Stamp != StateStamp(cfg) {
		t.Errorf("Stamp = %q, want the freshly computed %q", result.Stamp, StateStamp(cfg))
	}
}

// mutatingInstalledSet moves installation state during the read, standing in
// for an install that commits between activation's stat and its decode.
type mutatingInstalledSet struct {
	inner   *fakeInstalledSet
	onRead  func()
	stamped string // the stamp as it stood at the moment of the read
	cfg     *config.Config
}

func (m *mutatingInstalledSet) InstalledVersionsFor(names []string) (map[string][]string, error) {
	out, err := m.inner.InstalledVersionsFor(names)
	m.onRead()
	m.stamped = StateStamp(m.cfg)
	return out, err
}

// The stat is taken before installation state is read, and the recorded stamp
// is that same stat.
//
// The wrong order is stable-looking and broken: read at T1, an install commits
// at T2, stat at T3, and the shell has resolved against the old state while
// recording the new stamp -- so it never re-resolves, and the developer's
// install never takes effect. Here state moves during the read, so an
// implementation that stats afterwards records the post-install stamp and this
// fails.
func TestComputeActivation_StampIsTakenBeforeTheStateRead(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "[tools]\njq = \"latest\"\n",
		map[string][]string{"jq": {"1.6"}})
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))
	writeState(t, cfg, map[string][]string{"jq": {"1.6"}})

	before := StateStamp(cfg)

	mutating := &mutatingInstalledSet{
		inner: installed,
		cfg:   cfg,
		onRead: func() {
			// An install commits while activation is mid-read.
			writeState(t, cfg, map[string][]string{"jq": {"1.6", "1.7"}})
		},
	}

	result, err := ComputeActivation(projectDir, "", "", "", cfg, mutating)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mutating.stamped == before {
		t.Fatal("the fixture did not move installation state; the test proves nothing")
	}
	if result.Stamp != before {
		t.Errorf("Stamp = %q, want the pre-read stamp %q. Recording the post-read stamp %q "+
			"would mean resolving against old state while claiming the new state was seen, "+
			"so the next prompt never re-resolves.",
			result.Stamp, before, mutating.stamped)
	}
}

// FormatExports emits all three tracking variables on the activation path, and
// deactivation unsets exactly the same set.
func TestFormatExports_AllThreeVariables(t *testing.T) {
	active := &ActivationResult{
		PATH: "/tools/jq-1.7/bin:/usr/bin", Dir: "/p", PrevPath: "/usr/bin",
		Stamp: "123-45", Active: true,
	}

	for _, shell := range []string{"bash", "zsh", "fish"} {
		out := FormatExports(active, shell)
		for _, name := range trackedVars {
			if !strings.Contains(out, name) {
				t.Errorf("%s: activation output missing %s:\n%s", shell, name, out)
			}
		}
		if !strings.Contains(out, "123-45") {
			t.Errorf("%s: activation output missing the stamp value:\n%s", shell, out)
		}

		off := FormatExports(&ActivationResult{PATH: "/usr/bin"}, shell)
		for _, name := range trackedVars {
			if !strings.Contains(off, name) {
				t.Errorf("%s: deactivation does not unset %s:\n%s", shell, name, off)
			}
		}
		if strings.Contains(off, "123-45") {
			t.Errorf("%s: deactivation should not emit a stamp value:\n%s", shell, off)
		}
	}
}

// Every emitted value goes through the one quoting site. This is asserted so
// that adding a variable with a fresh Fprintf -- which would escape the single
// place the shell-quoting fix has to change -- shows up as a failure here.
func TestFormatExports_EveryValueGoesThroughExportLine(t *testing.T) {
	result := &ActivationResult{
		PATH: "/a", Dir: "/b", PrevPath: "/c", Stamp: "d", Active: true,
	}

	for _, shell := range []string{"bash", "fish"} {
		var want strings.Builder
		want.WriteString(exportLine(shell, "PATH", result.PATH))
		want.WriteString(exportLine(shell, "_TSUKU_DIR", result.Dir))
		want.WriteString(exportLine(shell, "_TSUKU_PREV_PATH", result.PrevPath))
		want.WriteString(exportLine(shell, "_TSUKU_STATE_STAMP", result.Stamp))

		if got := FormatExports(result, shell); got != want.String() {
			t.Errorf("%s: FormatExports = %q, want every value rendered by exportLine: %q",
				shell, got, want.String())
		}
	}
}
