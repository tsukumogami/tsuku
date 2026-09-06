package activation

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// The vocabulary of unhonorable conditions, one row per *producer* rather than
// per reason.
//
// Granularity is the whole design. A table keyed on the five reasons would not
// do the job: `bad-form` has two producers, a malformed name and a malformed
// version, so a reason-keyed table stays green while one of the two dies. The
// row is the condition that produces the reason, and a condition that can no
// longer be reached fails here.
//
// Why this exists at all: the disjointness table in cmd/tsuku proves the five
// messages stay distinguishable from each other, which catches two reasons
// converging. Nothing caught a reason going *dead* — a condition subsumed by
// another code path, still enumerated in the requirements, no longer producible.
// That was found by accident while dry-running a rebase, and "found by accident"
// is not a control. Together the two tables police the vocabulary in both
// directions: absent-from-the-others catches convergence, reachable-at-all
// catches death.
//
// Adding a reason or a new producing condition means adding a row. Removing a
// row is a deliberate statement that the condition is now owned by another
// channel, and belongs in the same change that removes its code.
var unhonorableConditions = []struct {
	name   string
	reason Reason
	// badName distinguishes the two producers of ReasonBadForm.
	badName bool
	build   func(t *testing.T) *ActivationResult
}{
	{
		name:   "no-match: nothing installed satisfies the pin",
		reason: ReasonNoMatch,
		build: func(t *testing.T) *ActivationResult {
			r, _ := activate(t, "[tools]\njq = \"2\"\n", map[string][]string{"jq": {"1.6.0"}})
			return r
		},
	},
	{
		name:    "bad-form (name): the derived name is not a safe path segment",
		reason:  ReasonBadForm,
		badName: true,
		build: func(t *testing.T) *ActivationResult {
			r, _ := activate(t, "[tools]\n'../../etc' = \"latest\"\n", nil)
			return r
		},
	},
	{
		name:    "bad-form (version): the declared version is malformed",
		reason:  ReasonBadForm,
		badName: false,
		build: func(t *testing.T) *ActivationResult {
			r, _ := activate(t, "[tools]\nnodejs = '>=26'\n", map[string][]string{"nodejs": {"20.16.0"}})
			return r
		},
	},
	{
		name:   "channel: the declaration names a channel",
		reason: ReasonChannel,
		build: func(t *testing.T) *ActivationResult {
			r, _ := activate(t, "[tools]\nnodejs = \"@lts\"\n", map[string][]string{"nodejs": {"20.16.0"}})
			return r
		},
	},
	{
		name:   "missing-files: a satisfying version's directory is gone",
		reason: ReasonMissingFiles,
		build: func(t *testing.T) *ActivationResult {
			projectDir, cfg, installed := setupProject(t, "[tools]\njq = \"1\"\n",
				map[string][]string{"jq": {"1.7.1"}})
			t.Setenv("PATH", "/usr/bin")
			t.Setenv("HOME", filepath.Dir(projectDir))
			if err := os.RemoveAll(filepath.Join(cfg.ToolsDir, "jq-1.7.1")); err != nil {
				t.Fatal(err)
			}
			r, err := ComputeActivation(projectDir, "", "", "", cfg, installed)
			if err != nil {
				t.Fatal(err)
			}
			return r
		},
	},
}

// Every condition in the vocabulary is still producible.
//
// A row that stops firing means the condition has been subsumed by another code
// path — the reported reason now comes from somewhere else, or not at all — and
// that is a requirements change rather than a test failure to paper over. Fix it
// by removing the row *and* the code that can no longer run, in one change, or
// by restoring the path.
func TestEveryUnhonorableConditionIsReachable(t *testing.T) {
	for _, tc := range unhonorableConditions {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.build(t)

			for _, u := range result.Unhonorable {
				if u.Reason == tc.reason && u.BadName == tc.badName {
					return
				}
			}
			t.Errorf("condition is no longer reachable: nothing produced reason %v "+
				"(badName=%v).\nGot: %+v\n\nIf another code path now handles this "+
				"condition, remove this row and the code behind it in one change; "+
				"a reason that cannot fire is a control the requirements claim and "+
				"the code does not have.", tc.reason, tc.badName, result.Unhonorable)
		})
	}
}

// unreadable is the sixth condition and is not a per-declaration reason, so it
// cannot use the table above — it is a property of the state read, reported on
// its own field. It is here because the vocabulary is five reasons and this one
// would otherwise be the member nothing checks for reachability.
func TestUnreadableConditionIsReachable(t *testing.T) {
	projectDir, cfg, installed := setupProject(t, "[tools]\njq = \"latest\"\n", nil)
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))
	installed.err = errors.New("state.json is not valid JSON")

	result, err := ComputeActivation(projectDir, "", "", "", cfg, installed)
	if err != nil {
		t.Fatal(err)
	}
	if result.Unreadable == nil {
		t.Error("the unreadable condition is no longer reachable: a failing state " +
			"read produced no StateUnreadable")
	}
}
