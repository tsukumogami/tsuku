package functional

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	gherkin "github.com/cucumber/gherkin/go/v26"
)

// featureStep is one step of one feature file, with enough location to point a
// reader at the offending line.
type featureStep struct {
	file string
	line int64
	text string
}

// TestEveryFeatureStepIsDefined checks that every step in every feature file
// matches exactly one registered step definition.
//
// The functional suite runs only @critical scenarios on a pull request and needs
// a built binary, so an unbound step in an untagged scenario would otherwise go
// unnoticed until someone read a push-CI log. This test parses the feature files
// directly -- no binary, no tsuku invocation -- so it runs in the ordinary
// unit-test job on every pull request that touches Go code or test/functional/.
func TestEveryFeatureStepIsDefined(t *testing.T) {
	defs := stepDefinitions()
	patterns := make([]*regexp.Regexp, len(defs))
	for i, def := range defs {
		patterns[i] = regexp.MustCompile(def.pattern)
	}

	files, err := filepath.Glob(filepath.Join("features", "*.feature"))
	if err != nil {
		t.Fatalf("globbing feature files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no feature files found; this check would pass vacuously")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			for _, step := range parseFeatureSteps(t, file) {
				var matched []string
				for i, pattern := range patterns {
					if pattern.MatchString(step.text) {
						matched = append(matched, defs[i].pattern)
					}
				}
				switch len(matched) {
				case 1:
					// Exactly one definition claims the step. This is the
					// only case godog can execute.
				case 0:
					t.Errorf("%s:%d: step `%s` matches no step definition; godog reports this as undefined and never runs it",
						step.file, step.line, step.text)
				default:
					t.Errorf("%s:%d: step `%s` matches %d step definitions (%s); godog reports this as ambiguous",
						step.file, step.line, step.text, len(matched), strings.Join(matched, ", "))
				}
			}
		})
	}
}

// TestStepPatternCarriesEscapedQuote pins the reason the check-deps assertions
// went unbound: the quoted-argument pattern has to survive an escaped quote in
// the step text, and the handler has to see the quote the feature author wrote.
func TestStepPatternCarriesEscapedQuote(t *testing.T) {
	pattern := regexp.MustCompile(`^the output does not contain ` + quotedArg + `$`)
	step := `the output does not contain "\"all_satisfied\": true"`

	match := pattern.FindStringSubmatch(step)
	if match == nil {
		t.Fatalf("pattern %s does not match step %s", pattern, step)
	}
	if got, want := unescapeArg(match[1]), `"all_satisfied": true`; got != want {
		t.Errorf("captured argument = %q, want %q", got, want)
	}
}

func TestUnescapeArg(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no escapes", `all_satisfied`, `all_satisfied`},
		{"escaped quotes", `\"all_satisfied\":true`, `"all_satisfied":true`},
		{"escaped backslash", `a\\b`, `a\b`},
		{"unknown escape passes through", `line\nbreak`, `line\nbreak`},
		{"trailing backslash", `path\`, `path\`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unescapeArg(tt.in); got != tt.want {
				t.Errorf("unescapeArg(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// parseFeatureSteps returns every step godog would run for a feature file.
// Pickles expand Scenario Outlines into concrete steps, and the AST supplies
// the line numbers the pickles drop.
func parseFeatureSteps(t *testing.T, path string) []featureStep {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	n := 0
	newID := func() string {
		n++
		return strconv.Itoa(n)
	}

	doc, err := gherkin.ParseGherkinDocument(bytes.NewReader(data), newID)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	if doc.Feature == nil {
		return nil
	}

	// Index AST step ids by line. A step under a Rule: block is not indexed
	// here and reports line 0; it is still checked, which is what matters.
	lines := make(map[string]int64)
	for _, child := range doc.Feature.Children {
		if child.Background != nil {
			for _, step := range child.Background.Steps {
				lines[step.Id] = step.Location.Line
			}
		}
		if child.Scenario != nil {
			for _, step := range child.Scenario.Steps {
				lines[step.Id] = step.Location.Line
			}
		}
	}

	var steps []featureStep
	seen := make(map[string]bool)
	for _, pickle := range gherkin.Pickles(*doc, path, newID) {
		for _, step := range pickle.Steps {
			var line int64
			if len(step.AstNodeIds) > 0 {
				line = lines[step.AstNodeIds[0]]
			}
			// A Background step appears in every pickle of the file, and an
			// outline step repeats per example row. Report each distinct
			// line-and-text pair once.
			key := strconv.FormatInt(line, 10) + "\x00" + step.Text
			if seen[key] {
				continue
			}
			seen[key] = true
			steps = append(steps, featureStep{file: path, line: line, text: step.Text})
		}
	}
	return steps
}
