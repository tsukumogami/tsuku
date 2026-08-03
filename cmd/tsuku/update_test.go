package main

import (
	"errors"
	"fmt"
	"testing"

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
func recordUpdateVersion(t *testing.T, mgr *install.Manager, tool, version string, actions []install.CleanupAction) {
	t.Helper()

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

// TestUpdateWithCleanup pins the ordering contract the three update paths share:
// the snapshot is taken before the install, the reconcile runs after it, and a
// failed install reconciles nothing.
//
// Ordering is the whole point of the helper. A reconcile run against a snapshot
// taken after the install would compare the new version with itself, find no
// change, and stay silent -- indistinguishable from a clean update.
func TestUpdateWithCleanup(t *testing.T) {
	tests := []struct {
		name       string
		installErr error
		install    func(t *testing.T, mgr *install.Manager)
		wantWarns  []string
		wantErr    bool
	}{
		{
			name: "a shell init rewrite during the install is announced",
			install: func(t *testing.T, mgr *install.Manager) {
				recordUpdateVersion(t, mgr, "tool", "2.0.0", []install.CleanupAction{
					updateFragment("tool", "2.0.0", "bash", "after"),
				})
			},
			wantWarns: []string{"shell init changed for tool (bash)"},
		},
		{
			name: "an unchanged shell init says nothing",
			install: func(t *testing.T, mgr *install.Manager) {
				recordUpdateVersion(t, mgr, "tool", "2.0.0", []install.CleanupAction{
					updateFragment("tool", "2.0.0", "bash", "before"),
				})
			},
		},
		{
			name:       "a failed install reconciles nothing",
			installErr: errors.New("download failed"),
			install: func(t *testing.T, mgr *install.Manager) {
				// A partial install that got as far as recording state is the
				// case worth pinning: the error still has to suppress the
				// reconcile, or an update that did not land warns about a
				// change the user never received.
				recordUpdateVersion(t, mgr, "tool", "2.0.0", []install.CleanupAction{
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
			recordUpdateVersion(t, mgr, "tool", "1.0.0", []install.CleanupAction{
				updateFragment("tool", "1.0.0", "bash", "before"),
			})

			var warns []string
			installed := false
			err := updateWithCleanup(mgr, "tool", func(format string, args ...any) {
				warns = append(warns, fmt.Sprintf(format, args...))
			}, func() error {
				installed = true
				tt.install(t, mgr)
				return tt.installErr
			})

			if (err != nil) != tt.wantErr {
				t.Fatalf("updateWithCleanup() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !installed {
				t.Fatal("updateWithCleanup() never ran the install")
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
