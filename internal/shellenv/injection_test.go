package shellenv

// NOTE for whoever rebases #2554 over this: this file is package shellenv and
// calls ComputeActivation, which that change moves to internal/activation. The
// build will break here, loudly, which is the intended behaviour -- it means a
// rebase cannot quietly drop this coverage. Move the file; do not delete it to
// fix the build. It carries the end-to-end reproduction of both traversal
// vectors from #2553.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/project"
)

// TestActivation_TraversalDoesNotReachPATH reproduces the two path escapes from
// the issue against the real ComputeActivation, and asserts the composed
// directory never reaches PATH.
//
// The preconditions are the whole reason this discriminates. Activation stats
// the composed directory before adding it, so without the escaped directory
// actually existing on disk, unfixed code also produces no PATH entry and the
// assertion passes against doing nothing. And the containment assertion is
// anchored at the tools directory rather than at $TSUKU_HOME: with the "jq-"
// prefix consuming a segment, "../.." still lands inside the tools tree, so a
// fixture with too few segments reports a false negative.
func TestActivation_TraversalDoesNotReachPATH(t *testing.T) {
	cases := []struct {
		name    string
		toolKey string
		version string
	}{
		{"traversal in the name", "../../escaped", "1.0.0"},
		// An ordinary name. No name rule reaches this one.
		// Three "..", not two. The "jq-" prefix makes "jq-.." a literal
		// component that the first ".." merely cancels, so two segments land
		// back inside the tools directory. The guard below caught this fixture
		// being wrong on the first attempt, which is the point of having it.
		{"traversal in the version", "jq", "../../../escaped"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			toolsDir := filepath.Join(home, "tools")
			cfg := &config.Config{HomeDir: home, ToolsDir: toolsDir}

			// Precondition: the escape must exist, or the stat gate makes an
			// unfixed binary look fixed.
			escaped := cfg.ToolBinDir(tc.toolKey, tc.version)
			if err := os.MkdirAll(escaped, 0o755); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(filepath.Clean(escaped), filepath.Clean(toolsDir)+string(filepath.Separator)) {
				t.Fatalf("fixture does not actually escape: %q is inside %q. "+
					"Add traversal segments -- the name-version join consumes one.",
					escaped, toolsDir)
			}

			projDir := t.TempDir()
			body := "[tools]\n\"" + tc.toolKey + "\" = \"" + tc.version + "\"\n"
			if err := os.WriteFile(filepath.Join(projDir, project.ConfigFileName), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}

			result, err := ComputeActivation(projDir, "/usr/bin", "", cfg)
			if err != nil {
				t.Fatalf("ComputeActivation: %v", err)
			}
			if result == nil {
				return // nothing activated at all, which is a pass
			}

			for _, entry := range strings.Split(result.PATH, ":") {
				if entry == "" || entry == "/usr/bin" {
					continue
				}
				clean := filepath.Clean(entry)
				if !strings.HasPrefix(clean, filepath.Clean(toolsDir)+string(filepath.Separator)) {
					t.Errorf("PATH entry escaped the tools directory: %q\nfull PATH: %s",
						clean, result.PATH)
				}
			}
		})
	}
}

// TestActivation_InjectionValueNeverReachesPATH covers the command-substitution
// half. The name contains no traversal, so no containment check catches it --
// only the character rule does.
func TestActivation_InjectionValueNeverReachesPATH(t *testing.T) {
	home := t.TempDir()
	toolsDir := filepath.Join(home, "tools")
	cfg := &config.Config{HomeDir: home, ToolsDir: toolsDir}

	const hostile = "x$(touch /tmp/tsuku-pwned-marker)y"
	if err := os.MkdirAll(cfg.ToolBinDir(hostile, "1.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}

	projDir := t.TempDir()
	body := "[tools]\n\"" + hostile + "\" = \"1.0.0\"\n"
	if err := os.WriteFile(filepath.Join(projDir, project.ConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ComputeActivation(projDir, "/usr/bin", "", cfg)
	if err != nil {
		t.Fatalf("ComputeActivation: %v", err)
	}
	if result != nil && strings.Contains(result.PATH, "$(") {
		t.Errorf("a command substitution reached PATH: %s", result.PATH)
	}
}

// TestActivation_ValidToolsStillActivate is the other direction, and the one no
// attack fixture covers: a validator that refuses everything passes every test
// above. It also pins the per-entry blast radius end to end -- one malformed
// declaration must not take its siblings down.
func TestActivation_ValidToolsStillActivate(t *testing.T) {
	home := t.TempDir()
	toolsDir := filepath.Join(home, "tools")
	cfg := &config.Config{HomeDir: home, ToolsDir: toolsDir}

	goodBin := cfg.ToolBinDir("jq", "1.7.1")
	if err := os.MkdirAll(goodBin, 0o755); err != nil {
		t.Fatal(err)
	}

	projDir := t.TempDir()
	body := "[tools]\njq = \"1.7.1\"\n\"x$(id)\" = \"1.0\"\n"
	if err := os.WriteFile(filepath.Join(projDir, project.ConfigFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ComputeActivation(projDir, "/usr/bin", "", cfg)
	if err != nil {
		t.Fatalf("ComputeActivation: %v", err)
	}
	if result == nil {
		t.Fatal("one malformed declaration disabled the whole config")
	}
	if !strings.Contains(result.PATH, goodBin) {
		t.Errorf("the valid tool did not activate: %s", result.PATH)
	}
}
