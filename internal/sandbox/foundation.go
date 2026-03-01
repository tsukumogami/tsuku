package sandbox

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/executor"
)

// FlatDep represents a flattened dependency with its complete plan.
type FlatDep struct {
	Tool    string                     // e.g., "rust"
	Version string                     // e.g., "1.82.0"
	Plan    *executor.InstallationPlan // complete plan (preserves nested deps)
}

// FlattenDependencies extracts and flattens the dependency tree from a plan.
// Returns dependencies in topological order (leaves first, alphabetical
// tiebreaking within siblings). Each plan preserves its dependency subtree
// intact -- deduplication happens at runtime via the executor's skip logic.
// Strips non-deterministic fields (GeneratedAt) from plans.
// Returns an empty (non-nil) slice when the plan has no dependencies.
func FlattenDependencies(plan *executor.InstallationPlan) []FlatDep {
	if len(plan.Dependencies) == 0 {
		return []FlatDep{}
	}

	seen := make(map[string]bool)
	var result []FlatDep

	flattenDFS(plan.Dependencies, seen, &result)

	return result
}

// flattenDFS walks the dependency tree depth-first, emitting leaves before
// parents. Siblings at the same level are sorted alphabetically by tool name.
// Deduplicates by tool+version key; first occurrence wins.
func flattenDFS(deps []executor.DependencyPlan, seen map[string]bool, result *[]FlatDep) {
	// Sort siblings alphabetically by tool name
	sorted := make([]executor.DependencyPlan, len(deps))
	copy(sorted, deps)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Tool < sorted[j].Tool
	})

	for _, dep := range sorted {
		// Recurse into children first (leaves before parents)
		if len(dep.Dependencies) > 0 {
			flattenDFS(dep.Dependencies, seen, result)
		}

		// Deduplicate by tool+version
		key := dep.Tool + "+" + dep.Version
		if seen[key] {
			continue
		}
		seen[key] = true

		// Convert DependencyPlan to standalone InstallationPlan
		plan := dependencyToPlan(&dep)

		*result = append(*result, FlatDep{
			Tool:    dep.Tool,
			Version: dep.Version,
			Plan:    plan,
		})
	}
}

// dependencyToPlan converts a DependencyPlan to a standalone InstallationPlan.
// The nested Dependencies subtree is preserved intact. GeneratedAt is zeroed
// for deterministic output. FormatVersion is set to PlanFormatVersion.
func dependencyToPlan(dep *executor.DependencyPlan) *executor.InstallationPlan {
	// Convert nested DependencyPlan entries (preserve subtree intact)
	nestedDeps := make([]executor.DependencyPlan, len(dep.Dependencies))
	copy(nestedDeps, dep.Dependencies)

	return &executor.InstallationPlan{
		FormatVersion: executor.PlanFormatVersion,
		Tool:          dep.Tool,
		Version:       dep.Version,
		GeneratedAt:   time.Time{}, // Zero value -- stripped for determinism
		Dependencies:  nestedDeps,
		Steps:         dep.Steps,
		Verify:        dep.Verify,
		RecipeType:    dep.RecipeType,
	}
}

// GenerateFoundationDockerfile creates a Dockerfile for pre-installing
// dependencies as Docker layers. Each dependency gets an interleaved
// COPY+RUN pair so Docker's layer cache operates per-dependency.
// Returns a valid Dockerfile even when deps is empty (FROM + setup + cleanup).
func GenerateFoundationDockerfile(packageImage string, deps []FlatDep) string {
	var sb strings.Builder

	sb.WriteString("FROM " + packageImage + "\n")
	sb.WriteString("COPY tsuku /usr/local/bin/tsuku\n")
	sb.WriteString("ENV TSUKU_HOME=/workspace/tsuku\n")
	sb.WriteString("ENV PATH=/workspace/tsuku/bin:$PATH\n")

	for i, dep := range deps {
		filename := fmt.Sprintf("dep-%02d-%s.json", i, dep.Tool)
		sb.WriteString(fmt.Sprintf("COPY plans/%s /tmp/plans/%s\n", filename, filename))
		sb.WriteString(fmt.Sprintf("RUN tsuku install --plan /tmp/plans/%s --force\n", filename))
	}

	sb.WriteString("RUN rm -rf /usr/local/bin/tsuku /tmp/plans\n")

	return sb.String()
}

// FoundationImageName returns the image tag for a foundation image based on
// the Dockerfile content hash. The tag format is:
//
//	tsuku/sandbox-foundation:{family}-{hash16}
//
// where hash16 is the first 16 hex characters of the SHA-256 hash of the
// Dockerfile content. Deterministic: same inputs always produce the same tag.
func FoundationImageName(family string, dockerfile string) string {
	h := sha256.Sum256([]byte(dockerfile))
	hash16 := fmt.Sprintf("%x", h[:8]) // 8 bytes = 16 hex chars
	return fmt.Sprintf("tsuku/sandbox-foundation:%s-%s", family, hash16)
}
