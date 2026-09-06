package shellquote

import (
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
	// Order is load-bearing. Escaping the quote first would escape the
	// backslash introduced by that escape, yielding '\\\'' rather than '\\\''.
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

// TestRoundTripUnderFish is the same contract for fish. Fish is provisioned in
// CI deliberately: a LookPath-guarded test that skips everywhere would be
// permanently green and read as coverage, and the POSIX/fish divergence is
// precisely what a skipped test would miss.
func TestRoundTripUnderFish(t *testing.T) {
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish not available -- CI provisions it; a local skip is expected")
	}
	for _, tc := range hostile {
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

// TestPOSIXQuoterIsWrongForFish records why two functions exist. It is not a
// test of our code -- it demonstrates that handing fish POSIX-quoted output
// corrupts a backslash, which is the mistake a shared quoter would make and
// which every other fixture in this file would fail to catch.
func TestPOSIXQuoterIsWrongForFish(t *testing.T) {
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish not available")
	}
	// Doubled, not single: POSIX-quoting a single backslash round-trips under
	// fish, so that fixture would assert nothing.
	const value = `a\\b`
	script := "set V " + POSIX(value) + "\nprintf '%s' $V\n"
	out, err := exec.Command(fish, "-c", script).Output()
	if err != nil {
		t.Fatalf("fish rejected the script: %v", err)
	}
	if string(out) == value {
		t.Fatalf("POSIX quoting round-tripped under fish for %q; if this ever passes, "+
			"the dialects have converged and Fish() may no longer be needed", value)
	}
}
