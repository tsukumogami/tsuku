package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
)

// A plan file exported before the storage conversion carried every field has no
// dependency tree, no verify block and no recipe type -- and nothing on the
// `tsuku install --plan` path used to say so, because that path has no recipe to
// fall back on. These tests drive the real command over real files, one written
// by each producer, and read back what the user sees.

const stalePlanTool = "staleplantool"

// stalePlanSteps installs the tool with a single non-evaluable command, the same
// trick the reinstall tests use to drive a real install offline.
func stalePlanSteps() []executor.ResolvedStep {
	return []executor.ResolvedStep{
		{
			Action:    "run_command",
			Evaluable: true,
			Params: map[string]any{
				"command": "mkdir -p {install_dir}/bin && " +
					"printf '#!/bin/sh\\necho ok\\n' > {install_dir}/bin/" + stalePlanTool + " && " +
					"chmod 0755 {install_dir}/bin/" + stalePlanTool,
			},
		},
	}
}

// currentEvalPlan is what `tsuku eval` writes today: the executor plan, marked
// by plan generation.
func currentEvalPlan() *executor.InstallationPlan {
	return &executor.InstallationPlan{
		FormatVersion:  executor.PlanFormatVersion,
		StorageVersion: install.PlanStorageVersion,
		Tool:           stalePlanTool,
		Version:        "1.0.0",
		Platform:       executor.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		GeneratedAt:    time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC),
		RecipeSource:   "registry",
		Steps:          stalePlanSteps(),
		Verify:         &executor.PlanVerify{Command: "true"},
		RecipeType:     "tool",
	}
}

// stalePlanJSON is what `tsuku plan export` wrote before the conversion carried
// every field: the storage record as it looked then. No storage_version key, no
// dependencies, no verify, no recipe type.
//
// Written as literal JSON rather than marshaled from the current struct. A
// struct fixture produces the same bytes today, and would quietly stop doing so
// the first time someone adds a field to the plan types without omitempty --
// the fixture would gain a key no old file ever had, and the test would still
// pass while no longer testing an old file. The subject here is a shape on
// disk, so the fixture is that shape.
func stalePlanJSON(t *testing.T) []byte {
	t.Helper()

	steps, err := json.Marshal(stalePlanSteps())
	if err != nil {
		t.Fatalf("marshal steps: %v", err)
	}

	raw := `{
		"format_version": ` + strconv.Itoa(executor.PlanFormatVersion) + `,
		"tool": "` + stalePlanTool + `",
		"version": "1.0.0",
		"platform": {"os": "` + runtime.GOOS + `", "arch": "` + runtime.GOARCH + `"},
		"generated_at": "2026-08-03T12:00:00Z",
		"recipe_source": "registry",
		"deterministic": false,
		"steps": ` + string(steps) + `
	}`

	// Fail loudly here rather than inside the command under test if the
	// hand-written shape ever stops being valid JSON.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		t.Fatalf("stale plan fixture is not valid JSON: %v", err)
	}
	if _, present := probe["storage_version"]; present {
		t.Fatal("stale plan fixture carries storage_version; it is meant to predate the key")
	}
	return []byte(raw)
}

// writePlanFile drops the bytes somewhere `tsuku install --plan` can read them.
func writePlanFile(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), stalePlanTool+".plan.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write plan file: %v", err)
	}
	return path
}

// newPlanInstallHome gives the command a throwaway $TSUKU_HOME and the context
// the install path threads through plan execution.
func newPlanInstallHome(t *testing.T) *config.Config {
	t.Helper()

	t.Setenv("TSUKU_HOME", t.TempDir())
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("config.DefaultConfig() error = %v", err)
	}
	if err := cfg.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories() error = %v", err)
	}

	origCtx := globalCtx
	t.Cleanup(func() { globalCtx = origCtx })
	globalCtx = lifecycleCtx()

	return cfg
}

// captureStderr runs fn with os.Stderr redirected and returns what it wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()

	fn()

	os.Stderr = orig
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	return <-done
}

// TestPlanBasedInstallWarnsOnlyOnAnUnmarkedPlan runs the real command over a
// file from each producer. The assertion is on what reaches the user's terminal
// and on whether the tool ends up installed -- not on what a validation helper
// returned, which is what let this reach a release in the first place.
func TestPlanBasedInstallWarnsOnlyOnAnUnmarkedPlan(t *testing.T) {
	tests := []struct {
		name       string
		planBytes  func(t *testing.T) []byte
		wantWarned bool
	}{
		{
			name:       "a plan exported before the fields existed",
			planBytes:  stalePlanJSON,
			wantWarned: true,
		},
		{
			name: "a plan written by the current tsuku plan export",
			planBytes: func(t *testing.T) []byte {
				t.Helper()
				encoded, err := json.MarshalIndent(executor.ToStoragePlan(currentEvalPlan()), "", "  ")
				if err != nil {
					t.Fatalf("marshal exported plan: %v", err)
				}
				return encoded
			},
			wantWarned: false,
		},
		{
			name: "a plan written by the current tsuku eval",
			planBytes: func(t *testing.T) []byte {
				t.Helper()
				encoded, err := json.MarshalIndent(currentEvalPlan(), "", "  ")
				if err != nil {
					t.Fatalf("marshal eval plan: %v", err)
				}
				return encoded
			},
			wantWarned: false,
		},
		{
			// The population the marker check alone gets wrong. `tsuku eval`
			// only started stamping in this change, and it was never lossy, so
			// every eval file written before it is unmarked and complete. The
			// verify block proves it did not come through the lossy conversion.
			name: "a plan written by an older tsuku eval, which was never lossy",
			planBytes: func(t *testing.T) []byte {
				t.Helper()
				plan := currentEvalPlan()
				plan.StorageVersion = 0
				encoded, err := json.MarshalIndent(plan, "", "  ")
				if err != nil {
					t.Fatalf("marshal old eval plan: %v", err)
				}
				return encoded
			},
			wantWarned: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newPlanInstallHome(t)
			planPath := writePlanFile(t, tt.planBytes(t))

			var installErr error
			stderr := captureStderr(t, func() {
				installErr = runPlanBasedInstall(planPath, "")
			})

			// Warning or not, the install runs. Refusing would strand anyone
			// holding an exported plan on a machine that cannot re-export.
			if installErr != nil {
				t.Fatalf("runPlanBasedInstall() error = %v (stderr: %q)", installErr, stderr)
			}
			binary := filepath.Join(cfg.ToolDir(stalePlanTool, "1.0.0"), "bin", stalePlanTool)
			if _, err := os.Stat(binary); err != nil {
				t.Fatalf("tool not installed at %s: %v", binary, err)
			}

			warned := strings.Contains(stderr, "carries no marker saying whether it is complete")
			if warned != tt.wantWarned {
				t.Fatalf("warned = %v, want %v (stderr: %q)", warned, tt.wantWarned, stderr)
			}
			if !tt.wantWarned {
				return
			}
			// The warning has to say what may be missing and how to get a
			// current plan, or it is just noise the user learns to skip.
			for _, want := range []string{"dependencies", "verification", "recipe type", "tsuku eval " + stalePlanTool} {
				if !strings.Contains(stderr, want) {
					t.Errorf("warning does not mention %q; got: %q", want, stderr)
				}
			}
		})
	}
}

// TestPlanBasedInstallDoesNotLaunderAnUnmarkedPlan covers what happens after the
// warning. Installing from an unmarked file writes a state.json record, and if
// that record claimed to be complete then `plan show` and `plan export` would
// stop warning about it -- the file would have laundered itself, and the next
// person to export it would get an incomplete plan with nothing said.
func TestPlanBasedInstallDoesNotLaunderAnUnmarkedPlan(t *testing.T) {
	cfg := newPlanInstallHome(t)
	planPath := writePlanFile(t, stalePlanJSON(t))

	var installErr error
	_ = captureStderr(t, func() {
		installErr = runPlanBasedInstall(planPath, "")
	})
	if installErr != nil {
		t.Fatalf("runPlanBasedInstall() error = %v", installErr)
	}

	toolState, err := install.New(cfg).GetState().GetToolState(stalePlanTool)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if toolState == nil {
		t.Fatal("no state recorded for the installed tool")
	}
	stored := toolState.Versions["1.0.0"].Plan
	if stored == nil {
		t.Fatal("no plan stored for the installed tool")
	}
	if stored.StorageVersion != 0 {
		t.Fatalf("stored StorageVersion = %d, want 0; the incomplete plan was recorded as complete",
			stored.StorageVersion)
	}

	// And the downstream reader still says so, which is the part the user sees.
	stderr := captureStderr(t, func() {
		warnIfPlanIncomplete(stalePlanTool, stored)
	})
	if !strings.Contains(stderr, "predates the current storage format") {
		t.Errorf("plan show would not warn about the record this install wrote; got: %q", stderr)
	}
}

// TestGoldenPlansDoNotWarn drives every golden plan in the repo through the
// real warning.
//
// These files are the largest population of unmarked-but-complete plans that
// exists, and CI installs them: validate-golden-execution.yml runs `tsuku
// install --plan` over the ones a PR changes, and the sandbox job does the same
// through --sandbox. A marker check on its own warns about all 110, none of
// which are missing anything -- which is how a warning becomes something people
// learn to scroll past. This is the guard on that.
func TestGoldenPlansDoNotWarn(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "golden", "plans")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("golden plans not present: %v", err)
	}

	var checked int
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("%s: read: %v", path, readErr)
			return nil
		}
		var plan executor.InstallationPlan
		if unmarshalErr := json.Unmarshal(data, &plan); unmarshalErr != nil {
			t.Errorf("%s: parse: %v", path, unmarshalErr)
			return nil
		}
		checked++

		stderr := captureStderr(t, func() { warnIfExternalPlanIncomplete(&plan) })
		if stderr != "" {
			t.Errorf("%s: warns, but golden plans are generated by tsuku eval and carry everything:\n%s",
				path, stderr)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", root, walkErr)
	}
	if checked == 0 {
		t.Fatal("no golden plans found; this test would pass vacuously")
	}
	t.Logf("checked %d golden plans, none warned", checked)
}

// TestUnmarkedPlanIsJudgedOnEverySalvagedField pins each arm of the structural
// suppressor separately.
//
// The pre-#2484 conversion dropped six fields, so any one of them proves a plan
// did not come through it. The golden corpus does not exercise the arms evenly
// -- 88 files carry a verify block but none carries a per-step phase -- so
// TestGoldenPlansDoNotWarn would keep passing with half the arms deleted. Each
// field gets a plan carrying it and nothing else.
func TestUnmarkedPlanIsJudgedOnEverySalvagedField(t *testing.T) {
	// The population the marker exists to catch: unmarked, and carrying none of
	// the six. Every case below is this plus exactly one field.
	bare := func() *executor.InstallationPlan {
		return &executor.InstallationPlan{
			FormatVersion: executor.PlanFormatVersion,
			Tool:          stalePlanTool,
			Version:       "1.0.0",
			Platform:      executor.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
			Steps:         []executor.ResolvedStep{{Action: "chmod", Params: map[string]any{"path": "bin/x"}}},
		}
	}

	tests := []struct {
		field      string
		set        func(*executor.InstallationPlan)
		wantWarned bool
	}{
		{field: "nothing at all", set: func(*executor.InstallationPlan) {}, wantWarned: true},
		{field: "dependencies", set: func(p *executor.InstallationPlan) {
			p.Dependencies = []executor.DependencyPlan{{Tool: "openssl", Version: "3.2.1"}}
		}},
		{field: "verify", set: func(p *executor.InstallationPlan) {
			p.Verify = &executor.PlanVerify{Command: "x --version"}
		}},
		{field: "recipe_type", set: func(p *executor.InstallationPlan) { p.RecipeType = "library" }},
		{field: "binaries", set: func(p *executor.InstallationPlan) { p.Binaries = []string{"bin/x"} }},
		{field: "platform.linux_family", set: func(p *executor.InstallationPlan) {
			p.Platform.LinuxFamily = "debian"
		}},
		{field: "step phase", set: func(p *executor.InstallationPlan) { p.Steps[0].Phase = "post-install" }},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			plan := bare()
			tt.set(plan)
			if plan.StorageVersion != 0 {
				t.Fatal("fixture is marked; it is meant to test the unmarked population")
			}

			stderr := captureStderr(t, func() { warnIfExternalPlanIncomplete(plan) })

			warned := stderr != ""
			if warned != tt.wantWarned {
				t.Fatalf("warned = %v, want %v for a plan carrying only %s (stderr: %q)",
					warned, tt.wantWarned, tt.field, stderr)
			}
		})
	}
}
