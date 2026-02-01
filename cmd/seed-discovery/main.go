package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/discover"
)

func main() {
	seedsDir := flag.String("seeds-dir", "data/discovery-seeds", "directory containing seed list JSON files")
	queueFile := flag.String("queue", "", "path to priority-queue.json (optional)")
	outputDir := flag.String("output", "recipes/discovery", "output directory for per-tool discovery files")
	recipesDir := flag.String("recipes-dir", "", "path to recipes directory for cross-referencing (stub)")
	validateOnly := flag.String("validate-only", "", "validate an existing discovery directory instead of generating")
	verbose := flag.Bool("verbose", false, "print progress information")
	flag.Parse()

	_ = recipesDir // stub for future cross-referencing

	if *validateOnly != "" {
		runValidateOnly(*validateOnly, *verbose)
		return
	}

	validators := buildValidators()

	cfg := discover.GenerateConfig{
		SeedsDir:   *seedsDir,
		QueueFile:  *queueFile,
		OutputDir:  *outputDir,
		Validators: validators,
		Verbose:    *verbose,
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Seeds dir: %s\n", *seedsDir)
		if *queueFile != "" {
			fmt.Fprintf(os.Stderr, "Queue file: %s\n", *queueFile)
		}
		fmt.Fprintf(os.Stderr, "Output dir: %s\n", *outputDir)
	}

	result, err := discover.Generate(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Processed %d entries: %d valid, %d failed\n",
		result.Total, result.Valid, len(result.Failures))

	if *verbose && len(result.Failures) > 0 {
		fmt.Fprintln(os.Stderr, "\nFailures:")
		for _, f := range result.Failures {
			fmt.Fprintf(os.Stderr, "  %s (%s): %v\n", f.Entry.Name, f.Entry.Source, f.Err)
		}
	}

	if len(result.Failures) > 0 {
		os.Exit(2)
	}
}

func runValidateOnly(dir string, verbose bool) {
	validators := buildValidators()
	result, err := discover.ValidateExisting(dir, validators)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Validated %d entries: %d valid, %d failed\n",
		result.Total, result.Valid, len(result.Failures))

	if verbose && len(result.Failures) > 0 {
		fmt.Fprintln(os.Stderr, "\nFailures:")
		for _, f := range result.Failures {
			fmt.Fprintf(os.Stderr, "  %s (%s): %v\n", f.Entry.Name, f.Entry.Source, f.Err)
		}
	}

	if len(result.Failures) > 0 {
		os.Exit(2)
	}
}

func buildValidators() map[string]discover.Validator {
	return map[string]discover.Validator{
		"github":   discover.NewGitHubValidator(nil),
		"homebrew": discover.NewHomebrewValidator(nil),
	}
}
