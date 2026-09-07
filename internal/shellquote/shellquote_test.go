package shellquote

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// hostile is the fixture set both dialects are held to. Every entry is here
// because some plausible implementation gets it wrong, not to illustrate that
// quoting happens.
var hostile = []struct {
	name  string
	value string
}{
	// Command substitution and expansion: the two %q leaves live.
	{"command_substitution", `x$(echo INJECTED)y`},
	// Live under bash and zsh. INERT under fish, which removed backtick
	// substitution -- so its side-effect half proves nothing there, and the
	// fish corpus below uses fish's own forms instead.
	{"backtick", "a`echo INJECTED`b"},
	{"parameter_expansion", `$HOME/x`},
	// The failure the obvious single-quote fix introduces.
	{"embedded_single_quote", `it's a path`},
	// %q renders this as the two characters \n, so it corrupts as well as exposes.
	{"newline", "line1\nline2"},
	// The only fixture that separates POSIX from fish. A *single* backslash
	// round-trips under either quoter -- verified against fish 3.7.1 -- so a
	// fixture using one would report the dialects as interchangeable.
	{"double_backslash", `a\\b`},
	{"single_backslash", `a\b`},
	// Round-trip must be asserted on plain values too, or an assertion on the
	// emitted literal passes for whatever produced the expectation.
	{"plain", "/usr/bin:/bin"},
	{"empty", ""},
	// Belt and braces on the remaining metacharacters.
	{"semicolon_and_glob", `a;b*c?d`},
	{"double_quote", `say "hi"`},
}

func TestPOSIX_EmptyStringIsQuoted(t *testing.T) {
	// An unquoted empty value vanishes from the command line entirely, so ''
	// is required rather than tidy.
	if got := POSIX(""); got != "''" {
		t.Errorf("POSIX(%q) = %q, want %q", "", got, "''")
	}
	if got := Fish(""); got != "''" {
		t.Errorf("Fish(%q) = %q, want %q", "", got, "''")
	}
}

func TestPOSIX_NewlineIsNotEscaped(t *testing.T) {
	// Newlines are literal inside POSIX single quotes. An implementation that
	// escapes them is emitting something the shell reads differently.
	got := POSIX("a\nb")
	if !strings.Contains(got, "\n") {
		t.Errorf("POSIX turned a real newline into an escape: %q", got)
	}
}

func TestFish_EscapesBackslashBeforeQuote(t *testing.T) {
	// Order is load-bearing. Escaping the quote first introduces a backslash,
	// which the backslash pass then escapes as well: the input's one backslash
	// and the introduced one both double, yielding '\\\\'' where the correct
	// order yields '\\\''.
	//
	// This sentence previously gave the same string on both sides of "rather
	// than", so it justified nothing -- and it is the reader's only reason not
	// to swap the two lines in Fish().
	got := Fish(`\'`)
	want := `'\\\''`
	if got != want {
		t.Errorf("Fish(%q) = %q, want %q -- backslash must be escaped first", `\'`, got, want)
	}
}

func TestFish_DiffersFromPOSIXOnBackslash(t *testing.T) {
	// If these ever agree, one function is quoting for the wrong dialect. This
	// is the property that makes two functions necessary; a single shared
	// quoter passes every other test in this file.
	const v = `a\b`
	if POSIX(v) == Fish(v) {
		t.Fatalf("POSIX and Fish agree on %q (%q); fish needs the backslash doubled", v, POSIX(v))
	}
}

// TestRoundTripUnderBash evaluates what we emit in a real bash and asserts two
// things: the value survives byte-identical, and nothing ran. The side-effect
// half is what makes this a safety test rather than a string comparison.
func TestRoundTripUnderBash(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	for _, tc := range hostile {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "marker")
			// Substituting the marker path in means a payload that executes
			// leaves evidence rather than merely differing.
			value := strings.ReplaceAll(tc.value, "INJECTED", "touch "+marker)

			script := "V=" + POSIX(value) + "\nprintf '%s' \"$V\"\n"
			out, err := exec.Command(bash, "--norc", "--noprofile", "-c", script).Output()
			if err != nil {
				t.Fatalf("bash rejected the emitted script: %v\nscript: %s", err, script)
			}
			if string(out) != value {
				t.Errorf("round trip changed the value\n got: %q\nwant: %q", string(out), value)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Errorf("emitted value executed a command under bash: %s", script)
			}
		})
	}
}

// requireFish returns the fish binary, or ends the test.
//
// It skips when fish is absent locally and **fails** when it is absent under
// TSUKU_REQUIRE_FISH, which CI sets. Provisioning fish in the workflow prevents
// today's skip; it does nothing about tomorrow's. A package rename, a base-image
// change, an apt mirror hiccup, or someone deleting the install step all revert
// these tests to skip-and-green, with nothing failing to say so.
//
// That is precisely the defect this whole change is about -- a control that is
// valid for the state it was written against and silently absent afterwards --
// so leaving it in this PR's own CI would be poor form. Fail closed: "CI ran
// without exercising fish" is a red test, not a green skip.
func requireFish(t *testing.T) string {
	t.Helper()
	fish, err := exec.LookPath("fish")
	if err == nil {
		return fish
	}
	if os.Getenv("TSUKU_REQUIRE_FISH") != "" {
		t.Fatal("fish is required here (TSUKU_REQUIRE_FISH is set) but was not found. " +
			"The workflow installs it; if that step changed, these tests were about to " +
			"pass without testing anything.")
	}
	t.Skip("fish not available -- CI provisions it and requires it; a local skip is expected")
	return ""
}

// fishLive holds payloads that actually execute under fish when unquoted.
//
// Re-running the bash corpus under fish is not the same test, and measuring
// which payloads are live there showed why. Against fish 4.0.2, run unquoted to
// simulate a broken quoter:
//
//	$(cmd)   LIVE, but only because fish 3.4 added it
//	(cmd)    LIVE -- fish's own canonical form, and absent from the bash corpus
//	{a,b}    expands, but executes nothing, so its safety half is vacuous;
//	         kept as a round-trip fixture only. (A reviewer reported this as
//	         live; measuring it, brace expansion produces words rather than
//	         running a command, and no marker appears. Recorded because the
//	         difference is exactly the kind a fixture list hides.)
//	`cmd`    INERT -- fish removed backtick substitution
//
// So the shared corpus carried a fixture that cannot fire under fish (the
// backtick, whose side-effect half is vacuous there) and omitted the form a
// fish-specific attacker would reach for. Worse, the one payload it did have
// was live only on new enough fish, which is why assertFishAtLeast34 below
// exists: without it a test on fish 3.3 degrades to a byte-identity check with
// no safety half, and reports success identically.
var fishLive = []struct {
	name  string
	value string
}{
	{"dollar_paren", "x$(touch INJECTED)y"},
	{"bare_paren", "x(touch INJECTED)y"},
	{"brace_expansion", "x{a,b}y"},
	{"embedded_single_quote", `it's a path`},
	{"double_backslash", `a\\b`},
	{"newline", "line1\nline2"},
	{"plain", "/usr/bin:/bin"},
}

// assertFishAtLeast34 pins the property the corpus depends on. $(cmd) is live
// only from fish 3.4, so on an older fish the safety assertions would pass
// without having exercised anything -- and would look identical to success.
func assertFishAtLeast34(t *testing.T, fish string) {
	t.Helper()
	out, err := exec.Command(fish, "--version").Output()
	if err != nil {
		t.Fatalf("could not read fish version: %v", err)
	}
	v := string(out)
	fields := strings.Fields(v)
	ver := fields[len(fields)-1]
	major, minor := 0, 0
	if _, err := fmt.Sscanf(ver, "%d.%d", &major, &minor); err != nil {
		t.Fatalf("could not parse fish version from %q", v)
	}
	if major < 3 || (major == 3 && minor < 4) {
		t.Fatalf("fish %s is older than 3.4, where $(cmd) became live. The safety "+
			"half of this corpus would pass without exercising anything.", ver)
	}
}

// TestRoundTripUnderFish is the same contract for fish, against a fish-derived
// payload set. Fish is provisioned in CI and required there: a LookPath-guarded
// test that skips everywhere would be permanently green and read as coverage,
// and the POSIX/fish divergence is precisely what a skipped test would miss.
func TestRoundTripUnderFish(t *testing.T) {
	fish := requireFish(t)
	assertFishAtLeast34(t, fish)
	for _, tc := range fishLive {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "marker")
			value := strings.ReplaceAll(tc.value, "INJECTED", "touch "+marker)

			script := "set V " + Fish(value) + "\nprintf '%s' $V\n"
			out, err := exec.Command(fish, "-c", script).Output()
			if err != nil {
				t.Fatalf("fish rejected the emitted script: %v\nscript: %s", err, script)
			}
			if string(out) != value {
				t.Errorf("round trip changed the value\n got: %q\nwant: %q", string(out), value)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Errorf("emitted value executed a command under fish: %s", script)
			}
		})
	}
}

// fishDivergent holds the values where POSIX quoting and fish quoting actually
// differ in *effect*, as opposed to differing in output.
//
// This corpus is derived from fish rather than inherited from the bash one, and
// the distinction matters: re-running the bash fixtures under fish proves less
// than it looks like, because seven of the eleven round-trip identically under
// both quoters and so cannot detect a shared-quoter mistake.
//
// Measured by handing the real POSIX and Fish functions to a real fish -- on
// 3.6.0, 3.7.0 (what CI provisions) and 4.0.2. Of nine candidates, exactly
// these four fail when POSIX-quoted output is handed to fish. Three that look
// like they should diverge and do not are recorded so nobody adds them
// expecting signal: a *single* backslash (`a\b`), a literal backslash-n
// (`a\nb`), and a single quote followed by a backslash (`a'\b`).
//
// "By handing the real functions to a real fish" is the load-bearing part. The
// first version of this measurement re-implemented the quoting in sed instead
// of calling POSIX and Fish, reached the correct conclusion about which values
// diverge, and produced the corrupted fixtures below off the wrong bytes. An
// emulation can agree with the real thing about the answer and still be
// measuring something else.

// backslashes states how many backslashes the value is supposed to contain.
// It is not decoration: these four fixtures were once written through a shell
// that interpreted the escapes, and three of them arrived holding something
// else entirely -- still valid Go, still compiling, still green everywhere
// except a real fish. Declaring the count beside the value lets a test hold the
// fixture to its own intent. See TestFishDivergentCorpusIsIntact.
var fishDivergent = []struct {
	name        string
	value       string
	backslashes int
}{
	{"two_backslashes", `a\\b`, 2},
	{"three_backslashes", `a\\\b`, 3},
	{"backslash_then_quote", `a\'b`, 1},
	{"trailing_backslash", `ab\`, 1},
}

// TestFishDivergentCorpusIsIntact holds the fixtures to what they claim to be,
// because the alternative is a corpus that reports success while testing
// nothing.
//
// This is not hypothetical. The corpus was authored through a shell that
// consumed the escapes, so `a\\b` landed as `a\b`, `a\\\b` landed as a
// backslash followed by a literal backspace byte, and `a\'b` lost its backslash
// and landed as plain `a'b`. All three still compile. Worse, `a\b` is a value
// the comment above lists as a known *non*-discriminator, so the corpus had
// quietly degenerated into the exact values it warns against. Nothing noticed
// until the negative control below met a real fish in CI.
//
// Both halves are load-bearing. The count catches an escape the shell ate; the
// control-byte scan catches an escape the shell turned into the byte it names,
// which no count would see.
func TestFishDivergentCorpusIsIntact(t *testing.T) {
	for _, tc := range fishDivergent {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Count(tc.value, `\`); got != tc.backslashes {
				t.Errorf("fixture holds %d backslashes but claims %d (value %q).\n\n"+
					"A fixture that lost a backslash still compiles and still round-trips, "+
					"so nothing else in this file will tell you. Check how it was written: "+
					"a shell heredoc eats these.", got, tc.backslashes, tc.value)
			}
			for i, r := range tc.value {
				if r < 0x20 || r == 0x7f {
					t.Errorf("fixture holds control byte %#x at index %d (value %q).\n\n"+
						"That is escape interpretation rather than an intended fixture: "+
						"a shell turned an escape into the byte it names.", r, i, tc.value)
				}
			}
		})
	}
}

// TestFish_DivergentValuesRoundTrip holds Fish() to the cases that separate the
// dialects. Without these, "we fixed the quoter" means "we fixed bash".
func TestFish_DivergentValuesRoundTrip(t *testing.T) {
	fish := requireFish(t)
	for _, tc := range fishDivergent {
		t.Run(tc.name, func(t *testing.T) {
			script := "set V " + Fish(tc.value) + "\nprintf '%s' $V\n"
			out, err := exec.Command(fish, "-c", script).Output()
			if err != nil {
				t.Fatalf("fish rejected the emitted script: %v\nscript: %s", err, script)
			}
			if string(out) != tc.value {
				t.Errorf("round trip changed the value\n got: %q\nwant: %q", string(out), tc.value)
			}
		})
	}
}

// TestPOSIXQuoterUnderFishFailsOnDivergentValues is the negative control. Each
// of these must fail when POSIX-quoted and read by fish -- if one ever stops
// failing, it has lost its discriminating power and the corpus above is weaker
// than it looks.
func TestPOSIXQuoterUnderFishFailsOnDivergentValues(t *testing.T) {
	fish := requireFish(t)
	for _, tc := range fishDivergent {
		t.Run(tc.name, func(t *testing.T) {
			script := "set V " + POSIX(tc.value) + "\nprintf '%s' $V\n"
			out, err := exec.Command(fish, "-c", script).Output()
			if err == nil && string(out) == tc.value {
				t.Errorf("POSIX quoting round-tripped %q under fish; this fixture no "+
					"longer discriminates between the dialects", tc.value)
			}
		})
	}
}
