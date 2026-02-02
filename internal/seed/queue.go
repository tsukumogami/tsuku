package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Package represents a single entry in the priority queue.
type Package struct {
	ID            string         `json:"id"`
	Source        string         `json:"source"`
	Name          string         `json:"name"`
	Tier          int            `json:"tier"`
	Status        string         `json:"status"`
	AddedAt       string         `json:"added_at"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	ForceOverride bool           `json:"force_override,omitempty"`
}

// PriorityQueue is the top-level structure matching priority-queue.schema.json.
type PriorityQueue struct {
	SchemaVersion int               `json:"schema_version"`
	UpdatedAt     string            `json:"updated_at"`
	Tiers         map[string]string `json:"tiers,omitempty"`
	Packages      []Package         `json:"packages"`
}

// Load reads a priority queue from disk. Returns an empty queue if the file
// doesn't exist.
func Load(path string) (*PriorityQueue, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &PriorityQueue{
			SchemaVersion: 1,
			Packages:      []Package{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read queue: %w", err)
	}
	var q PriorityQueue
	if err := json.Unmarshal(data, &q); err != nil {
		return nil, fmt.Errorf("parse queue: %w", err)
	}
	return &q, nil
}

// Save writes the queue to disk as formatted JSON.
func (q *PriorityQueue) Save(path string) error {
	q.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal queue: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// Merge adds new packages that don't already exist in the queue (by ID).
// Existing entries are never modified. Returns the number of packages added.
func (q *PriorityQueue) Merge(newPackages []Package) int {
	existing := make(map[string]bool, len(q.Packages))
	for _, p := range q.Packages {
		existing[p.ID] = true
	}
	added := 0
	for _, p := range newPackages {
		if !existing[p.ID] {
			q.Packages = append(q.Packages, p)
			existing[p.ID] = true
			added++
		}
	}
	return added
}
