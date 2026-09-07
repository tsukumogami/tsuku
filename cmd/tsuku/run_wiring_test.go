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

// TestRunWiring_DeclaredRecipeReachesTheInstaller drives what `tsuku run`
// composes -- project config discovered from the working directory, the
// declaration resolver built over it, and the lookup this package supplies --
// against a known index, and checks both things that composition can get
// wrong.
//
// It reconstructs the wiring rather than invoking runCmd because runCmd ends
// in exitWithCode or syscall.Exec, and neither has a seam yet. What makes the
// reconstruction worth running is that both properties below live in the
// wiring and not in either package it joins:
//
//   - the recipe that reaches the installer is the one the file declared,
//     which is only true if the resolver cmd_run.go builds is handed to the
//     runner and the runner narrows on it;
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

	projectCfg, err := project.LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("LoadProjectConfig(%q) error = %v", dir, err)
	}
	if projectCfg == nil {
		t.Fatalf("LoadProjectConfig(%q) found no config", dir)
	}

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("config.DefaultConfig() error = %v", err)
	}
	indexLookup := func(ctx context.Context, command string) ([]index.BinaryMatch, error) {
		return binaryCommandLookup(ctx, cfg, command)
	}

	installer := &runWiringInstaller{}
	stdout := &bytes.Buffer{}
	runner := autoinstall.NewRunner(cfg, stdout, &bytes.Buffer{})
	runner.Lookup = indexLookup
	runner.Installer = installer
	runner.RecipeHasVerification = func(string) bool { return true }
	runner.Exec = func(string, []string, []string) error { return nil }

	err = runner.Run(context.Background(), indexfixture.CommandTwoProviders, nil,
		autoinstall.ModeConfirm, project.NewResolver(projectCfg))
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
