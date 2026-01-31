package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/seed"
)

func main() {
	source := flag.String("source", "", "package source (homebrew)")
	limit := flag.Int("limit", 100, "max packages to fetch")
	output := flag.String("output", "data/priority-queue.json", "output file path")
	flag.Parse()

	if *source == "" {
		fmt.Fprintln(os.Stderr, "error: -source is required")
		flag.Usage()
		os.Exit(1)
	}

	var src seed.Source
	switch *source {
	case "homebrew":
		src = &seed.HomebrewSource{}
	default:
		fmt.Fprintf(os.Stderr, "error: unsupported source %q\n", *source)
		os.Exit(1)
	}

	queue, err := seed.Load(*output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	packages, err := src.Fetch(*limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	added := queue.Merge(packages)
	if err := queue.Save(*output); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Merged %d new packages (%d total)\n", added, len(queue.Packages))
}
