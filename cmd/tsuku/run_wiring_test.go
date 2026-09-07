package main

import (
	"bytes"
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/tsukumogami/tsuku/internal/autoinstall"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
	"github.com/tsukumogami/tsuku/internal/indexfixture"
	"github.com/tsukumogami/tsuku/internal/project"
)

// runWiringInstaller records what the run path asked to install.
type runWiringInstaller struct {
	recipe  string
	version string
}

func (i *runWiringInstaller) Install(_ context.Context, recipe, version string) error {
	i.recipe = recipe
	i.version = version
	return nil
}

// TestRunWiring_DeclaredRecipeReachesTheInstaller drives newRunWiring -- the
// function `tsuku run` builds its resolver and lookup with -- against a known
// index, and checks the two things that composition can get wrong.
//
// It calls newRunWiring rather than runCmd because runCmd ends in exitWithCode
// or syscall.Exec, neither of which has a seam yet. It calls newRunWiring
// rather than reconstructing those four lines because a reconstruction passes
// while the production wiring is wrong: rewire the run path to
// project.NewResolver(nil) -- exactly the defect this work repairs -- and a
// test that built its own resolver would not notice.
//
// The two properties, both of which live in the joining and in neither package
// it joins:
//
//   - the recipe that reaches the installer is the one the file declared,
//     which holds only if the resolver built from the discovered config is the
//     one the runner is handed;
//   - the binary index is opened once per run. It used to be opened twice
//     whenever a .tsuku.toml was found, because the resolver looked the
//     command up again for itself; taking the matches instead is what removed
//     that, and nothing inside either package can observe it.
func TestRunWiring_DeclaredRecipeReachesTheInstaller(t *testing.T) {
	fx := indexfixture.New(t)

	var lookups atomic.Int32
	original := binaryCommandLookup
	binaryCommandLookup = func(ctx context.Context, _ *config.Config, command string) ([]index.BinaryMatch, error) {
		lookups.Add(1)
		return fx.Lookup(ctx, command)
	}
	t.Cleanup(func() { binaryCommandLookup = original })

	// A project declaring the provider the index ranks *second*, so the
	// declared recipe and the index's own preference disagree.
	dir := t.TempDir()
	t.Setenv(project.EnvCeilingPaths, filepath.Dir(dir))
	fx.WriteProjectConfig(t, dir, map[string]string{
		indexfixture.DeclaredRecipe: indexfixture.SharedVersion,
	})

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("config.DefaultConfig() error = %v", err)
	}

	wiring := newRunWiring(cfg, dir)
	if wiring.projectCfg == nil {
		t.Fatalf("newRunWiring found no config below %q; one was written there", dir)
	}

	installer := &runWiringInstaller{}
	stdout := &bytes.Buffer{}
	runner := autoinstall.NewRunner(cfg, stdout, &bytes.Buffer{})
	runner.Lookup = wiring.lookup
	runner.Installer = installer
	runner.RecipeHasVerification = func(string) bool { return true }
	runner.Exec = func(string, []string, []string) error { return nil }

	// ModeConfirm with no ConsentReader: reaching a prompt would fail rather
	// than hang, so a run that completes is one the declaration consented to.
	err = runner.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
		autoinstall.ModeConfirm, wiring.resolver)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if installer.recipe != indexfixture.DeclaredRecipe {
		t.Errorf("installed %q, want the declared recipe %q (the index ranks %q first)",
			installer.recipe, indexfixture.DeclaredRecipe, indexfixture.RecipeDupFirst)
	}
	if installer.version != indexfixture.SharedVersion {
		t.Errorf("installed version %q, want %q", installer.version, indexfixture.SharedVersion)
	}
	if got := lookups.Load(); got != 1 {
		t.Errorf("the binary index was opened %d times, want 1", got)
	}
}
