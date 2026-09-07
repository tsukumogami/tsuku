package shellquote

import (
	"os"
	"strings"
	"testing"
)

// TestEveryCorpusIsIntact holds all three fixture sets to what their names say.
//
// TestFishDivergentCorpusIsIntact covers only fishDivergent, and that gap was
// not theoretical: fishLive carried a fixture named "double_backslash" holding
// a single backslash -- the exact value this package's own comments name as the
// one that round-trips identically under both quoters and therefore proves
// nothing. Third instance of the same defect on this branch, and the first two
// were what the divergent-corpus guard was written for.
//
// The lesson is about where a guard is wired rather than what it checks. A
// check that covers one of three corpora reports success identically whether
// the other two are intact or gutted, which is the same shape as a validator
// wired to one consumer.
func TestEveryCorpusIsIntact(t *testing.T) {
	corpora := map[string][]struct{ name, value string }{}

	for _, tc := range hostile {
		corpora["hostile"] = append(corpora["hostile"], struct{ name, value string }{tc.name, tc.value})
	}
	for _, tc := range fishLive {
		corpora["fishLive"] = append(corpora["fishLive"], struct{ name, value string }{tc.name, tc.value})
	}
	for _, tc := range fishDivergent {
		corpora["fishDivergent"] = append(corpora["fishDivergent"], struct{ name, value string }{tc.name, tc.value})
	}

	// A fixture whose name states a backslash count must hold that many. The
	// names are the only record of intent once the value is wrong, because a
	// value with the wrong number of backslashes is still a valid Go string and
	// still round-trips.
	wantCount := map[string]int{
		"single_backslash":  1,
		"double_backslash":  2,
		"three_backslashes": 3,
		"two_backslashes":   2,
	}

	for corpus, fixtures := range corpora {
		for _, tc := range fixtures {
			t.Run(corpus+"/"+tc.name, func(t *testing.T) {
				if want, ok := wantCount[tc.name]; ok {
					if got := strings.Count(tc.value, `\`); got != want {
						t.Errorf("%s fixture %q holds %d backslashes, its name says %d (value %q).\n\n"+
							"A fixture that lost a backslash still compiles and still round-trips, "+
							"so nothing else here will tell you.", corpus, tc.name, got, want, tc.value)
					}
				}
				for i, r := range tc.value {
					// Newline is a deliberate fixture in two of these corpora;
					// every other control byte is escape interpretation.
					if r == '\n' {
						continue
					}
					if r < 0x20 || r == 0x7f {
						t.Errorf("%s fixture %q holds control byte %#x at index %d (value %q).\n\n"+
							"That is a shell or parser turning an escape into the byte it names, "+
							"not an intended fixture.", corpus, tc.name, r, i, tc.value)
					}
				}
			})
		}
	}
}

// TestSourceHasNoTypographicQuotes guards the package documentation against its
// own formatter.
//
// gofmt reformats doc comments and rewrites a pair of apostrophes as a Unicode
// right quotation mark. In this package that is not cosmetic: the close-reopen
// idiom and the empty-quoted-pair are the two things the documentation exists
// to state exactly, and both are spelled with apostrophe pairs. It had already
// happened -- two U+201D characters were sitting in POSIX's doc comment,
// gofmt-clean, describing an escape idiom that no longer contained the escape.
//
// The fix was to move those into indented lines, which gofmt treats as
// preformatted. This test fails if anyone moves them back into prose, because
// the formatter will convert them again and nothing else would say so.
func TestSourceHasNoTypographicQuotes(t *testing.T) {
	for _, name := range []string{"shellquote.go", "shellquote_test.go", "corpus_intact_test.go"} {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		// Written as escapes, not literals: a literal here would trip the
		// check against this very file.
		for _, bad := range []string{"\u201c", "\u201d", "\u2018", "\u2019"} {
			if i := strings.Index(string(src), bad); i >= 0 {
				line := 1 + strings.Count(string(src)[:i], "\n")
				t.Errorf("%s:%d contains the typographic quote %q.\n\n"+
					"gofmt produces these from apostrophe pairs in doc-comment prose. "+
					"Put the idiom on an indented line instead -- those are preformatted "+
					"and the formatter leaves them alone.", name, line, bad)
			}
		}
	}
}
