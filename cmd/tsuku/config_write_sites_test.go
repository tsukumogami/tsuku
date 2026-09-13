package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// permittedConfigWriters are the functions allowed to save the user config.
//
// Only the first is reachable from an install path, and that is the whole
// point: the deferred write gates one door, and gating one door is sufficient
// rather than merely necessary only while there is one. The other three are
// terminal command handlers -- a user typing `tsuku registry add`,
// `tsuku registry remove` or `tsuku config set` -- which are the user's own
// action rather than something a project file caused.
var permittedConfigWriters = map[string]string{
	"autoRegisterSource": "the one install-path write, gated by the consent primitives",
	"runRegistryAdd":     "tsuku registry add, the user's own action",
	"runRegistryRemove":  "tsuku registry remove, the user's own action",
	"runConfigSet":       "tsuku config set, the user's own action",
}

// TestSaveIsTheOnlyConfigWriter pins the claim the consent work rests on.
//
// #2552's two validation scripts check that nothing writes config.toml when the
// user was not asked. Everything this change does to make that true gates one
// call -- and nothing asserted that it is the only one. A fifth writer added on
// an install path would satisfy every other test in this package and quietly
// reopen the defect.
//
// It is deliberately a check on the writers, not on the callers of the gate: a
// new write is the thing that breaks the property, whoever calls it.
func TestSaveIsTheOnlyConfigWriter(t *testing.T) {
	fset := token.NewFileSet()
	var scanned int

	for _, dir := range []string{".", "../../internal"} {
		walkGoFiles(t, dir, func(path string) {
			scanned++
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}

			var enclosing string
			ast.Inspect(f, func(n ast.Node) bool {
				if fn, ok := n.(*ast.FuncDecl); ok {
					enclosing = fn.Name.Name
					return true
				}
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Save" || len(call.Args) != 0 {
					return true
				}
				if !looksLikeUserConfig(sel.X) {
					return true
				}
				if _, ok := permittedConfigWriters[enclosing]; ok {
					return true
				}
				t.Errorf("%s saves the user config from %s, at %s.\n\n"+
					"Only these may: %s.\n\n"+
					"Gating the install path's one write is what makes a dry run, a "+
					"missing terminal and a declined prompt leave config.toml alone. "+
					"A second writer on an install path makes that a claim about one "+
					"door in a room with two. If this write is genuinely the user's "+
					"own action, add it to permittedConfigWriters and say why.",
					path, enclosing, fset.Position(call.Pos()), permittedList())
				return true
			})
		})
	}

	if scanned < 20 {
		t.Fatalf("the walk found %d non-test .go files; the skip predicate is wrong "+
			"and this check is passing over an almost empty set", scanned)
	}
}

// TestConfigWriterCheckCanFail is the canary.
//
// The check above reads the tree at runtime, so a wrong skip predicate would
// examine nothing and pass. This asserts at least one permitted writer is still
// found where it is expected, so the exclusion list hides something real rather
// than describing functions that no longer exist.
func TestConfigWriterCheckCanFail(t *testing.T) {
	src, err := os.ReadFile("install_distributed.go")
	if err != nil {
		t.Fatalf("reading the install-path writer: %v", err)
	}
	if !strings.Contains(string(src), "func autoRegisterSource(") {
		t.Fatal("autoRegisterSource is no longer in install_distributed.go, so the " +
			"exclusion above hides nothing. Re-point the list at wherever the " +
			"install path's write moved.")
	}
	if !strings.Contains(string(src), ".Save()") {
		t.Fatal("the install path's writer no longer calls Save(), so the check " +
			"above is looking for the wrong call and would pass over a new one.")
	}
}

// looksLikeUserConfig reports whether the receiver of a Save() call is a user
// config rather than some other type with a Save method.
//
// It matches on the identifier's spelling, which is what an AST check without
// type information can do. A false negative would be a config saved through a
// differently named variable; the canary above does not catch that, and the
// naming convention in this tree is consistent enough that widening the match
// would cost more in false positives than it buys.
func looksLikeUserConfig(x ast.Expr) bool {
	ident, ok := x.(*ast.Ident)
	if !ok {
		return false
	}
	name := strings.ToLower(ident.Name)
	return strings.Contains(name, "cfg") || strings.Contains(name, "config")
}

func permittedList() string {
	var out []string
	for name, why := range permittedConfigWriters {
		out = append(out, name+" ("+why+")")
	}
	return strings.Join(out, "; ")
}

func walkGoFiles(t *testing.T, root string, fn func(path string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		fn(path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}
