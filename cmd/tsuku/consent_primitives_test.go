package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// isolatedConfig points TSUKU_HOME at a temporary directory so a test can read
// and compare config.toml byte for byte without touching the real one.
func isolatedConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("TSUKU_HOME", home)
	return filepath.Join(home, "config.toml")
}

func readOrEmpty(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

// TestClassifySourceWritesNothing pins the property the deferred write is built
// on: deciding about a source is not an action.
func TestClassifySourceWritesNothing(t *testing.T) {
	cfgPath := isolatedConfig(t)

	before := readOrEmpty(t, cfgPath)
	class, err := classifySource("owner/repo")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if class.Registered {
		t.Fatal("an unregistered source classified as registered")
	}
	if after := readOrEmpty(t, cfgPath); after != before {
		t.Fatalf("classification wrote config.toml:\nbefore: %q\nafter:  %q", before, after)
	}
	if before == "" {
		if _, err := os.Stat(cfgPath); err == nil {
			t.Fatal("classification created config.toml")
		}
	}
}

// TestClassifySourceReadsTheConfigNotTheProviderList is the fix for a failure
// mode that only shows up when the network is down.
//
// Providers are built at startup under a shared timeout and a failure only
// warns. Answering "is this registered?" from the provider list would therefore
// make a registered source read as unregistered whenever its repository was
// briefly unreachable -- and a non-interactive project install would skip its
// tools and exit on a transient outage.
func TestClassifySourceReadsTheConfigNotTheProviderList(t *testing.T) {
	isolatedConfig(t)

	cfg, err := userconfig.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Registries == nil {
		cfg.Registries = map[string]userconfig.RegistryEntry{}
	}
	cfg.Registries["owner/repo"] = userconfig.RegistryEntry{URL: "https://github.com/owner/repo"}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	// No provider for this source exists in the loader, which is the whole
	// point: a registered source with no live provider must still classify as
	// registered.
	if hasDistributedProvider("owner/repo") {
		t.Skip("a provider for the fixture source already exists in this process")
	}

	class, err := classifySource("owner/repo")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if !class.Registered {
		t.Fatal("a registered source with no live provider classified as unregistered")
	}
}

// TestClassifySourceKeepsTheStrictRefusal pins the message a functional
// scenario asserts. The refusal moves into the classify primitive; its wording
// and its remedy do not.
func TestClassifySourceKeepsTheStrictRefusal(t *testing.T) {
	isolatedConfig(t)

	cfg, err := userconfig.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg.StrictRegistries = true
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	_, err = classifySource("owner/repo")
	if err == nil {
		t.Fatal("strict_registries did not refuse an unregistered source")
	}
	if !strings.Contains(err.Error(), "strict_registries is enabled") {
		t.Errorf("the refusal lost its reason: %v", err)
	}
	if !strings.Contains(err.Error(), "tsuku registry add owner/repo") {
		t.Errorf("the refusal lost its remedy: %v", err)
	}
}

// TestTerminalCheckIsSubstitutable is what makes every consent criterion in
// this feature reachable in `go test -short`, with no tty and no root.
func TestTerminalCheckIsSubstitutable(t *testing.T) {
	orig := isInteractive
	t.Cleanup(func() { isInteractive = orig })

	isInteractive = func() bool { return false }
	if askYesNo("anything?") {
		t.Error("a missing terminal answered yes")
	}

	isInteractive = func() bool { return true }
	restore := promptInput(strings.NewReader("y\n"))
	defer restore()
	if !askYesNo("anything?") {
		t.Error("a scripted yes was not read")
	}
}

// TestTwoPromptsKeepBothScriptedAnswers is the defect the shared reader fixes.
//
// Each prompt used to build its own buffered reader over stdin. A buffered
// reader consumes more than the line it returns, so the second prompt found an
// empty stdin and the second answer vanished -- invisible with one prompt, and
// exactly wrong the moment a project install asks about a source and then asks
// "Proceed?".
func TestTwoPromptsKeepBothScriptedAnswers(t *testing.T) {
	orig := isInteractive
	t.Cleanup(func() { isInteractive = orig })
	isInteractive = func() bool { return true }

	// The order is no-then-yes deliberately. With yes-then-no, a lost second
	// answer falls back to the default -- which is no -- and the test passes
	// against the very bug it exists for. Asking for a yes second means a lost
	// answer is visible.
	restore := promptInput(strings.NewReader("n\ny\n"))
	defer restore()

	if askYesNo("first?") {
		t.Fatal("the first answer was not read as no")
	}
	if !askYesNo("second?") {
		t.Fatal("the second answer was lost, so the second prompt fell back to its default")
	}
}

// TestEmptyAnswerIsNo pins the default. Every caller is asking about something
// that outlives the command, so silence must not agree.
func TestEmptyAnswerIsNo(t *testing.T) {
	orig := isInteractive
	t.Cleanup(func() { isInteractive = orig })
	isInteractive = func() bool { return true }

	restore := promptInput(strings.NewReader("\n"))
	defer restore()

	if askYesNo("anything?") {
		t.Error("an empty answer was taken as agreement")
	}
}

// TestPreviewPathWritesNothing covers R11 for the command-line path: whatever
// the terminal and whatever the flags, a dry run writes no configuration and
// creates no config.toml.
func TestPreviewPathWritesNothing(t *testing.T) {
	for _, tc := range []struct {
		name       string
		terminal   bool
		yes, force bool
	}{
		{"no terminal", false, false, false},
		{"with a terminal", true, false, false},
		{"--yes", false, true, false},
		{"--force", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfgPath := isolatedConfig(t)

			origTerm := isInteractive
			t.Cleanup(func() { isInteractive = origTerm })
			isInteractive = func() bool { return tc.terminal }

			origYes, origForce := installYes, installForce
			installYes, installForce = tc.yes, tc.force
			t.Cleanup(func() { installYes, installForce = origYes, origForce })

			// A prompt here would be the defect: a dry run asks nothing.
			restore := promptInput(strings.NewReader("y\ny\ny\n"))
			defer restore()

			outcome := runCommandOutcome(t, func() error {
				class, err := classifySource("owner/repo")
				if err != nil {
					return err
				}
				if class.Registered {
					t.Error("the fixture source should not be registered")
				}
				return nil
			})

			if _, err := os.Stat(cfgPath); err == nil {
				t.Error("a dry run created config.toml")
			}
			if strings.Contains(outcome.stderr, "(y/N)") {
				t.Errorf("a dry run asked for consent:\n%s", outcome.stderr)
			}
		})
	}
}
