package main

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/testutil"
)

func TestUpdateOutcomeMessage(t *testing.T) {
	cases := []struct {
		name   string
		tool   string
		oldVer string
		newVer string
		want   string
	}{
		{
			name:   "no version (defensive)",
			tool:   "kubectl",
			oldVer: "1.30.0",
			newVer: "",
			want:   "",
		},
		{
			name:   "already at latest",
			tool:   "nodejs",
			oldVer: "25.9.0",
			newVer: "25.9.0",
			want:   "nodejs is already at the latest version (25.9.0).",
		},
		{
			// Real updates already get a permanent "✅ <name>@<version>"
			// line from the install reporter (#2280); no extra line
			// from updateOutcomeMessage.
			name:   "updated to a newer version yields no extra line",
			tool:   "kubectl",
			oldVer: "1.30.0",
			newVer: "1.31.0",
			want:   "",
		},
		{
			// Same: a fresh install (empty old version) is "different
			// from previous", install reporter handles its own line.
			name:   "first install yields no extra line",
			tool:   "kubectl",
			oldVer: "",
			newVer: "1.31.0",
			want:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := updateOutcomeMessage(tc.tool, tc.oldVer, tc.newVer)
			if got != tc.want {
				t.Errorf("updateOutcomeMessage(%q, %q, %q) = %q, want %q",
					tc.tool, tc.oldVer, tc.newVer, got, tc.want)
			}
		})
	}
}

// recordUpdateVersion writes a version's state directly and makes it active,
// standing in for the install that would normally do so.
//
// The version directory has to exist alongside the state entry: Manager.List
// skips a version state records but disk does not have, so a state-only entry
// would be invisible to the version readback.
func recordUpdateVersion(t *testing.T, cfg *config.Config, mgr *install.Manager, tool, version string, actions []install.CleanupAction) {
	t.Helper()

	if err := os.MkdirAll(cfg.ToolDir(tool, version), 0755); err != nil {
		t.Fatalf("creating the version directory for %s@%s: %v", tool, version, err)
	}

	if err := mgr.GetState().UpdateTool(tool, func(ts *install.ToolState) {
		if ts.Versions == nil {
			ts.Versions = map[string]install.VersionState{}
		}
		ts.Versions[version] = install.VersionState{CleanupActions: actions}
		ts.ActiveVersion = version
	}); err != nil {
		t.Fatalf("UpdateTool(%s@%s) error = %v", tool, version, err)
	}
}

// updateFragment builds the cleanup action a version-keyed shell.d fragment
// records, the same shape install_shell_init produces.
func updateFragment(target, version, shell, hash string) install.CleanupAction {
	return install.CleanupAction{
		Action:      "delete_file",
		Path:        "share/shell.d/" + target + "@" + version + "." + shell,
		ContentHash: hash,
	}
}

// collectWarns returns a WarnFunc that appends formatted messages to into, so a
// test can assert on the message a user would see rather than on stderr.
func collectWarns(into *[]string) install.WarnFunc {
	return func(format string, args ...any) {
		*into = append(*into, fmt.Sprintf(format, args...))
	}
}

// TestUpdateInstalledTool pins the ordering contract both foreground update
// commands share: the snapshot is taken before the install, the reconcile runs
// after it, a failed install reconciles nothing, and the version reported back
// is the one that ended up active.
//
// Ordering is the whole point of the helper. A reconcile run against a snapshot
// taken after the install would compare the new version with itself, find no
// change, and stay silent -- indistinguishable from a clean update.
func TestUpdateInstalledTool(t *testing.T) {
	tests := []struct {
		name        string
		installErr  error
		install     func(t *testing.T, cfg *config.Config, mgr *install.Manager)
		wantWarns   []string
		wantVersion string
		wantErr     bool
	}{
		{
			name: "a shell init rewrite during the install is announced",
			install: func(t *testing.T, cfg *config.Config, mgr *install.Manager) {
				recordUpdateVersion(t, cfg, mgr, "tool", "2.0.0", []install.CleanupAction{
					updateFragment("tool", "2.0.0", "bash", "after"),
				})
			},
			wantWarns:   []string{"shell init changed for tool (bash)"},
			wantVersion: "2.0.0",
		},
		{
			name: "an unchanged shell init says nothing",
			install: func(t *testing.T, cfg *config.Config, mgr *install.Manager) {
				recordUpdateVersion(t, cfg, mgr, "tool", "2.0.0", []install.CleanupAction{
					updateFragment("tool", "2.0.0", "bash", "before"),
				})
			},
			wantVersion: "2.0.0",
		},
		{
			name:    "an install that changed nothing reports the version already active",
			install: func(*testing.T, *config.Config, *install.Manager) {},
			// The tool was already at latest: no warning, and the version the
			// caller reads back is the one it started on, which is how
			// `--all` tells "up to date" from "updated".
			wantVersion: "1.0.0",
		},
		{
			name:       "a failed install reconciles nothing",
			installErr: errors.New("download failed"),
			install: func(t *testing.T, cfg *config.Config, mgr *install.Manager) {
				// A partial install that got as far as recording state is the
				// case worth pinning: the error still has to suppress the
				// reconcile, or an update that did not land warns about a
				// change the user never received.
				recordUpdateVersion(t, cfg, mgr, "tool", "2.0.0", []install.CleanupAction{
					updateFragment("tool", "2.0.0", "bash", "after"),
				})
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, cleanup := testutil.NewTestConfig(t)
			defer cleanup()

			mgr := install.New(cfg)
			recordUpdateVersion(t, cfg, mgr, "tool", "1.0.0", []install.CleanupAction{
				updateFragment("tool", "1.0.0", "bash", "before"),
			})

			var warns []string
			installed := false
			gotVersion, err := updateInstalledTool(mgr, "tool", collectWarns(&warns), func() error {
				installed = true
				tt.install(t, cfg, mgr)
				return tt.installErr
			})

			if (err != nil) != tt.wantErr {
				t.Fatalf("updateInstalledTool() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !installed {
				t.Fatal("updateInstalledTool() never ran the install")
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("updateInstalledTool() version = %q, want %q", gotVersion, tt.wantVersion)
			}
			if len(warns) != len(tt.wantWarns) {
				t.Fatalf("warnings = %q, want %q", warns, tt.wantWarns)
			}
			for i, want := range tt.wantWarns {
				if warns[i] != want {
					t.Errorf("warning %d = %q, want %q", i, warns[i], want)
				}
			}
		})
	}
}
