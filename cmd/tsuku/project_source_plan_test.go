package main

import (
	"os"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// planWithOneUnregisteredSource is the shape every case below starts from: a
// project declaring one tool from a source the user has not registered.
func planWithOneUnregisteredSource(t *testing.T) (*projectSourcePlan, string) {
	t.Helper()
	cfgPath := isolatedConfig(t)
	plan := newProjectSourcePlan("/work/.tsuku.toml")
	plan.add("owner/repo", "owner/repo:tool")
	plan.classify()
	return plan, cfgPath
}

// TestClassifyWritesNothing pins the first half of the deferred write: deciding
// is not acting.
func TestClassifyWritesNothing(t *testing.T) {
	plan, cfgPath := planWithOneUnregisteredSource(t)

	if st, _ := plan.state("owner/repo"); st != sourceNeedsApproval {
		t.Fatalf("want the source to need approval, got state %d", st)
	}
	if _, err := os.Stat(cfgPath); err == nil {
		t.Fatal("classification created config.toml")
	}
}

// TestNoTerminalIsNotConsent covers R12. The absence of somebody to ask is not
// an answer, and the code that runs when nobody can be asked must not be the
// code that runs when somebody said yes.
func TestNoTerminalIsNotConsent(t *testing.T) {
	plan, cfgPath := planWithOneUnregisteredSource(t)

	plan.decideConsent(consentInputs{
		AutoApprove: false,
		Interactive: func() bool { return false },
		Ask:         func(string) bool { t.Fatal("asked with no terminal"); return false },
	})

	if st, _ := plan.state("owner/repo"); st != sourceNeedsApproval {
		t.Fatalf("a missing terminal approved the source: state %d", st)
	}
	if err := plan.commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, err := os.Stat(cfgPath); err == nil {
		t.Fatal("config.toml was written for a source nobody approved")
	}
}

// TestCIEnvironmentIsNotConsent pins R22's other half: a detected CI
// environment grants nothing. The consent inputs are a struct precisely so the
// answer to "what can grant consent?" is the field list, and the environment is
// not in it.
func TestCIEnvironmentIsNotConsent(t *testing.T) {
	t.Setenv("CI", "true")
	plan, cfgPath := planWithOneUnregisteredSource(t)

	plan.decideConsent(consentInputs{
		AutoApprove: false,
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	})
	if err := plan.commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if _, err := os.Stat(cfgPath); err == nil {
		t.Fatal("CI=true registered a source")
	}
}

// TestDeclinedSourceLeavesConfigUntouched covers R14 for the source prompt.
func TestDeclinedSourceLeavesConfigUntouched(t *testing.T) {
	plan, cfgPath := planWithOneUnregisteredSource(t)

	plan.decideConsent(consentInputs{
		Interactive: func() bool { return true },
		Ask:         func(string) bool { return false },
	})
	if err := plan.commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if _, err := os.Stat(cfgPath); err == nil {
		t.Fatal("declining the source prompt still wrote config.toml")
	}
}

// TestApprovedSourceIsWrittenOnlyByCommit is the ordering the whole feature
// turns on: a yes at the source prompt is not a write. The write happens when
// the install proceeds, and if the user then declines "Proceed?" the caller
// never calls commit and nothing was written.
func TestApprovedSourceIsWrittenOnlyByCommit(t *testing.T) {
	plan, cfgPath := planWithOneUnregisteredSource(t)

	plan.decideConsent(consentInputs{
		Interactive: func() bool { return true },
		Ask:         func(string) bool { return true },
	})

	if _, err := os.Stat(cfgPath); err == nil {
		t.Fatal("saying yes to the source prompt wrote config.toml before the install proceeded")
	}

	if err := plan.commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	cfg, err := userconfig.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	entry, ok := cfg.Registries["owner/repo"]
	if !ok {
		t.Fatal("commit did not write the approved source")
	}
	if !entry.AutoRegistered {
		t.Error("the entry lost its auto-registration flag")
	}
}

// TestYesFlagApproves covers the other consent route in R12.
func TestYesFlagApproves(t *testing.T) {
	plan, _ := planWithOneUnregisteredSource(t)

	plan.decideConsent(consentInputs{
		AutoApprove: true,
		Interactive: func() bool { return false },
		Ask:         func(string) bool { t.Fatal("--yes still asked"); return false },
	})

	if st, _ := plan.state("owner/repo"); st != sourceApproved {
		t.Fatalf("--yes did not approve the source: state %d", st)
	}
}

// TestCommitWritesEveryApprovedSourceOnce covers the single-save shape: two
// sources approved together produce one write, and a second run over a source
// already present leaves its record alone.
func TestCommitWritesEveryApprovedSourceOnce(t *testing.T) {
	isolatedConfig(t)

	plan := newProjectSourcePlan("/work/.tsuku.toml")
	plan.add("owner/one", "owner/one:a")
	plan.add("owner/two", "owner/two:b")
	plan.classify()
	plan.decideConsent(consentInputs{AutoApprove: true, Interactive: func() bool { return false }})
	if err := plan.commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	cfg, err := userconfig.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, name := range []string{"owner/one", "owner/two"} {
		if _, ok := cfg.Registries[name]; !ok {
			t.Errorf("%s was not written", name)
		}
	}

	// A second project naming one of them again must not rewrite the record.
	second := newProjectSourcePlan("/elsewhere/.tsuku.toml")
	second.add("owner/one", "owner/one:a")
	second.classify()
	if st, _ := second.state("owner/one"); st != sourceRegistered {
		t.Fatalf("an already-written source needed approval again: state %d", st)
	}
}

// TestSkippedReportNamesBothApprovalRoutes covers R15's message. Somebody who
// hits this has two ways forward and the line has to name both, because the
// obvious one -- re-running -- does not work.
func TestSkippedReportNamesBothApprovalRoutes(t *testing.T) {
	plan, _ := planWithOneUnregisteredSource(t)
	plan.decideConsent(consentInputs{Interactive: func() bool { return false }})

	outcome := runCommandOutcome(t, func() error {
		plan.reportSkipped()
		return nil
	})

	for _, want := range []string{"owner/repo", "tsuku install --yes", "tsuku registry add owner/repo", "/work/.tsuku.toml"} {
		if !strings.Contains(outcome.stderr, want) {
			t.Errorf("the skipped report does not mention %q:\n%s", want, outcome.stderr)
		}
	}
	if !strings.Contains(outcome.stderr, "no terminal") {
		t.Errorf("the report does not distinguish a missing terminal from a decline:\n%s", outcome.stderr)
	}
	if outcome.stdout != "" {
		t.Errorf("the skipped report reached stdout: %q", outcome.stdout)
	}
}

// TestSkippedReportQuotesHostileNames pins that the declaring path and the tool
// names are quoted. Both come from a repository the invoking user may not
// control.
func TestSkippedReportQuotesHostileNames(t *testing.T) {
	isolatedConfig(t)

	plan := newProjectSourcePlan("/work/a\nb$(x)`y`\x1b[31m/.tsuku.toml")
	plan.add("owner/repo", "evil\x1b[31mtool")
	plan.classify()
	plan.decideConsent(consentInputs{Interactive: func() bool { return false }})

	outcome := runCommandOutcome(t, func() error {
		plan.reportSkipped()
		return nil
	})

	if strings.Contains(outcome.stderr, "\x1b") {
		t.Errorf("a raw escape sequence reached the terminal:\n%q", outcome.stderr)
	}
	if strings.Contains(outcome.stderr, "\nb$(x)") {
		t.Errorf("a raw newline from the path reached the terminal:\n%q", outcome.stderr)
	}
}

// TestNeedsApprovalOutranksInstallFailure covers R15's exit-code precedence.
//
// A run that both skips a source and fails another tool returns the
// needs-approval code, because that is the one a script can act on: approve and
// re-run. The failure is still reported.
func TestNeedsApprovalOutranksInstallFailure(t *testing.T) {
	results := []projectToolResult{
		{Name: "good", Status: "installed"},
		{Name: "broken", Status: "failed", Error: os.ErrPermission},
		{Name: "unapproved", Status: "needs-approval"},
	}

	summary := buildProjectSummaryJSON(results, ExitNeedsApproval)
	if summary.Status != "needs-approval" {
		t.Errorf("want status needs-approval, got %q", summary.Status)
	}
	if summary.ExitCode != ExitNeedsApproval {
		t.Errorf("want exit_code %d, got %d", ExitNeedsApproval, summary.ExitCode)
	}

	byName := map[string]projectToolJSON{}
	for _, tj := range summary.Tools {
		byName[tj.Name] = tj
	}
	if byName["unapproved"].Status != "needs-approval" {
		t.Errorf("a skipped tool was reported as %q", byName["unapproved"].Status)
	}
	if byName["broken"].Status != "failed" || byName["broken"].Error == "" {
		t.Errorf("the unrelated failure lost its status or its error: %+v", byName["broken"])
	}
	if byName["good"].Status != "installed" {
		t.Errorf("an installed tool was reported as %q", byName["good"].Status)
	}
}

// TestSummaryBucketsSkippedSeparately pins that a skipped tool is not counted
// as installed. Folding it in would report a tool that was never attempted as
// present, which is exactly how a skipped source becomes invisible.
func TestSummaryBucketsSkippedSeparately(t *testing.T) {
	results := []projectToolResult{
		{Name: "good", Status: "installed"},
		{Name: "unapproved", Status: "needs-approval"},
	}

	outcome := runCommandOutcome(t, func() error {
		printProjectSummary(results)
		return nil
	})

	if !strings.Contains(outcome.stdout, "Installed: 1") {
		t.Errorf("want one installed tool:\n%s", outcome.stdout)
	}
	if !strings.Contains(outcome.stdout, "Skipped: 1") {
		t.Errorf("the skipped tool was not reported separately:\n%s", outcome.stdout)
	}
}

// TestExitCodeIsDistinctAndOutranksTheFailureCodes pins the constant itself,
// since a script reads the number rather than the name.
func TestExitCodeIsDistinctAndOutranksTheFailureCodes(t *testing.T) {
	if ExitNeedsApproval != 16 {
		t.Errorf("ExitNeedsApproval is %d, want 16", ExitNeedsApproval)
	}
	for name, code := range map[string]int{
		"ExitInstallFailed":  ExitInstallFailed,
		"ExitPartialFailure": ExitPartialFailure,
		"ExitForbidden":      ExitForbidden,
		"ExitUserDeclined":   ExitUserDeclined,
		"ExitNotInteractive": ExitNotInteractive,
	} {
		if code == ExitNeedsApproval {
			t.Errorf("%s collides with ExitNeedsApproval at %d", name, code)
		}
	}
}
