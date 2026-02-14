package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
)

// disambiguationSeeds is the structure of data/discovery-seeds/disambiguations.json
type disambiguationSeeds struct {
	Comment  string      `json:"_comment"`
	Category string      `json:"category"`
	Entries  []seedEntry `json:"entries"`
}

type seedEntry struct {
	Name           string `json:"name"`
	Builder        string `json:"builder"`
	Source         string `json:"source"`
	Disambiguation bool   `json:"disambiguation"`
}

func main() {
	seedsFile := flag.String("seeds", "data/discovery-seeds/disambiguations.json", "path to disambiguation seeds JSON")
	outputDir := flag.String("output", "data/disambiguations", "output directory for JSONL files")
	flag.Parse()

	// Read seeds file
	data, err := os.ReadFile(*seedsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading seeds: %v\n", err)
		os.Exit(1)
	}

	var seeds disambiguationSeeds
	if err := json.Unmarshal(data, &seeds); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing seeds: %v\n", err)
		os.Exit(1)
	}

	// Convert to DisambiguationRecords
	var records []batch.DisambiguationRecord
	for _, entry := range seeds.Entries {
		if !entry.Disambiguation {
			continue
		}

		// Format selected as "builder:source"
		selected := fmt.Sprintf("%s:%s", entry.Builder, entry.Source)

		// These are manual selections, so mark as curated (not high-risk)
		record := batch.DisambiguationRecord{
			Tool:            entry.Name,
			Selected:        selected,
			Alternatives:    []string{}, // Seed file doesn't list alternatives
			SelectionReason: batch.SelectionCurated,
			HighRisk:        false, // Curated selections are reviewed
		}
		records = append(records, record)
	}

	if len(records) == 0 {
		fmt.Fprintln(os.Stderr, "no disambiguation entries found")
		os.Exit(0)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output dir: %v\n", err)
		os.Exit(1)
	}

	// Write to JSONL file (using "curated" as ecosystem since these are manual)
	outputFile := filepath.Join(*outputDir, "curated.jsonl")

	file := batch.DisambiguationFile{
		SchemaVersion:   1,
		Ecosystem:       "curated",
		Environment:     "manual-selection",
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
		Disambiguations: records,
	}

	jsonData, err := json.Marshal(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputFile, append(jsonData, '\n'), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Wrote %d disambiguation records to %s\n", len(records), outputFile)
}
