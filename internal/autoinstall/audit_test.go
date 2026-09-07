package autoinstall

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/index"
	"github.com/tsukumogami/tsuku/internal/indexfixture"
)

// The durable record: what it says about an install, and which installs it
// says anything about at all.
//
// R12 wants the origin of the consent mode on every install `tsuku run`
// performs, and R12a wants the mode-lowering gate that changed the mode where
// one did. The defect underneath both is that the entry used to be written
// only on the auto path, so the installs that ended somewhere other than where
// the configuration pointed -- a gate diverted them to a prompt -- were
// precisely the ones leaving no trace.
//
// Two wrong implementations shape most of what is below.
//
// The first writes an entry only when the mode dispatched as auto. It passes
// every case whose run reaches auto, which is most of this package's corpus,
// so no assertion about a correct auto entry can catch it. What catches it is
// an install a gate diverted: [TestRun_AC24_AGateDivertedInstallNamesTheGate]
// and the confirm rows of [TestRun_AC22_EveryInstallRecordsOneOfFiveOrigins].
//
// The second records the mode again under the name "origin" -- or, subtler,
// reads Run's origin parameter rather than the value the elevation returned.
// [TestRun_AC22_TheOriginIsNotTheModeUnderAnotherName] pins the first by
// putting two installs with the same recorded mode and different origins side
// by side; [TestRun_AC22_AnElevatedDeclarationRecordsProject] pins the second,
// because on an elevated run the parameter still says default and only
// elevate's result says project.

// recordedEntry is the audit log as a reader of the file sees it.
//
// It restates the JSON keys rather than decoding into auditEntry, and that is
// the point: decoding into the production struct would follow a renamed tag
// wherever it went, so a field quietly renamed would keep every assertion here
// green while the file on disk changed shape.
type recordedEntry struct {
	Timestamp string `json:"ts"`
	Action    string `json:"action"`
	Recipe    string `json:"recipe"`
	Version   string `json:"version"`
	Mode      string `json:"mode"`
	Origin    string `json:"origin"`
	Gate      string `json:"gate"`
}

// recordedOrigins is the enum R12 requires the origin field to hold exactly
// one of. "unset" is deliberately not in it: OriginUnset is what a caller that
// resolved no origin has, and an install recording it has recorded no origin
// at all -- which is the failure this list exists to fail on rather than a
// sixth permitted value. See TestOriginZeroValueIsUnset for why that slot is
// the zero value.
var recordedOrigins = []string{"default", "flag", "environment", "config", "project"}

// auditLog decodes every entry the run under test wrote, or reports that the
// file is absent.
func auditLog(t *testing.T, homeDir string) ([]recordedEntry, bool) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(homeDir, "audit.log"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false
	}
	if err != nil {
		t.Fatalf("reading the audit log: %v", err)
	}

	var entries []recordedEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var entry recordedEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("the audit log is not NDJSON: %v\nline: %s", err, line)
		}
		entries = append(entries, entry)
	}
	return entries, true
}

// soleEntry is the one entry a single-install run wrote, and it fails rather
// than returning a zero value where the run wrote none. The absent-file case
// is the one an auto-path-only implementation produces for every confirm
// install, so the message says so.
func soleEntry(t *testing.T, homeDir string) recordedEntry {
	t.Helper()

	entries, written := auditLog(t, homeDir)
	if !written {
		t.Fatalf("this run installed a tool and wrote no audit log.\n" +
			"Every install is recorded, not only the ones that dispatched as " +
			"auto: an install a gate diverted to a prompt is exactly the run " +
			"worth finding in the log later.")
	}
	if len(entries) != 1 {
		t.Fatalf("the audit log holds %d entries, want 1: %+v", len(entries), entries)
	}
	return entries[0]
}

// installRun is one install driven through Run, and what it recorded.
type installRun struct {
	homeDir string
	stderr  string
}

// runInstall drives one install to completion and returns the home it wrote
// into. Every case below installs the same recipe under a different consent
// state, so the wiring that does not vary lives here.
//
// verified and consent are the two levers that decide which path the run
// takes: an unverified recipe fires the recipe-verification gate, and the
// consent answer is what a run diverted to a prompt needs to get past it.
func runInstall(t *testing.T, mode Mode, origin Origin, verified bool, resolver ProjectDeclarationResolver) installRun {
	t.Helper()

	r, _, stderr := newTestRunner(t)
	installer := &mockInstaller{}
	execRec := &execRecorder{}

	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}
	r.Installer = installer
	r.Exec = execRec.exec
	r.RecipeHasVerification = func(_ string) bool { return verified }
	// A run the gates divert answers the prompt and proceeds, which is the
	// state AC24 is stated in: diverted away from auto, and installed anyway.
	r.ConsentReader = strings.NewReader("y\n")

	// The file the configuration-permission gate guards, at permissions it
	// accepts, so that the gate under test is the one the case chose.
	if err := os.WriteFile(r.cfg.ConfigFile, []byte(""), 0o600); err != nil {
		t.Fatalf("writing config.toml: %v", err)
	}

	if err := r.Run(context.Background(), "jq", nil, mode, origin, resolver); err != nil {
		t.Fatalf("Run() error = %v, want nil\nstderr:\n%s", err, stderr.String())
	}
	if !installer.called {
		t.Fatalf("nothing was installed, so there is no install to have recorded\nstderr:\n%s", stderr.String())
	}
	return installRun{homeDir: r.cfg.HomeDir, stderr: stderr.String()}
}

// declaresJq is the resolver for the rows that need a project declaration, so
// that the elevation applies and the recorded origin can be project.
func declaresJq() *mockDeclarationResolver {
	return &mockDeclarationResolver{versions: map[string]string{"jq": "1.7.1"}}
}

// AC22. Every install is recorded, and the origin it records is one of the
// five R12 names.
//
// The rows are the install paths this package can reach, and the confirm ones
// carry the weight: an implementation that kept the old `if effectiveMode ==
// ModeAuto` guard writes nothing for them, so soleEntry fails before any field
// is looked at.
//
// The origin-membership check is the other half, and it is the one that
// catches a field left unpopulated. Origin's zero value is OriginUnset by
// design, so a record site that forgot to set the field gets a valid Go value
// and no error -- the entry simply says "unset", or nothing at all, and only
// an assertion naming the permitted five can tell.
func TestRun_AC22_EveryInstallRecordsOneOfFiveOrigins(t *testing.T) {
	cases := []struct {
		name     string
		mode     Mode
		origin   Origin
		verified bool
		resolver ProjectDeclarationResolver

		wantMode   string
		wantOrigin string
		wantGate   string
	}{
		{
			// The ordinary install: nothing configured, nothing declared, a
			// prompt answered. It is the row the enum assertion above needs
			// most, because "default" is the origin adjacent to the "unset"
			// that assertion exists to catch -- a record site special-casing
			// the resolved default into an unpopulated field is invisible to
			// every other row here.
			name: "the unset default, answered at the prompt",
			mode: ModeConfirm, origin: OriginDefault, verified: true,
			wantMode: "confirm", wantOrigin: "default",
		},
		{
			name: "auto set by the flag", mode: ModeAuto, origin: OriginFlag, verified: true,
			wantMode: "auto", wantOrigin: "flag",
		},
		{
			name: "confirm set by the flag", mode: ModeConfirm, origin: OriginFlag, verified: true,
			wantMode: "confirm", wantOrigin: "flag",
		},
		{
			name: "auto from the config, diverted by an unverified recipe",
			mode: ModeAuto, origin: OriginConfig, verified: false,
			wantMode: "confirm", wantOrigin: "config", wantGate: gateRecipeVerification,
		},
		{
			name: "confirm from the environment", mode: ModeConfirm, origin: OriginEnvironment, verified: true,
			wantMode: "confirm", wantOrigin: "environment",
		},
		{
			name: "a declaration raising the unset default",
			mode: ModeConfirm, origin: OriginDefault, verified: true, resolver: declaresJq(),
			wantMode: "auto", wantOrigin: "project",
		},
		{
			name: "a declaration raised, then diverted by an unverified recipe",
			mode: ModeConfirm, origin: OriginDefault, verified: false, resolver: declaresJq(),
			wantMode: "confirm", wantOrigin: "project", wantGate: gateRecipeVerification,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := soleEntry(t, runInstall(t, tc.mode, tc.origin, tc.verified, tc.resolver).homeDir)

			if !slices.Contains(recordedOrigins, got.Origin) {
				t.Errorf("the entry records origin %q, which is not one of %v.\n"+
					"An origin field nobody populated reads as \"unset\" or as "+
					"nothing, and neither says where the mode came from.",
					got.Origin, recordedOrigins)
			}
			if got.Origin != tc.wantOrigin {
				t.Errorf("origin = %q, want %q", got.Origin, tc.wantOrigin)
			}
			if got.Mode != tc.wantMode {
				t.Errorf("mode = %q, want %q", got.Mode, tc.wantMode)
			}
			if got.Gate != tc.wantGate {
				t.Errorf("gate = %q, want %q", got.Gate, tc.wantGate)
			}
			if got.Recipe != "jq" {
				t.Errorf("recipe = %q, want %q", got.Recipe, "jq")
			}
			// The action names the feature and is the same constant whichever
			// mode governed the run. DESIGN-auto-install.md specifies it that
			// way, and the confirm rows here are what keep the widening from
			// quietly turning it into a second spelling of the mode.
			if got.Action != "auto-install" {
				t.Errorf("action = %q, want %q on every row", got.Action, "auto-install")
			}
		})
	}
}

// AC24. An install a mode-lowering gate diverted away from auto, which then
// proceeded after a prompt, is in the log and names the gate that diverted it.
//
// All three registered gates rather than one, because a gate is named by an
// identifier and an implementation hard-coding a single string satisfies one
// row while failing the other two.
//
// The first row also checks that the identifier the record names is the one
// stderr announced. That agreement is by construction -- lowerMode prints and
// returns the same gate.id from one traversal -- so one row asserting it is
// the seam, not a property each row has to re-establish.
func TestRun_AC24_AGateDivertedInstallNamesTheGate(t *testing.T) {
	t.Run("the recipe-verification gate", func(t *testing.T) {
		got := runInstall(t, ModeAuto, OriginConfig, false, nil)
		entry := soleEntry(t, got.homeDir)

		if entry.Gate != gateRecipeVerification {
			t.Errorf("gate = %q, want %q", entry.Gate, gateRecipeVerification)
		}
		if entry.Mode != "confirm" {
			t.Errorf("mode = %q, want confirm: this run was diverted to a prompt", entry.Mode)
		}
		if !strings.Contains(got.stderr, entry.Gate) {
			t.Errorf("the record names gate %q, which stderr never announced:\n%s", entry.Gate, got.stderr)
		}
	})

	t.Run("the configuration-permission gate", func(t *testing.T) {
		r, _, stderr := newTestRunner(t)
		installer := &mockInstaller{}
		execRec := &execRecorder{}

		r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
			return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
		}
		r.Installer = installer
		r.Exec = execRec.exec
		r.RecipeHasVerification = func(_ string) bool { return true }
		r.ConsentReader = strings.NewReader("y\n")

		// Readable beyond its owner, which is what this gate fires on.
		if err := os.WriteFile(r.cfg.ConfigFile, []byte(""), 0o644); err != nil {
			t.Fatalf("writing config.toml: %v", err)
		}

		if err := r.Run(context.Background(), "jq", nil, ModeAuto, OriginEnvironment, nil); err != nil {
			t.Fatalf("Run() error = %v, want nil\nstderr:\n%s", err, stderr.String())
		}
		if !installer.called {
			t.Fatalf("nothing was installed\nstderr:\n%s", stderr.String())
		}

		entry := soleEntry(t, r.cfg.HomeDir)
		if entry.Gate != gateConfigPermissions {
			t.Errorf("gate = %q, want %q", entry.Gate, gateConfigPermissions)
		}
		if entry.Origin != "environment" {
			t.Errorf("origin = %q, want environment: a gate lowers the mode and is not itself an origin", entry.Origin)
		}
	})

	// The third registered gate, and the one that needs the fixture: it fires
	// on a command with more than one provider, and R17 forbids assembling a
	// multi-provider case by hand. Its condition is a property of the
	// candidate list rather than of the recipe, which is what makes it the
	// gate a write site reading the wrong thing is most likely to miss.
	t.Run("the multiple-provider gate", func(t *testing.T) {
		fx := indexfixture.New(t)
		r, installer, _, _, stderr := newFixtureRunner(t, fx)
		r.ConsentReader = strings.NewReader("y\n")

		// Nothing declared, so nothing narrows the two providers to one and
		// nothing raises the mode either -- auto is the mode this call passes.
		if err := r.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
			ModeAuto, OriginFlag, nil); err != nil {
			t.Fatalf("Run() error = %v, want nil\nstderr:\n%s", err, stderr.String())
		}
		if !installer.called {
			t.Fatalf("nothing was installed\nstderr:\n%s", stderr.String())
		}

		entry := soleEntry(t, fx.Cfg.HomeDir)
		if entry.Gate != gateMultipleProviders {
			t.Errorf("gate = %q, want %q", entry.Gate, gateMultipleProviders)
		}
		if entry.Mode != "confirm" {
			t.Errorf("mode = %q, want confirm: this run was diverted to a prompt", entry.Mode)
		}
		if entry.Origin != "flag" {
			t.Errorf("origin = %q, want flag", entry.Origin)
		}
	})
}

// The record is of installs that happened, which is why it is written below
// the install rather than above it. An entry written before Installer.Install
// runs would name a tool a failed install never put on the machine, and no
// assertion about a successful run can see the difference -- both orderings
// write exactly one entry for every install that worked.
func TestRun_AFailedInstallIsNotRecorded(t *testing.T) {
	r, _, _ := newTestRunner(t)
	r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
		return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
	}
	r.Installer = &mockInstaller{err: errors.New("the download failed")}
	r.Exec = (&execRecorder{}).exec
	r.RecipeHasVerification = func(_ string) bool { return true }

	if err := os.WriteFile(r.cfg.ConfigFile, []byte(""), 0o600); err != nil {
		t.Fatalf("writing config.toml: %v", err)
	}

	if err := r.Run(context.Background(), "jq", nil, ModeAuto, OriginFlag, nil); err == nil {
		t.Fatal("Run() error = nil, want the install failure")
	}
	if entries, written := auditLog(t, r.cfg.HomeDir); written {
		t.Errorf("the install failed but the record claims it happened: %+v", entries)
	}
}

// AC22, against the reading of it that records nothing.
//
// An explicitly set confirm and an auto a gate lowered to confirm reach the
// same dispatch and are recorded with the same mode. They are different
// states: the first is what the user asked for, the second is a run that was
// configured to install unattended and did not. The origin is the field that
// separates them, and an implementation writing the mode under that name --
// or deriving one from the other -- writes "confirm" in both rows and passes
// every assertion stated in terms of a single run.
func TestRun_AC22_TheOriginIsNotTheModeUnderAnotherName(t *testing.T) {
	asked := soleEntry(t, runInstall(t, ModeConfirm, OriginFlag, true, nil).homeDir)
	lowered := soleEntry(t, runInstall(t, ModeAuto, OriginConfig, false, nil).homeDir)

	if asked.Mode != lowered.Mode {
		t.Fatalf("these two runs recorded different modes (%q and %q), so the pair no longer "+
			"tests what the origin adds beyond the mode", asked.Mode, lowered.Mode)
	}
	if asked.Origin == lowered.Origin {
		t.Errorf("both runs recorded origin %q. A confirm the user set and a confirm a gate "+
			"produced are the same mode and different origins; a record that cannot tell them "+
			"apart has not recorded the origin.", asked.Origin)
	}
	if asked.Origin != "flag" {
		t.Errorf("the explicitly set run records origin %q, want flag", asked.Origin)
	}
	if lowered.Origin != "config" {
		t.Errorf("the diverted run records origin %q, want config: the origin is where the mode "+
			"was resolved from, before the gate ran", lowered.Origin)
	}
	if asked.Gate != "" {
		t.Errorf("no gate fired on the explicitly set run, but it recorded gate %q", asked.Gate)
	}
	if lowered.Gate == "" {
		t.Error("the diverted run recorded no gate, so nothing in the entry says the mode was lowered")
	}
}

// AC22, against the reading that records Run's origin parameter.
//
// Where a declaration raises the unset default, the origin is project. Nothing
// the caller passed says so -- the parameter is still OriginDefault -- and only
// elevate's second return value carries it. A record built from the parameter
// writes "default" here, which claims nobody set a mode for a run that
// installed unattended because a file in the working tree said to.
func TestRun_AC22_AnElevatedDeclarationRecordsProject(t *testing.T) {
	got := soleEntry(t, runInstall(t, ModeConfirm, OriginDefault, true, declaresJq()).homeDir)

	if got.Origin != "project" {
		t.Errorf("origin = %q, want project: the declaration raised the mode, and the origin "+
			"the record names is the elevation's, not the one Run was called with", got.Origin)
	}
	if got.Mode != "auto" {
		t.Errorf("mode = %q, want auto: the elevation is what this row is about", got.Mode)
	}
	if got.Version != "1.7.1" {
		t.Errorf("version = %q, want the declared 1.7.1", got.Version)
	}
}

// The other half of "every install": a run that installs nothing records
// nothing. Without this the criterion is met by writing an entry on every run,
// which turns the log into a record of invocations and loses the property that
// every line in it is a tool that reached the machine.
//
// The two fast paths are here alongside the two dispatches, and they are the
// rows that need saying. Suggest and a declined prompt return from below the
// mode dispatch, where a write site is plainly on the other side of a return;
// the fast paths return from above it, before any origin has been settled and
// before the gates have run. An entry written there would carry the "unset"
// that recordedOrigins exists to reject, for a tool that was already on the
// machine and that this run only executed.
func TestRun_RunsThatInstallNothingRecordNothing(t *testing.T) {
	t.Run("the already-installed fast path execs without installing", func(t *testing.T) {
		r, _, _ := newTestRunner(t)
		installer := &mockInstaller{}
		execRec := &execRecorder{}
		r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
			return []index.BinaryMatch{{Recipe: "jq", Command: "jq", Installed: true}}, nil
		}
		r.Installer = installer
		r.Exec = execRec.exec

		if err := r.Run(context.Background(), "jq", nil, ModeAuto, OriginFlag, nil); err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		if installer.called {
			t.Fatal("the fast path installed something")
		}
		if !execRec.called {
			t.Fatal("the fast path did not exec, so it is not the path under test")
		}
		if entries, written := auditLog(t, r.cfg.HomeDir); written {
			t.Errorf("an already-installed tool was executed, not installed, but recorded %+v", entries)
		}
	})

	t.Run("the declared fast path execs the declared version without installing", func(t *testing.T) {
		r, _, _ := newTestRunner(t)
		installer := &mockInstaller{}
		execRec := &execRecorder{}
		r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
			return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
		}
		r.Installer = installer
		r.Exec = execRec.exec
		layDownTool(t, r.cfg, "jq", "1.7.1", "jq")

		if err := r.Run(context.Background(), "jq", nil, ModeConfirm, OriginDefault, declaresJq()); err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
		if installer.called {
			t.Fatal("the declared fast path installed something")
		}
		if !execRec.called {
			t.Fatal("the declared fast path did not exec, so it is not the path under test")
		}
		if entries, written := auditLog(t, r.cfg.HomeDir); written {
			t.Errorf("the declared version was already present and only executed, but recorded %+v", entries)
		}
	})

	t.Run("suggest prints an instruction and installs nothing", func(t *testing.T) {
		r, _, _ := newTestRunner(t)
		r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
			return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
		}
		r.Installer = &mockInstaller{}
		r.Exec = (&execRecorder{}).exec

		if err := r.Run(context.Background(), "jq", nil, ModeSuggest, OriginFlag, nil); !errors.Is(err, ErrSuggestOnly) {
			t.Fatalf("Run() error = %v, want ErrSuggestOnly", err)
		}
		if entries, written := auditLog(t, r.cfg.HomeDir); written {
			t.Errorf("suggest installed nothing but recorded %+v", entries)
		}
	})

	t.Run("a declined prompt installs nothing", func(t *testing.T) {
		r, _, _ := newTestRunner(t)
		installer := &mockInstaller{}
		r.Lookup = func(_ context.Context, _ string) ([]index.BinaryMatch, error) {
			return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}, nil
		}
		r.Installer = installer
		r.Exec = (&execRecorder{}).exec
		r.ConsentReader = strings.NewReader("n\n")

		if err := r.Run(context.Background(), "jq", nil, ModeConfirm, OriginFlag, nil); !errors.Is(err, ErrUserDeclined) {
			t.Fatalf("Run() error = %v, want ErrUserDeclined", err)
		}
		if installer.called {
			t.Fatal("the installer ran after the prompt was declined")
		}
		if entries, written := auditLog(t, r.cfg.HomeDir); written {
			t.Errorf("a declined run installed nothing but recorded %+v", entries)
		}
	})
}
