// Command reorder-queue re-orders priority queue entries within each tier
// based on transitive blocking impact, so the batch pipeline processes
// high-leverage recipes (those that unblock the most other packages) first.
//
// Usage:
//
//	reorder-queue [-queue path] [-failures-dir path] [-output path] [-dry-run]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/reorder"
)

func main() {
	queueFile := flag.String("queue", "data/queues/priority-queue.json", "path to unified priority queue file")
	failuresDir := flag.String("failures-dir", "data/failures", "directory containing failures JSONL files")
	output := flag.String("output", "", "output file path (default: overwrite queue file)")
	dryRun := flag.Bool("dry-run", false, "compute and report changes without writing")
	jsonOutput := flag.Bool("json", false, "output result as JSON")
	flag.Parse()

	opts := reorder.Options{
		QueueFile:   *queueFile,
		FailuresDir: *failuresDir,
		OutputFile:  *output,
		DryRun:      *dryRun,
	}

	result, err := reorder.Run(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error marshaling result: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}

	// Human-readable output
	fmt.Fprintf(os.Stderr, "Queue reorder complete\n")
	fmt.Fprintf(os.Stderr, "  Total entries: %d\n", result.TotalEntries)
	fmt.Fprintf(os.Stderr, "  Entries moved: %d\n", result.Reordered)
	for tier, count := range result.ByTier {
		fmt.Fprintf(os.Stderr, "  Tier %d: %d entries\n", tier, count)
	}

	if len(result.TopScores) > 0 {
		fmt.Fprintf(os.Stderr, "\nTop blocking scores:\n")
		for _, s := range result.TopScores {
			fmt.Fprintf(os.Stderr, "  %-30s score=%-4d tier=%d\n", s.Name, s.Score, s.Tier)
		}
	}

	if *dryRun {
		fmt.Fprintf(os.Stderr, "\n(dry-run: no files written)\n")
	} else {
		target := *output
		if target == "" {
			target = *queueFile
		}
		fmt.Fprintf(os.Stderr, "\nQueue written to: %s\n", target)
	}
}
