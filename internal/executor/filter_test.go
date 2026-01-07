package executor

import (
	"testing"

	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

func TestFilterStepsByTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		steps    []recipe.Step
		target   platform.Target
		wantLen  int
		wantActs []string // expected action names in order
	}{
		{
			name:     "empty steps returns empty",
			steps:    []recipe.Step{},
			target:   platform.NewTarget("linux/amd64", "debian"),
			wantLen:  0,
			wantActs: nil,
		},
		{
			name: "steps with no constraints pass through",
			steps: []recipe.Step{
				{Action: "download_file", Params: map[string]interface{}{}},
				{Action: "extract", Params: map[string]interface{}{}},
			},
			target:   platform.NewTarget("linux/amd64", "debian"),
			wantLen:  2,
			wantActs: []string{"download_file", "extract"},
		},
		{
			name: "apt_install filtered out for rhel target",
			steps: []recipe.Step{
				{Action: "apt_install", Params: map[string]interface{}{"packages": []interface{}{"curl"}}},
			},
			target:   platform.NewTarget("linux/amd64", "rhel"),
			wantLen:  0,
			wantActs: nil,
		},
		{
			name: "brew_cask filtered out for linux/amd64 target",
			steps: []recipe.Step{
				{Action: "brew_cask", Params: map[string]interface{}{"packages": []interface{}{"docker"}}},
			},
			target:   platform.NewTarget("linux/amd64", "debian"),
			wantLen:  0,
			wantActs: nil,
		},
		{
			name: "apt_install passes for debian target",
			steps: []recipe.Step{
				{Action: "apt_install", Params: map[string]interface{}{"packages": []interface{}{"curl"}}},
			},
			target:   platform.NewTarget("linux/amd64", "debian"),
			wantLen:  1,
			wantActs: []string{"apt_install"},
		},
		{
			name: "brew_cask passes for darwin target",
			steps: []recipe.Step{
				{Action: "brew_cask", Params: map[string]interface{}{"packages": []interface{}{"docker"}}},
			},
			target:   platform.NewTarget("darwin/arm64", ""),
			wantLen:  1,
			wantActs: []string{"brew_cask"},
		},
		{
			name: "dnf_install passes for rhel target",
			steps: []recipe.Step{
				{Action: "dnf_install", Params: map[string]interface{}{"packages": []interface{}{"docker"}}},
			},
			target:   platform.NewTarget("linux/amd64", "rhel"),
			wantLen:  1,
			wantActs: []string{"dnf_install"},
		},
		{
			name: "pacman_install filtered out for debian target",
			steps: []recipe.Step{
				{Action: "pacman_install", Params: map[string]interface{}{"packages": []interface{}{"curl"}}},
			},
			target:   platform.NewTarget("linux/amd64", "debian"),
			wantLen:  0,
			wantActs: nil,
		},
		{
			name: "step with explicit when clause filtered correctly",
			steps: []recipe.Step{
				{
					Action: "download_file",
					When:   &recipe.WhenClause{OS: []string{"linux"}},
					Params: map[string]interface{}{},
				},
			},
			target:   platform.NewTarget("darwin/arm64", ""),
			wantLen:  0,
			wantActs: nil,
		},
		{
			name: "step with explicit when clause passes when matched",
			steps: []recipe.Step{
				{
					Action: "download_file",
					When:   &recipe.WhenClause{OS: []string{"linux"}},
					Params: map[string]interface{}{},
				},
			},
			target:   platform.NewTarget("linux/amd64", "debian"),
			wantLen:  1,
			wantActs: []string{"download_file"},
		},
		{
			name: "mixed steps filtered correctly for debian target",
			steps: []recipe.Step{
				{Action: "apt_install", Params: map[string]interface{}{"packages": []interface{}{"curl"}}},
				{Action: "brew_cask", Params: map[string]interface{}{"packages": []interface{}{"docker"}}},
				{Action: "dnf_install", Params: map[string]interface{}{"packages": []interface{}{"docker"}}},
				{Action: "download_file", Params: map[string]interface{}{}},
			},
			target:   platform.NewTarget("linux/amd64", "debian"),
			wantLen:  2,
			wantActs: []string{"apt_install", "download_file"},
		},
		{
			name: "mixed steps filtered correctly for darwin target",
			steps: []recipe.Step{
				{Action: "apt_install", Params: map[string]interface{}{"packages": []interface{}{"curl"}}},
				{Action: "brew_cask", Params: map[string]interface{}{"packages": []interface{}{"docker"}}},
				{Action: "brew_install", Params: map[string]interface{}{"packages": []interface{}{"wget"}}},
				{Action: "download_file", Params: map[string]interface{}{}},
			},
			target:   platform.NewTarget("darwin/arm64", ""),
			wantLen:  3,
			wantActs: []string{"brew_cask", "brew_install", "download_file"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FilterStepsByTarget(tt.steps, tt.target)
			if len(got) != tt.wantLen {
				t.Errorf("FilterStepsByTarget() returned %d steps, want %d", len(got), tt.wantLen)
			}
			if tt.wantActs != nil {
				for i, step := range got {
					if i >= len(tt.wantActs) {
						t.Errorf("unexpected step at index %d: %s", i, step.Action)
						continue
					}
					if step.Action != tt.wantActs[i] {
						t.Errorf("step[%d].Action = %s, want %s", i, step.Action, tt.wantActs[i])
					}
				}
			}
		})
	}
}

func TestStepMatchesTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		step   recipe.Step
		target platform.Target
		want   bool
	}{
		{
			name:   "action without constraint matches any target",
			step:   recipe.Step{Action: "extract", Params: map[string]interface{}{}},
			target: platform.NewTarget("linux/amd64", "rhel"),
			want:   true,
		},
		{
			name:   "apt_install matches debian",
			step:   recipe.Step{Action: "apt_install", Params: map[string]interface{}{"packages": []interface{}{"curl"}}},
			target: platform.NewTarget("linux/amd64", "debian"),
			want:   true,
		},
		{
			name:   "apt_install does not match rhel",
			step:   recipe.Step{Action: "apt_install", Params: map[string]interface{}{"packages": []interface{}{"curl"}}},
			target: platform.NewTarget("linux/amd64", "rhel"),
			want:   false,
		},
		{
			name:   "brew_install matches darwin",
			step:   recipe.Step{Action: "brew_install", Params: map[string]interface{}{"packages": []interface{}{"curl"}}},
			target: platform.NewTarget("darwin/arm64", ""),
			want:   true,
		},
		{
			name:   "brew_install does not match linux",
			step:   recipe.Step{Action: "brew_install", Params: map[string]interface{}{"packages": []interface{}{"curl"}}},
			target: platform.NewTarget("linux/amd64", "debian"),
			want:   false,
		},
		{
			name: "when clause OS mismatch",
			step: recipe.Step{
				Action: "download_file",
				When:   &recipe.WhenClause{OS: []string{"darwin"}},
				Params: map[string]interface{}{},
			},
			target: platform.NewTarget("linux/amd64", "debian"),
			want:   false,
		},
		{
			name: "when clause platform mismatch",
			step: recipe.Step{
				Action: "download_file",
				When:   &recipe.WhenClause{Platform: []string{"darwin/arm64"}},
				Params: map[string]interface{}{},
			},
			target: platform.NewTarget("linux/amd64", "debian"),
			want:   false,
		},
		{
			name: "when clause platform match",
			step: recipe.Step{
				Action: "download_file",
				When:   &recipe.WhenClause{Platform: []string{"linux/amd64"}},
				Params: map[string]interface{}{},
			},
			target: platform.NewTarget("linux/amd64", "debian"),
			want:   true,
		},
		{
			name: "implicit constraint passes but when clause fails",
			step: recipe.Step{
				Action: "apt_install",
				When:   &recipe.WhenClause{Platform: []string{"linux/arm64"}},
				Params: map[string]interface{}{"packages": []interface{}{"curl"}},
			},
			target: platform.NewTarget("linux/amd64", "debian"),
			want:   false,
		},
		{
			name: "both implicit constraint and when clause pass",
			step: recipe.Step{
				Action: "apt_install",
				When:   &recipe.WhenClause{Platform: []string{"linux/amd64"}},
				Params: map[string]interface{}{"packages": []interface{}{"curl"}},
			},
			target: platform.NewTarget("linux/amd64", "debian"),
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := stepMatchesTarget(tt.step, tt.target)
			if got != tt.want {
				t.Errorf("stepMatchesTarget() = %v, want %v", got, tt.want)
			}
		})
	}
}
