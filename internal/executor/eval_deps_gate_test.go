package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// installUnderHome leaves a tool behind the way tsuku does: a version
// directory under $TSUKU_HOME/tools and an entry in state.json.
func installUnderHome(t *testing.T, home, name, version string) {
	t.Helper()
	cfg := &config.Config{HomeDir: home, ToolsDir: filepath.Join(home, "tools")}
	if err := os.MkdirAll(cfg.ToolDir(name, version), 0o755); err != nil {
		t.Fatalf("create %s-%s directory: %v", name, version, err)
	}
	if err := install.New(cfg).EnsureDependencyEntry(name, version, "", nil); err != nil {
		t.Fatalf("record %s-%s in state: %v", name, version, err)
	}
}

func goInstallStep() recipe.Step {
	return recipe.Step{
		Action: "go_install",
		Params: map[string]interface{}{
			"module":      "github.com/example/tool",
			"executables": []interface{}{"tool"},
		},
	}
}

// The eval-deps gate is what stands between a user who has not installed go
// and a decomposition that shells out to it. go_install declares go as an
// eval-time dependency, and go-task is a real registry entry whose name starts
// with "go-", so a user who installed go-task and not go used to walk straight
// past this gate. What they got was a decomposition failure; what they should
// get is the offer to install go.
//
// resolveStep is called directly rather than through GeneratePlan because
// GeneratePlan resolves a version over the network first. Nothing on the
// Executor is read before the gate.
func TestResolveStep_PrefixSharingToolDoesNotSatisfyEvalDep(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TSUKU_HOME", home)
	installUnderHome(t, home, "go-task", "3.44.0")

	var offered []string
	autoAccept := false
	cfg := PlanConfig{
		AutoAcceptEvalDeps: true,
		OnEvalDepsNeeded: func(deps []string, auto bool) error {
			offered = append(offered, deps...)
			autoAccept = auto
			// Stand in for the user declining, or the install failing. Either
			// way resolveStep must stop rather than decompose.
			return errors.New("declined")
		},
	}

	e := &Executor{}
	_, err := e.resolveStep(context.Background(), goInstallStep(), nil, nil, cfg, &actions.EvalContext{VersionTag: "v1.0.0"})

	if err == nil {
		t.Fatal("resolveStep decomposed go_install with go missing")
	}
	if !strings.Contains(err.Error(), "eval-time dependencies not satisfied") {
		t.Errorf("resolveStep error = %q, want the eval-deps surface", err)
	}
	if len(offered) != 1 || offered[0] != "go" {
		t.Errorf("offered to install %v, want [go]", offered)
	}
	if !autoAccept {
		t.Error("the install path auto-accepts eval deps; the callback was told otherwise")
	}
}

// The other half: go itself still satisfies the dependency, with the
// prefix-sharing neighbor installed alongside it. Decomposition goes ahead
// and fails on its own terms -- the fixture has no real Go toolchain in it --
// which is fine. What matters is that the gate did not stop it.
func TestResolveStep_InstalledEvalDepPassesTheGate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TSUKU_HOME", home)
	installUnderHome(t, home, "go-task", "3.44.0")
	installUnderHome(t, home, "go", "1.23.4")

	called := false
	cfg := PlanConfig{
		AutoAcceptEvalDeps: true,
		OnEvalDepsNeeded: func(deps []string, auto bool) error {
			called = true
			return nil
		},
	}

	e := &Executor{}
	_, err := e.resolveStep(context.Background(), goInstallStep(), nil, nil, cfg, &actions.EvalContext{VersionTag: "v1.0.0"})

	if called {
		t.Error("resolveStep asked to install go while go was installed")
	}
	if err != nil && strings.Contains(err.Error(), "eval-time dependencies not satisfied") {
		t.Errorf("resolveStep error = %q, want the gate to have passed", err)
	}
}
