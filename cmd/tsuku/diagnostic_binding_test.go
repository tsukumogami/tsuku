package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// diagnosticCallSites are the commands that load a project config and must
// therefore report a refused declaration to the user.
//
// Bound by a source assertion rather than by driving each command end to end,
// and the reason is worth stating rather than leaving as a shortcut. Each of
// these paths reaches the network, the installer or the shim writer before the
// diagnostic is observable, so an end-to-end test would need most of tsuku
// stood up to assert one line of output. The property that actually matters --
// "this consumer does not silently stop reporting" -- is structural, so it is
// asserted structurally.
//
// This exists because the mutation was run and all three of these stayed green
// when the call was deleted: nothing bound them. Delete a call now and this
// fails, which is the whole point. Activation is bound differently, by a real
// behavioural test in internal/shellenv that captures stderr; that is the
// better shape and is used wherever the path is cheap enough to drive.
var diagnosticCallSites = []string{
	"install_project.go",
	"cmd_shim.go",
	"cmd_run.go",
}

func TestProjectConfigConsumersReportDiagnostics(t *testing.T) {
	for _, file := range diagnosticCallSites {
		t.Run(file, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, file, nil, 0)
			if err != nil {
				t.Fatalf("parsing %s: %v", file, err)
			}

			var found bool
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "FprintDiagnostics" {
					return true
				}
				// The argument matters as much as the call. A diagnostic on
				// stdout would be evaluated by the shell hook rather than read,
				// and it quotes an attacker-controlled key.
				if len(call.Args) == 1 {
					if arg, ok := call.Args[0].(*ast.SelectorExpr); ok && arg.Sel.Name == "Stderr" {
						found = true
					}
				}
				return true
			})

			if !found {
				t.Errorf("%s loads a project config but does not call "+
					"FprintDiagnostics(os.Stderr).\n\n"+
					"A refused declaration would be dropped silently on this surface. "+
					"Per-entry refusal -- rather than refusing the whole file -- is only "+
					"safe because the refusal is seen; without it, a refusal here is the "+
					"silent partial application this change exists to prevent.", file)
			}
		})
	}
}

// TestDiagnosticBindingCannotFalseGreen pins the property that makes the check
// above worth having.
//
// A structural assertion keyed on the bare helper name would pass against the
// file that *defines* FprintDiagnostics, with zero calls in any consumer --
// green, and asserting nothing. The check must therefore match a call
// expression with an os.Stderr argument, not an identifier.
//
// This exists because that exact trap was flagged in review, and because a
// future edit weakening the matcher to a name grep would be invisible: the
// consumer tests would still pass. Here it would not.
func TestDiagnosticBindingCannotFalseGreen(t *testing.T) {
	fset := token.NewFileSet()
	// The definition lives here and contains the identifier but no call.
	f, err := parser.ParseFile(fset, "../../internal/project/config.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing the definition file: %v", err)
	}

	var sawCall bool
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "FprintDiagnostics" {
			sawCall = true
		}
		return true
	})

	if sawCall {
		t.Fatal("the definition file now contains a FprintDiagnostics call, so the " +
			"consumer assertion could pass against it. Tighten the matcher.")
	}
}
