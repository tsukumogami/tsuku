package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
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
		RecipeHash:    "abc123",
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
		Tool:       "gh",
		RecipeHash: "abc123",
		OS:         "linux",
		Arch:       "amd64",
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
		RecipeHash:    "abc123",
		RecipeSource:  "registry",
		Deterministic: true,
		Steps:         []executor.ResolvedStep{},
	}

	resolver := &mockVersionResolver{version: "2.40.0"}
	generator := &mockPlanGenerator{plan: generatedPlan}
	cacheReader := &mockPlanCacheReader{plan: nil} // Cache miss

	cfg := planRetrievalConfig{
		Tool:       "gh",
		RecipeHash: "abc123",
		OS:         "linux",
		Arch:       "amd64",
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
		RecipeHash:   "abc123",
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
		RecipeHash:   "abc123",
		RecipeSource: "fresh-registry",
	}

	resolver := &mockVersionResolver{version: "2.40.0"}
	generator := &mockPlanGenerator{plan: generatedPlan}
	cacheReader := &mockPlanCacheReader{plan: cachedPlan}

	cfg := planRetrievalConfig{
		Tool:       "gh",
		RecipeHash: "abc123",
		OS:         "linux",
		Arch:       "amd64",
		Fresh:      true, // Fresh flag bypasses cache
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
		RecipeHash: "abc123",
	}

	generatedPlan := &executor.InstallationPlan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          "gh",
		Version:       "2.40.0",
		Platform: executor.Platform{
			OS:   "linux",
			Arch: "amd64",
		},
		RecipeHash:   "abc123",
		RecipeSource: "regenerated",
	}

	resolver := &mockVersionResolver{version: "2.40.0"}
	generator := &mockPlanGenerator{plan: generatedPlan}
	cacheReader := &mockPlanCacheReader{plan: cachedPlan}

	cfg := planRetrievalConfig{
		Tool:       "gh",
		RecipeHash: "abc123",
		OS:         "linux",
		Arch:       "amd64",
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

func TestGetOrGeneratePlanWith_InvalidCachedPlan_RecipeHashChanged(t *testing.T) {
	ctx := context.Background()

	// Create a cached plan with different recipe hash
	cachedPlan := &install.Plan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          "gh",
		Version:       "2.40.0",
		Platform: install.PlanPlatform{
			OS:   "linux",
			Arch: "amd64",
		},
		RecipeHash: "old_hash", // Different from config
	}

	generatedPlan := &executor.InstallationPlan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          "gh",
		Version:       "2.40.0",
		Platform: executor.Platform{
			OS:   "linux",
			Arch: "amd64",
		},
		RecipeHash:   "new_hash",
		RecipeSource: "regenerated",
	}

	resolver := &mockVersionResolver{version: "2.40.0"}
	generator := &mockPlanGenerator{plan: generatedPlan}
	cacheReader := &mockPlanCacheReader{plan: cachedPlan}

	cfg := planRetrievalConfig{
		Tool:       "gh",
		RecipeHash: "new_hash", // Different from cached plan
		OS:         "linux",
		Arch:       "amd64",
	}

	result, err := getOrGeneratePlanWith(ctx, resolver, generator, cacheReader, cfg)
	if err != nil {
		t.Fatalf("getOrGeneratePlanWith() error = %v, want nil", err)
	}

	// Should regenerate due to recipe hash mismatch
	if result.RecipeSource != "regenerated" {
		t.Errorf("result.RecipeSource = %q, want %q (regenerated plan)", result.RecipeSource, "regenerated")
	}
}

func TestGetOrGeneratePlanWith_VersionResolutionError(t *testing.T) {
	ctx := context.Background()

	resolver := &mockVersionResolver{err: errors.New("network error")}
	generator := &mockPlanGenerator{}
	cacheReader := &mockPlanCacheReader{}

	cfg := planRetrievalConfig{
		Tool: "gh",
	}

	_, err := getOrGeneratePlanWith(ctx, resolver, generator, cacheReader, cfg)
	if err == nil {
		t.Fatal("getOrGeneratePlanWith() error = nil, want error")
	}

	if !errors.Is(err, errors.New("network error")) {
		// Check that the error message contains the expected content
		if err.Error() != "version resolution failed: network error" {
			t.Errorf("error = %q, want %q", err.Error(), "version resolution failed: network error")
		}
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
		Tool:       "gh",
		RecipeHash: "abc123",
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

func TestComputeRecipeHashForPlan(t *testing.T) {
	t.Run("computes hash for valid recipe", func(t *testing.T) {
		r := &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name:        "test-tool",
				Description: "A test tool",
			},
		}

		hash, err := computeRecipeHashForPlan(r)
		if err != nil {
			t.Fatalf("computeRecipeHashForPlan() error = %v", err)
		}

		if hash == "" {
			t.Error("computeRecipeHashForPlan() returned empty hash")
		}

		// Hash should be hex-encoded SHA256 (64 chars)
		if len(hash) != 64 {
			t.Errorf("hash length = %d, want 64", len(hash))
		}
	})

	t.Run("same recipe produces same hash", func(t *testing.T) {
		r1 := &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name:        "test-tool",
				Description: "A test tool",
			},
		}
		r2 := &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name:        "test-tool",
				Description: "A test tool",
			},
		}

		hash1, err := computeRecipeHashForPlan(r1)
		if err != nil {
			t.Fatalf("computeRecipeHashForPlan(r1) error = %v", err)
		}

		hash2, err := computeRecipeHashForPlan(r2)
		if err != nil {
			t.Fatalf("computeRecipeHashForPlan(r2) error = %v", err)
		}

		if hash1 != hash2 {
			t.Errorf("hashes differ: %q vs %q", hash1, hash2)
		}
	})

	t.Run("different recipes produce different hashes", func(t *testing.T) {
		r1 := &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name: "tool-a",
			},
		}
		r2 := &recipe.Recipe{
			Metadata: recipe.MetadataSection{
				Name: "tool-b",
			},
		}

		hash1, err := computeRecipeHashForPlan(r1)
		if err != nil {
			t.Fatalf("computeRecipeHashForPlan(r1) error = %v", err)
		}

		hash2, err := computeRecipeHashForPlan(r2)
		if err != nil {
			t.Fatalf("computeRecipeHashForPlan(r2) error = %v", err)
		}

		if hash1 == hash2 {
			t.Error("different recipes should produce different hashes")
		}
	})
}

func TestPlanRetrievalConfig(t *testing.T) {
	t.Run("config struct fields", func(t *testing.T) {
		cfg := planRetrievalConfig{
			Tool:              "gh",
			VersionConstraint: "2.40.0",
			Fresh:             true,
			OS:                "linux",
			Arch:              "amd64",
			RecipeHash:        "abc123",
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
		if cfg.RecipeHash != "abc123" {
			t.Errorf("RecipeHash = %q, want %q", cfg.RecipeHash, "abc123")
		}
	})
}
