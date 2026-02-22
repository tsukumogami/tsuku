// Command queue-maintain performs queue maintenance: requeuing blocked entries
// whose dependencies have been resolved, then reordering entries within each
// tier by transitive blocking impact. Both steps run by default; use
// --skip-requeue or --skip-reorder to skip individual steps.
//
// Usage:
//
//	queue-maintain [-queue path] [-failures-dir path] [-output path] [-dry-run] [-json]
//	               [-skip-requeue] [-skip-reorder]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/batch"
	"github.com/tsukumogami/tsuku/internal/reorder"
	"github.com/tsukumogami/tsuku/internal/requeue"
)

// maintainResult holds the combined results from both steps for JSON output.
type maintainResult struct {
	Requeue *requeue.Result `json:"requeue,omitempty"`
	Reorder *reorder.Result `json:"reorder,omitempty"`
}

func main() {
	queueFile := flag.String("queue", "data/queues/priority-queue.json", "path to unified priority queue file")
	failuresDir := flag.String("failures-dir", "data/failures", "directory containing failures JSONL files")
	output := flag.String("output", "", "output file path (default: overwrite queue file)")
	dryRun := flag.Bool("dry-run", false, "compute and report changes without writing")
	jsonOutput := flag.Bool("json", false, "output result as JSON")
	skipRequeue := flag.Bool("skip-requeue", false, "skip the requeue step")
	skipReorder := flag.Bool("skip-reorder", false, "skip the reorder step")
	flag.Parse()

	// Load queue once
	queue, err := batch.LoadUnifiedQueue(*queueFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load queue: %v\n", err)
		os.Exit(1)
	}

	var combined maintainResult

	// Step 1: Requeue
	if !*skipRequeue {
		requeueResult, err := requeue.Run(queue, *failuresDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: requeue: %v\n", err)
			os.Exit(1)
		}
		combined.Requeue = requeueResult
	}

	// Step 2: Reorder
	if !*skipReorder {
		reorderResult, err := reorder.Run(queue, *failuresDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: reorder: %v\n", err)
			os.Exit(1)
		}
		combined.Reorder = reorderResult
	}

	if *jsonOutput {
		data, err := json.MarshalIndent(combined, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error marshaling result: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		if *dryRun {
			return
		}
	} else {
		printHumanOutput(combined, *dryRun)
	}

	// Write once
	if !*dryRun {
		outputPath := *output
		if outputPath == "" {
			outputPath = *queueFile
		}
		if err := batch.SaveUnifiedQueue(outputPath, queue); err != nil {
			fmt.Fprintf(os.Stderr, "error: save queue: %v\n", err)
			os.Exit(1)
		}
		if !*jsonOutput {
			fmt.Fprintf(os.Stderr, "\nQueue written to: %s\n", outputPath)
		}
	}
}

func printHumanOutput(combined maintainResult, dryRun bool) {
	if combined.Requeue != nil {
		fmt.Fprintf(os.Stderr, "Requeue complete\n")
		fmt.Fprintf(os.Stderr, "  Entries requeued: %d\n", combined.Requeue.Requeued)
		fmt.Fprintf(os.Stderr, "  Entries still blocked: %d\n", combined.Requeue.Remaining)
		for _, c := range combined.Requeue.Details {
			fmt.Fprintf(os.Stderr, "  - %s (resolved by: %v)\n", c.Name, c.ResolvedBy)
		}
	}

	if combined.Reorder != nil {
		fmt.Fprintf(os.Stderr, "Reorder complete\n")
		fmt.Fprintf(os.Stderr, "  Total entries: %d\n", combined.Reorder.TotalEntries)
		fmt.Fprintf(os.Stderr, "  Entries moved: %d\n", combined.Reorder.Reordered)
		for tier, count := range combined.Reorder.ByTier {
			fmt.Fprintf(os.Stderr, "  Tier %d: %d entries\n", tier, count)
		}

		if len(combined.Reorder.TopScores) > 0 {
			fmt.Fprintf(os.Stderr, "\nTop blocking scores:\n")
			for _, s := range combined.Reorder.TopScores {
				fmt.Fprintf(os.Stderr, "  %-30s score=%-4d tier=%d\n", s.Name, s.Score, s.Tier)
			}
		}
	}

	if dryRun {
		fmt.Fprintf(os.Stderr, "\n(dry-run: no files written)\n")
	}
}
