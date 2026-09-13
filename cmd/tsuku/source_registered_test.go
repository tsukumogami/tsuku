package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// TestWiredSourceRegisteredIsExact tests the implementation, not the contract.
//
// There is a sibling test in internal/autoinstall asserting that the Runner
// honors whatever predicate it is given, including a case-sensitive one. That
// one passes against a case-folding implementation, because it injects its own
// predicate and never touches the real one — so the property it appears to
// protect is not protected at all. This test calls the function the command
// actually wires.
//
// The distinction matters beyond this case: a criterion pinned only at the
// contract is a criterion that cannot fail, and a test that cannot fail reads
// as coverage while measuring nothing.
func TestWiredSourceRegisteredIsExact(t *testing.T) {
	cfg := &userconfig.Config{
		Registries: map[string]userconfig.RegistryEntry{
			"Owner/Repo": {URL: "https://github.com/Owner/Repo"},
		},
	}

	registered := sourceRegisteredIn(cfg)

	if !registered("Owner/Repo") {
		t.Error("the exact spelling in the configuration reads as unregistered")
	}
	if registered("owner/repo") {
		t.Fatal("a case-folded match reported a source as registered that the " +
			"user's configuration does not contain. GitHub treats the two as one " +
			"repository; the user's config file does not, and waiving the install " +
			"prompt on a key they never wrote is the wrong direction for a consent " +
			"decision.")
	}
	if registered("someone/else") {
		t.Error("an absent source reads as registered")
	}
}

// TestSourceRegisteredIsWhatTheRunnerGets closes the gap between the function
// tested above and the decision it feeds.
//
// Testing the function proves it is exact; it does not prove the command wires
// *it* rather than an inline closure that drifted. Asserted structurally
// because the alternative — reaching the elevation from here — would mean
// exporting an internal predicate for a test, which widens the package's API to
// check something a parser can see.
func TestSourceRegisteredIsWhatTheRunnerGets(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "cmd_run.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing cmd_run.go: %v", err)
	}

	var wired bool
	ast.Inspect(f, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		sel, ok := assign.Lhs[0].(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "SourceRegistered" {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			t.Errorf("SourceRegistered is wired from something other than a call at %s; "+
				"an inline closure here is not covered by the exactness test above",
				fset.Position(assign.Pos()))
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "sourceRegisteredIn" {
			t.Errorf("SourceRegistered is wired from something other than sourceRegisteredIn at %s",
				fset.Position(assign.Pos()))
			return true
		}
		wired = true
		return true
	})

	if !wired {
		t.Fatal("cmd_run.go does not assign runner.SourceRegistered from sourceRegisteredIn, " +
			"so the exactness test above is testing a function nothing uses")
	}
}
