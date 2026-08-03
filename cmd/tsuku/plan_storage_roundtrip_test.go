package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/testutil"
)

// planWithDependencyAndVerify is the shape that used to lose the most on the
// way into state.json: a tool with an install-time dependency, a verify block,
// a library dependency, and a binary name that no install_binaries step could
// have been inferred from.
func planWithDependencyAndVerify() *executor.InstallationPlan {
	return &executor.InstallationPlan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          "httpie",
		Version:       "3.2.2",
		Platform:      executor.Platform{OS: "linux", Arch: "amd64"},
		GeneratedAt:   time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC),
		RecipeSource:  "registry",
		Deterministic: true,
		Dependencies: []executor.DependencyPlan{
			{
				Tool:       "openssl",
				Version:    "3.2.1",
				RecipeType: "library",
				Steps: []executor.ResolvedStep{
					{Action: "chmod", Params: map[string]interface{}{"path": "lib/libssl.so"}},
				},
				Verify: &executor.PlanVerify{Command: "openssl version", Pattern: "OpenSSL"},
			},
		},
		// pipx_install leaves no install_binaries step, so plan.Binaries is
		// the only record of what the tool actually installs.
		Steps: []executor.ResolvedStep{
			{
				Action:        "pipx_install",
				Params:        map[string]interface{}{"package": "httpie"},
				Evaluable:     false,
				Deterministic: false,
			},
		},
		Verify:     &executor.PlanVerify{Command: "http --version", Pattern: "3.2.2"},
		RecipeType: "tool",
		Binaries:   []string{"bin/http"},
	}
}

// TestStoredPlanSurvivesExportIntoPlanBasedInstall drives the two documented
// commands that made this bug reachable without any hidden entry or
// multi-version setup:
//
//	tsuku plan export httpie -o httpie.json
//	tsuku install --plan httpie.json
//
// The assertions are on what the second command consumes, not on which JSON
// keys are present: the dependency the executor would install, the verification
// that would otherwise be skipped silently, the recipe type that decides
// library handling, and the binary name the symlinks are built from.
func TestStoredPlanSurvivesExportIntoPlanBasedInstall(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	original := planWithDependencyAndVerify()

	// What the install path writes: the plan, converted for storage, saved
	// into this tool's version state.
	mgr := install.New(cfg)
	err := mgr.GetState().UpdateTool("httpie", func(ts *install.ToolState) {
		ts.ActiveVersion = "3.2.2"
		ts.Versions = map[string]install.VersionState{
			"3.2.2": {Plan: executor.ToStoragePlan(original)},
		}
	})
	if err != nil {
		t.Fatalf("seed state: %v", err)
	}

	// What `tsuku plan export` writes: the stored record, serialized to a file.
	toolState, err := mgr.GetState().GetToolState("httpie")
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	stored := toolState.Versions[toolState.ActiveVersion].Plan
	if stored == nil {
		t.Fatal("no plan stored for httpie@3.2.2")
	}
	encoded, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		t.Fatalf("marshal exported plan: %v", err)
	}
	exportPath := filepath.Join(t.TempDir(), "httpie-3.2.2-linux-amd64.plan.json")
	if err := os.WriteFile(exportPath, encoded, 0o644); err != nil {
		t.Fatalf("write exported plan: %v", err)
	}

	// What `tsuku install --plan` reads.
	loaded, err := loadPlanFromSource(exportPath)
	if err != nil {
		t.Fatalf("load exported plan: %v", err)
	}
	if err := validateExternalPlan(loaded, "httpie"); err != nil {
		t.Fatalf("validate exported plan: %v", err)
	}

	// The dependency the plan-based install would install. That path has no
	// recipe loader, so an empty tree means httpie is installed without
	// OpenSSL and fails at run time.
	if len(loaded.Dependencies) != 1 {
		t.Fatalf("len(Dependencies) = %d, want 1; the exported plan installs without its dependencies",
			len(loaded.Dependencies))
	}
	dep := loaded.Dependencies[0]
	if dep.Tool != "openssl" || dep.Version != "3.2.1" {
		t.Errorf("dependency = %s@%s, want openssl@3.2.1", dep.Tool, dep.Version)
	}
	if dep.RecipeType != "library" {
		t.Errorf("dependency RecipeType = %q, want %q; a library installed as a tool lands in the wrong directory",
			dep.RecipeType, "library")
	}
	if dep.Verify == nil || dep.Verify.Command != "openssl version" {
		t.Errorf("dependency Verify = %v, want the recipe's verification command", dep.Verify)
	}

	// The verification. A nil Verify is treated as "nothing to check" rather
	// than an error, so losing it means the install passes by not verifying.
	if loaded.Verify == nil {
		t.Fatal("Verify is nil; the exported plan installs without verification")
	}
	if loaded.Verify.Command != "http --version" || loaded.Verify.Pattern != "3.2.2" {
		t.Errorf("Verify = %+v, want command %q pattern %q", loaded.Verify, "http --version", "3.2.2")
	}

	// What runPlanBasedInstall builds its minimal recipe from.
	minimal := &recipe.Recipe{Metadata: recipe.MetadataSection{Name: loaded.Tool, Type: loaded.RecipeType}}
	if minimal.Metadata.Type != "tool" {
		t.Errorf("minimal recipe Type = %q, want %q", minimal.Metadata.Type, "tool")
	}

	// What the install manager names the symlinks after. pipx_install leaves
	// no install_binaries step, so without plan.Binaries this falls back to
	// bin/httpie -- a path that does not exist, since the executable is http.
	binaries := executor.ExtractBinariesFromPlan(loaded)
	if len(binaries) != 1 || binaries[0] != "bin/http" {
		t.Errorf("ExtractBinariesFromPlan = %v, want [bin/http]", binaries)
	}
}

// TestWarnIfPlanIncomplete covers what plan show and plan export tell a user
// about a record written before the conversion carried every field. The record
// still displays and still exports -- refusing would break a command over a
// problem the user did not cause -- so the warning is the only thing standing
// between an incomplete export and an install that silently skips half of it.
func TestWarnIfPlanIncomplete(t *testing.T) {
	tests := []struct {
		name       string
		plan       *install.Plan
		wantWarned bool
	}{
		{
			name:       "record written before the fields existed",
			plan:       &install.Plan{Tool: "gh"},
			wantWarned: true,
		},
		{
			name:       "record written by the current conversion",
			plan:       executor.ToStoragePlan(planWithDependencyAndVerify()),
			wantWarned: false,
		},
		{
			name:       "no stored plan at all",
			plan:       nil,
			wantWarned: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStderr := os.Stderr
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("pipe: %v", err)
			}
			os.Stderr = w

			warnIfPlanIncomplete("gh", tt.plan)

			if err := w.Close(); err != nil {
				t.Fatalf("close pipe: %v", err)
			}
			os.Stderr = oldStderr

			var buf bytes.Buffer
			if _, err := buf.ReadFrom(r); err != nil {
				t.Fatalf("read pipe: %v", err)
			}

			warned := buf.Len() > 0
			if warned != tt.wantWarned {
				t.Fatalf("warned = %v, want %v (stderr: %q)", warned, tt.wantWarned, buf.String())
			}
			if tt.wantWarned && !strings.Contains(buf.String(), "tsuku install gh --fresh") {
				t.Errorf("warning does not name the refresh command; got: %q", buf.String())
			}
		})
	}
}

// TestStoredPlanCarriesStorageVersion pins the marker the read sites gate on.
// Without it a freshly written record is indistinguishable from one written by
// the conversion that dropped these fields, and every read would have to assume
// the worst.
func TestStoredPlanCarriesStorageVersion(t *testing.T) {
	stored := executor.ToStoragePlan(planWithDependencyAndVerify())

	if stored.StorageVersion != install.PlanStorageVersion {
		t.Errorf("StorageVersion = %d, want %d", stored.StorageVersion, install.PlanStorageVersion)
	}

	// And it survives the trip through state.json, which is where the read
	// sites actually find it.
	encoded, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded install.Plan
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.StorageVersion != install.PlanStorageVersion {
		t.Errorf("StorageVersion after JSON round trip = %d, want %d",
			decoded.StorageVersion, install.PlanStorageVersion)
	}
}
