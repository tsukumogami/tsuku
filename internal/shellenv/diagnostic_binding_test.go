package shellenv

import (
	"os"
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
	home := t.TempDir()
	cfg := &config.Config{HomeDir: home, ToolsDir: filepath.Join(home, "tools")}

	projDir := t.TempDir()
	body := "[tools]\n\"x$(id)y\" = \"1.0\"\n"
	if err := os.WriteFile(filepath.Join(projDir, project.ConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var result *ActivationResult
	_ = captureStderr(t, func() {
		r, err := ComputeActivation(projDir, "/usr/bin", "", cfg)
		if err != nil {
			t.Errorf("ComputeActivation: %v", err)
		}
		result = r
	})

	stdout := FormatExports(result, "bash")
	if strings.Contains(stdout, "x$(id)y") {
		t.Errorf("the refused key reached the evaluated stream:\n%s", stdout)
	}
	if strings.Contains(stdout, "ignoring") {
		t.Errorf("a diagnostic reached the evaluated stream:\n%s", stdout)
	}
}
