package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// TestProvenanceRecordsTheApprovalRoute covers R16's first half: the record
// says how the source got in, and the two routes are distinguishable.
func TestProvenanceRecordsTheApprovalRoute(t *testing.T) {
	for name, tc := range map[string]struct {
		inputs consentInputs
		want   string
	}{
		"interactive yes": {
			inputs: consentInputs{Interactive: func() bool { return true }, Ask: func(string) bool { return true }},
			want:   userconfig.ApprovedViaPrompt,
		},
		"--yes": {
			inputs: consentInputs{AutoApprove: true, Interactive: func() bool { return false }},
			want:   userconfig.ApprovedViaYesFlag,
		},
	} {
		t.Run(name, func(t *testing.T) {
			isolatedConfig(t)
			declaring := filepath.Join(t.TempDir(), ".tsuku.toml")
			if err := os.WriteFile(declaring, []byte("[tools]\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			plan := newProjectSourcePlan(declaring)
			plan.add("owner/repo", "owner/repo:tool")
			plan.classify()
			plan.decideConsent(tc.inputs)
			if err := plan.commit(); err != nil {
				t.Fatalf("commit: %v", err)
			}

			cfg, err := userconfig.Load()
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			entry := cfg.Registries["owner/repo"]
			if entry.ApprovedVia != tc.want {
				t.Errorf("approved_via = %q, want %q", entry.ApprovedVia, tc.want)
			}
			if entry.DeclaredIn == "" {
				t.Error("declared_in was not recorded")
			}
			if !filepath.IsAbs(entry.DeclaredIn) {
				t.Errorf("declared_in is not absolute: %q", entry.DeclaredIn)
			}
			if !entry.AutoRegistered {
				t.Error("the entry lost its auto-registration flag")
			}
		})
	}
}

// TestProvenanceIsWrittenOnce covers R16's second half: a later install does
// not rewrite how a source first got in.
func TestProvenanceIsWrittenOnce(t *testing.T) {
	isolatedConfig(t)
	first := filepath.Join(t.TempDir(), ".tsuku.toml")
	if err := os.WriteFile(first, []byte("[tools]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := newProjectSourcePlan(first)
	plan.add("owner/repo", "owner/repo:tool")
	plan.classify()
	plan.decideConsent(consentInputs{Interactive: func() bool { return true }, Ask: func(string) bool { return true }})
	if err := plan.commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	cfg, _ := userconfig.Load()
	original := cfg.Registries["owner/repo"]

	// A second project, approved a different way, naming the same source.
	second := filepath.Join(t.TempDir(), ".tsuku.toml")
	if err := os.WriteFile(second, []byte("[tools]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan2 := newProjectSourcePlan(second)
	plan2.add("owner/repo", "owner/repo:tool")
	plan2.classify()
	if st, _ := plan2.state("owner/repo"); st != sourceRegistered {
		t.Fatalf("an already-registered source was asked about again: state %d", st)
	}
	plan2.decideConsent(consentInputs{AutoApprove: true, Interactive: func() bool { return false }})
	if err := plan2.commit(); err != nil {
		t.Fatalf("second commit: %v", err)
	}

	cfg2, _ := userconfig.Load()
	if got := cfg2.Registries["owner/repo"]; got != original {
		t.Errorf("the record was rewritten by a later install:\nwas:  %+v\nnow:  %+v", original, got)
	}
}

// TestOtherRegistrationRoutesRecordNothing covers R17's byte-for-byte promise
// for the two paths that are the user's own action, and for a command-line
// install.
func TestOtherRegistrationRoutesRecordNothing(t *testing.T) {
	isolatedConfig(t)

	cfg, err := userconfig.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := autoRegisterSource(cfg, sourceProvenance{}, "owner/repo"); err != nil {
		t.Fatalf("register: %v", err)
	}

	reloaded, err := userconfig.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	entry := reloaded.Registries["owner/repo"]
	if entry.ApprovedVia != "" || entry.DeclaredIn != "" {
		t.Errorf("a command-line registration carried a project record: %+v", entry)
	}
	if !entry.AutoRegistered {
		t.Error("the entry lost its auto-registration flag")
	}
}

// TestPreviousVersionConfigLoadsUnchanged is the compatibility assertion.
//
// An entry written before these fields existed has to load, keep its
// annotation, and -- the part that is easy to get wrong -- survive a save
// without gaining empty scaffolding. A nested table without omitempty would
// write a bare header into every such entry the first time anything saved.
func TestPreviousVersionConfigLoadsUnchanged(t *testing.T) {
	cfgPath := isolatedConfig(t)

	old := "telemetry = true\n\n[registries]\n  [registries.\"owner/repo\"]\n    url = \"https://github.com/owner/repo\"\n    auto_registered = true\n"
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := userconfig.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	entry := cfg.Registries["owner/repo"]
	if entry.URL != "https://github.com/owner/repo" || !entry.AutoRegistered {
		t.Fatalf("the old entry did not survive the load: %+v", entry)
	}
	if entry.ApprovedVia != "" || entry.DeclaredIn != "" {
		t.Errorf("the old entry gained a record it never had: %+v", entry)
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	saved, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"approved_via", "declared_in"} {
		if strings.Contains(string(saved), unwanted) {
			t.Errorf("saving added %q to an entry that has none:\n%s", unwanted, saved)
		}
	}
}

// TestProvenanceLineRendering covers what `tsuku registry list` shows.
func TestProvenanceLineRendering(t *testing.T) {
	t.Run("no record renders no line", func(t *testing.T) {
		if line := provenanceLine(userconfig.RegistryEntry{AutoRegistered: true}); line != "" {
			t.Errorf("an entry with no record got a line: %q", line)
		}
	})

	t.Run("the prompt route reads in words", func(t *testing.T) {
		line := provenanceLine(userconfig.RegistryEntry{
			ApprovedVia: userconfig.ApprovedViaPrompt,
			DeclaredIn:  "/work/.tsuku.toml",
		})
		if !strings.Contains(line, "approved at the prompt") {
			t.Errorf("the route was rendered as a raw token: %q", line)
		}
		if !strings.Contains(line, `"/work/.tsuku.toml"`) {
			t.Errorf("the declaring path is not quoted: %q", line)
		}
	})

	t.Run("an unexpected value is shown rather than dropped", func(t *testing.T) {
		line := provenanceLine(userconfig.RegistryEntry{ApprovedVia: "hand-edited"})
		if !strings.Contains(line, "hand-edited") {
			t.Errorf("a hand-edited value was dropped: %q", line)
		}
	})

	t.Run("a hostile path reaches the terminal quoted", func(t *testing.T) {
		line := provenanceLine(userconfig.RegistryEntry{
			ApprovedVia: userconfig.ApprovedViaYesFlag,
			DeclaredIn:  "/work/a\nb\x1b[31m/.tsuku.toml",
		})
		if strings.Contains(line, "\x1b") {
			t.Errorf("a raw escape sequence would reach the terminal: %q", line)
		}
		if strings.Contains(line, "\n") {
			t.Errorf("a raw newline would break the listing into two lines: %q", line)
		}
	})
}

// TestRegistryListKeepsItsFirstLine pins that nothing about the existing output
// moved. An entry with no record is exactly what it was.
func TestRegistryListKeepsItsFirstLine(t *testing.T) {
	isolatedConfig(t)

	cfg, err := userconfig.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Registries == nil {
		cfg.Registries = map[string]userconfig.RegistryEntry{}
	}
	cfg.Registries["plain/one"] = userconfig.RegistryEntry{URL: "https://github.com/plain/one", AutoRegistered: true}
	cfg.Registries["recorded/two"] = userconfig.RegistryEntry{
		URL: "https://github.com/recorded/two", AutoRegistered: true,
		ApprovedVia: userconfig.ApprovedViaYesFlag, DeclaredIn: "/work/.tsuku.toml",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	outcome := runCommandOutcome(t, func() error {
		runRegistryList(nil, nil)
		return nil
	})

	lines := strings.Split(outcome.stdout, "\n")
	followedByRecord := func(name string) bool {
		for i, line := range lines {
			if !strings.Contains(line, name) {
				continue
			}
			return i+1 < len(lines) && strings.HasPrefix(lines[i+1], "      ")
		}
		t.Fatalf("%s is not in the listing:\n%s", name, outcome.stdout)
		return false
	}

	if followedByRecord("plain/one") {
		t.Errorf("an entry with no record got a second line:\n%s", outcome.stdout)
	}
	if !followedByRecord("recorded/two") {
		t.Errorf("an entry with a record got no second line:\n%s", outcome.stdout)
	}
	if !strings.Contains(outcome.stdout, "(auto-registered)") {
		t.Errorf("the auto-registered annotation is gone:\n%s", outcome.stdout)
	}
}
