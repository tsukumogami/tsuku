package main_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// selfexecPackageDir is the one place allowed to resolve this process's own
// executable. Everything else asks it.
const selfexecPackageDir = "internal/selfexec"

// TestSelfExecIsTheOnlyBinaryResolver keeps the guard from being bypassed by
// the next site rather than only fixing the sites that exist today.
//
// Calling os.Executable() and then running, mounting or overwriting the result
// is the shape behind tsuku#2580: under `go test` that path is the package test
// binary, which discards its -test.* flags when handed a positional subcommand
// and runs its whole suite. It is also the shape behind the quieter failure in
// a sibling command such as cmd/seed-queue, where the path is a real binary
// that does not understand `tsuku install`.
//
// internal/selfexec answers both questions in one place. This test fails if any
// other non-test file resolves its own executable, which is how a new site
// would otherwise arrive unguarded -- nobody re-reads the incident that
// produced the guard before adding a call.
func TestSelfExecIsTheOnlyBinaryResolver(t *testing.T) {
	var violations []string

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "vendor", "testdata", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.HasPrefix(filepath.ToSlash(path), selfexecPackageDir+"/") {
			return nil
		}

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Executable" {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "os" {
				return true
			}
			pos := fset.Position(call.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d", path, pos.Line))
			return true
		})

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if len(violations) > 0 {
		t.Fatalf("os.Executable() called outside %s:\n  %s\n\n"+
			"Resolving this process's own binary and then running, mounting or\n"+
			"overwriting it is only safe when the binary is the tsuku CLI and not\n"+
			"a Go test binary. Call selfexec.Binary() instead: it answers both\n"+
			"questions, and returns false when the answer is no. If this site\n"+
			"genuinely needs the raw path and never execs it, say so here.",
			selfexecPackageDir, strings.Join(violations, "\n  "))
	}
}
