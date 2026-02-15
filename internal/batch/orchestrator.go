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

// MaxBackoff is the maximum backoff duration for exponential retry delays.
const MaxBackoff = 7 * 24 * time.Hour // 7 days

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

// nowFunc is the time source used by the orchestrator. Tests replace it
// to control time-dependent behavior (backoff, retry windows).
var nowFunc = func() time.Time { return time.Now().UTC() }

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

// Orchestrator manages batch recipe generation using the unified queue.
type Orchestrator struct {
	cfg   Config
	queue *UnifiedQueue
}

// NewOrchestrator creates an orchestrator with the given config and unified queue.
func NewOrchestrator(cfg Config, queue *UnifiedQueue) *Orchestrator {
	return &Orchestrator{cfg: cfg, queue: queue}
}

// Run processes pending entries from the unified queue. It selects entries
// matching the configured ecosystem and priority, invokes tsuku create for
// each, and collects results. Queue entries are updated in place with status
// changes, failure counts, and backoff timestamps.
func (o *Orchestrator) Run() (*BatchResult, error) {
	candidates := o.selectCandidates()
	if len(candidates) == 0 {
		return &BatchResult{BatchID: generateBatchID(o.cfg.Ecosystem)}, nil
	}

	result := &BatchResult{
		BatchID:   generateBatchID(o.cfg.Ecosystem),
		Ecosystem: o.cfg.Ecosystem,
		Timestamp: nowFunc(),
	}

	bin := o.cfg.TsukuBin
	if bin == "" {
		bin = "tsuku"
	}

	rateLimit := ecosystemRateLimits[o.cfg.Ecosystem]

	for i, idx := range candidates {
		pkg := &o.queue.Entries[idx]

		// Rate limit: sleep between packages (not before the first one)
		if i > 0 && rateLimit > 0 {
			time.Sleep(rateLimit)
		}

		recipePath := recipeOutputPath(o.cfg.OutputDir, pkg.Name)
		genResult := o.generate(bin, *pkg, recipePath)
		result.Total++

		if genResult.Err != nil {
			result.Failed++
			result.Failures = append(result.Failures, genResult.Failure)
			o.recordFailure(idx)
			continue
		}

		// Validate the generated recipe by attempting installation.
		valResult := o.validate(bin, *pkg, recipePath)
		if valResult.Err != nil {
			result.Failures = append(result.Failures, valResult.Failure)
			os.Remove(recipePath)
			if valResult.Failure.Category == "missing_dep" || valResult.Failure.Category == "recipe_not_found" {
				result.Blocked++
				pkg.Status = StatusBlocked
			} else {
				result.Failed++
				o.recordFailure(idx)
			}
			continue
		}

		result.Succeeded++
		result.Recipes = append(result.Recipes, recipePath)
		o.recordSuccess(idx)
	}

	return result, nil
}

// SaveResults writes failure records and updates the queue file.
func (o *Orchestrator) SaveResults(result *BatchResult) error {
	if err := SaveUnifiedQueue(o.cfg.QueuePath, o.queue); err != nil {
		return fmt.Errorf("save queue: %w", err)
	}

	if len(result.Failures) > 0 {
		if err := WriteFailures(o.cfg.FailuresDir, o.cfg.Ecosystem, result.Failures); err != nil {
			return fmt.Errorf("write failures: %w", err)
		}
	}

	return nil
}

// selectCandidates returns indices into queue.Entries for entries that are
// eligible for processing: pending status, matching ecosystem, within
// priority limit, and not in a backoff window.
func (o *Orchestrator) selectCandidates() []int {
	var candidates []int
	prefix := o.cfg.Ecosystem + ":"
	now := nowFunc()

	for i, entry := range o.queue.Entries {
		if entry.Status != StatusPending && entry.Status != StatusFailed {
			continue
		}
		if !strings.HasPrefix(entry.Source, prefix) {
			continue
		}
		if entry.Priority > o.cfg.MaxTier {
			continue
		}
		// Skip entries in a backoff window
		if entry.NextRetryAt != nil && entry.NextRetryAt.After(now) {
			continue
		}
		candidates = append(candidates, i)
		if len(candidates) >= o.cfg.BatchSize {
			break
		}
	}

	return candidates
}

// recordFailure increments the failure count, sets the status to failed,
// and computes the next retry time using exponential backoff.
func (o *Orchestrator) recordFailure(idx int) {
	entry := &o.queue.Entries[idx]
	entry.FailureCount++
	entry.Status = StatusFailed
	entry.NextRetryAt = calculateNextRetryAt(entry.FailureCount, nowFunc())
}

// recordSuccess resets failure tracking and marks the entry as successful.
func (o *Orchestrator) recordSuccess(idx int) {
	entry := &o.queue.Entries[idx]
	entry.FailureCount = 0
	entry.NextRetryAt = nil
	entry.Status = StatusSuccess
}

// calculateNextRetryAt computes the backoff delay based on consecutive failures.
//
// Schedule:
//   - 1st failure (count=1): no delay, retry on next batch
//   - 2nd failure (count=2): now + 24 hours
//   - 3rd failure (count=3): now + 72 hours
//   - 4th+ failure: double previous delay, capped at 7 days
func calculateNextRetryAt(failureCount int, now time.Time) *time.Time {
	if failureCount <= 1 {
		return nil
	}

	var delay time.Duration
	switch failureCount {
	case 2:
		delay = 24 * time.Hour
	case 3:
		delay = 72 * time.Hour
	default:
		// 4th+: start from 72h and double for each additional failure.
		// count=4 -> 144h, count=5 -> 288h, etc., capped at MaxBackoff.
		delay = 72 * time.Hour
		for i := 3; i < failureCount; i++ {
			delay *= 2
			if delay > MaxBackoff {
				delay = MaxBackoff
				break
			}
		}
	}

	t := now.Add(delay)
	return &t
}

type generateResult struct {
	Err     error
	Failure FailureRecord
}

// generate invokes tsuku create for a single package with retry on network errors.
// It uses the entry's Source field directly for the --from flag.
func (o *Orchestrator) generate(bin string, pkg QueueEntry, recipePath string) generateResult {
	args := []string{
		"create", pkg.Name,
		"--from", pkg.Source,
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
		lastErr = fmt.Errorf("tsuku create %s: exit %d: %s", pkg.Source, exitCode, truncateOutput(output))

		if exitCode != ExitNetwork {
			return generateResult{
				Err: lastErr,
				Failure: FailureRecord{
					PackageID: pkg.Source,
					Category:  categoryFromExitCode(exitCode),
					Message:   truncateOutput(output),
					Timestamp: nowFunc(),
				},
			}
		}
	}

	return generateResult{
		Err: lastErr,
		Failure: FailureRecord{
			PackageID: pkg.Source,
			Category:  "api_error",
			Message:   fmt.Sprintf("failed after %d retries: %v", MaxRetries, lastErr),
			Timestamp: nowFunc(),
		},
	}
}

// validate runs tsuku install --recipe --json to verify the generated recipe works.
// It uses the same retry logic as generate for transient network errors.
// On failure, it parses the structured JSON response from --json to extract
// the failure category and missing dependency names.
func (o *Orchestrator) validate(bin string, pkg QueueEntry, recipePath string) generateResult {
	args := []string{"install", "--json", "--recipe", recipePath}

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
		lastErr = fmt.Errorf("tsuku install %s: exit %d: %s", pkg.Source, exitCode, truncateOutput(lastStderr))

		if exitCode != ExitNetwork {
			category, blockedBy := parseInstallJSON(lastStdout, exitCode)
			return generateResult{
				Err: lastErr,
				Failure: FailureRecord{
					PackageID: pkg.Source,
					Category:  category,
					BlockedBy: blockedBy,
					Message:   truncateOutput(lastStderr),
					Timestamp: nowFunc(),
				},
			}
		}
	}

	category, blockedBy := parseInstallJSON(lastStdout, ExitNetwork)
	return generateResult{
		Err: lastErr,
		Failure: FailureRecord{
			PackageID: pkg.Source,
			Category:  category,
			BlockedBy: blockedBy,
			Message:   fmt.Sprintf("failed after %d retries: %s", MaxRetries, truncateOutput(lastStderr)),
			Timestamp: nowFunc(),
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

func generateBatchID(ecosystem string) string {
	return fmt.Sprintf("%s-%s", nowFunc().Format("2006-01-02"), ecosystem)
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
