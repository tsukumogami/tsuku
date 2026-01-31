package batch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/tsukumogami/tsuku/internal/seed"
)

// ExitNetwork is the tsuku CLI exit code for transient network errors.
const ExitNetwork = 5

// MaxRetries is the number of retry attempts for transient failures.
const MaxRetries = 3

// Config holds batch generation settings.
type Config struct {
	Ecosystem   string
	BatchSize   int
	MaxTier     int
	QueuePath   string
	OutputDir   string
	FailuresDir string

	// TsukuBin overrides the tsuku binary path. If empty, "tsuku" is used
	// from PATH.
	TsukuBin string
}

// Orchestrator manages batch recipe generation.
type Orchestrator struct {
	cfg   Config
	queue *seed.PriorityQueue
}

// NewOrchestrator creates an orchestrator with the given config and queue.
func NewOrchestrator(cfg Config, queue *seed.PriorityQueue) *Orchestrator {
	return &Orchestrator{cfg: cfg, queue: queue}
}

// Run processes pending packages from the queue. It selects packages matching
// the configured ecosystem and tier, invokes tsuku create for each, and
// collects results. Queue statuses are updated in place.
func (o *Orchestrator) Run() (*BatchResult, error) {
	candidates := o.selectCandidates()
	if len(candidates) == 0 {
		return &BatchResult{BatchID: generateBatchID(o.cfg.Ecosystem)}, nil
	}

	result := &BatchResult{
		BatchID:   generateBatchID(o.cfg.Ecosystem),
		Ecosystem: o.cfg.Ecosystem,
		Timestamp: time.Now().UTC(),
	}

	bin := o.cfg.TsukuBin
	if bin == "" {
		bin = "tsuku"
	}

	for _, pkg := range candidates {
		o.setStatus(pkg.ID, "in_progress")

		recipePath := recipeOutputPath(o.cfg.OutputDir, pkg.Name)
		genResult := o.generate(bin, pkg, recipePath)
		result.Total++

		if genResult.Err != nil {
			result.Failed++
			result.Failures = append(result.Failures, genResult.Failure)
			o.setStatus(pkg.ID, "failed")
			continue
		}

		result.Generated++
		result.Recipes = append(result.Recipes, recipePath)
		o.setStatus(pkg.ID, "success")
	}

	return result, nil
}

// SaveResults writes failure records and updates the queue file.
func (o *Orchestrator) SaveResults(result *BatchResult) error {
	if err := o.queue.Save(o.cfg.QueuePath); err != nil {
		return fmt.Errorf("save queue: %w", err)
	}

	if len(result.Failures) > 0 {
		if err := WriteFailures(o.cfg.FailuresDir, o.cfg.Ecosystem, result.Failures); err != nil {
			return fmt.Errorf("write failures: %w", err)
		}
	}

	return nil
}

// selectCandidates picks pending packages matching ecosystem and tier.
func (o *Orchestrator) selectCandidates() []seed.Package {
	var candidates []seed.Package
	prefix := o.cfg.Ecosystem + ":"

	for _, pkg := range o.queue.Packages {
		if pkg.Status != "pending" {
			continue
		}
		if !strings.HasPrefix(pkg.ID, prefix) {
			continue
		}
		if pkg.Tier > o.cfg.MaxTier {
			continue
		}
		candidates = append(candidates, pkg)
		if len(candidates) >= o.cfg.BatchSize {
			break
		}
	}

	return candidates
}

type generateResult struct {
	Err     error
	Failure FailureRecord
}

// generate invokes tsuku create for a single package with retry on network errors.
func (o *Orchestrator) generate(bin string, pkg seed.Package, recipePath string) generateResult {
	args := []string{
		"create",
		"--from", pkg.ID,
		"--output", recipePath,
	}

	var lastErr error
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			time.Sleep(delay)
		}

		cmd := exec.Command(bin, args...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return generateResult{}
		}

		exitCode := exitCodeFrom(err)
		lastErr = fmt.Errorf("tsuku create %s: exit %d: %s", pkg.ID, exitCode, truncateOutput(output))

		if exitCode != ExitNetwork {
			return generateResult{
				Err: lastErr,
				Failure: FailureRecord{
					PackageID: pkg.ID,
					Category:  categoryFromExitCode(exitCode),
					Message:   truncateOutput(output),
					Timestamp: time.Now().UTC(),
				},
			}
		}
	}

	return generateResult{
		Err: lastErr,
		Failure: FailureRecord{
			PackageID: pkg.ID,
			Category:  "api_error",
			Message:   fmt.Sprintf("failed after %d retries: %v", MaxRetries, lastErr),
			Timestamp: time.Now().UTC(),
		},
	}
}

func (o *Orchestrator) setStatus(id, status string) {
	for i := range o.queue.Packages {
		if o.queue.Packages[i].ID == id {
			o.queue.Packages[i].Status = status
			return
		}
	}
}

func generateBatchID(ecosystem string) string {
	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("2006-01-02"), ecosystem)
}

// recipeOutputPath computes the recipe file path: recipes/{first-letter}/{name}.toml
func recipeOutputPath(outputDir, name string) string {
	if name == "" {
		return filepath.Join(outputDir, "unknown.toml")
	}
	first := string(unicode.ToLower(rune(name[0])))
	return filepath.Join(outputDir, first, name+".toml")
}

func exitCodeFrom(err error) int {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}

func categoryFromExitCode(code int) string {
	switch code {
	case 5:
		return "api_error"
	case 6:
		return "validation_failed"
	case 7:
		return "validation_failed"
	case 8:
		return "missing_dep"
	default:
		return "validation_failed"
	}
}

func truncateOutput(output []byte) string {
	s := strings.TrimSpace(string(output))
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

// SetTsukuBin sets the path to the tsuku binary for testing.
func (o *Orchestrator) SetTsukuBin(path string) {
	o.cfg.TsukuBin = path
}

// EnsureOutputDir creates the output directory if needed.
func EnsureOutputDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}
