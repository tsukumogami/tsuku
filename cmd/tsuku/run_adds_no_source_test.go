package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// runPathEntryPoints are the files that make up `tsuku run` and the two paths
// that share its wiring: the command-not-found hook and the shims both reach
// the same command.
var runPathEntryPoints = []string{
	"cmd_run.go",
}

// TestRunAddsNoSourceToTheRegistries covers R23, and it is the property the
// whole escalation rule rests on.
//
// The rule's sufficient half is membership in the user's configured registries.
// That is only a boundary while a run cannot enlarge it: if `tsuku run` could
// register a source, a project file would be able to put one in and then be
// trusted for naming it, which is the widening the rule exists to prevent.
//
// Asserted structurally rather than by running an install, because the property
// is about what the code can reach rather than about what one run happened to
// do. A run that adds no provider today because its recipe was cached would
// pass a behavioral test and prove nothing.
func TestRunAddsNoSourceToTheRegistries(t *testing.T) {
	forbidden := map[string]string{
		"addDistributedProvider":             "builds a session provider",
		"autoRegisterSource":                 "writes config.toml",
		"writeSourceRegistration":            "writes config.toml",
		"ensureDistributedSource":            "can write config.toml",
		"prepareDistributedSourceForPreview": "builds a session provider",
	}

	fset := token.NewFileSet()
	var scanned int

	for _, name := range runPathEntryPoints {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			// A file that is not there is a real failure: the check would
			// otherwise pass by examining nothing.
			t.Fatalf("parsing %s: %v (if this file was renamed, update runPathEntryPoints)", name, err)
		}
		scanned++

		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if why, bad := forbidden[ident.Name]; bad {
				t.Errorf("%s calls %s at %s, which %s.\n\n"+
					"A tsuku run must leave the configured registries exactly as it "+
					"found them. The escalation rule trusts a source because it is in "+
					"the registries; a run that can put one there would let a project "+
					"file grant itself the trust the rule asks about.",
					name, ident.Name, fset.Position(call.Pos()), why)
			}
			return true
		})
	}

	if scanned != len(runPathEntryPoints) {
		t.Fatalf("scanned %d of %d run-path files", scanned, len(runPathEntryPoints))
	}
}

// TestRunPathCheckCanFail is the canary.
//
// The check above names files and function names as strings, either of which
// can go stale silently. This asserts the files it reads are the ones that
// actually wire the runner, so a rename cannot leave it examining code that no
// longer matters.
func TestRunPathCheckCanFail(t *testing.T) {
	fset := token.NewFileSet()
	var wiresRunner bool

	for _, name := range runPathEntryPoints {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if ok && sel.Sel.Name == "SourceRegistered" {
				wiresRunner = true
			}
			return true
		})
	}

	if !wiresRunner {
		t.Fatal("none of the run-path files wires SourceRegistered, so the check above " +
			"is reading files that no longer make up `tsuku run`. Update runPathEntryPoints.")
	}
}

// TestForbiddenListNamesRealFunctions keeps the forbidden list honest: a
// function that was renamed out of existence would silently stop being checked.
func TestForbiddenListNamesRealFunctions(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "install_distributed.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing install_distributed.go: %v", err)
	}

	declared := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok && fn.Recv == nil {
			declared[fn.Name.Name] = true
		}
		return true
	})

	for _, name := range []string{
		"addDistributedProvider",
		"autoRegisterSource",
		"writeSourceRegistration",
		"ensureDistributedSource",
		"prepareDistributedSourceForPreview",
	} {
		if !declared[name] {
			t.Errorf("%s is no longer declared in install_distributed.go, so the check "+
				"above is looking for a name nothing has. Re-point it at wherever it moved.", name)
		}
	}
}
