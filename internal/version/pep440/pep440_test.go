package pep440

import (
	"errors"
	"strings"
	"testing"
)

// surveyRequirements is a representative sample of real-world
// `requires_python` strings drawn from the L5 research (poetry,
// ansible, black, mypy, ruff, flake8, pylint, isort, tox, pipx,
// httpx, requests, django, numpy, pandas, plus a few extras
// covering the documented format-variance cases).
//
// All entries must parse without error and produce a non-empty
// Specifier. They serve as the golden table for the parser's
// positive coverage.
var surveyRequirements = []string{
	// ansible-core minor lines
	">=3.10",
	">=3.11",
	">=3.12",
	// poetry-style with upper bound and whitespace variance
	">=3.6,<4.0",
	">= 3.6.0.0, < 4.0.0.0",
	">=3.7,<4.0",
	">=3.8,<4.0",
	">=3.9,<4.0",
	">=3.10,<4.0",
	"<4.0,>=3.10",
	// requests / numpy / flake8 / tox / pylint / isort / cookiecutter:
	// !=X.Y.* exclusions (the long-tail wildcard form, ~40% of clauses).
	">=2.7,!=3.0.*,!=3.1.*,!=3.2.*,!=3.3.*",
	">=2.7,!=3.0.*,!=3.1.*,!=3.2.*,!=3.3.*,!=3.4.*",
	"!=3.0.*,!=3.1.*,!=3.2.*,>=2.7",
	">=3.6,!=3.6.0,!=3.6.1",
	// black, mypy, ruff
	">=3.8",
	">=3.9",
	// patch-level floors
	">=3.6.2",
	">=3.6.0",
	">=3.7.1",
	">=3.8.1",
	// upper bounds without lower
	"<3.13",
	"<3.14",
	// 4-segment versions
	">=3.6.0.0,<4.0.0.0",
	// django range
	">=3.10,<5.3",
	// httpx / pipx
	">=3.8,<4",
	// equality (rare, but valid)
	"==3.10",
	"==3.10.*",
	// negation equality (rare)
	"!=3.0.0",
}

// targetVersionsForSurvey is a set of plausible "bundled Python"
// versions tsuku might ship. Used to verify Satisfies returns a
// boolean (not crash) for every survey string.
var targetVersionsForSurvey = []string{"3.10", "3.11", "3.12", "3.13", "3.10.4", "3.13.0"}

func TestParseSpecifier_SurveyStrings(t *testing.T) {
	for _, in := range surveyRequirements {
		t.Run(in, func(t *testing.T) {
			spec, err := ParseSpecifier(in)
			if err != nil {
				t.Fatalf("ParseSpecifier(%q) error: %v", in, err)
			}
			// Sanity: every target version should evaluate without panicking.
			for _, tv := range targetVersionsForSurvey {
				v, perr := ParseVersion(tv)
				if perr != nil {
					t.Fatalf("ParseVersion(%q) failed: %v", tv, perr)
				}
				_ = spec.Satisfies(v)
			}
		})
	}
}

func TestCanonical_SurveyRoundTrip(t *testing.T) {
	for _, in := range surveyRequirements {
		t.Run(in, func(t *testing.T) {
			c := Canonical(in)
			if c == "<malformed>" {
				t.Fatalf("Canonical(%q) returned malformed marker", in)
			}
			// Canonical output must itself parse cleanly.
			if _, err := ParseSpecifier(c); err != nil {
				t.Fatalf("Canonical(%q) = %q does not re-parse: %v", in, c, err)
			}
			// Canonical output is ASCII.
			for i := range len(c) {
				if c[i] > 0x7F {
					t.Fatalf("Canonical(%q) contains non-ASCII byte at %d: %q", in, i, c)
				}
			}
		})
	}
}

func TestParseSpecifier_HardeningChecks(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr error
	}{
		{
			name:    "total length over 1024 bytes",
			input:   ">=" + strings.Repeat("3", 1100),
			wantErr: ErrInputTooLong,
		},
		{
			name:    "more than 32 clauses",
			input:   ">=3.6," + strings.Repeat(">=3.7,", 35) + ">=3.8",
			wantErr: ErrInputTooLong,
		},
		{
			name:    "single clause longer than 256 bytes",
			input:   ">=3." + strings.Repeat("0", 300),
			wantErr: ErrInputTooLong,
		},
		{
			name:    "non-ASCII byte (zero-width space)",
			input:   ">=3.10​",
			wantErr: ErrNonASCII,
		},
		{
			name:    "segment with > 6 digits",
			input:   ">=99999999",
			wantErr: ErrSegmentTooLarge,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSpecifier(tc.input)
			if err == nil {
				t.Fatalf("ParseSpecifier(%q) expected error, got nil", tc.input)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ParseSpecifier(%q) error = %v, want wrap of %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestParseSpecifier_UnsupportedOperators(t *testing.T) {
	cases := []string{
		"~=3.6",
		"===3.10",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, err := ParseSpecifier(in)
			if err == nil {
				t.Fatalf("ParseSpecifier(%q) expected ErrUnsupportedOperator, got nil", in)
			}
			if !errors.Is(err, ErrUnsupportedOperator) {
				t.Errorf("ParseSpecifier(%q) error = %v, want wrap of ErrUnsupportedOperator", in, err)
			}
		})
	}
}

func TestParseSpecifier_Malformed(t *testing.T) {
	cases := []string{
		"",             // empty
		"   ",          // whitespace only
		"3.10",         // missing operator
		">=",           // missing version
		">=3.10,",      // trailing comma → empty clause
		">=,<3.13",     // empty leading clause
		">=3.10,<=*",   // wildcard without prefix
		">=3.10.*",     // wildcard not allowed for >=
		">=foo",        // non-numeric
		">=3.foo",      // non-numeric in segment
		">=3.10.0.0.0", // too many segments
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, err := ParseSpecifier(in)
			if err == nil {
				t.Fatalf("ParseSpecifier(%q) expected error, got nil", in)
			}
		})
	}
}

func TestCanonical_MalformedReturnsLiteral(t *testing.T) {
	cases := []string{
		"",
		">=" + strings.Repeat("3", 1100), // length cap
		">=3.6," + strings.Repeat(">=3.7,", 35) + ">=3.8", // clause count
		">=3.10​",    // non-ASCII
		">=99999999", // segment too large
		"~=3.6",      // unsupported operator
		"3.10",       // missing operator
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			got := Canonical(in)
			if got != "<malformed>" {
				t.Errorf("Canonical(%q) = %q, want \"<malformed>\"", in, got)
			}
		})
	}
}

func TestSatisfies_BundledPythonScenarios(t *testing.T) {
	cases := []struct {
		spec   string
		target string
		want   bool
	}{
		// ansible-core release lines vs bundled Python 3.10
		{">=3.10", "3.10", true},
		{">=3.11", "3.10", false},
		{">=3.12", "3.10", false},
		// Python 3.13 (current python-build-standalone) against modern pins
		{">=3.10", "3.13", true},
		{">=3.11", "3.13", true},
		{">=3.12", "3.13", true},
		// Range with upper bound (poetry-style)
		{">=3.6,<4.0", "3.10", true},
		{">=3.6,<4.0", "4.0", false},
		// Wildcards
		{"==3.10.*", "3.10.4", true},
		{"==3.10.*", "3.11.0", false},
		{"!=3.0.*", "3.0.5", false},
		{"!=3.0.*", "3.10", true},
		// Patch-level floor
		{">=3.6.2", "3.6.1", false},
		{">=3.6.2", "3.6.2", true},
		{">=3.6.2", "3.7.0", true},
		// 4-segment compatibility (segment padding to zero)
		{">=3.6.0.0,<4.0.0.0", "3.10", true},
		// Long requests-style exclusion
		{">=2.7,!=3.0.*,!=3.1.*,!=3.2.*,!=3.3.*", "3.10", true},
		{">=2.7,!=3.0.*,!=3.1.*,!=3.2.*,!=3.3.*", "3.0.5", false},
		// Operator-order variance
		{"<4.0,>=3.10", "3.13", true},
		{"<4.0,>=3.10", "3.9", false},
	}
	for _, tc := range cases {
		t.Run(tc.spec+"|"+tc.target, func(t *testing.T) {
			spec, err := ParseSpecifier(tc.spec)
			if err != nil {
				t.Fatalf("ParseSpecifier(%q) error: %v", tc.spec, err)
			}
			v, err := ParseVersion(tc.target)
			if err != nil {
				t.Fatalf("ParseVersion(%q) error: %v", tc.target, err)
			}
			got := spec.Satisfies(v)
			if got != tc.want {
				t.Errorf("ParseSpecifier(%q).Satisfies(%q) = %v, want %v", tc.spec, tc.target, got, tc.want)
			}
		})
	}
}

func TestVersion_Compare(t *testing.T) {
	mustParse := func(s string) Version {
		v, err := ParseVersion(s)
		if err != nil {
			t.Fatalf("ParseVersion(%q) error: %v", s, err)
		}
		return v
	}
	cases := []struct {
		a, b string
		want int
	}{
		{"3.10", "3.10", 0},
		{"3.10", "3.10.0", 0}, // missing components default to 0
		{"3.10", "3.10.0.0", 0},
		{"3.10.1", "3.10", 1},
		{"3.10", "3.11", -1},
		{"3.9", "3.10", -1},
		{"4", "3.13", 1},
	}
	for _, tc := range cases {
		t.Run(tc.a+" vs "+tc.b, func(t *testing.T) {
			got := mustParse(tc.a).Compare(mustParse(tc.b))
			if got != tc.want {
				t.Errorf("%s.Compare(%s) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestParseVersion_Errors(t *testing.T) {
	cases := []struct {
		input   string
		wantErr error
	}{
		{"", ErrMalformed},
		{"3.foo", ErrMalformed},
		{"3..10", ErrMalformed},
		{"99999999", ErrSegmentTooLarge},
		{"3.10​", ErrNonASCII},
		{"3.10.0.0.0", ErrMalformed}, // > 4 segments
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			_, err := ParseVersion(tc.input)
			if err == nil {
				t.Fatalf("ParseVersion(%q) expected error, got nil", tc.input)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ParseVersion(%q) error = %v, want wrap of %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestVersion_String(t *testing.T) {
	cases := []string{"3", "3.10", "3.10.0", "3.10.0.0"}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			v, err := ParseVersion(in)
			if err != nil {
				t.Fatalf("ParseVersion(%q) error: %v", in, err)
			}
			got := v.String()
			if got != in {
				t.Errorf("Version(%q).String() = %q, want %q", in, got, in)
			}
		})
	}
}
