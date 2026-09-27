package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/version"
)

// stubTagProvider answers ResolveVersion from a fixed table and counts calls.
type stubTagProvider struct {
	tags  map[string]*version.VersionInfo
	err   error
	calls int
}

func (s *stubTagProvider) ResolveLatest(context.Context) (*version.VersionInfo, error) {
	return nil, errors.New("not used")
}

func (s *stubTagProvider) ResolveVersion(_ context.Context, v string) (*version.VersionInfo, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if info, ok := s.tags[v]; ok {
		return info, nil
	}
	return nil, errors.New("version " + v + " not found")
}

func (s *stubTagProvider) SourceDescription() string { return "stub" }

func withStubTagProvider(t *testing.T, stub *stubTagProvider) {
	t.Helper()
	orig := pinnedTagProvider
	pinnedTagProvider = func(*version.Resolver, *recipe.Recipe) (version.VersionResolver, error) {
		return stub, nil
	}
	t.Cleanup(func() { pinnedTagProvider = orig })
}

func pinnedRecipe(tagPrefix string) *recipe.Recipe {
	return &recipe.Recipe{
		Metadata: recipe.MetadataSection{Name: "pinned-tool"},
		Version:  recipe.VersionSection{TagPrefix: tagPrefix},
		Steps: []recipe.Step{{
			Action: "download",
			Params: map[string]interface{}{
				"url":  "https://example.invalid/releases/download/{version_tag}/pinned-tool-{version}",
				"dest": "pinned-tool",
			},
		}},
	}
}

// pinnedInfo runs pinnedVersionInfo for a recipe and returns it with any warnings.
func pinnedInfo(t *testing.T, r *recipe.Recipe, pinned string) (*version.VersionInfo, []string) {
	t.Helper()
	exec, err := New(r)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer exec.Cleanup()
	var warnings []string
	info := exec.pinnedVersionInfo(context.Background(), version.New(), PlanConfig{
		PinnedVersion: pinned,
		OnWarning:     func(_, msg string) { warnings = append(warnings, msg) },
	})
	return info, warnings
}

func TestPinnedVersionInfo_UsesProviderTag(t *testing.T) {
	stub := &stubTagProvider{tags: map[string]*version.VersionInfo{
		"2.37.1": {Version: "2.37.1", Tag: "v2.37.1"},
	}}
	withStubTagProvider(t, stub)

	info, warnings := pinnedInfo(t, pinnedRecipe(""), "2.37.1")
	if info.Version != "2.37.1" {
		t.Errorf("Version = %q, want the pinned 2.37.1 unchanged", info.Version)
	}
	if info.Tag != "v2.37.1" {
		t.Errorf("Tag = %q, want v2.37.1 from the provider", info.Tag)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestPinnedVersionInfo_DeclaredPrefixNeedsNoLookup(t *testing.T) {
	stub := &stubTagProvider{err: errors.New("must not be called")}
	withStubTagProvider(t, stub)

	info, warnings := pinnedInfo(t, pinnedRecipe("v"), "1.2.3")
	if info.Tag != "v1.2.3" || info.Version != "1.2.3" {
		t.Errorf("got Version=%q Tag=%q, want 1.2.3 / v1.2.3", info.Version, info.Tag)
	}
	if stub.calls != 0 {
		t.Errorf("provider called %d times; a declared tag_prefix needs no lookup", stub.calls)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestPinnedVersionInfo_LookupFailureWarns(t *testing.T) {
	stub := &stubTagProvider{err: errors.New("rate limited")}
	withStubTagProvider(t, stub)

	info, warnings := pinnedInfo(t, pinnedRecipe(""), "2.37.1")
	if info.Tag != "2.37.1" {
		t.Errorf("Tag = %q, want the pinned version as the fallback tag", info.Tag)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "could not look up the release tag") || !strings.Contains(warnings[0], "rate limited") {
		t.Errorf("warnings = %v, want one naming the failed lookup and its cause", warnings)
	}
}

// A provider's ResolveVersion is fuzzy ("1.29" can resolve to "1.29.3"). The
// pin must not move, so a different version is treated as a failed lookup.
func TestPinnedVersionInfo_FuzzyMatchDoesNotMoveThePin(t *testing.T) {
	stub := &stubTagProvider{tags: map[string]*version.VersionInfo{
		"1.29": {Version: "1.29.3", Tag: "v1.29.3"},
	}}
	withStubTagProvider(t, stub)

	info, warnings := pinnedInfo(t, pinnedRecipe(""), "1.29")
	if info.Version != "1.29" || info.Tag != "1.29" {
		t.Errorf("got Version=%q Tag=%q, want the pin 1.29 kept for both", info.Version, info.Tag)
	}
	if len(warnings) != 1 {
		t.Errorf("warnings = %v, want one", warnings)
	}
}

// End to end through GeneratePlan: the resolved tag reaches the step's URL,
// and the plan still records the bare pinned version.
func TestGeneratePlan_PinnedVersionUsesResolvedTag(t *testing.T) {
	stub := &stubTagProvider{tags: map[string]*version.VersionInfo{
		"2.37.1": {Version: "2.37.1", Tag: "v2.37.1"},
	}}
	withStubTagProvider(t, stub)

	exec, err := New(pinnedRecipe(""))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer exec.Cleanup()
	plan, err := exec.GeneratePlan(context.Background(), PlanConfig{
		OS: "linux", Arch: "amd64", RecipeSource: "test", PinnedVersion: "2.37.1",
	})
	if err != nil {
		t.Fatalf("GeneratePlan() error: %v", err)
	}
	if plan.Version != "2.37.1" {
		t.Errorf("plan.Version = %q, want 2.37.1", plan.Version)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("plan has no steps")
	}
	url, _ := plan.Steps[0].Params["url"].(string)
	if !strings.Contains(url, "/download/v2.37.1/") {
		t.Errorf("step url = %q, want the v2.37.1 tag in it", url)
	}
}
