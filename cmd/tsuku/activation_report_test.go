package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/activation"
)

// The substring PRD R18 pins for each reason, and the reason that must produce
// it. Each substring must also be absent from all four other messages, which is
// what makes the five distinguishable rather than merely worded differently.
var reasonSubstrings = map[string]string{
	"no-match":      "nothing installed matches",
	"bad-form":      "not a valid version string",
	"channel":       "channel pin",
	"unreadable":    "could not read",
	"missing-files": "recorded as installed but its files are missing",
}

// oneMessagePerReason builds a result carrying exactly one reason, so each
// message can be examined alone.
func oneMessagePerReason(t *testing.T) map[string]string {
	t.Helper()

	results := map[string]*activation.ActivationResult{
		"no-match": {Entered: true, Unhonorable: []activation.Unhonorable{
			{Tool: "jq", Declared: "2", Reason: activation.ReasonNoMatch},
		}},
		"bad-form": {Entered: true, Unhonorable: []activation.Unhonorable{
			{Tool: "nodejs", Declared: ">=26", Reason: activation.ReasonBadForm},
		}},
		"channel": {Entered: true, Unhonorable: []activation.Unhonorable{
			{Tool: "nodejs", Declared: "@lts", Reason: activation.ReasonChannel},
		}},
		"missing-files": {Entered: true, Unhonorable: []activation.Unhonorable{
			{Tool: "jq", Declared: "latest", Reason: activation.ReasonMissingFiles, Version: "1.7.1"},
		}},
		"unreadable": {Entered: true, Unreadable: &activation.StateUnreadable{Tools: []string{"jq"}}},
	}

	out := map[string]string{}
	for name, r := range results {
		lines := activationMessages(r)
		if len(lines) != 1 {
			t.Fatalf("%s: got %d messages, want 1: %v", name, len(lines), lines)
		}
		out[name] = lines[0]
	}
	return out
}

func TestActivationMessages_EachReasonHasItsPinnedSubstring(t *testing.T) {
	messages := oneMessagePerReason(t)

	for reason, substring := range reasonSubstrings {
		if !strings.Contains(messages[reason], substring) {
			t.Errorf("%s message %q is missing %q", reason, messages[reason], substring)
		}
	}
}

// Each pinned substring appears in exactly one of the five messages. Without
// this, five sentences sharing a phrase would satisfy every containment
// assertion above while being indistinguishable to a developer reading them.
func TestActivationMessages_SubstringsAreDisjoint(t *testing.T) {
	messages := oneMessagePerReason(t)

	for reason, substring := range reasonSubstrings {
		for other, msg := range messages {
			if other == reason {
				continue
			}
			if strings.Contains(msg, substring) {
				t.Errorf("%q belongs to %s but also appears in the %s message: %q",
					substring, reason, other, msg)
			}
		}
	}
}

// Every message names the tool the developer has to go and fix.
func TestActivationMessages_EachNamesItsTool(t *testing.T) {
	messages := oneMessagePerReason(t)

	wantTool := map[string]string{
		"no-match": "jq", "bad-form": "nodejs", "channel": "nodejs",
		"missing-files": "jq", "unreadable": "jq",
	}
	for reason, tool := range wantTool {
		if !strings.Contains(messages[reason], tool) {
			t.Errorf("%s message %q does not name %q", reason, messages[reason], tool)
		}
	}
}

// The payloads are used in the messages, not merely carried on the struct. A
// write-only field satisfies "the reasons carry different data" on inspection
// while the output is still one template with dead fields.
func TestActivationMessages_PayloadsReachTheOutput(t *testing.T) {
	messages := oneMessagePerReason(t)

	// missing-files carries the version to reinstall.
	if !strings.Contains(messages["missing-files"], "1.7.1") {
		t.Errorf("missing-files message %q does not name the version to reinstall", messages["missing-files"])
	}
	// bad-form and channel carry the declared string the developer will edit.
	if !strings.Contains(messages["bad-form"], ">=26") {
		t.Errorf("bad-form message %q does not name the declared string", messages["bad-form"])
	}
	if !strings.Contains(messages["channel"], "@lts") {
		t.Errorf("channel message %q does not name the declared string", messages["channel"])
	}
	// no-match carries neither: it is about the installed set, and a message
	// that quoted the pin here would make a single shared template possible.
	if strings.Contains(messages["no-match"], "1.7.1") {
		t.Errorf("no-match message %q should not carry a version", messages["no-match"])
	}
}

// A malformed tool name gets its own sentence, and that sentence does not blame
// the version.
//
// Found by running the real binary, not by a unit test: the reason and the tool
// name were both correct, and every assertion about them passed, while the
// message told the developer to fix a version string that was perfectly good.
// Asserting the classification is not the same as reading the sentence.
func TestActivationMessages_BadNameDoesNotBlameTheVersion(t *testing.T) {
	lines := activationMessages(&activation.ActivationResult{
		Entered: true,
		Unhonorable: []activation.Unhonorable{{
			Tool:     "../../../etc",
			Declared: "latest",
			Reason:   activation.ReasonBadForm,
			BadName:  true,
		}},
	})

	if len(lines) != 1 {
		t.Fatalf("got %d messages, want 1: %v", len(lines), lines)
	}
	msg := lines[0]

	if strings.Contains(msg, "not a valid version string") {
		t.Errorf("message blames the version, which is fine here: %q", msg)
	}
	if !strings.Contains(msg, "tool name") {
		t.Errorf("message should say the name is the problem: %q", msg)
	}
	if !strings.Contains(msg, "../../../etc") {
		t.Errorf("message should quote the offending key: %q", msg)
	}
	// "latest" is the version, and it is not what is wrong. It must not be
	// presented as the thing to fix.
	if strings.Contains(msg, `as "latest"`) {
		t.Errorf("message presents a good version as the fault: %q", msg)
	}
}

// And a malformed version still gets the version sentence.
func TestActivationMessages_BadVersionStillBlamesTheVersion(t *testing.T) {
	lines := activationMessages(&activation.ActivationResult{
		Entered: true,
		Unhonorable: []activation.Unhonorable{{
			Tool: "nodejs", Declared: ">=26", Reason: activation.ReasonBadForm,
		}},
	})

	if !strings.Contains(lines[0], "not a valid version string") {
		t.Errorf("message = %q, want the version phrasing", lines[0])
	}
	if !strings.Contains(lines[0], ">=26") {
		t.Errorf("message = %q, want the declared string", lines[0])
	}
}

// Undecodable state produces exactly one message however many tools needed the
// read, because the failure is a property of the read rather than of any
// declaration.
func TestActivationMessages_UnreadableIsOneMessageForTenTools(t *testing.T) {
	tools := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	lines := activationMessages(&activation.ActivationResult{
		Entered:    true,
		Unreadable: &activation.StateUnreadable{Tools: tools},
	})

	if len(lines) != 1 {
		t.Fatalf("got %d messages for ten unresolvable tools, want 1: %v", len(lines), lines)
	}
	for _, tool := range tools {
		if !strings.Contains(lines[0], tool) {
			t.Errorf("message %q does not name %q", lines[0], tool)
		}
	}
}

// A file mixing a satisfiable and an unsatisfiable declaration reports only the
// second, and two unsatisfiable declarations produce two messages, each naming
// its own tool.
func TestActivationMessages_OnePerUnhonorableDeclaration(t *testing.T) {
	lines := activationMessages(&activation.ActivationResult{
		Entered: true,
		Unhonorable: []activation.Unhonorable{
			{Tool: "jq", Declared: "2", Reason: activation.ReasonNoMatch},
			{Tool: "nodejs", Declared: "@lts", Reason: activation.ReasonChannel},
		},
	})

	if len(lines) != 2 {
		t.Fatalf("got %d messages, want 2: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "jq") || !strings.Contains(lines[1], "nodejs") {
		t.Errorf("messages should name their own tools in order, got %v", lines)
	}
}

func TestActivationMessages_NothingToReportProducesNothing(t *testing.T) {
	if lines := activationMessages(&activation.ActivationResult{Entered: true}); len(lines) != 0 {
		t.Errorf("got %v, want no messages", lines)
	}
	if lines := activationMessages(nil); len(lines) != 0 {
		t.Errorf("got %v for a nil result, want no messages", lines)
	}
}

// captureStreams runs fn with both standard streams replaced, and returns what
// each received. Asserting the split by string comparison is the point: a test
// that only checked the shell survived an eval would pass with a diagnostic on
// stdout that happened to be a comment or a no-op.
func captureStreams(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	// fn writes well under the pipe buffer, so reading after it returns cannot
	// deadlock. A larger writer would need a concurrent reader.
	fn()
	os.Stdout, os.Stderr = origOut, origErr

	_ = outW.Close()
	_ = errW.Close()

	return readAll(t, outR), readAll(t, errR)
}

func readAll(t *testing.T, f *os.File) string {
	t.Helper()
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := f.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	_ = f.Close()
	return b.String()
}

// Standard output carries exclusively the shell code both entry points are
// eval'd for; the diagnostics are on stderr and nowhere else.
func TestReportActivation_StreamsAreSeparate(t *testing.T) {
	for _, shell := range []string{"bash", "fish"} {
		t.Run(shell, func(t *testing.T) {
			toml := `
[tools]
jq = "@lts"
node = "20.16.0"
`
			projectDir, cfg := shellSetupProject(t, toml, map[string][]string{
				"node": {"20.16.0"},
			})
			t.Setenv("PATH", "/usr/bin")
			t.Setenv("HOME", filepath.Dir(projectDir))

			var output string
			stdout, stderr := captureStreams(t, func() {
				var err error
				output, err = runShell(projectDir, "", shell, cfg)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			})

			// runShell returns the shell code rather than printing it, so
			// stdout must be empty and the diagnostic must be on stderr.
			if stdout != "" {
				t.Errorf("stdout = %q, want empty: only shell code belongs there", stdout)
			}
			if !strings.Contains(stderr, "channel pin") {
				t.Errorf("stderr = %q, want the channel diagnostic", stderr)
			}
			if strings.Contains(output, "channel pin") {
				t.Errorf("shell code contains a diagnostic:\n%s", output)
			}

			wantEntry := filepath.Join(cfg.ToolsDir, "node-20.16.0", "bin")
			if !strings.Contains(output, wantEntry) {
				t.Errorf("shell code should still activate node, got:\n%s", output)
			}
		})
	}
}

// --quiet suppresses every reason on both commands, and changes nothing about
// PATH.
func TestReportActivation_QuietSuppressesEveryReason(t *testing.T) {
	orig := quietFlag
	t.Cleanup(func() { quietFlag = orig })

	result := &activation.ActivationResult{
		Entered: true,
		Unhonorable: []activation.Unhonorable{
			{Tool: "a", Declared: "2", Reason: activation.ReasonNoMatch},
			{Tool: "b", Declared: ">=1", Reason: activation.ReasonBadForm},
			{Tool: "c", Declared: "@lts", Reason: activation.ReasonChannel},
			{Tool: "d", Declared: "1", Reason: activation.ReasonMissingFiles, Version: "1.0.0"},
		},
		Unreadable: &activation.StateUnreadable{Tools: []string{"e"}},
	}

	quietFlag = false
	_, loud := captureStreams(t, func() { reportActivation(result) })
	if strings.Count(loud, "\n") != 5 {
		t.Fatalf("without --quiet, got %d lines, want 5:\n%s", strings.Count(loud, "\n"), loud)
	}

	quietFlag = true
	_, quiet := captureStreams(t, func() { reportActivation(result) })
	if quiet != "" {
		t.Errorf("--quiet should suppress all five, got:\n%s", quiet)
	}
}

// The once-per-entry budget, end to end through the real hook-env command:
// entering a project with an unhonorable declaration reports once, and staying
// there reports nothing however many prompts fire.
//
// This exists because the two unit tests below and in internal/activation cover
// the halves and not the composition. TestReportActivation_SilentWhenNotEntering
// hands itself Entered=false, so it would keep passing if ComputeActivation
// stopped setting the flag correctly in the real flow; TestResolve_EnteredTracks
// TheDirectoryChange checks the flag but never renders a message. A cadence that
// broke between them -- the hook nagging on every cd -- would leave both green.
//
// Each subsequent environment is built only from what the previous invocation
// emitted. Assembling it by hand would manufacture exactly the state a broken
// implementation failed to produce.
func TestHookEnv_ReasonsAreReportedOncePerEntry(t *testing.T) {
	projectDir, cfg := shellSetupProject(t, "[tools]\njq = \"9\"\n",
		map[string][]string{"jq": {"1.7.1"}})

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))
	t.Setenv("TSUKU_HOME", cfg.HomeDir)
	chdir(t, projectDir)

	// Arrival: the declaration cannot be honored, so it is reported.
	stdout, stderr, err := runHookEnv(t, "bash")
	if err != nil {
		t.Fatalf("unexpected error on arrival: %v", err)
	}
	if !strings.Contains(stderr, "nothing installed matches") {
		t.Fatalf("expected the no-match reason on arrival, got:\n%s", stderr)
	}

	applyExports(t, stdout)

	// Every subsequent prompt in the same project is silent.
	for i := 2; i <= 4; i++ {
		out, errOut, err := runHookEnv(t, "bash")
		if err != nil {
			t.Fatalf("unexpected error on prompt %d: %v", i, err)
		}
		if errOut != "" {
			t.Errorf("prompt %d in the same project reported again, so the hook "+
				"nags on every cd:\n%s", i, errOut)
		}
		applyExports(t, out)
	}

	// A subdirectory of the same project is still the same project.
	sub := filepath.Join(projectDir, "pkg")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	chdir(t, sub)

	if _, errOut, err := runHookEnv(t, "bash"); err != nil {
		t.Fatalf("unexpected error from a subdirectory: %v", err)
	} else if errOut != "" {
		t.Errorf("moving within the project reported again:\n%s", errOut)
	}
}

// applyExports feeds an emitted block back into the environment, the way the
// shell's eval would — assignments and unsets both.
//
// The unsets matter as much as the assignments: deactivation's whole job is to
// clear the tracking variables, and a replay that only applied the assignments
// would leave them set and make a later re-entry look like a fresh one for the
// wrong reason.
func applyExports(t *testing.T, script string) {
	t.Helper()
	for name, value := range parseExports(script) {
		t.Setenv(name, value)
	}
	for _, name := range parseUnsets(script) {
		t.Setenv(name, "")
	}
}

// Leaving the project and coming back is a new entry, so it reports again --
// the budget is per entry, not once per shell.
func TestHookEnv_ReasonsReportAgainOnReEntry(t *testing.T) {
	projectDir, cfg := shellSetupProject(t, "[tools]\njq = \"9\"\n",
		map[string][]string{"jq": {"1.7.1"}})
	outside := t.TempDir()

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("HOME", filepath.Dir(projectDir))
	t.Setenv("TSUKU_HOME", cfg.HomeDir)
	chdir(t, projectDir)

	stdout, stderr, err := runHookEnv(t, "bash")
	if err != nil || !strings.Contains(stderr, "nothing installed matches") {
		t.Fatalf("expected a report on first entry, got err=%v stderr=%q", err, stderr)
	}
	applyExports(t, stdout)

	// Leave: deactivation, and nothing to say on the way out.
	chdir(t, outside)
	stdout, stderr, err = runHookEnv(t, "bash")
	if err != nil {
		t.Fatalf("unexpected error leaving: %v", err)
	}
	if stderr != "" {
		t.Errorf("leaving a project should say nothing, got:\n%s", stderr)
	}
	// Apply what deactivation emitted, including its unsets, rather than
	// clearing the variables by hand. Hand-clearing would make the re-entry
	// below pass even if deactivation failed to unset anything -- the same
	// self-blinding move this file warns about one test up.
	applyExports(t, stdout)

	// Come back: a fresh entry, so the developer is told again.
	chdir(t, projectDir)
	if _, stderr, err = runHookEnv(t, "bash"); err != nil {
		t.Fatalf("unexpected error on re-entry: %v", err)
	} else if !strings.Contains(stderr, "nothing installed matches") {
		t.Errorf("re-entering the project should report again, got:\n%s", stderr)
	}
}

// The once-per-entry budget is gated on the flag activation computed, so a
// prompt hook firing again in the same directory says nothing more.
func TestReportActivation_SilentWhenNotEntering(t *testing.T) {
	result := &activation.ActivationResult{
		Entered:     false,
		Unhonorable: []activation.Unhonorable{{Tool: "jq", Reason: activation.ReasonNoMatch}},
	}
	_, stderr := captureStreams(t, func() { reportActivation(result) })
	if stderr != "" {
		t.Errorf("re-activating the same directory should say nothing, got:\n%s", stderr)
	}
}
