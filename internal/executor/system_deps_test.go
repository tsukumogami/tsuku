package executor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// TestSystemDepsRecipes validates that testdata *-system.toml recipes
// exercise M30 system dependency actions correctly.
func TestSystemDepsRecipes(t *testing.T) {
	t.Parallel()

	testdataDir := filepath.Join("..", "..", "testdata", "recipes")

	tests := []struct {
		name         string
		recipeFile   string
		target       platform.Target
		wantActions  []string // expected actions after filtering
		wantFiltered []string // actions that should be filtered out
	}{
		{
			name:       "build-tools-system on Debian includes apt_install",
			recipeFile: "build-tools-system.toml",
			target:     platform.NewTarget("linux/amd64", "debian", "glibc"),
			wantActions: []string{
				"apt_install",     // Debian-specific
				"require_command", // Cross-platform verification
			},
			wantFiltered: []string{
				"dnf_install",    // RHEL-only
				"pacman_install", // Arch-only
				"apk_install",    // Alpine-only
				"zypper_install", // SUSE-only
			},
		},
		{
			name:       "build-tools-system on RHEL includes dnf_install",
			recipeFile: "build-tools-system.toml",
			target:     platform.NewTarget("linux/amd64", "rhel", "glibc"),
			wantActions: []string{
				"dnf_install",
				"require_command",
			},
			wantFiltered: []string{
				"apt_install",
				"pacman_install",
				"apk_install",
				"zypper_install",
			},
		},
		{
			name:       "build-tools-system on Arch includes pacman_install",
			recipeFile: "build-tools-system.toml",
			target:     platform.NewTarget("linux/amd64", "arch", "glibc"),
			wantActions: []string{
				"pacman_install",
				"require_command",
			},
			wantFiltered: []string{
				"apt_install",
				"dnf_install",
				"apk_install",
				"zypper_install",
			},
		},
		{
			name:       "build-tools-system on Alpine includes apk_install",
			recipeFile: "build-tools-system.toml",
			target:     platform.NewTarget("linux/amd64", "alpine", "musl"),
			wantActions: []string{
				"apk_install",
				"require_command",
			},
			wantFiltered: []string{
				"apt_install",
				"dnf_install",
				"pacman_install",
				"zypper_install",
			},
		},
		{
			name:       "build-tools-system on SUSE includes zypper_install",
			recipeFile: "build-tools-system.toml",
			target:     platform.NewTarget("linux/amd64", "suse", "glibc"),
			wantActions: []string{
				"zypper_install",
				"require_command",
			},
			wantFiltered: []string{
				"apt_install",
				"dnf_install",
				"pacman_install",
				"apk_install",
			},
		},
		{
			name:       "build-tools-system on Darwin includes manual action",
			recipeFile: "build-tools-system.toml",
			target:     platform.NewTarget("darwin/arm64", "", ""),
			wantActions: []string{
				"manual",          // Darwin-specific manual instruction
				"require_command", // Cross-platform verification
			},
			wantFiltered: []string{
				"apt_install",
				"dnf_install",
				"pacman_install",
				"apk_install",
				"zypper_install",
			},
		},
		{
			name:       "ssl-libs-system on Debian includes apt_install",
			recipeFile: "ssl-libs-system.toml",
			target:     platform.NewTarget("linux/amd64", "debian", "glibc"),
			wantActions: []string{
				"apt_install",
				"require_command",
			},
			wantFiltered: []string{
				"dnf_install",
				"brew_install",
			},
		},
		{
			name:       "ssl-libs-system on Darwin includes brew_install",
			recipeFile: "ssl-libs-system.toml",
			target:     platform.NewTarget("darwin/arm64", "", ""),
			wantActions: []string{
				"brew_install",
				"require_command",
			},
			wantFiltered: []string{
				"apt_install",
				"dnf_install",
				"pacman_install",
			},
		},
		{
			name:       "ca-certs-system on Debian includes apt_install with fallback",
			recipeFile: "ca-certs-system.toml",
			target:     platform.NewTarget("linux/amd64", "debian", "glibc"),
			wantActions: []string{
				"apt_install",
				"manual", // Manual CA update instruction (has os=linux when clause)
			},
			wantFiltered: []string{
				"dnf_install",
				"brew_install",
			},
		},
		{
			name:       "ca-certs-system on Darwin includes brew_install",
			recipeFile: "ca-certs-system.toml",
			target:     platform.NewTarget("darwin/arm64", "", ""),
			wantActions: []string{
				"brew_install",
			},
			wantFiltered: []string{
				"apt_install",
				"manual", // Linux-only manual action (has os=linux when clause)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Load recipe
			recipePath := filepath.Join(testdataDir, tt.recipeFile)
			rec, err := recipe.ParseFile(recipePath)
			if err != nil {
				t.Fatalf("failed to parse recipe: %v", err)
			}

			// Filter steps by target platform
			filtered := FilterStepsByTarget(rec.Steps, tt.target)

			// Verify expected actions are present
			gotActions := make(map[string]bool)
			for _, step := range filtered {
				gotActions[step.Action] = true
			}

			for _, wantAction := range tt.wantActions {
				if !gotActions[wantAction] {
					t.Errorf("expected action %q not found in filtered steps for target %+v", wantAction, tt.target)
				}
			}

			// Verify filtered-out actions are not present
			for _, filteredAction := range tt.wantFiltered {
				if gotActions[filteredAction] {
					t.Errorf("action %q should be filtered out for target %+v but was present", filteredAction, tt.target)
				}
			}
		})
	}
}

// TestSystemDepsDescribe validates that M30 system dependency actions
// generate correct Describe() output for installation instructions.
func TestSystemDepsDescribe(t *testing.T) {
	t.Parallel()

	testdataDir := filepath.Join("..", "..", "testdata", "recipes")

	tests := []struct {
		name         string
		recipeFile   string
		target       platform.Target
		wantContains []string // expected substrings in Describe() output
	}{
		{
			name:       "build-tools-system on Debian describes apt-get install",
			recipeFile: "build-tools-system.toml",
			target:     platform.NewTarget("linux/amd64", "debian", "glibc"),
			wantContains: []string{
				"apt-get install",
				"build-essential",
				"pkg-config",
			},
		},
		{
			name:       "build-tools-system on RHEL describes dnf install",
			recipeFile: "build-tools-system.toml",
			target:     platform.NewTarget("linux/amd64", "rhel", "glibc"),
			wantContains: []string{
				"dnf install",
				"gcc",
				"make",
			},
		},
		{
			name:       "ssl-libs-system on Debian describes libssl-dev",
			recipeFile: "ssl-libs-system.toml",
			target:     platform.NewTarget("linux/amd64", "debian", "glibc"),
			wantContains: []string{
				"apt-get install",
				"libssl-dev",
			},
		},
		{
			name:       "ssl-libs-system on RHEL describes openssl-devel",
			recipeFile: "ssl-libs-system.toml",
			target:     platform.NewTarget("linux/amd64", "rhel", "glibc"),
			wantContains: []string{
				"dnf install",
				"openssl-devel",
			},
		},
		{
			name:       "ssl-libs-system on Darwin describes brew install",
			recipeFile: "ssl-libs-system.toml",
			target:     platform.NewTarget("darwin/arm64", "", ""),
			wantContains: []string{
				"brew install",
				"openssl@3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Load recipe
			recipePath := filepath.Join(testdataDir, tt.recipeFile)
			rec, err := recipe.ParseFile(recipePath)
			if err != nil {
				t.Fatalf("failed to parse recipe: %v", err)
			}

			// Filter steps by target platform
			filtered := FilterStepsByTarget(rec.Steps, tt.target)

			// Generate describe output for each step
			var descriptions []string
			for _, step := range filtered {
				// Create action based on step type
				var desc string
				switch step.Action {
				case "apt_install":
					desc = (&actions.AptInstallAction{}).Describe(step.Params)
				case "dnf_install":
					desc = (&actions.DnfInstallAction{}).Describe(step.Params)
				case "pacman_install":
					desc = (&actions.PacmanInstallAction{}).Describe(step.Params)
				case "apk_install":
					desc = (&actions.ApkInstallAction{}).Describe(step.Params)
				case "zypper_install":
					desc = (&actions.ZypperInstallAction{}).Describe(step.Params)
				case "brew_install":
					desc = (&actions.BrewInstallAction{}).Describe(step.Params)
				case "brew_cask":
					desc = (&actions.BrewCaskAction{}).Describe(step.Params)
				case "require_command":
					desc = (&actions.RequireCommandAction{}).Describe(step.Params)
				case "manual":
					desc = (&actions.ManualAction{}).Describe(step.Params)
				default:
					t.Logf("skipping step %s (no describe test)", step.Action)
					continue
				}

				if desc != "" {
					descriptions = append(descriptions, desc)
				}
			}

			// Verify expected strings appear in descriptions
			allDescriptions := ""
			for _, desc := range descriptions {
				allDescriptions += desc + "\n"
			}

			for _, wantStr := range tt.wantContains {
				found := false
				for _, desc := range descriptions {
					if strings.Contains(desc, wantStr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected substring %q not found in any description.\nAll descriptions:\n%s", wantStr, allDescriptions)
				}
			}
		})
	}
}
