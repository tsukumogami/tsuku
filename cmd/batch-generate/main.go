package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tsukumogami/tsuku/internal/batch"
)

// batchControlFile represents the structure of batch-control.json, used to
// extract per-ecosystem circuit breaker state.
type batchControlFile struct {
	CircuitBreaker map[string]struct {
		State string `json:"state"`
	} `json:"circuit_breaker"`
}

func main() {
	filterEcosystem := flag.String("filter-ecosystem", "", "optional: only process entries from this ecosystem (for debugging)")
	batchSize := flag.Int("batch-size", 25, "max recipes per run")
	tier := flag.Int("tier", 2, "max queue priority to process (1=critical, 2=popular, 3=all)")
	queuePath := flag.String("queue", "data/queues/priority-queue.json", "path to unified priority queue")
	outputDir := flag.String("output-dir", "recipes", "directory for generated recipes")
	failuresDir := flag.String("failures-dir", "data/failures", "directory for failure records")
	controlFile := flag.String("control-file", "batch-control.json", "path to batch-control.json")
	flag.Parse()

	queue, err := batch.LoadUnifiedQueue(*queuePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading queue: %v\n", err)
		os.Exit(1)
	}

	// Read circuit breaker state from batch-control.json.
	breakerState := make(map[string]string)
	if data, err := os.ReadFile(*controlFile); err == nil {
		var ctrl batchControlFile
		if err := json.Unmarshal(data, &ctrl); err == nil {
			for eco, cb := range ctrl.CircuitBreaker {
				breakerState[eco] = cb.State
			}
		}
	}

	cfg := batch.Config{
		BatchSize:       *batchSize,
		MaxTier:         *tier,
		QueuePath:       *queuePath,
		OutputDir:       *outputDir,
		FailuresDir:     *failuresDir,
		ControlFile:     *controlFile,
		BreakerState:    breakerState,
		FilterEcosystem: *filterEcosystem,
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

	fmt.Fprintf(os.Stderr, "Batch complete: %d succeeded, %d failed, %d blocked (%d total)\n",
		result.Succeeded, result.Failed, result.Blocked, result.Total)

	// Write summary for use in PR body.
	summaryPath := filepath.Join(*outputDir, "..", "data", "batch-summary.md")
	if err := os.WriteFile(summaryPath, []byte(result.Summary()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write summary: %v\n", err)
	}
}
