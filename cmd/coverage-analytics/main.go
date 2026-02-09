package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// CoverageData is the output JSON structure for the coverage website.
type CoverageData struct {
	GeneratedAt  string           `json:"generated_at"`
	TotalRecipes int              `json:"total_recipes"`
	Summary      CoverageSummary  `json:"summary"`
	Recipes      []RecipeCoverage `json:"recipes"`
	Exclusions   []ExclusionInfo  `json:"exclusions"`
}

// CoverageSummary provides aggregate statistics.
type CoverageSummary struct {
	ByPlatform map[string]PlatformStats `json:"by_platform"`
	ByCategory map[string]CategoryStats `json:"by_category"`
}

// PlatformStats shows how many recipes support a platform.
type PlatformStats struct {
	Supported int     `json:"supported"`
	Total     int     `json:"total"`
	Percent   float64 `json:"pct"`
}

// CategoryStats shows coverage for a recipe category.
type CategoryStats struct {
	Total        int `json:"total"`
	MuslSupport  int `json:"musl_support"`
	GlibcSupport int `json:"glibc_support"`
}

// convertCategoryStats converts pointer map to value map for JSON.
func convertCategoryStats(m map[string]*CategoryStats) map[string]CategoryStats {
	result := make(map[string]CategoryStats)
	for k, v := range m {
		result[k] = *v
	}
	return result
}

// RecipeCoverage describes platform coverage for one recipe.
type RecipeCoverage struct {
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Platforms map[string]bool `json:"platforms"`
	Gaps      []string        `json:"gaps"`
	Errors    []string        `json:"errors"`
	Warnings  []string        `json:"warnings"`
}

// ExclusionInfo describes a recipe excluded from testing.
type ExclusionInfo struct {
	Recipe string `json:"recipe"`
	Issue  string `json:"issue"`
	Reason string `json:"reason"`
}

// ExecutionExclusions matches the schema of testdata/golden/execution-exclusions.json
type ExecutionExclusions struct {
	Exclusions []struct {
		Recipe string `json:"recipe"`
		Issue  string `json:"issue"`
		Reason string `json:"reason"`
	} `json:"exclusions"`
}

func main() {
	recipesDir := flag.String("recipes", "recipes/", "directory containing recipe files")
	exclusionsFile := flag.String("exclusions", "testdata/golden/execution-exclusions.json", "execution exclusions file")
	output := flag.String("output", "website/coverage/coverage.json", "output file path")
	flag.Parse()

	// Load exclusions for context
	exclusions, err := loadExclusions(*exclusionsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load exclusions: %v\n", err)
		exclusions = []ExclusionInfo{}
	}

	// Load all recipes
	recipes, err := loadAllRecipes(*recipesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading recipes: %v\n", err)
		os.Exit(1)
	}

	// Analyze coverage for each recipe
	var coverageReports []RecipeCoverage
	platformCounts := map[string]int{"glibc": 0, "musl": 0, "darwin": 0}
	categoryCounts := map[string]*CategoryStats{
		"library": {Total: 0, MuslSupport: 0, GlibcSupport: 0},
		"tool":    {Total: 0, MuslSupport: 0, GlibcSupport: 0},
	}

	for _, r := range recipes {
		report := recipe.AnalyzeRecipeCoverage(r)

		// Determine recipe type
		recipeType := "tool"
		if r.IsLibrary() {
			recipeType = "library"
		}

		// Build coverage data
		platforms := map[string]bool{
			"glibc":  report.HasGlibc,
			"musl":   report.HasMusl,
			"darwin": report.HasDarwin,
		}

		var gaps []string
		for platform, supported := range platforms {
			if !supported {
				gaps = append(gaps, platform)
			} else {
				platformCounts[platform]++
			}
		}
		sort.Strings(gaps)

		coverageReports = append(coverageReports, RecipeCoverage{
			Name:      r.Metadata.Name,
			Type:      recipeType,
			Platforms: platforms,
			Gaps:      gaps,
			Errors:    report.Errors,
			Warnings:  report.Warnings,
		})

		// Update category stats
		stats := categoryCounts[recipeType]
		stats.Total++
		if report.HasMusl {
			stats.MuslSupport++
		}
		if report.HasGlibc {
			stats.GlibcSupport++
		}
	}

	// Sort recipes by name
	sort.Slice(coverageReports, func(i, j int) bool {
		return coverageReports[i].Name < coverageReports[j].Name
	})

	// Build summary
	totalRecipes := len(recipes)
	summary := CoverageSummary{
		ByPlatform: map[string]PlatformStats{
			"glibc": {
				Supported: platformCounts["glibc"],
				Total:     totalRecipes,
				Percent:   float64(platformCounts["glibc"]) / float64(totalRecipes) * 100,
			},
			"musl": {
				Supported: platformCounts["musl"],
				Total:     totalRecipes,
				Percent:   float64(platformCounts["musl"]) / float64(totalRecipes) * 100,
			},
			"darwin": {
				Supported: platformCounts["darwin"],
				Total:     totalRecipes,
				Percent:   float64(platformCounts["darwin"]) / float64(totalRecipes) * 100,
			},
		},
		ByCategory: convertCategoryStats(categoryCounts),
	}

	// Build final output
	data := CoverageData{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		TotalRecipes: totalRecipes,
		Summary:      summary,
		Recipes:      coverageReports,
		Exclusions:   exclusions,
	}

	// Write output JSON
	if err := writeJSON(*output, data); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Coverage report generated: %s\n", *output)
	fmt.Fprintf(os.Stderr, "Total recipes: %d\n", totalRecipes)
	fmt.Fprintf(os.Stderr, "Platform coverage: glibc=%d (%.1f%%), musl=%d (%.1f%%), darwin=%d (%.1f%%)\n",
		platformCounts["glibc"], summary.ByPlatform["glibc"].Percent,
		platformCounts["musl"], summary.ByPlatform["musl"].Percent,
		platformCounts["darwin"], summary.ByPlatform["darwin"].Percent)
}

func loadAllRecipes(dir string) ([]*recipe.Recipe, error) {
	var recipes []*recipe.Recipe

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !strings.HasSuffix(path, ".toml") {
			return nil
		}

		r, err := recipe.ParseFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", path, err)
			return nil
		}

		recipes = append(recipes, r)
		return nil
	})

	return recipes, err
}

func loadExclusions(path string) ([]ExclusionInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var exclusions ExecutionExclusions
	if err := json.Unmarshal(data, &exclusions); err != nil {
		return nil, err
	}

	result := make([]ExclusionInfo, len(exclusions.Exclusions))
	for i, e := range exclusions.Exclusions {
		result[i] = ExclusionInfo{
			Recipe: e.Recipe,
			Issue:  e.Issue,
			Reason: e.Reason,
		}
	}

	return result, nil
}

func writeJSON(path string, data interface{}) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Marshal with indentation
	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	return os.WriteFile(path, append(output, '\n'), 0644)
}
