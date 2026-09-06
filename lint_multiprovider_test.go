package main_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// multiProviderPackages are the only packages that construct or consume
// index.BinaryMatch. A multi-provider case built anywhere in here has to come
// from internal/indexfixture, which is where the sanctioned constructor lives
// and is deliberately not in this list.
var multiProviderPackages = []string{
	"internal/autoinstall",
	"internal/project",
	"internal/index",
	"cmd/tsuku",
}

// fixtureCheckScanRoot is the tree the negative control lives in:
// internal/indexfixture, which holds conforming test files of its own plus
// testdata/fixturecheck/nonconforming_test.go, a file that violates both
// rules.
//
// The control sits under testdata/ because the Go tool never compiles anything
// there, and a file that breaks the rules on purpose has to be a file that
// never builds. That is why the scanner below descends into testdata/ rather
// than skipping it the way TestNoStdlibLog does.
//
// The scan root is the package directory rather than the testdata directory
// itself, and that matters: pointing it straight at testdata/fixturecheck
// would make the opt-in unfalsifiable, because a walk rooted inside testdata
// never meets a directory named testdata and so cannot skip one.
//
// This tree is deliberately not in multiProviderPackages -- it is where the
// sanctioned constructor lives, so the control is not a violation of anything.
const fixtureCheckScanRoot = "internal/indexfixture"

// A multiProviderViolation is one place a test builds a multi-provider case
// without going through the fixture.
type multiProviderViolation struct {
	Pos  string // file:line
	Rule string // which of the two rules fired
	What string // the detail that identifies the construct
}

func (v multiProviderViolation) String() string {
	return fmt.Sprintf("%s: %s: %s", v.Pos, v.Rule, v.What)
}

// TestMultiProviderCasesUseTheFixture is R17's static check.
//
// It fails on two constructs. The first is a composite literal of two or more
// index.BinaryMatch elements: a hand-written match slice is a multi-provider
// case that no index ever produced. The second is a recipe map with two keys
// yielding the same command, which is a multi-provider case assembled for
// Rebuild.
//
// The rule keys on element count rather than on recipe names, and that is what
// makes it writable at all. R17 says a grep cannot work because several
// registry names are ordinary words; it also cannot work in the other
// direction, because roughly twenty-five single-element literals across these
// packages are legitimate single-provider cases that R18 freezes, and any
// name-keyed rule flags every one of them. The count rule flags none of them.
//
// The per-package file count is asserted separately from the violations,
// because a whole-scan "matched nothing" guard would be satisfied by a scan
// that silently walked one package of four.
func TestMultiProviderCasesUseTheFixture(t *testing.T) {
	for _, pkg := range multiProviderPackages {
		t.Run(pkg, func(t *testing.T) {
			violations, files, err := scanMultiProviderConstruction(pkg)
			if err != nil {
				t.Fatalf("scanning %s: %v", pkg, err)
			}
			if files == 0 {
				t.Fatalf("%s: scanned 0 test files; the check is not looking at this package", pkg)
			}
			for _, v := range violations {
				t.Errorf("%s\n  build this through internal/indexfixture instead: "+
					"a multi-provider case written by hand stops exercising anything the moment "+
					"the names in it stop meaning what they meant", v)
			}
		})
	}
}

// TestMultiProviderCheckFiresOnNonConformingFixture exercises the check
// against a file that breaks both rules, which is what AC48 requires: a check
// that cannot fail is worse than no check.
//
// It carries the assertion that the scanner opts back into testdata/, as its
// own check rather than as a side effect. Every violation the negative control
// produces comes from under testdata/, so a scanner that skipped testdata/
// would report nothing here -- and "the check found nothing wrong" is exactly
// how a check that no longer works reads.
func TestMultiProviderCheckFiresOnNonConformingFixture(t *testing.T) {
	violations, files, err := scanMultiProviderConstruction(fixtureCheckScanRoot)
	if err != nil {
		t.Fatalf("scanning %s: %v", fixtureCheckScanRoot, err)
	}
	if files == 0 {
		t.Fatalf("%s: scanned 0 files", fixtureCheckScanRoot)
	}

	sawTestdata := false
	fired := map[string]bool{}
	for _, v := range violations {
		fired[v.Rule] = true
		if strings.Contains(filepath.ToSlash(v.Pos), "/testdata/") {
			sawTestdata = true
		}
	}
	if !sawTestdata {
		t.Fatalf("no violation was reported from under testdata/ in %s; the scanner is skipping testdata/, "+
			"so the negative control was never read and this check is unexercised. Found: %v",
			fixtureCheckScanRoot, violations)
	}
	for _, rule := range []string{ruleMatchLiteral, ruleRecipeMap} {
		if !fired[rule] {
			t.Errorf("negative control did not trigger %q; found: %v", rule, violations)
		}
	}
}

// TestMultiProviderCheckAllowsSingleProviderCases pins the other half of the
// rule: a one-element literal is a single-provider case, and R18 requires
// those to keep working untouched.
func TestMultiProviderCheckAllowsSingleProviderCases(t *testing.T) {
	src := `package p

import "github.com/tsukumogami/tsuku/internal/index"

func f() []index.BinaryMatch {
	return []index.BinaryMatch{{Recipe: "jq", Command: "jq"}}
}

func g() map[string][]byte {
	return map[string][]byte{"jq": recipeTOML("bin/jq")}
}
`
	violations, err := checkMultiProviderSource("single_test.go", src)
	if err != nil {
		t.Fatalf("parsing source: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("single-provider constructs were flagged: %v", violations)
	}
}

const (
	ruleMatchLiteral = "multi-element BinaryMatch literal"
	ruleRecipeMap    = "recipe map with two keys yielding the same command"
)

// scanMultiProviderConstruction parses every _test.go file under root,
// including files under testdata/ directories, and returns the violations
// found along with the number of files parsed.
//
// Descending into testdata/ is deliberate and is the one place this scanner
// differs from TestNoStdlibLog's walk: the negative control is a file that
// must never compile, so it can only live where the Go tool does not look.
func scanMultiProviderConstruction(root string) ([]multiProviderViolation, int, error) {
	var violations []multiProviderViolation
	files := 0

	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(p) //nolint:gosec // p comes from a walk of the repository
		if readErr != nil {
			return readErr
		}
		files++
		found, parseErr := checkMultiProviderSource(p, string(data))
		if parseErr != nil {
			return parseErr
		}
		violations = append(violations, found...)
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return violations, files, nil
}

// checkMultiProviderSource applies both rules to one file's source.
func checkMultiProviderSource(name, src string) ([]multiProviderViolation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	var violations []multiProviderViolation
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		pos := fset.Position(lit.Pos()).String()

		if isBinaryMatchSlice(lit.Type) && len(lit.Elts) >= 2 {
			violations = append(violations, multiProviderViolation{
				Pos:  pos,
				Rule: ruleMatchLiteral,
				What: fmt.Sprintf("%d elements", len(lit.Elts)),
			})
			return true
		}

		if isRecipeMap(lit.Type) {
			if cmd, n := duplicateCommandInRecipeMap(lit); n >= 2 {
				violations = append(violations, multiProviderViolation{
					Pos:  pos,
					Rule: ruleRecipeMap,
					What: fmt.Sprintf("%d keys yield command %q", n, cmd),
				})
			}
		}
		return true
	})
	return violations, nil
}

// isBinaryMatchSlice reports whether expr is []index.BinaryMatch or, inside
// the index package itself, []BinaryMatch.
func isBinaryMatchSlice(expr ast.Expr) bool {
	arr, ok := expr.(*ast.ArrayType)
	if !ok || arr.Len != nil {
		return false
	}
	switch elt := arr.Elt.(type) {
	case *ast.Ident:
		return elt.Name == "BinaryMatch"
	case *ast.SelectorExpr:
		return elt.Sel.Name == "BinaryMatch"
	}
	return false
}

// isRecipeMap reports whether expr is map[string][]byte, which is the shape
// every recipe map handed to Rebuild has: recipe name to raw TOML.
func isRecipeMap(expr ast.Expr) bool {
	m, ok := expr.(*ast.MapType)
	if !ok {
		return false
	}
	key, ok := m.Key.(*ast.Ident)
	if !ok || key.Name != "string" {
		return false
	}
	val, ok := m.Value.(*ast.ArrayType)
	if !ok || val.Len != nil {
		return false
	}
	elt, ok := val.Elt.(*ast.Ident)
	return ok && elt.Name == "byte"
}

// duplicateCommandInRecipeMap returns the command that two or more entries of
// lit yield, and how many entries yield it.
//
// The command a recipe TOML yields is the base name of its declared binary
// path, which Rebuild derives the same way. Two forms are read: a helper call
// carrying the binary path as its only string argument (minimalRecipeTOML
// ("bin/vi")), and a literal TOML string, from which every bin/<name> is
// taken.
func duplicateCommandInRecipeMap(lit *ast.CompositeLit) (string, int) {
	counts := map[string]int{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		for _, cmd := range commandsYieldedBy(kv.Value) {
			counts[cmd]++
		}
	}
	for cmd, n := range counts {
		if n >= 2 {
			return cmd, n
		}
	}
	return "", 0
}

// commandsYieldedBy extracts the command names a recipe map value declares.
func commandsYieldedBy(expr ast.Expr) []string {
	switch v := expr.(type) {
	case *ast.CallExpr:
		// A helper that turns a binary path into recipe TOML. Only calls with
		// exactly one string-literal argument are read, so an unrelated
		// two-argument helper cannot collide by accident.
		if len(v.Args) != 1 {
			return nil
		}
		s, ok := stringLiteral(v.Args[0])
		if !ok || !strings.Contains(s, "/") {
			return nil
		}
		return []string{commandFromBinaryPath(s)}
	case *ast.BasicLit:
		s, ok := stringLiteral(v)
		if !ok {
			return nil
		}
		return commandsInRecipeTOML(s)
	}
	return nil
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// commandsInRecipeTOML pulls every bin/<name> out of an inline recipe TOML.
func commandsInRecipeTOML(toml string) []string {
	var cmds []string
	rest := toml
	for {
		i := strings.Index(rest, "bin/")
		if i < 0 {
			return cmds
		}
		rest = rest[i+len("bin/"):]
		end := strings.IndexFunc(rest, func(r rune) bool {
			return r == '"' || r == '\'' || r == ' ' || r == '\n' || r == ']' || r == ','
		})
		name := rest
		if end >= 0 {
			name = rest[:end]
		}
		if name != "" {
			cmds = append(cmds, strings.TrimSuffix(name, ".exe"))
		}
	}
}

// commandFromBinaryPath mirrors how Rebuild derives a command name from a
// declared binary path.
func commandFromBinaryPath(binaryPath string) string {
	return strings.TrimSuffix(path.Base(binaryPath), ".exe")
}
