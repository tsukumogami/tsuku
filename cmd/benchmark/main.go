// Package main implements a benchmark harness for measuring LLM recipe generation success rate.
//
// Usage:
//
//	go run ./cmd/benchmark --corpus testdata/benchmark-repos.txt
//	go run ./cmd/benchmark --corpus testdata/benchmark-repos.txt --limit 5
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/builders"
	"github.com/tsukumogami/tsuku/internal/validate"
)

// BenchmarkResult holds the result of benchmarking a single repository.
type BenchmarkResult struct {
	Repo    string
	Passed  bool
	Error   string
	Repairs int
	Elapsed time.Duration
}

func main() {
	var (
		corpusPath string
		limit      int
	)

	flag.StringVar(&corpusPath, "corpus", "", "Path to corpus file (required)")
	flag.IntVar(&limit, "limit", 0, "Maximum repos to test (0 = all)")
	flag.Parse()

	if corpusPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --corpus is required")
		flag.Usage()
		os.Exit(1)
	}

	repos, err := loadCorpus(corpusPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading corpus: %v\n", err)
		os.Exit(1)
	}

	if len(repos) == 0 {
		fmt.Fprintln(os.Stderr, "Error: corpus is empty")
		os.Exit(1)
	}

	// Apply limit
	if limit > 0 && limit < len(repos) {
		repos = repos[:limit]
	}

	fmt.Printf("Running benchmark with %d repositories...\n\n", len(repos))

	results := runBenchmark(repos)
	printResults(results)
}

// loadCorpus reads repositories from a corpus file.
// Skips empty lines and lines starting with #.
func loadCorpus(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var repos []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		repos = append(repos, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return repos, nil
}

// runBenchmark runs the benchmark against all repositories.
func runBenchmark(repos []string) []BenchmarkResult {
	ctx := context.Background()

	// Initialize validation executor
	detector := validate.NewRuntimeDetector()
	predownloader := validate.NewPreDownloader()
	executor := validate.NewExecutor(detector, predownloader)

	// Create builder with validation
	builder, err := builders.NewGitHubReleaseBuilder(ctx, builders.WithExecutor(executor))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating builder: %v\n", err)
		os.Exit(1)
	}

	results := make([]BenchmarkResult, 0, len(repos))

	for i, repo := range repos {
		fmt.Printf("[%d/%d] Testing %s...\n", i+1, len(repos), repo)

		start := time.Now()
		result := benchmarkRepo(ctx, builder, repo)
		result.Elapsed = time.Since(start)

		if result.Passed {
			fmt.Printf("  PASS (repairs: %d, time: %s)\n", result.Repairs, result.Elapsed.Round(time.Second))
		} else {
			fmt.Printf("  FAIL: %s\n", result.Error)
		}

		results = append(results, result)
	}

	return results
}

// benchmarkRepo tests a single repository.
func benchmarkRepo(ctx context.Context, builder *builders.GitHubReleaseBuilder, repo string) BenchmarkResult {
	// Extract tool name from repo (e.g., "cli/cli" -> "cli")
	parts := strings.Split(repo, "/")
	toolName := parts[len(parts)-1]

	result, err := builder.Build(ctx, builders.BuildRequest{
		Package:   toolName,
		SourceArg: repo,
	})

	if err != nil {
		return BenchmarkResult{
			Repo:   repo,
			Passed: false,
			Error:  err.Error(),
		}
	}

	return BenchmarkResult{
		Repo:    repo,
		Passed:  true,
		Repairs: result.RepairAttempts,
	}
}

// printResults prints the benchmark summary.
func printResults(results []BenchmarkResult) {
	var passed, failed int
	var failedRepos []BenchmarkResult

	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
			failedRepos = append(failedRepos, r)
		}
	}

	total := len(results)
	rate := float64(passed) / float64(total) * 100

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("Benchmark Results:")
	fmt.Printf("  Total: %d\n", total)
	fmt.Printf("  Passed: %d\n", passed)
	fmt.Printf("  Failed: %d\n", failed)
	fmt.Printf("  Success Rate: %.0f%%\n", rate)

	if len(failedRepos) > 0 {
		fmt.Println()
		fmt.Println("Failed repositories:")
		for _, r := range failedRepos {
			// Truncate long errors
			errorMsg := r.Error
			if len(errorMsg) > 100 {
				errorMsg = errorMsg[:97] + "..."
			}
			fmt.Printf("  - %s: %s\n", r.Repo, errorMsg)
		}
	}

	fmt.Println("========================================")

	// Exit with error if below threshold
	if rate < 80 {
		fmt.Printf("\nWARNING: Success rate %.0f%% is below 80%% threshold\n", rate)
		os.Exit(1)
	}
}
