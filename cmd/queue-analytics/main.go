package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/dashboard"
)

func main() {
	queueFile := flag.String("queue", "data/priority-queue.json", "path to priority queue JSON")
	failuresFile := flag.String("failures", "data/failures/homebrew.jsonl", "path to failures JSONL")
	metricsFile := flag.String("metrics", "data/metrics/batch-runs.jsonl", "path to metrics JSONL")
	output := flag.String("output", "website/pipeline/dashboard.json", "output file path")
	flag.Parse()

	opts := dashboard.Options{
		QueueFile:    *queueFile,
		FailuresFile: *failuresFile,
		MetricsFile:  *metricsFile,
		OutputFile:   *output,
	}

	if err := dashboard.Generate(opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Dashboard generated: %s\n", *output)
}
