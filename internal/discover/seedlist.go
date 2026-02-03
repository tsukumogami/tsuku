package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SeedEntry defines a single tool in a seed list file.
type SeedEntry struct {
	Name           string `json:"name"`
	Builder        string `json:"builder"`
	Source         string `json:"source"`
	Binary         string `json:"binary,omitempty"`
	Description    string `json:"description,omitempty"`
	Homepage       string `json:"homepage,omitempty"`
	Repo           string `json:"repo,omitempty"`
	Disambiguation bool   `json:"disambiguation,omitempty"`
	Downloads      int    `json:"downloads,omitempty"`
	VersionCount   int    `json:"version_count,omitempty"`
	HasRepository  bool   `json:"has_repository,omitempty"`
}

// SeedList holds the contents of a seed list JSON file.
type SeedList struct {
	Category string      `json:"category"`
	Entries  []SeedEntry `json:"entries"`
}

// LoadSeedList reads a single seed list file.
func LoadSeedList(path string) (*SeedList, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read seed list %s: %w", path, err)
	}
	var sl SeedList
	if err := json.Unmarshal(data, &sl); err != nil {
		return nil, fmt.Errorf("parse seed list %s: %w", path, err)
	}
	return &sl, nil
}

// LoadSeedDir reads all *.json files from a directory and returns merged entries.
// Later files override earlier ones on name collision (alphabetical order).
func LoadSeedDir(dir string) ([]SeedEntry, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob seed dir %s: %w", dir, err)
	}

	var all []SeedEntry
	for _, path := range matches {
		sl, err := LoadSeedList(path)
		if err != nil {
			return nil, err
		}
		all = append(all, sl.Entries...)
	}
	return all, nil
}

// PriorityQueueFile represents the priority-queue.json format.
type PriorityQueueFile struct {
	SchemaVersion int                  `json:"schema_version"`
	Packages      []PriorityQueueEntry `json:"packages"`
}

// PriorityQueueEntry is a single entry from the priority queue.
type PriorityQueueEntry struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Name   string `json:"name"`
}

// LoadPriorityQueue reads and parses the priority queue file.
func LoadPriorityQueue(path string) (*PriorityQueueFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read priority queue %s: %w", path, err)
	}
	var pq PriorityQueueFile
	if err := json.Unmarshal(data, &pq); err != nil {
		return nil, fmt.Errorf("parse priority queue %s: %w", path, err)
	}
	return &pq, nil
}

// PriorityQueueToSeedEntries converts priority queue entries to seed entries.
// All priority queue entries are homebrew-sourced.
func PriorityQueueToSeedEntries(pq *PriorityQueueFile) []SeedEntry {
	entries := make([]SeedEntry, 0, len(pq.Packages))
	for _, pkg := range pq.Packages {
		entries = append(entries, SeedEntry{
			Name:    pkg.Name,
			Builder: "homebrew",
			Source:  pkg.Name,
		})
	}
	return entries
}

// MergeSeedEntries merges two slices of seed entries. Entries from "override"
// take precedence over "base" when names collide.
func MergeSeedEntries(base, override []SeedEntry) []SeedEntry {
	seen := make(map[string]int) // name -> index in result
	result := make([]SeedEntry, 0, len(base)+len(override))

	for _, e := range base {
		if _, exists := seen[e.Name]; exists {
			continue
		}
		seen[e.Name] = len(result)
		result = append(result, e)
	}

	for _, e := range override {
		if idx, exists := seen[e.Name]; exists {
			result[idx] = e // override
		} else {
			seen[e.Name] = len(result)
			result = append(result, e)
		}
	}
	return result
}
