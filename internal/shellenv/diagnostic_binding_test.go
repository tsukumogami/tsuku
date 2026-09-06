package shellenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/project"
)

// captureStderr runs fn with os.Stderr redirected, and returns what was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, rerr := r.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
			}
			if rerr != nil {
				break
			}
		}
		done <- sb.String()
	}()
	fn()
	os.Stderr = orig
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// TestComputeActivation_ReportsRefusalOnStderr binds activation to the
// diagnostic, rather than testing the helper in isolation.
//
// This is the visibility half of the per-entry blast-radius decision: refusing
// one declaration instead of the whole file is only safe because the refusal is
// *seen*. Without a test at this layer, the FprintDiagnostics call here can be
// deleted and the suite stays green -- and a per-entry refusal on this surface
// silently becomes the partial-application the fix exists to prevent.
func TestComputeActivation_ReportsRefusalOnStderr(t *testing.T) {
	home := t.TempDir()
	cfg := &config.Config{HomeDir: home, ToolsDir: filepath.Join(home, "tools")}

	projDir := t.TempDir()
	body := "[tools]\njq = \"1.7.1\"\n\"x$(id)y\" = \"1.0\"\n"
	if err := os.WriteFile(filepath.Join(projDir, project.ConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	stderr := captureStderr(t, func() {
		if _, err := ComputeActivation(projDir, "/usr/bin", "", cfg); err != nil {
			t.Errorf("ComputeActivation: %v", err)
		}
	})

	if !strings.Contains(stderr, "x$(id)y") {
		t.Errorf("activation did not report the refused declaration on stderr.\ngot: %q", stderr)
	}
}

// TestFormatExports_StdoutCarriesNoDiagnostic is the security half.
//
// The diagnostic quotes the offending key, which is attacker-controlled, and
// this command's stdout is what the shell hook evaluates. A diagnostic reaching
// stdout would therefore carry the payload into evaluated text -- the fix's own
// error path becoming a delivery mechanism, firing precisely when the validator
// works.
func TestFormatExports_StdoutCarriesNoDiagnostic(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	home := t.TempDir()
	cfg := &config.Config{HomeDir: home, ToolsDir: filepath.Join(home, "tools")}

	scratch := t.TempDir()
	marker := filepath.Join(scratch, "pwned")

	projDir := t.TempDir()
	// A payload that leaves evidence. "x$(id)y" only differs when evaluated;
	// this one creates a file, which is the difference between "the text does
	// not look dangerous" and "nothing happened".
	body := "[tools]\n\"x$(touch " + marker + ")y\" = \"1.0\"\n"
	if err := os.WriteFile(filepath.Join(projDir, project.ConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var result *ActivationResult
	stderr := captureStderr(t, func() {
		r, err := ComputeActivation(projDir, "/usr/bin", "", cfg)
		if err != nil {
			t.Errorf("ComputeActivation: %v", err)
		}
		result = r
	})

	// Precondition. If the declaration was not refused, this test is asserting
	// nothing about what a refusal puts on stdout.
	if !strings.Contains(stderr, "ignoring") {
		t.Fatalf("the hostile declaration was not refused, so there is no refusal "+
			"to check the stdout of.\nstderr: %s", stderr)
	}

	stdout := FormatExports(result, "bash")

	// The criterion is evaluability, not absence of a substring, and the
	// distinction is the point: a diagnostic reworded to say "skipped" rather
	// than "ignoring" would pass a substring check and still be evaluated by
	// the shell hook, because hook-env's stdout is what the hook evaluates.
	// So evaluate it.
	script := stdout + "\ntrue\n"
	if err := exec.Command(bash, "--norc", "--noprofile", "-c", script).Run(); err != nil {
		t.Fatalf("what hook-env would write to stdout is not valid shell: %v\n%s", err, stdout)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("evaluating hook-env's stdout after a refusal ran the payload.\n\n"+
			"stdout:\n%s", stdout)
	}

	// And the refused key must not be there at all, evaluated or not -- a shell
	// hook is not the only thing that reads this stream.
	if strings.Contains(stdout, "touch "+marker) {
		t.Errorf("the refused key reached the evaluated stream:\n%s", stdout)
	}
}
