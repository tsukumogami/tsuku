package main_test

import (
	"bytes"
	"errors"
	"os/exec"
	"testing"
)

func TestGolangCILint(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping golangci-lint")
	}
	rungo(t, "run", "github.com/golangci/golangci-lint/cmd/golangci-lint@latest", "run", "--timeout=5m")
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
