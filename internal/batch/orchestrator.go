package batch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/tsukumogami/tsuku/internal/seed"
)

// installResult is the subset of tsuku install --json output that the
// orchestrator needs for failure classification.
type installResult struct {
	Category       string   `json:"category"`
	MissingRecipes []string `json:"missing_recipes"`
}

// ExitNetwork is the tsuku CLI exit code for transient network errors.
const ExitNetwork = 5

// MaxRetries is the number of retry attempts for transient failures.
const MaxRetries = 3

// ecosystemRateLimits defines the sleep duration between package generations
// per ecosystem, to respect API rate limits.
var ecosystemRateLimits = map[string]time.Duration{
	"homebrew": 1 * time.Second,
	"cargo":    1 * time.Second,
	"npm":      1 * time.Second,
	"pypi":     1 * time.Second,
	"go":       1 * time.Second,
	"rubygems": 6 * time.Second,
	"cpan":     1 * time.Second,
	"cask":     1 * time.Second,
}

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

	rateLimit := ecosystemRateLimits[o.cfg.Ecosystem]

	for i, pkg := range candidates {
		// Rate limit: sleep between packages (not before the first one)
		if i > 0 && rateLimit > 0 {
			time.Sleep(rateLimit)
		}
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

		// Validate the generated recipe by attempting installation.
		valResult := o.validate(bin, pkg, recipePath)
		if valResult.Err != nil {
			result.Failures = append(result.Failures, valResult.Failure)
			os.Remove(recipePath)
			if valResult.Failure.Category == "missing_dep" || valResult.Failure.Category == "recipe_not_found" {
				result.Blocked++
				o.setStatus(pkg.ID, "blocked")
			} else {
				result.Failed++
				o.setStatus(pkg.ID, "failed")
			}
			continue
		}

		result.Succeeded++
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
		"create", pkg.Name,
		"--from", pkg.ID,
		"--output", recipePath,
		"--yes",
		"--skip-sandbox",
		"--deterministic-only",
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

// validate runs tsuku install --recipe --json to verify the generated recipe works.
// It uses the same retry logic as generate for transient network errors.
// On failure, it parses the structured JSON response from --json to extract
// the failure category and missing dependency names.
func (o *Orchestrator) validate(bin string, pkg seed.Package, recipePath string) generateResult {
	args := []string{"install", "--json", "--recipe", recipePath}
	if pkg.ForceOverride {
		args = append(args, "--force")
	}

	var lastErr error
	var lastStdout, lastStderr []byte
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			time.Sleep(delay)
		}

		cmd := exec.Command(bin, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			return generateResult{}
		}

		lastStdout = stdout.Bytes()
		lastStderr = stderr.Bytes()
		exitCode := exitCodeFrom(err)
		lastErr = fmt.Errorf("tsuku install %s: exit %d: %s", pkg.ID, exitCode, truncateOutput(lastStderr))

		if exitCode != ExitNetwork {
			category, blockedBy := parseInstallJSON(lastStdout, exitCode)
			return generateResult{
				Err: lastErr,
				Failure: FailureRecord{
					PackageID: pkg.ID,
					Category:  category,
					BlockedBy: blockedBy,
					Message:   truncateOutput(lastStderr),
					Timestamp: time.Now().UTC(),
				},
			}
		}
	}

	category, blockedBy := parseInstallJSON(lastStdout, ExitNetwork)
	return generateResult{
		Err: lastErr,
		Failure: FailureRecord{
			PackageID: pkg.ID,
			Category:  category,
			BlockedBy: blockedBy,
			Message:   fmt.Sprintf("failed after %d retries: %s", MaxRetries, truncateOutput(lastStderr)),
			Timestamp: time.Now().UTC(),
		},
	}
}

// parseInstallJSON extracts the failure category and missing recipes from the
// structured JSON output of tsuku install --json. If JSON parsing fails, it
// falls back to exit-code-based classification.
func parseInstallJSON(stdout []byte, exitCode int) (category string, blockedBy []string) {
	var result installResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		return categoryFromExitCode(exitCode), nil
	}
	cat := result.Category
	if cat == "" {
		cat = categoryFromExitCode(exitCode)
	}
	return cat, result.MissingRecipes
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
	case 9:
		return "deterministic_insufficient"
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
