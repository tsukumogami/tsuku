package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/batch"
	"github.com/tsukumogami/tsuku/internal/seed"
)

func main() {
	ecosystem := flag.String("ecosystem", "", "ecosystem to process (homebrew, cargo, npm, pypi, rubygems, go, cpan, cask)")
	batchSize := flag.Int("batch-size", 25, "max recipes per run")
	tier := flag.Int("tier", 2, "max queue tier to process (1=critical, 2=popular, 3=all)")
	queuePath := flag.String("queue", "data/priority-queue.json", "path to priority queue")
	outputDir := flag.String("output-dir", "recipes", "directory for generated recipes")
	failuresDir := flag.String("failures-dir", "data/failures", "directory for failure records")
	flag.Parse()

	if *ecosystem == "" {
		fmt.Fprintln(os.Stderr, "error: -ecosystem is required")
		flag.Usage()
		os.Exit(1)
	}

	queue, err := seed.Load(*queuePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading queue: %v\n", err)
		os.Exit(1)
	}

	cfg := batch.Config{
		Ecosystem:   *ecosystem,
		BatchSize:   *batchSize,
		MaxTier:     *tier,
		QueuePath:   *queuePath,
		OutputDir:   *outputDir,
		FailuresDir: *failuresDir,
	}

	orch := batch.NewOrchestrator(cfg, queue)
	result, err := orch.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := orch.SaveResults(result); err != nil {
		fmt.Fprintf(os.Stderr, "error saving results: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Batch complete: %d generated, %d failed (%d total)\n",
		result.Generated, result.Failed, result.Total)
}
