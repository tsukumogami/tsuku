package install

import (
	"testing"
)

func shellDCleanup(path, hash string) CleanupAction {
	return CleanupAction{Action: "delete_file", Path: path, ContentHash: hash}
}

func TestBuildShellDSelection(t *testing.T) {
	tests := []struct {
		name      string
		state     *State
		wantAct   map[string]string
		wantKnown map[string]string
	}{
		{
			name:      "nil state",
			state:     nil,
			wantAct:   map[string]string{},
			wantKnown: map[string]string{},
		},
		{
			name: "single active version",
			state: &State{Installed: map[string]ToolState{
				"nvm": {
					ActiveVersion: "0.40.6",
					Versions: map[string]VersionState{
						"0.40.6": {CleanupActions: []CleanupAction{
							shellDCleanup("share/shell.d/nvm@0.40.6.bash", "aaa"),
						}},
					},
				},
			}},
			wantAct:   map[string]string{"share/shell.d/nvm@0.40.6.bash": "aaa"},
			wantKnown: map[string]string{"share/shell.d/nvm@0.40.6.bash": "aaa"},
		},
		{
			name: "an inactive version lands in Known only",
			state: &State{Installed: map[string]ToolState{
				"nvm": {
					ActiveVersion: "0.40.6",
					Versions: map[string]VersionState{
						"0.40.5": {CleanupActions: []CleanupAction{
							shellDCleanup("share/shell.d/nvm@0.40.5.bash", "old"),
						}},
						"0.40.6": {CleanupActions: []CleanupAction{
							shellDCleanup("share/shell.d/nvm@0.40.6.bash", "new"),
						}},
					},
				},
			}},
			wantAct: map[string]string{"share/shell.d/nvm@0.40.6.bash": "new"},
			wantKnown: map[string]string{
				"share/shell.d/nvm@0.40.5.bash": "old",
				"share/shell.d/nvm@0.40.6.bash": "new",
			},
		},
		{
			name: "non-shell.d cleanup paths are ignored",
			state: &State{Installed: map[string]ToolState{
				"nvm": {
					ActiveVersion: "0.40.6",
					Versions: map[string]VersionState{
						"0.40.6": {CleanupActions: []CleanupAction{
							shellDCleanup("share/completions/nvm.bash", "aaa"),
							shellDCleanup("share/shell.d/nvm@0.40.6.bash", "bbb"),
						}},
					},
				},
			}},
			wantAct:   map[string]string{"share/shell.d/nvm@0.40.6.bash": "bbb"},
			wantKnown: map[string]string{"share/shell.d/nvm@0.40.6.bash": "bbb"},
		},
		{
			name: "legacy state files fall back to the deprecated Version field",
			state: &State{Installed: map[string]ToolState{
				"nvm": {
					Version: "0.40.6",
					Versions: map[string]VersionState{
						"0.40.6": {CleanupActions: []CleanupAction{
							shellDCleanup("share/shell.d/nvm.bash", "legacy"),
						}},
					},
				},
			}},
			wantAct:   map[string]string{"share/shell.d/nvm.bash": "legacy"},
			wantKnown: map[string]string{"share/shell.d/nvm.bash": "legacy"},
		},
		{
			name: "a version with no active marker is Known only",
			state: &State{Installed: map[string]ToolState{
				"nvm": {
					Versions: map[string]VersionState{
						"0.40.6": {CleanupActions: []CleanupAction{
							shellDCleanup("share/shell.d/nvm@0.40.6.bash", "orphan"),
						}},
					},
				},
			}},
			wantAct:   map[string]string{},
			wantKnown: map[string]string{"share/shell.d/nvm@0.40.6.bash": "orphan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildShellDSelection(tt.state)
			assertStringMap(t, "Active", got.Active, tt.wantAct)
			assertStringMap(t, "Known", got.Known, tt.wantKnown)
		})
	}
}

func assertStringMap(t *testing.T, label string, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s[%q] = %q, want %q", label, k, got[k], v)
		}
	}
}

func TestShellsRecordedFor(t *testing.T) {
	ts := ToolState{
		ActiveVersion: "2.0.0",
		Versions: map[string]VersionState{
			"1.0.0": {CleanupActions: []CleanupAction{
				shellDCleanup("share/shell.d/tool@1.0.0.fish", ""),
			}},
			"2.0.0": {CleanupActions: []CleanupAction{
				shellDCleanup("share/shell.d/tool@2.0.0.bash", ""),
				shellDCleanup("share/completions/tool.zsh", ""),
			}},
		},
	}

	// Every version counts, not only the active one: switching away from a
	// version is exactly when its shell needs a rebuild.
	got := shellsRecordedFor(ts)
	if len(got) != 2 || !got["bash"] || !got["fish"] {
		t.Errorf("shellsRecordedFor() = %v, want {bash, fish}", got)
	}
}

func TestTargetFromCleanupPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"share/shell.d/nvm@0.40.6.bash", "nvm"},
		{"share/shell.d/nvm@0.40.5.bash", "nvm"},
		{"share/shell.d/00-env-nvm@0.40.6.zsh", "00-env-nvm"},
		{"share/shell.d/legacy.bash", "legacy"},
		{"share/completions/nvm.bash", ""},
		{"bin/nvm", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := TargetFromCleanupPath(tt.path); got != tt.want {
				t.Errorf("TargetFromCleanupPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
