package main

import (
	"os"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// TestForceDoesNotApproveAProjectNamedSource covers R12's narrowing.
//
// The two flags meant the same thing on this path, and only one of them is
// documented for it. --yes answers consent questions; --force forces an
// operation through. A permanent new entry in the user's global configuration
// is not what somebody forcing an install asked for -- particularly when the
// source was named by a file they may not have written.
func TestForceDoesNotApproveAProjectNamedSource(t *testing.T) {
	cfgPath := isolatedConfig(t)

	origYes, origForce := installYes, installForce
	installYes, installForce = false, true
	t.Cleanup(func() { installYes, installForce = origYes, origForce })

	plan := newProjectSourcePlan("/work/.tsuku.toml")
	plan.add("owner/repo", "owner/repo:tool")
	plan.classify()

	// The project install passes installYes alone. --force is not among the
	// consent inputs at all, which is the point: the struct is the answer to
	// "what can grant consent here?".
	plan.decideConsent(consentInputs{
		AutoApprove: installYes,
		Interactive: func() bool { return false },
		Ask:         func(string) bool { t.Fatal("asked with no terminal"); return false },
	})
	if err := plan.commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if st, _ := plan.state("owner/repo"); st != sourceNeedsApproval {
		t.Fatalf("--force approved a project-named source: state %d", st)
	}
	if _, err := os.Stat(cfgPath); err == nil {
		t.Fatal("--force wrote config.toml for a project-named source")
	}
}

// TestYesStillApprovesAProjectNamedSource is the other half: narrowing --force
// must not have narrowed the route that is supposed to work.
func TestYesStillApprovesAProjectNamedSource(t *testing.T) {
	isolatedConfig(t)

	origYes, origForce := installYes, installForce
	installYes, installForce = true, false
	t.Cleanup(func() { installYes, installForce = origYes, origForce })

	plan := newProjectSourcePlan("/work/.tsuku.toml")
	plan.add("owner/repo", "owner/repo:tool")
	plan.classify()
	plan.decideConsent(consentInputs{
		AutoApprove: installYes,
		Interactive: func() bool { return false },
	})

	if st, _ := plan.state("owner/repo"); st != sourceApproved {
		t.Fatalf("--yes did not approve a project-named source: state %d", st)
	}
}

// TestForceStillRegistersACommandLineNamedSource covers R17.
//
// The narrowing is scoped to a source the user learned about only from a
// project file. A source they typed still registers exactly as it did, --force
// included and with no terminal attached, because typing it is the deliberate
// act the flag rides on.
func TestForceStillRegistersACommandLineNamedSource(t *testing.T) {
	isolatedConfig(t)

	cfg, err := userconfig.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	outcome := runCommandOutcome(t, func() error {
		return writeSourceRegistration(cfg, "owner/repo")
	})

	if outcome.err != nil {
		t.Fatalf("the command-line path failed to register: %v", outcome.err)
	}
	if !strings.Contains(outcome.stderr, "Auto-registered source") {
		t.Errorf("the command-line path stopped announcing the registration:\n%s", outcome.stderr)
	}

	reloaded, err := userconfig.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	entry, ok := reloaded.Registries["owner/repo"]
	if !ok {
		t.Fatal("a command-line-named source was not registered")
	}
	if !entry.AutoRegistered {
		t.Error("the entry lost its auto-registration flag")
	}
	if entry.ApprovedVia != "" || entry.DeclaredIn != "" {
		t.Errorf("a command-line registration carried a project record: %+v", entry)
	}
}

// TestForceHelpDoesNotClaimItSkipsPrompts pins the string a user reads before
// they reach for the flag.
//
// It said "proceed without prompts", which was untrue before this change --
// --force never skipped the install confirmation -- and is further untrue now.
func TestForceHelpDoesNotClaimItSkipsPrompts(t *testing.T) {
	flag := installCmd.Flags().Lookup("force")
	if flag == nil {
		t.Fatal("the --force flag is gone")
		return // t.Fatal ends the test; this keeps the analysis simple.
	}
	if strings.Contains(strings.ToLower(flag.Usage), "without prompts") {
		t.Errorf("--force still claims it proceeds without prompts: %q", flag.Usage)
	}
	if !strings.Contains(strings.ToLower(flag.Usage), "security warnings") {
		t.Errorf("--force no longer describes what it does do: %q", flag.Usage)
	}
}
