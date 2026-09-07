package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/autoinstall"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/indexfixture"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// The routes a consent mode arrives by, run against a project that declares
// the tool. What the runner does with an origin is pinned in
// internal/autoinstall; the half that exists only here is that the flag, the
// environment variable and the configuration key produce the origins they are
// supposed to -- a route wired to the wrong one satisfies every criterion
// stated in terms of origins and still ships the defect.
//
// These cases assemble what runCmd assembles -- resolveMode over the real
// config file and environment, newRunWiring over the real project discovery --
// rather than driving runCmd itself, and the reason is specific: runCmd hands
// off with syscall.Exec, and a wrong implementation here is exactly one that
// installs and hands off. That replaces the test binary mid-run, and `go test`
// reads the tool's own exit status as a passing package. A test that cannot
// report the failure it exists to catch is worse than no test, so the exec is
// a recorder here and the run's own error is what is asserted.

// consentRoute sets one of the three ways a mode can be supplied.
type consentRoute func(t *testing.T, cfg *config.Config)

// byFlag is --mode. The flag is a package variable, put back after the test,
// because these cases do not go through cobra's parsing.
func byFlag(mode string) consentRoute {
	return func(t *testing.T, _ *config.Config) {
		orig := runModeFlag
		t.Cleanup(func() { runModeFlag = orig })
		runModeFlag = mode
	}
}

// byEnvironment is TSUKU_AUTO_INSTALL_MODE.
func byEnvironment(mode string) consentRoute {
	return func(t *testing.T, _ *config.Config) {
		t.Setenv("TSUKU_AUTO_INSTALL_MODE", mode)
	}
}

// byConfig is auto_install_mode in $TSUKU_HOME/config.toml, written at the
// permissions the configuration-permission gate accepts.
func byConfig(mode string) consentRoute {
	return func(t *testing.T, cfg *config.Config) {
		body := "auto_install_mode = \"" + mode + "\"\n"
		if err := os.WriteFile(filepath.Join(cfg.HomeDir, "config.toml"), []byte(body), 0o600); err != nil {
			t.Fatalf("writing config.toml: %v", err)
		}
	}
}

// declaredRun is what a `tsuku run` of the declared command produced.
type declaredRun struct {
	err       error
	stdout    string
	stderr    string
	installer *runWiringInstaller
	execed    bool
}

// runDeclaredCommand runs the fixture's two-provider command in a project that
// declares one of its providers, with the consent mode supplied by route.
//
// Everything between the mode's source and the runner is the production path:
// resolveMode reads the flag, the environment and the config file, and
// newRunWiring discovers the .tsuku.toml by walking up from the working
// directory. Only the install and the process handoff are recorded instead of
// performed.
func runDeclaredCommand(t *testing.T, route consentRoute, terminal bool) declaredRun {
	t.Helper()

	_, dir := twoProviderProject(t, map[string]string{
		indexfixture.DeclaredRecipe: indexfixture.SharedVersion,
	})

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("config.DefaultConfig() error = %v", err)
	}
	route(t, cfg)

	userCfg, err := userconfig.Load()
	if err != nil {
		t.Fatalf("userconfig.Load() error = %v", err)
	}
	mode, origin, err := resolveMode(runModeFlag, userCfg)
	if err != nil {
		t.Fatalf("resolveMode() error = %v", err)
	}

	got := declaredRun{installer: &runWiringInstaller{}}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	wiring := newRunWiring(cfg, dir)

	runner := autoinstall.NewRunner(cfg, stdout, stderr)
	runner.Lookup = wiring.lookup
	runner.Installer = got.installer
	runner.RecipeHasVerification = func(string) bool { return true }
	runner.IsTerminal = func() bool { return terminal }
	runner.Exec = func(string, []string, []string) error {
		got.execed = true
		return nil
	}
	// No ConsentReader: a run that reached the prompt would read end-of-file
	// and decline, which is distinguishable from every outcome asserted below.

	got.err = runner.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
		mode, origin, wiring.resolver)
	got.stdout, got.stderr = stdout.String(), stderr.String()
	return got
}

// AC33, AC37 and D1-3. With suggest set by the flag, by the environment
// variable and by the configuration key in turn, a project-declared tool
// prints an install instruction and installs nothing.
//
// This is the floor the decision rests on. `suggest` is never a default, so a
// user who set it has said "never install unattended", and the elevation is
// bounded precisely so that a file shipped by a cloned repository cannot
// overrule that. All three routes ended in a silent install before this
// change, which is the defect the work answers.
func TestRunCmd_AC33_AnExplicitSuggestIsHonoredThroughEveryRoute(t *testing.T) {
	routes := []struct {
		name  string
		route consentRoute
	}{
		{"flag", byFlag("suggest")},
		{"environment variable", byEnvironment("suggest")},
		{"config key", byConfig("suggest")},
	}

	for _, tt := range routes {
		t.Run(tt.name, func(t *testing.T) {
			got := runDeclaredCommand(t, tt.route, true)

			if !errors.Is(got.err, autoinstall.ErrSuggestOnly) {
				t.Fatalf("Run() error = %v, want ErrSuggestOnly: suggest set by the %s was not honored\nstdout:\n%s\nstderr:\n%s",
					got.err, tt.name, got.stdout, got.stderr)
			}
			want := "tsuku install " + indexfixture.DeclaredRecipe + "@" + indexfixture.SharedVersion
			if !strings.Contains(got.stdout, want) {
				t.Errorf("stdout names no instruction for the declared recipe, want %q:\n%s", want, got.stdout)
			}
			if got.installer.recipe != "" {
				t.Errorf("installed %q under suggest", got.installer.recipe)
			}
			if got.execed {
				t.Error("handed off to a binary under suggest")
			}
		})
	}
}

// D1-5, both halves together, and the criterion that distinguishes this
// decision from the one it was nearly confused with.
//
// TSUKU_AUTO_INSTALL_MODE=auto with no corroborating config is the state the
// escalation restriction exists for -- the shape a hostile .envrc takes -- and
// the mode is lowered to confirm. A declaration must not put it back: the
// declared command prompts, and with no terminal to prompt on the run stops at
// the terminal check rather than installing unattended.
//
// Both outcomes are asserted from one run because they are one claim. That the
// mode is confirm is what ErrNotInteractive reports, and it reports it only
// from a state where a prompt was needed.
func TestRunCmd_ADeclarationDoesNotRestoreAnUncorroboratedEnvironmentAuto(t *testing.T) {
	got := runDeclaredCommand(t, byEnvironment("auto"), false)

	if !errors.Is(got.err, autoinstall.ErrNotInteractive) {
		t.Fatalf("Run() error = %v, want ErrNotInteractive: the declaration re-raised the mode the escalation restriction lowered\nstdout:\n%s\nstderr:\n%s",
			got.err, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stderr, "requires a terminal") {
		t.Errorf("stderr does not say why the run stopped:\n%s", got.stderr)
	}
	if got.installer.recipe != "" {
		t.Errorf("installed %q unattended", got.installer.recipe)
	}
	if got.execed {
		t.Error("handed off to a binary without consent")
	}
}

// The same environment variable with the config corroborating it: the mode is
// auto, and it is auto because the config said so rather than because anything
// was declared.
//
// Without this row the criterion above is met by an implementation that
// ignores TSUKU_AUTO_INSTALL_MODE entirely, and the escalation restriction it
// is about would be indistinguishable from having no environment route at all.
func TestRunCmd_ACorroboratedEnvironmentAutoStillReachesAuto(t *testing.T) {
	got := runDeclaredCommand(t, func(t *testing.T, cfg *config.Config) {
		byEnvironment("auto")(t, cfg)
		byConfig("auto")(t, cfg)
	}, false)

	if got.err != nil {
		t.Fatalf("Run() error = %v, want nil\nstdout:\n%s\nstderr:\n%s", got.err, got.stdout, got.stderr)
	}
	if got.installer.recipe != indexfixture.DeclaredRecipe {
		t.Errorf("installed %q, want the declared recipe %q", got.installer.recipe, indexfixture.DeclaredRecipe)
	}
	if strings.Contains(got.stdout, "[y/N]") {
		t.Errorf("a prompt appeared under a corroborated auto: %q", got.stdout)
	}
}
