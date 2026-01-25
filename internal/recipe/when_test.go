package recipe

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// TestWhenClause_IsEmpty tests the IsEmpty method
func TestWhenClause_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		when *WhenClause
		want bool
	}{
		{
			name: "nil clause is empty",
			when: nil,
			want: true,
		},
		{
			name: "zero-value clause is empty",
			when: &WhenClause{},
			want: true,
		},
		{
			name: "clause with platform is not empty",
			when: &WhenClause{Platform: []string{"darwin/arm64"}},
			want: false,
		},
		{
			name: "clause with OS is not empty",
			when: &WhenClause{OS: []string{"linux"}},
			want: false,
		},
		{
			name: "clause with package_manager is not empty",
			when: &WhenClause{PackageManager: "brew"},
			want: false,
		},
		{
			name: "clause with empty arrays is empty",
			when: &WhenClause{Platform: []string{}, OS: []string{}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.when.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestWhenClause_Matches tests the Matches method
func TestWhenClause_Matches(t *testing.T) {
	tests := []struct {
		name string
		when *WhenClause
		os   string
		arch string
		want bool
	}{
		{
			name: "nil clause matches all",
			when: nil,
			os:   "darwin",
			arch: "arm64",
			want: true,
		},
		{
			name: "empty clause matches all",
			when: &WhenClause{},
			os:   "linux",
			arch: "amd64",
			want: true,
		},
		{
			name: "platform exact match",
			when: &WhenClause{Platform: []string{"darwin/arm64"}},
			os:   "darwin",
			arch: "arm64",
			want: true,
		},
		{
			name: "platform no match",
			when: &WhenClause{Platform: []string{"darwin/arm64"}},
			os:   "linux",
			arch: "amd64",
			want: false,
		},
		{
			name: "platform multiple options - first matches",
			when: &WhenClause{Platform: []string{"darwin/arm64", "linux/amd64"}},
			os:   "darwin",
			arch: "arm64",
			want: true,
		},
		{
			name: "platform multiple options - second matches",
			when: &WhenClause{Platform: []string{"darwin/arm64", "linux/amd64"}},
			os:   "linux",
			arch: "amd64",
			want: true,
		},
		{
			name: "platform multiple options - neither matches",
			when: &WhenClause{Platform: []string{"darwin/arm64", "linux/amd64"}},
			os:   "darwin",
			arch: "amd64",
			want: false,
		},
		{
			name: "OS match any arch",
			when: &WhenClause{OS: []string{"darwin"}},
			os:   "darwin",
			arch: "arm64",
			want: true,
		},
		{
			name: "OS match any arch (amd64)",
			when: &WhenClause{OS: []string{"darwin"}},
			os:   "darwin",
			arch: "amd64",
			want: true,
		},
		{
			name: "OS no match",
			when: &WhenClause{OS: []string{"darwin"}},
			os:   "linux",
			arch: "amd64",
			want: false,
		},
		{
			name: "OS multiple options - first matches",
			when: &WhenClause{OS: []string{"darwin", "linux"}},
			os:   "darwin",
			arch: "arm64",
			want: true,
		},
		{
			name: "OS multiple options - second matches",
			when: &WhenClause{OS: []string{"darwin", "linux"}},
			os:   "linux",
			arch: "amd64",
			want: true,
		},
		{
			name: "OS multiple options - neither matches",
			when: &WhenClause{OS: []string{"darwin", "linux"}},
			os:   "freebsd",
			arch: "amd64",
			want: false,
		},
		{
			name: "package_manager only always matches (runtime check)",
			when: &WhenClause{PackageManager: "brew"},
			os:   "darwin",
			arch: "arm64",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := NewMatchTarget(tt.os, tt.arch, "", "")
			if got := tt.when.Matches(target); got != tt.want {
				t.Errorf("Matches(%s, %s) = %v, want %v", tt.os, tt.arch, got, tt.want)
			}
		})
	}
}

// TestWhenClause_Matches_ArchAndLinuxFamily tests the new Arch and LinuxFamily filters
func TestWhenClause_Matches_ArchAndLinuxFamily(t *testing.T) {
	tests := []struct {
		name        string
		when        *WhenClause
		os          string
		arch        string
		linuxFamily string
		want        bool
	}{
		{
			name: "arch filter matches",
			when: &WhenClause{Arch: "amd64"},
			os:   "linux", arch: "amd64", linuxFamily: "debian",
			want: true,
		},
		{
			name: "arch filter does not match",
			when: &WhenClause{Arch: "amd64"},
			os:   "linux", arch: "arm64", linuxFamily: "debian",
			want: false,
		},
		{
			name: "linux_family filter matches",
			when: &WhenClause{LinuxFamily: "debian"},
			os:   "linux", arch: "amd64", linuxFamily: "debian",
			want: true,
		},
		{
			name: "linux_family filter does not match",
			when: &WhenClause{LinuxFamily: "debian"},
			os:   "linux", arch: "amd64", linuxFamily: "rhel",
			want: false,
		},
		{
			name: "OS with arch filter - both match",
			when: &WhenClause{OS: []string{"linux"}, Arch: "arm64"},
			os:   "linux", arch: "arm64", linuxFamily: "debian",
			want: true,
		},
		{
			name: "OS with arch filter - OS matches, arch does not",
			when: &WhenClause{OS: []string{"linux"}, Arch: "arm64"},
			os:   "linux", arch: "amd64", linuxFamily: "debian",
			want: false,
		},
		{
			name: "OS with arch filter - OS does not match",
			when: &WhenClause{OS: []string{"darwin"}, Arch: "arm64"},
			os:   "linux", arch: "arm64", linuxFamily: "debian",
			want: false,
		},
		{
			name: "OS with linux_family filter - both match",
			when: &WhenClause{OS: []string{"linux"}, LinuxFamily: "debian"},
			os:   "linux", arch: "amd64", linuxFamily: "debian",
			want: true,
		},
		{
			name: "OS with linux_family filter - OS matches, family does not",
			when: &WhenClause{OS: []string{"linux"}, LinuxFamily: "debian"},
			os:   "linux", arch: "amd64", linuxFamily: "rhel",
			want: false,
		},
		{
			name: "arch and linux_family combined - both match",
			when: &WhenClause{Arch: "amd64", LinuxFamily: "debian"},
			os:   "linux", arch: "amd64", linuxFamily: "debian",
			want: true,
		},
		{
			name: "arch and linux_family combined - arch matches, family does not",
			when: &WhenClause{Arch: "amd64", LinuxFamily: "debian"},
			os:   "linux", arch: "amd64", linuxFamily: "rhel",
			want: false,
		},
		{
			name: "arch and linux_family combined - family matches, arch does not",
			when: &WhenClause{Arch: "amd64", LinuxFamily: "debian"},
			os:   "linux", arch: "arm64", linuxFamily: "debian",
			want: false,
		},
		{
			name: "all filters combined - all match",
			when: &WhenClause{OS: []string{"linux"}, Arch: "amd64", LinuxFamily: "debian"},
			os:   "linux", arch: "amd64", linuxFamily: "debian",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := NewMatchTarget(tt.os, tt.arch, tt.linuxFamily, "")
			if got := tt.when.Matches(target); got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMatchTarget tests the MatchTarget struct and NewMatchTarget constructor
func TestMatchTarget(t *testing.T) {
	t.Run("NewMatchTarget creates correct values", func(t *testing.T) {
		target := NewMatchTarget("linux", "amd64", "debian", "glibc")
		if target.OS() != "linux" {
			t.Errorf("OS() = %q, want %q", target.OS(), "linux")
		}
		if target.Arch() != "amd64" {
			t.Errorf("Arch() = %q, want %q", target.Arch(), "amd64")
		}
		if target.LinuxFamily() != "debian" {
			t.Errorf("LinuxFamily() = %q, want %q", target.LinuxFamily(), "debian")
		}
	})

	t.Run("MatchTarget with empty linux_family", func(t *testing.T) {
		target := NewMatchTarget("darwin", "arm64", "", "")
		if target.LinuxFamily() != "" {
			t.Errorf("LinuxFamily() = %q, want empty", target.LinuxFamily())
		}
	})
}

// TestWhenClause_UnmarshalTOML_Platform tests unmarshaling platform arrays
func TestWhenClause_UnmarshalTOML_Platform(t *testing.T) {
	tomlData := `
[[steps]]
action = "run_command"
when = { platform = ["darwin/arm64", "linux/amd64"] }
command = "echo test"
`

	var recipe struct {
		Steps []Step `toml:"steps"`
	}

	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	step := recipe.Steps[0]

	if step.When == nil {
		t.Fatal("When should not be nil")
	}

	if len(step.When.Platform) != 2 {
		t.Fatalf("Platform length = %d, want 2", len(step.When.Platform))
	}

	if step.When.Platform[0] != "darwin/arm64" {
		t.Errorf("Platform[0] = %s, want darwin/arm64", step.When.Platform[0])
	}

	if step.When.Platform[1] != "linux/amd64" {
		t.Errorf("Platform[1] = %s, want linux/amd64", step.When.Platform[1])
	}
}

// TestWhenClause_UnmarshalTOML_OS tests unmarshaling OS arrays
func TestWhenClause_UnmarshalTOML_OS(t *testing.T) {
	tomlData := `
[[steps]]
action = "run_command"
when = { os = ["darwin", "linux"] }
command = "echo test"
`

	var recipe struct {
		Steps []Step `toml:"steps"`
	}

	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	step := recipe.Steps[0]

	if step.When == nil {
		t.Fatal("When should not be nil")
	}

	if len(step.When.OS) != 2 {
		t.Fatalf("OS length = %d, want 2", len(step.When.OS))
	}

	if step.When.OS[0] != "darwin" {
		t.Errorf("OS[0] = %s, want darwin", step.When.OS[0])
	}

	if step.When.OS[1] != "linux" {
		t.Errorf("OS[1] = %s, want linux", step.When.OS[1])
	}
}

// TestWhenClause_UnmarshalTOML_SingleString tests that single strings are converted to arrays
func TestWhenClause_UnmarshalTOML_SingleString(t *testing.T) {
	tomlData := `
[[steps]]
action = "run_command"
when = { os = "linux" }
command = "echo test"
`

	var recipe struct {
		Steps []Step `toml:"steps"`
	}

	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	step := recipe.Steps[0]

	if step.When == nil {
		t.Fatal("When should not be nil")
	}

	if len(step.When.OS) != 1 {
		t.Fatalf("OS length = %d, want 1", len(step.When.OS))
	}

	if step.When.OS[0] != "linux" {
		t.Errorf("OS[0] = %s, want linux", step.When.OS[0])
	}
}

// TestWhenClause_UnmarshalTOML_MutualExclusivity tests that platform and OS cannot coexist
func TestWhenClause_UnmarshalTOML_MutualExclusivity(t *testing.T) {
	tomlData := `
[[steps]]
action = "run_command"
when = { platform = ["darwin/arm64"], os = ["linux"] }
command = "echo test"
`

	var recipe struct {
		Steps []Step `toml:"steps"`
	}

	err := toml.Unmarshal([]byte(tomlData), &recipe)
	if err == nil {
		t.Fatal("Expected error for platform and OS coexisting, got nil")
	}

	expectedSubstring := "when clause cannot have both 'platform' and 'os' fields"
	if !strings.Contains(err.Error(), expectedSubstring) {
		t.Errorf("Error = %q, want to contain %q", err.Error(), expectedSubstring)
	}
}

// TestWhenClause_ToMap tests serialization back to TOML format
func TestWhenClause_ToMap(t *testing.T) {
	step := Step{
		Action: "run_command",
		When: &WhenClause{
			Platform: []string{"darwin/arm64", "linux/amd64"},
		},
		Params: map[string]interface{}{
			"command": "echo test",
		},
	}

	m := step.ToMap()

	whenMap, ok := m["when"].(map[string]interface{})
	if !ok {
		t.Fatal("when field should be a map")
	}

	platform, ok := whenMap["platform"].([]string)
	if !ok {
		t.Fatal("platform should be a []string")
	}

	if len(platform) != 2 {
		t.Fatalf("platform length = %d, want 2", len(platform))
	}

	if platform[0] != "darwin/arm64" {
		t.Errorf("platform[0] = %s, want darwin/arm64", platform[0])
	}

	if platform[1] != "linux/amd64" {
		t.Errorf("platform[1] = %s, want linux/amd64", platform[1])
	}
}

// TestWhenClause_ValidationErrors tests that validation catches invalid when clauses
func TestWhenClause_ValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		recipe   *Recipe
		wantErrs int
	}{
		{
			name: "invalid platform tuple format (no slash)",
			recipe: &Recipe{
				Metadata: MetadataSection{Name: "test"},
				Steps: []Step{
					{
						Action: "run_command",
						When:   &WhenClause{Platform: []string{"darwin"}},
					},
				},
			},
			wantErrs: 1,
		},
		{
			name: "platform tuple not in supported platforms",
			recipe: &Recipe{
				Metadata: MetadataSection{
					Name:        "test",
					SupportedOS: []string{"linux"},
				},
				Steps: []Step{
					{
						Action: "run_command",
						When:   &WhenClause{Platform: []string{"darwin/arm64"}},
					},
				},
			},
			wantErrs: 1,
		},
		{
			name: "OS not in supported platforms",
			recipe: &Recipe{
				Metadata: MetadataSection{
					Name:        "test",
					SupportedOS: []string{"linux"},
				},
				Steps: []Step{
					{
						Action: "run_command",
						When:   &WhenClause{OS: []string{"darwin"}},
					},
				},
			},
			wantErrs: 1,
		},
		{
			name: "valid platform tuple",
			recipe: &Recipe{
				Metadata: MetadataSection{Name: "test"},
				Steps: []Step{
					{
						Action: "run_command",
						When:   &WhenClause{Platform: []string{"darwin/arm64"}},
					},
				},
			},
			wantErrs: 0,
		},
		{
			name: "valid OS array",
			recipe: &Recipe{
				Metadata: MetadataSection{Name: "test"},
				Steps: []Step{
					{
						Action: "run_command",
						When:   &WhenClause{OS: []string{"darwin", "linux"}},
					},
				},
			},
			wantErrs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.recipe.ValidateStepsAgainstPlatforms()
			if len(errs) != tt.wantErrs {
				t.Errorf("ValidateStepsAgainstPlatforms() returned %d errors, want %d", len(errs), tt.wantErrs)
				for _, err := range errs {
					t.Logf("  Error: %v", err)
				}
			}
		})
	}
}
