package main_test

import (
	"bytes"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGolangCILint(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping golangci-lint")
	}
	rungo(t, "run", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest", "run", "--timeout=5m")
}

func TestGoFmt(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping gofmt")
	}
	cmd := exec.Command("gofmt", "-l", ".")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		t.Fatalf("gofmt failed to run: %v\nOutput:\n%s", err, out.String())
	}
	if out.Len() > 0 {
		t.Errorf("gofmt found unformatted files:\n%s", out.String())
	}
}

func TestGoModTidy(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping go mod tidy")
	}
	rungo(t, "mod", "tidy", "-diff")
}

func TestGoVet(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping go vet")
	}
	rungo(t, "vet", "./...")
}

func TestGovulncheck(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping govulncheck")
	}
	rungo(t, "run", "golang.org/x/vuln/cmd/govulncheck@latest", "./...")
}

func rungo(t *testing.T, args ...string) {
	t.Helper()

	cmd := exec.Command("go", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		if ee := (*exec.ExitError)(nil); errors.As(err, &ee) && len(ee.Stderr) > 0 {
			t.Fatalf("%v: %v\n%s", cmd, err, ee.Stderr)
		}
		t.Fatalf("%v: %v\n%s", cmd, err, output)
	}
}

// TestNoStdlibLog ensures code uses the internal/log package instead of stdlib "log".
// The stdlib log package lacks structured logging support.
// Allowed: "log/slog" (stdlib structured logging), "github.com/tsukumogami/tsuku/internal/log"
func TestNoStdlibLog(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping stdlib log check")
	}

	var violations []string

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip non-Go files, test files, and vendor
		if info.IsDir() {
			if info.Name() == "vendor" || info.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip test files - tests may legitimately use stdlib log
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		node, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}

		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			// Forbid stdlib "log" package (not "log/slog")
			if importPath == "log" {
				violations = append(violations, path+`: imports "log" - use "github.com/tsukumogami/tsuku/internal/log" instead`)
			}
		}

		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk directory: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("found forbidden stdlib log imports:\n  %s", strings.Join(violations, "\n  "))
	}
}
