package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tsukumogami/tsuku/internal/builders"
	"github.com/tsukumogami/tsuku/internal/discover"
)

func main() {
	seedsDir := flag.String("seeds-dir", "data/discovery-seeds", "directory containing seed list JSON files")
	queueFile := flag.String("queue", "", "path to priority-queue.json (optional)")
	outputDir := flag.String("output", "recipes/discovery", "output directory for per-tool discovery files")
	recipesDir := flag.String("recipes-dir", "", "path to recipes directory for graduation cross-referencing")
	validateOnly := flag.String("validate-only", "", "validate an existing discovery directory instead of generating")
	verbose := flag.Bool("verbose", false, "print progress information")
	flag.Parse()

	if *validateOnly != "" {
		runValidateOnly(*validateOnly, *verbose)
		return
	}

	validators := buildValidators()
	probers := buildProbers()

	cfg := discover.GenerateConfig{
		SeedsDir:   *seedsDir,
		QueueFile:  *queueFile,
		RecipesDir: *recipesDir,
		OutputDir:  *outputDir,
		Validators: validators,
		Probers:    probers,
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

	fmt.Fprintf(os.Stderr, "Processed %d entries: %d graduated, %d valid, %d failed, %d probed, %d rejected\n",
		result.Total, result.Graduated, result.Valid, len(result.Failures), result.Probed, result.Rejected)

	if *verbose && len(result.Backfills) > 0 {
		fmt.Fprintln(os.Stderr, "\nMetadata backfills:")
		for _, bf := range result.Backfills {
			var fields []string
			if bf.Description {
				fields = append(fields, "description")
			}
			if bf.Homepage {
				fields = append(fields, "homepage")
			}
			fmt.Fprintf(os.Stderr, "  %s: added %s\n", bf.Name, strings.Join(fields, ", "))
		}
	}

	if *verbose && len(result.Rejections) > 0 {
		fmt.Fprintln(os.Stderr, "\nQuality rejections:")
		for _, r := range result.Rejections {
			fmt.Fprintf(os.Stderr, "  %s (%s/%s): %s\n", r.Entry.Name, r.Entry.Builder, r.Entry.Source, r.Reason)
		}
	}

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

func buildProbers() map[string]builders.EcosystemProber {
	hb := builders.NewHomebrewBuilder()
	return map[string]builders.EcosystemProber{
		hb.Name(): hb,
	}
}
