package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
)

// Mock implementations for testing

type mockVersionResolver struct {
	version string
	err     error
}

func (m *mockVersionResolver) ResolveVersion(ctx context.Context, constraint string) (string, error) {
	return m.version, m.err
}

type mockPlanGenerator struct {
	plan *executor.InstallationPlan
	err  error
}

func (m *mockPlanGenerator) GeneratePlan(ctx context.Context, cfg executor.PlanConfig) (*executor.InstallationPlan, error) {
	return m.plan, m.err
}

type mockPlanCacheReader struct {
	plan *install.Plan
	err  error
}

func (m *mockPlanCacheReader) GetCachedPlan(tool, version string) (*install.Plan, error) {
	return m.plan, m.err
}

func TestGetOrGeneratePlanWith_CacheHit(t *testing.T) {
	ctx := context.Background()

	// Create a valid cached plan
	cachedPlan := &install.Plan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          "gh",
		Version:       "2.40.0",
		Platform: install.PlanPlatform{
			OS:   "linux",
			Arch: "amd64",
		},
		GeneratedAt:   time.Now(),
		RecipeSource:  "registry",
		Deterministic: true,
		Steps:         []install.PlanStep{},
	}

	resolver := &mockVersionResolver{version: "2.40.0"}
	generator := &mockPlanGenerator{
		plan: &executor.InstallationPlan{}, // Should not be called
	}
	cacheReader := &mockPlanCacheReader{plan: cachedPlan}

	cfg := planRetrievalConfig{
		Tool: "gh",
		OS:   "linux",
		Arch: "amd64",
	}

	result, err := getOrGeneratePlanWith(ctx, resolver, generator, cacheReader, cfg)
	if err != nil {
		t.Fatalf("getOrGeneratePlanWith() error = %v, want nil", err)
	}

	if result == nil {
		t.Fatal("getOrGeneratePlanWith() returned nil, want plan")
	}

	if result.Tool != "gh" {
		t.Errorf("result.Tool = %q, want %q", result.Tool, "gh")
	}
	if result.Version != "2.40.0" {
		t.Errorf("result.Version = %q, want %q", result.Version, "2.40.0")
	}
}

func TestGetOrGeneratePlanWith_CacheMiss(t *testing.T) {
	ctx := context.Background()

	generatedPlan := &executor.InstallationPlan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          "gh",
		Version:       "2.40.0",
		Platform: executor.Platform{
			OS:   "linux",
			Arch: "amd64",
		},
		GeneratedAt:   time.Now(),
		RecipeSource:  "registry",
		Deterministic: true,
		Steps:         []executor.ResolvedStep{},
	}

	resolver := &mockVersionResolver{version: "2.40.0"}
	generator := &mockPlanGenerator{plan: generatedPlan}
	cacheReader := &mockPlanCacheReader{plan: nil} // Cache miss

	cfg := planRetrievalConfig{
		Tool: "gh",
		OS:   "linux",
		Arch: "amd64",
	}

	result, err := getOrGeneratePlanWith(ctx, resolver, generator, cacheReader, cfg)
	if err != nil {
		t.Fatalf("getOrGeneratePlanWith() error = %v, want nil", err)
	}

	if result == nil {
		t.Fatal("getOrGeneratePlanWith() returned nil, want plan")
	}

	if result.Tool != "gh" {
		t.Errorf("result.Tool = %q, want %q", result.Tool, "gh")
	}
}

func TestGetOrGeneratePlanWith_FreshFlag(t *testing.T) {
	ctx := context.Background()

	// Create a valid cached plan (should be ignored due to --fresh)
	cachedPlan := &install.Plan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          "gh",
		Version:       "2.40.0",
		Platform: install.PlanPlatform{
			OS:   "linux",
			Arch: "amd64",
		},
		GeneratedAt:  time.Now(),
		RecipeSource: "registry",
	}

	generatedPlan := &executor.InstallationPlan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          "gh",
		Version:       "2.40.0",
		Platform: executor.Platform{
			OS:   "linux",
			Arch: "amd64",
		},
		GeneratedAt:  time.Now(),
		RecipeSource: "fresh-registry",
	}

	resolver := &mockVersionResolver{version: "2.40.0"}
	generator := &mockPlanGenerator{plan: generatedPlan}
	cacheReader := &mockPlanCacheReader{plan: cachedPlan}

	cfg := planRetrievalConfig{
		Tool:  "gh",
		OS:    "linux",
		Arch:  "amd64",
		Fresh: true, // Fresh flag bypasses cache
	}

	result, err := getOrGeneratePlanWith(ctx, resolver, generator, cacheReader, cfg)
	if err != nil {
		t.Fatalf("getOrGeneratePlanWith() error = %v, want nil", err)
	}

	// Should return the freshly generated plan, not the cached one
	if result.RecipeSource != "fresh-registry" {
		t.Errorf("result.RecipeSource = %q, want %q (fresh plan)", result.RecipeSource, "fresh-registry")
	}
}

func TestGetOrGeneratePlanWith_InvalidCachedPlan_FormatVersion(t *testing.T) {
	ctx := context.Background()

	// Create a cached plan with old format version
	cachedPlan := &install.Plan{
		FormatVersion: 1, // Old version
		Tool:          "gh",
		Version:       "2.40.0",
		Platform: install.PlanPlatform{
			OS:   "linux",
			Arch: "amd64",
		},
	}

	generatedPlan := &executor.InstallationPlan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          "gh",
		Version:       "2.40.0",
		Platform: executor.Platform{
			OS:   "linux",
			Arch: "amd64",
		},
		RecipeSource: "regenerated",
	}

	resolver := &mockVersionResolver{version: "2.40.0"}
	generator := &mockPlanGenerator{plan: generatedPlan}
	cacheReader := &mockPlanCacheReader{plan: cachedPlan}

	cfg := planRetrievalConfig{
		Tool: "gh",
		OS:   "linux",
		Arch: "amd64",
	}

	result, err := getOrGeneratePlanWith(ctx, resolver, generator, cacheReader, cfg)
	if err != nil {
		t.Fatalf("getOrGeneratePlanWith() error = %v, want nil", err)
	}

	// Should regenerate due to format version mismatch
	if result.RecipeSource != "regenerated" {
		t.Errorf("result.RecipeSource = %q, want %q (regenerated plan)", result.RecipeSource, "regenerated")
	}
}

func TestGetOrGeneratePlanWith_VersionResolutionFallback(t *testing.T) {
	ctx := context.Background()

	// When version resolution fails, the function should fall back to "dev" version
	// and continue with plan generation (matching Execute() behavior)
	generatedPlan := &executor.InstallationPlan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          "gh",
		Version:       "dev",
		Platform: executor.Platform{
			OS:   "linux",
			Arch: "amd64",
		},
	}

	resolver := &mockVersionResolver{err: errors.New("network error")}
	generator := &mockPlanGenerator{plan: generatedPlan}
	cacheReader := &mockPlanCacheReader{}

	cfg := planRetrievalConfig{
		Tool: "gh",
		OS:   "linux",
		Arch: "amd64",
	}

	result, err := getOrGeneratePlanWith(ctx, resolver, generator, cacheReader, cfg)
	if err != nil {
		t.Fatalf("getOrGeneratePlanWith() error = %v, want nil (fallback to 'dev')", err)
	}

	if result.Version != "dev" {
		t.Errorf("result.Version = %q, want %q", result.Version, "dev")
	}
}

func TestGetOrGeneratePlanWith_VersionResolutionError_WithConstraint(t *testing.T) {
	ctx := context.Background()

	// When a version constraint is specified and resolution fails,
	// the function should return an error (not fall back to "dev")
	resolver := &mockVersionResolver{err: errors.New("version 99.99.99 not found")}
	generator := &mockPlanGenerator{} // Should not be called
	cacheReader := &mockPlanCacheReader{}

	cfg := planRetrievalConfig{
		Tool:              "go",
		VersionConstraint: "99.99.99", // User requested a specific version
		OS:                "linux",
		Arch:              "amd64",
	}

	result, err := getOrGeneratePlanWith(ctx, resolver, generator, cacheReader, cfg)
	if err == nil {
		t.Fatal("getOrGeneratePlanWith() should return error when version constraint fails")
	}

	if result != nil {
		t.Errorf("result should be nil when error is returned, got %+v", result)
	}

	// Error should mention version resolution
	if !errors.Is(err, resolver.err) && err.Error() == "" {
		t.Errorf("error should wrap the resolution error, got: %v", err)
	}
}

func TestGetOrGeneratePlanWith_DefaultOSArch(t *testing.T) {
	ctx := context.Background()

	generatedPlan := &executor.InstallationPlan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          "gh",
		Version:       "2.40.0",
	}

	resolver := &mockVersionResolver{version: "2.40.0"}
	generator := &mockPlanGenerator{plan: generatedPlan}
	cacheReader := &mockPlanCacheReader{plan: nil}

	cfg := planRetrievalConfig{
		Tool: "gh",
		// OS and Arch not set - should use runtime defaults
	}

	result, err := getOrGeneratePlanWith(ctx, resolver, generator, cacheReader, cfg)
	if err != nil {
		t.Fatalf("getOrGeneratePlanWith() error = %v, want nil", err)
	}

	if result == nil {
		t.Fatal("getOrGeneratePlanWith() returned nil, want plan")
	}
}

func TestPlanRetrievalConfig(t *testing.T) {
	t.Run("config struct fields", func(t *testing.T) {
		cfg := planRetrievalConfig{
			Tool:              "gh",
			VersionConstraint: "2.40.0",
			Fresh:             true,
			OS:                "linux",
			Arch:              "amd64",
		}

		if cfg.Tool != "gh" {
			t.Errorf("Tool = %q, want %q", cfg.Tool, "gh")
		}
		if cfg.VersionConstraint != "2.40.0" {
			t.Errorf("VersionConstraint = %q, want %q", cfg.VersionConstraint, "2.40.0")
		}
		if !cfg.Fresh {
			t.Error("Fresh = false, want true")
		}
		if cfg.OS != "linux" {
			t.Errorf("OS = %q, want %q", cfg.OS, "linux")
		}
		if cfg.Arch != "amd64" {
			t.Errorf("Arch = %q, want %q", cfg.Arch, "amd64")
		}
	})
}
