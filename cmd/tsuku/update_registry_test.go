package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// mockRefreshableProvider implements RecipeProvider and RefreshableProvider for testing.
type mockRefreshableProvider struct {
	source     recipe.RecipeSource
	refreshErr error
	refreshed  bool
}

func (m *mockRefreshableProvider) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("not found")
}

func (m *mockRefreshableProvider) List(_ context.Context) ([]recipe.RecipeInfo, error) {
	return nil, nil
}

func (m *mockRefreshableProvider) Source() recipe.RecipeSource {
	return m.source
}

func (m *mockRefreshableProvider) Refresh(_ context.Context) error {
	m.refreshed = true
	return m.refreshErr
}

// mockPlainProvider implements RecipeProvider but NOT RefreshableProvider.
type mockPlainProvider struct {
	source recipe.RecipeSource
}

func (m *mockPlainProvider) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("not found")
}

func (m *mockPlainProvider) List(_ context.Context) ([]recipe.RecipeInfo, error) {
	return nil, nil
}

func (m *mockPlainProvider) Source() recipe.RecipeSource {
	return m.source
}

func TestRefreshDistributedSources_NoDistributed(t *testing.T) {
	origLoader := loader
	defer func() { loader = origLoader }()

	// Only a central registry provider (non-refreshable mock)
	loader = recipe.NewLoader(&mockPlainProvider{source: recipe.SourceRegistry})

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ctx := context.Background()
	refreshDistributedSources(ctx)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	// No distributed providers, so no output expected
	if buf.Len() != 0 {
		t.Errorf("expected no output when no distributed providers, got %q", buf.String())
	}
}

func TestRefreshDistributedSources_RefreshesDistributed(t *testing.T) {
	origLoader := loader
	origQuiet := quietFlag
	defer func() {
		loader = origLoader
		quietFlag = origQuiet
	}()
	quietFlag = false

	dist1 := &mockRefreshableProvider{source: "myorg/repo1"}
	dist2 := &mockRefreshableProvider{source: "other/repo2"}

	loader = recipe.NewLoader(
		&mockPlainProvider{source: recipe.SourceRegistry},
		dist1,
		dist2,
	)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ctx := context.Background()
	refreshDistributedSources(ctx)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !dist1.refreshed {
		t.Error("expected myorg/repo1 to be refreshed")
	}
	if !dist2.refreshed {
		t.Error("expected other/repo2 to be refreshed")
	}

	if !strings.Contains(output, "myorg/repo1") {
		t.Errorf("expected output to mention myorg/repo1, got %q", output)
	}
	if !strings.Contains(output, "other/repo2") {
		t.Errorf("expected output to mention other/repo2, got %q", output)
	}
	if !strings.Contains(output, "Refreshed 2 distributed source(s)") {
		t.Errorf("expected summary with 2 refreshed, got %q", output)
	}
}

func TestRefreshDistributedSources_SkipsCentralRegistry(t *testing.T) {
	origLoader := loader
	defer func() { loader = origLoader }()

	central := &mockRefreshableProvider{source: recipe.SourceRegistry}
	dist := &mockRefreshableProvider{source: "myorg/repo1"}

	loader = recipe.NewLoader(central, dist)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ctx := context.Background()
	refreshDistributedSources(ctx)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if central.refreshed {
		t.Error("central registry provider should NOT be refreshed by refreshDistributedSources")
	}
	if !dist.refreshed {
		t.Error("distributed provider should be refreshed")
	}
}

func TestRefreshDistributedSources_ErrorDoesNotBlock(t *testing.T) {
	origLoader := loader
	origQuiet := quietFlag
	defer func() {
		loader = origLoader
		quietFlag = origQuiet
	}()
	quietFlag = false

	failing := &mockRefreshableProvider{
		source:     "failing/repo",
		refreshErr: fmt.Errorf("network timeout"),
	}
	succeeding := &mockRefreshableProvider{source: "good/repo"}

	loader = recipe.NewLoader(failing, succeeding)

	// Capture both stdout and stderr
	oldOut := os.Stdout
	oldErr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	ctx := context.Background()
	refreshDistributedSources(ctx)

	wOut.Close()
	wErr.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr

	var bufOut, bufErr bytes.Buffer
	_, _ = bufOut.ReadFrom(rOut)
	_, _ = bufErr.ReadFrom(rErr)

	stdout := bufOut.String()
	stderr := bufErr.String()

	// The failing provider should have been attempted
	if !failing.refreshed {
		t.Error("expected failing provider to have Refresh called")
	}
	// The succeeding provider should still be refreshed despite the earlier failure
	if !succeeding.refreshed {
		t.Error("expected good/repo to be refreshed even after failing/repo error")
	}

	// Error should appear on stderr
	if !strings.Contains(stderr, "network timeout") {
		t.Errorf("expected stderr to contain error message, got %q", stderr)
	}

	// Summary should report both refreshed and errors
	if !strings.Contains(stdout, "1 error") {
		t.Errorf("expected summary to mention 1 error, got %q", stdout)
	}
}

func TestRefreshDistributedSources_SkipsNonRefreshable(t *testing.T) {
	origLoader := loader
	defer func() { loader = origLoader }()

	plain := &mockPlainProvider{source: "some/source"}
	dist := &mockRefreshableProvider{source: "myorg/repo1"}

	loader = recipe.NewLoader(plain, dist)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ctx := context.Background()
	refreshDistributedSources(ctx)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !dist.refreshed {
		t.Error("expected refreshable distributed provider to be refreshed")
	}
	// Summary should show 1 refreshed, not 2
	if !strings.Contains(output, "Refreshed 1 distributed source(s)") {
		t.Errorf("expected summary with 1 refreshed, got %q", output)
	}
}
