package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/dashboard"
)

func main() {
	queueFile := flag.String("queue", "data/queues", "path to priority queue directory or file")
	failuresDir := flag.String("failures-dir", "data/failures", "directory containing failures JSONL files")
	metricsDir := flag.String("metrics-dir", "data/metrics", "directory containing metrics JSONL files")
	output := flag.String("output", "website/pipeline/dashboard.json", "output file path")
	flag.Parse()

	opts := dashboard.Options{
		QueueFile:   *queueFile,
		FailuresDir: *failuresDir,
		MetricsDir:  *metricsDir,
		OutputFile:  *output,
	}

	if err := dashboard.Generate(opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Dashboard generated: %s\n", *output)
}
