package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/indexfixture"
)

// AC23. Where more than one source supplies a consent mode, the origin the
// record names is the highest-precedence one that supplied a value.
//
// This lives here rather than in internal/autoinstall because the precedence
// is resolveMode's, and internal/autoinstall never sees it: the runner is
// handed one origin and records it. A package that only ever receives the
// answer cannot tell a right ordering from a wrong one, so a corpus stated
// entirely there is satisfied by an implementation that ranks the sources
// backwards.
//
// The rows contest each adjacent pair in the order at least once -- flag
// against environment, environment against config, config against default --
// which is what separates a wrong ordering of any two of them from an
// implementation that simply always writes the same value. A single row would
// not: with only "everything set, expect flag" on the books, swapping
// environment and config below the winner changes nothing observable.
//
// Every row asks for auto from every source it sets, and the reason is that
// the assertion should be about the origin alone. Rows that varied the mode
// as well would fail through a second channel -- a losing suggest would print
// an instruction and install nothing, so the failure would report a missing
// audit log rather than a misordered origin. Here all three orderings install
// the same tool the same way and differ only in the field under test.

// auditOrigin returns the origin recorded by the single audit entry the run
// wrote, failing if it wrote none.
func auditOrigin(t *testing.T, cfg *config.Config) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(cfg.HomeDir, "audit.log"))
	if err != nil {
		t.Fatalf("reading the audit log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("the audit log holds %d entries, want 1:\n%s", len(lines), data)
	}

	var entry struct {
		Mode   string `json:"mode"`
		Origin string `json:"origin"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("the audit log is not NDJSON: %v\nline: %s", err, lines[0])
	}
	if entry.Mode != "auto" {
		t.Fatalf("this run recorded mode %q rather than auto, so it did not reach the "+
			"state the contest is stated in", entry.Mode)
	}
	return entry.Origin
}

func TestRunCmd_AC23_TheRecordedOriginIsTheHighestPrecedenceSource(t *testing.T) {
	cases := []struct {
		name       string
		route      consentRoute
		wantOrigin string
		// loser is the source the row contests the winner against, named in
		// the failure so a wrong ordering reads as one.
		loser string
	}{
		{
			name: "the flag outranks the environment variable",
			route: func(t *testing.T, cfg *config.Config) {
				byFlag("auto")(t, cfg)
				byEnvironment("auto")(t, cfg)
				byConfig("auto")(t, cfg)
			},
			wantOrigin: "flag",
			loser:      "environment",
		},
		{
			name: "the environment variable outranks the config key",
			route: func(t *testing.T, cfg *config.Config) {
				byEnvironment("auto")(t, cfg)
				// Also what the escalation restriction requires before an
				// environment-supplied auto takes effect at all, so this row
				// is the corroborated case rather than the blocked one.
				byConfig("auto")(t, cfg)
			},
			wantOrigin: "environment",
			loser:      "config",
		},
		{
			// Nothing else is set, so the alternative here is the unset
			// default -- and under a project declaration that alternative is
			// visible rather than silent: an implementation ranking default
			// above config lets the elevation raise it and records "project".
			name:       "the config key outranks the unset default",
			route:      byConfig("auto"),
			wantOrigin: "config",
			loser:      "default",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runDeclaredCommand(t, tc.route, false)

			if got.err != nil {
				t.Fatalf("Run() error = %v, want nil\nstdout:\n%s\nstderr:\n%s",
					got.err, got.stdout, got.stderr)
			}
			if got.installer.recipe != indexfixture.DeclaredRecipe {
				t.Fatalf("installed %q, want the declared recipe %q", got.installer.recipe, indexfixture.DeclaredRecipe)
			}

			if origin := auditOrigin(t, got.cfg); origin != tc.wantOrigin {
				t.Errorf("the record names origin %q, want %q: %s supplied a mode too and is "+
					"outranked by %s, so recording it means the sources are ordered wrongly",
					origin, tc.wantOrigin, tc.loser, tc.wantOrigin)
			}
		})
	}
}
