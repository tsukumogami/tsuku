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

// TestMultiProviderCheckReadsInlineRecipeTOML covers the second of the two
// shapes a recipe map value can take. Nothing in the repository writes a
// recipe map this way today, which is exactly why it has a test: without one
// the TOML scanner is a branch nobody ever runs, and a branch nobody runs is
// indistinguishable from a branch that does not work.
func TestMultiProviderCheckReadsInlineRecipeTOML(t *testing.T) {
	const toml = "[metadata]\\nname = \\\"x\\\"\\nbinaries = [\\\"bin/vi\\\"]\\n"
	src := `package p

func g() map[string][]byte {
	return map[string][]byte{
		"neovim": []byte("` + toml + `"),
		"vim":    []byte("` + toml + `"),
	}
}

func h() map[string][]byte {
	return map[string][]byte{
		"jq":   []byte("[metadata]\nbinaries = [\"bin/jq\"]\n"),
		"ripgrep": []byte("[metadata]\nbinaries = [\"bin/rg\"]\n"),
	}
}
`
	violations, err := checkMultiProviderSource("inline_test.go", src)
	if err != nil {
		t.Fatalf("parsing source: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1 (the pair yielding \"vi\"): %v", len(violations), violations)
	}
	if violations[0].Rule != ruleRecipeMap {
		t.Errorf("rule = %q, want %q", violations[0].Rule, ruleRecipeMap)
	}
	if !strings.Contains(violations[0].What, `"vi"`) {
		t.Errorf("violation does not name the duplicated command: %s", violations[0].What)
	}
}

// TestMultiProviderCheckReadsHelperCallArgument is the other half of the pair
// above, and exists to make the []byte-conversion branch falsifiable in both
// directions. Reading a helper's argument as TOML rather than as a path finds
// nothing here, because "libexec/vi" contains no "bin/" for the TOML scanner
// to key on -- so a checker that took the wrong branch would report zero and
// this test would fail.
func TestMultiProviderCheckReadsHelperCallArgument(t *testing.T) {
	src := `package p

func g() map[string][]byte {
	return map[string][]byte{
		"neovim": recipeTOML("libexec/vi"),
		"vim":    recipeTOML("libexec/vi"),
	}
}
`
	violations, err := checkMultiProviderSource("helper_test.go", src)
	if err != nil {
		t.Fatalf("parsing source: %v", err)
	}
	if len(violations) != 1 || violations[0].Rule != ruleRecipeMap {
		t.Fatalf("got %v, want one %s violation", violations, ruleRecipeMap)
	}
}

// TestMultiProviderCheckCatchesElidedLiterals covers the shape a command-keyed
// LookupFunc stub takes. The inner literals carry no type of their own, so the
// type-keyed rule cannot see them and they have to be reached through their
// container.
func TestMultiProviderCheckCatchesElidedLiterals(t *testing.T) {
	src := `package p

import "github.com/tsukumogami/tsuku/internal/index"

var byCommand = map[string][]index.BinaryMatch{
	"vi": {
		{Recipe: "neovim", Command: "vi"},
		{Recipe: "vim", Command: "vi"},
	},
	"jq": {
		{Recipe: "jq", Command: "jq"},
	},
}

var nested = [][]index.BinaryMatch{
	{
		{Recipe: "neovim", Command: "vi"},
		{Recipe: "vim", Command: "vi"},
	},
}

var fixed = [2]index.BinaryMatch{
	{Recipe: "neovim", Command: "vi"},
	{Recipe: "vim", Command: "vi"},
}
`
	violations, err := checkMultiProviderSource("elided_test.go", src)
	if err != nil {
		t.Fatalf("parsing source: %v", err)
	}
	if len(violations) != 3 {
		t.Fatalf("got %d violations, want 3 (map value, nested slice, fixed array): %v",
			len(violations), violations)
	}
	for _, v := range violations {
		if v.Rule != ruleMatchLiteral {
			t.Errorf("rule = %q, want %q", v.Rule, ruleMatchLiteral)
		}
	}
}

// TestMultiProviderCheckCatchesSharedRecipeVariable covers the recipe-map form
// whose values are identifiers. What the variable holds is unknowable here,
// but two keys bound to the same one hold the same recipe, so the pair is a
// multi-provider case whatever that recipe turns out to declare.
func TestMultiProviderCheckCatchesSharedRecipeVariable(t *testing.T) {
	src := `package p

func g(toml, other []byte) map[string][]byte {
	return map[string][]byte{"neovim": toml, "vim": toml}
}

func h(toml, other []byte) map[string][]byte {
	return map[string][]byte{"neovim": toml, "vim": other}
}

func i(toml []byte) map[string][]byte {
	return map[string][]byte{"multi": toml}
}
`
	violations, err := checkMultiProviderSource("sharedvar_test.go", src)
	if err != nil {
		t.Fatalf("parsing source: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1 (only the pair sharing one variable): %v",
			len(violations), violations)
	}
	if !strings.Contains(violations[0].What, "toml") {
		t.Errorf("violation does not name the shared variable: %s", violations[0].What)
	}
	if strings.Contains(violations[0].What, identPrefix) {
		t.Errorf("internal marker leaked into the message: %s", violations[0].What)
	}
}

const (
	ruleMatchLiteral = "multi-element BinaryMatch literal"
	ruleRecipeMap    = "recipe map with two keys yielding the same command"

	// identPrefix marks a command name that stands for "whatever this variable
	// holds" rather than for a real command, so the two cannot collide.
	identPrefix = "ident:"
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

		// A literal whose elements are themselves []BinaryMatch elides the
		// inner types: in
		//
		//	map[string][]index.BinaryMatch{"vi": {{...}, {...}}}
		//
		// the inner literal has a nil Type, so the check above never sees it.
		// That is the natural shape for a command-keyed LookupFunc stub, which
		// is what the units consuming this fixture reach for, so it is caught
		// here rather than left as a documented gap.
		if elt, ok := elementType(lit.Type); ok && isBinaryMatchSlice(elt) {
			for _, e := range lit.Elts {
				inner := e
				if kv, ok := e.(*ast.KeyValueExpr); ok {
					inner = kv.Value
				}
				il, ok := inner.(*ast.CompositeLit)
				if ok && il.Type == nil && len(il.Elts) >= 2 {
					violations = append(violations, multiProviderViolation{
						Pos:  fset.Position(il.Pos()).String(),
						Rule: ruleMatchLiteral,
						What: fmt.Sprintf("%d elements, in an elided literal", len(il.Elts)),
					})
				}
			}
		}

		if isRecipeMap(lit.Type) {
			if cmd, n := duplicateCommandInRecipeMap(lit); n >= 2 {
				what := fmt.Sprintf("%d keys yield command %q", n, cmd)
				if name, ok := strings.CutPrefix(cmd, identPrefix); ok {
					what = fmt.Sprintf("%d keys are bound to %s, so they hold the same recipe", n, name)
				}
				violations = append(violations, multiProviderViolation{
					Pos:  pos,
					Rule: ruleRecipeMap,
					What: what,
				})
			}
		}
		return true
	})
	return violations, nil
}

// isBinaryMatchSlice reports whether expr is []index.BinaryMatch or, inside
// the index package itself, []BinaryMatch. Fixed-size arrays count: an
// [2]index.BinaryMatch literal is the same construct with a length on it.
func isBinaryMatchSlice(expr ast.Expr) bool {
	arr, ok := expr.(*ast.ArrayType)
	if !ok {
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

// elementType returns the type a composite literal of type expr gives its
// elements, which is the type the elements may elide.
func elementType(expr ast.Expr) (ast.Expr, bool) {
	switch t := expr.(type) {
	case *ast.ArrayType:
		return t.Elt, true
	case *ast.MapType:
		return t.Value, true
	}
	return nil, false
}

// isRecipeMap reports whether expr is map[string][]byte, which is the shape
// every recipe map handed to Rebuild has: recipe name to raw TOML.
//
// The type is generic, so this rule is scoped to multiProviderPackages rather
// than run repository-wide: internal/recipe/loader_test.go alone has fifteen
// map[string][]byte literals that have nothing to do with the index. Within
// the four scanned packages every such map is a Rebuild input, but that is a
// fact about those packages, not about the type.
//
// If this ever produces a false positive -- a map[string][]byte in one of the
// four that is not a recipe map, whose values happen to yield one name twice
// -- the fix is to narrow duplicateCommandInRecipeMap, not to add a suppression
// comment. There is deliberately no exemption mechanism: an escape hatch on
// this rule is an escape hatch on R17.
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
//
// A map[string][]byte entry cannot be a bare string literal -- an untyped
// string constant is not assignable to []byte -- so there is no literal form
// to read. Three forms are:
//
//   - a []byte conversion wrapping inline TOML, []byte("[metadata]\n..."),
//     read by scanning the TOML for the binaries it declares;
//   - a helper carrying the binary path as its only string argument,
//     minimalRecipeTOML("bin/vi"), read as that path;
//   - an identifier, map[string][]byte{"neovim": toml, "vim": toml}. Its
//     contents are not known here, but two keys bound to the *same* variable
//     hold the same TOML and so yield the same command, whatever it is. The
//     identifier's own name stands in for that command.
//
// Only single-argument calls are read, so an unrelated two-argument helper
// cannot collide by accident. Two keys bound to two *different* variables that
// happen to declare the same command are not caught; see the package comment
// on internal/indexfixture for the full list of what this rule misses.
func commandsYieldedBy(expr ast.Expr) []string {
	switch v := expr.(type) {
	case *ast.Ident:
		// Prefixed so an identifier named "vi" cannot collide with a binary
		// path that yields the command "vi".
		return []string{identPrefix + v.Name}
	case *ast.CallExpr:
		if len(v.Args) != 1 {
			return nil
		}
		s, ok := stringLiteral(v.Args[0])
		if !ok {
			return nil
		}
		if isByteSliceConversion(v.Fun) {
			return commandsInRecipeTOML(s)
		}
		if !strings.Contains(s, "/") {
			return nil
		}
		return []string{commandFromBinaryPath(s)}
	}
	return nil
}

// isByteSliceConversion reports whether fun is the []byte in []byte("...").
func isByteSliceConversion(fun ast.Expr) bool {
	arr, ok := fun.(*ast.ArrayType)
	if !ok || arr.Len != nil {
		return false
	}
	elt, ok := arr.Elt.(*ast.Ident)
	return ok && elt.Name == "byte"
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
