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

// TestLoadProjectConfigReportingIsTheOnlyCaller keeps the reporting helper from
// being routed around.
//
// Per-declaration refusal -- refusing one malformed entry rather than the whole
// file -- is only safe while the refusal is seen. A command that calls
// project.LoadProjectConfig directly gets the validated config and drops the
// diagnostics on the floor, which is the silent partial application the config
// boundary exists to prevent.
//
// This replaces a per-file assertion that named three consumers explicitly. That
// shape had the defect this one does not: a fourth command added later was not in
// the list, so the case most likely to actually happen -- a new consumer -- was
// the one case it could not catch.
func TestLoadProjectConfigReportingIsTheOnlyCaller(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading package dir: %v", err)
	}

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// The helper is the one place the direct call is correct.
		if name == "project_config.go" {
			continue
		}

		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "LoadProjectConfig" {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "project" {
				return true
			}
			t.Errorf("%s calls project.LoadProjectConfig directly at %s.\n\n"+
				"Use loadProjectConfigReporting instead. Loading without reporting drops "+
				"the refused-declaration diagnostics, and a refusal nobody sees is the "+
				"silent partial application the config boundary exists to prevent.",
				name, fset.Position(call.Pos()))
			return true
		})
	}
}

// TestOnlyCallerCheckCanFail pins that the check above can actually fail.
//
// It reads the package directory at runtime, so a bad skip predicate -- a wrong
// extension test, an over-broad filename exclusion -- would silently examine
// nothing and pass. This asserts the walk reaches real files and that the helper
// itself carries the call the check is looking for, which is the one occurrence
// deliberately excluded.
func TestOnlyCallerCheckCanFail(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading package dir: %v", err)
	}
	var scanned int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++
	}
	if scanned < 2 {
		t.Fatalf("the walk found %d non-test .go files in this package; the skip "+
			"predicate is wrong and the check above is passing over an empty set", scanned)
	}

	src, err := os.ReadFile("project_config.go")
	if err != nil {
		t.Fatalf("reading the helper: %v", err)
	}
	if !strings.Contains(string(src), "project.LoadProjectConfig(") {
		t.Fatal("project_config.go no longer calls project.LoadProjectConfig, so the " +
			"exclusion in the check above now hides nothing and the check proves less " +
			"than it claims. Re-point it at wherever the call moved.")
	}
}
