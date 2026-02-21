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

// batchControlFile represents the structure of batch-control.json, used to
// extract and update per-ecosystem circuit breaker state.
type batchControlFile struct {
	Enabled            bool                            `json:"enabled"`
	DisabledEcosystems []string                        `json:"disabled_ecosystems"`
	Reason             string                          `json:"reason"`
	IncidentURL        string                          `json:"incident_url"`
	DisabledBy         string                          `json:"disabled_by"`
	DisabledAt         string                          `json:"disabled_at"`
	ExpectedResume     string                          `json:"expected_resume"`
	CircuitBreaker     map[string]*circuitBreakerEntry `json:"circuit_breaker"`
	Budget             *budgetEntry                    `json:"budget"`
}

type circuitBreakerEntry struct {
	State       string `json:"state"`
	Failures    int    `json:"failures"`
	LastFailure string `json:"last_failure"`
	OpensAt     string `json:"opens_at"`
}

type budgetEntry struct {
	MacosMinutesUsed int    `json:"macos_minutes_used"`
	LinuxMinutesUsed int    `json:"linux_minutes_used"`
	WeekStart        string `json:"week_start"`
	SamplingActive   bool   `json:"sampling_active"`
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

	// Read circuit breaker state from batch-control.json. Open breakers past
	// their recovery timeout are transitioned to half-open so the orchestrator
	// allows a single probe entry per ecosystem.
	breakerState := make(map[string]string)
	if data, err := os.ReadFile(*controlFile); err == nil {
		var ctrl batchControlFile
		if err := json.Unmarshal(data, &ctrl); err == nil {
			var modified bool
			breakerState, modified = transitionOpenBreakers(&ctrl, time.Now().UTC())
			if modified {
				if updated, err := json.MarshalIndent(ctrl, "", "  "); err == nil {
					updated = append(updated, '\n')
					if err := os.WriteFile(*controlFile, updated, 0644); err != nil {
						fmt.Fprintf(os.Stderr, "warning: could not update control file: %v\n", err)
					}
				}
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

// transitionOpenBreakers checks each ecosystem's circuit breaker state and
// transitions open breakers to half-open when their recovery timeout (opens_at)
// has elapsed. It returns the effective breaker state map for the orchestrator
// and whether any transitions were made (indicating the control file should be
// rewritten).
func transitionOpenBreakers(ctrl *batchControlFile, now time.Time) (map[string]string, bool) {
	state := make(map[string]string)
	modified := false
	for eco, cb := range ctrl.CircuitBreaker {
		state[eco] = cb.State
		if cb.State == "open" && cb.OpensAt != "" {
			opensAt, err := time.Parse("2006-01-02T15:04:05Z", cb.OpensAt)
			if err == nil && now.After(opensAt) {
				cb.State = "half-open"
				state[eco] = "half-open"
				modified = true
				fmt.Fprintf(os.Stderr, "Circuit breaker half-open for %s (recovery timeout elapsed)\n", eco)
			}
		}
	}
	return state, modified
}
