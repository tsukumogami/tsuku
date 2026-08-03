package install

import (
	"reflect"
	"testing"

	"github.com/tsukumogami/tsuku/internal/shellenv"
)

func TestBuildManagedBinaries(t *testing.T) {
	tests := []struct {
		name  string
		state *State
		want  []shellenv.ManagedBinaries
	}{
		{
			name:  "nil state yields nothing",
			state: nil,
		},
		{
			name:  "empty state yields nothing",
			state: &State{Installed: map[string]ToolState{}},
		},
		{
			name: "the active version's binaries are used",
			state: &State{Installed: map[string]ToolState{
				"ripgrep": {
					ActiveVersion: "14.1.0",
					Versions: map[string]VersionState{
						"14.1.0": {Binaries: []string{"rg"}},
						"13.0.0": {Binaries: []string{"rg-old"}},
					},
				},
			}},
			want: []shellenv.ManagedBinaries{{Tool: "ripgrep", Binaries: []string{"rg"}}},
		},
		{
			name: "a hidden tool is excluded",
			state: &State{Installed: map[string]ToolState{
				"node": {
					ActiveVersion: "22.0.0",
					IsHidden:      true,
					Versions:      map[string]VersionState{"22.0.0": {Binaries: []string{"node", "npm"}}},
				},
				"koto": {
					ActiveVersion: "0.11.3",
					Versions:      map[string]VersionState{"0.11.3": {Binaries: []string{"koto"}}},
				},
			}},
			want: []shellenv.ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}},
		},
		{
			name: "a visible tool installed as a dependency is included",
			state: &State{Installed: map[string]ToolState{
				"jq": {
					ActiveVersion: "1.7",
					IsExplicit:    false,
					RequiredBy:    []string{"somethingelse"},
					Versions:      map[string]VersionState{"1.7": {Binaries: []string{"jq"}}},
				},
			}},
			want: []shellenv.ManagedBinaries{{Tool: "jq", Binaries: []string{"jq"}}},
		},
		{
			name: "the deprecated tool-level list is the first fallback",
			state: &State{Installed: map[string]ToolState{
				"ripgrep": {
					ActiveVersion: "14.1.0",
					Binaries:      []string{"rg"},
					Versions:      map[string]VersionState{"14.1.0": {}},
				},
			}},
			want: []shellenv.ManagedBinaries{{Tool: "ripgrep", Binaries: []string{"rg"}}},
		},
		{
			name: "the tool's own name is the last fallback",
			state: &State{Installed: map[string]ToolState{
				"koto": {
					ActiveVersion: "0.11.3",
					Versions:      map[string]VersionState{"0.11.3": {}},
				},
			}},
			want: []shellenv.ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}},
		},
		{
			name: "a legacy state file's Version field names the active version",
			state: &State{Installed: map[string]ToolState{
				"koto": {
					Version:  "0.10.0",
					Versions: map[string]VersionState{"0.10.0": {Binaries: []string{"koto"}}},
				},
			}},
			want: []shellenv.ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}},
		},
		{
			name: "a recorded path is reduced to the basename that goes on PATH",
			state: &State{Installed: map[string]ToolState{
				"ripgrep": {
					ActiveVersion: "14.1.0",
					Versions:      map[string]VersionState{"14.1.0": {Binaries: []string{"bin/rg"}}},
				},
			}},
			want: []shellenv.ManagedBinaries{{Tool: "ripgrep", Binaries: []string{"rg"}}},
		},
		{
			name: "duplicate binary names collapse",
			state: &State{Installed: map[string]ToolState{
				"ripgrep": {
					ActiveVersion: "14.1.0",
					Versions:      map[string]VersionState{"14.1.0": {Binaries: []string{"rg", "bin/rg"}}},
				},
			}},
			want: []shellenv.ManagedBinaries{{Tool: "ripgrep", Binaries: []string{"rg"}}},
		},
		{
			name: "output is ordered by tool name",
			state: &State{Installed: map[string]ToolState{
				"zoxide":  {ActiveVersion: "1", Versions: map[string]VersionState{"1": {Binaries: []string{"z"}}}},
				"atuin":   {ActiveVersion: "1", Versions: map[string]VersionState{"1": {Binaries: []string{"atuin"}}}},
				"koto":    {ActiveVersion: "1", Versions: map[string]VersionState{"1": {Binaries: []string{"koto"}}}},
				"shirabe": {ActiveVersion: "1", Versions: map[string]VersionState{"1": {Binaries: []string{"shirabe"}}}},
			}},
			want: []shellenv.ManagedBinaries{
				{Tool: "atuin", Binaries: []string{"atuin"}},
				{Tool: "koto", Binaries: []string{"koto"}},
				{Tool: "shirabe", Binaries: []string{"shirabe"}},
				{Tool: "zoxide", Binaries: []string{"z"}},
			},
		},
		{
			name: "a tool providing several binaries lists them all",
			state: &State{Installed: map[string]ToolState{
				"git": {
					ActiveVersion: "2.45.0",
					Versions:      map[string]VersionState{"2.45.0": {Binaries: []string{"git", "git-lfs"}}},
				},
			}},
			want: []shellenv.ManagedBinaries{{Tool: "git", Binaries: []string{"git", "git-lfs"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildManagedBinaries(tt.state)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildManagedBinaries() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestBuildManagedBinariesOrderIsStable guards against the map-iteration order
// leaking into doctor's output, which would make two runs of an unchanged
// environment disagree.
func TestBuildManagedBinariesOrderIsStable(t *testing.T) {
	state := &State{Installed: map[string]ToolState{
		"zoxide":  {ActiveVersion: "1", Versions: map[string]VersionState{"1": {Binaries: []string{"z"}}}},
		"atuin":   {ActiveVersion: "1", Versions: map[string]VersionState{"1": {Binaries: []string{"atuin"}}}},
		"koto":    {ActiveVersion: "1", Versions: map[string]VersionState{"1": {Binaries: []string{"koto"}}}},
		"shirabe": {ActiveVersion: "1", Versions: map[string]VersionState{"1": {Binaries: []string{"shirabe"}}}},
		"jq":      {ActiveVersion: "1", Versions: map[string]VersionState{"1": {Binaries: []string{"jq"}}}},
	}}

	first := BuildManagedBinaries(state)
	for i := 0; i < 20; i++ {
		if got := BuildManagedBinaries(state); !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d returned %+v, want %+v", i, got, first)
		}
	}
}
