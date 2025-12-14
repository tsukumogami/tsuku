package builders

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/llm"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/telemetry"
	"github.com/tsukumogami/tsuku/internal/validate"
)

const (
	// maxHomebrewResponseSize limits response body to prevent memory exhaustion (1MB)
	maxHomebrewResponseSize = 1 * 1024 * 1024
	// defaultHomebrewAPIURL is the Homebrew formula API endpoint
	defaultHomebrewAPIURL = "https://formulae.brew.sh"
)

// Tool names for Homebrew builder conversation
const (
	ToolFetchFormulaJSON    = "fetch_formula_json"
	ToolFetchFormulaRuby    = "fetch_formula_ruby"
	ToolInspectBottle       = "inspect_bottle"
	ToolExtractRecipe       = "extract_recipe"
	ToolExtractSourceRecipe = "extract_source_recipe"
)

// RegistryChecker checks if a recipe exists in the registry.
type RegistryChecker interface {
	HasRecipe(name string) bool
}

// DependencyNode represents a formula and its dependencies in the tree.
type DependencyNode struct {
	Formula       string            // Homebrew formula name
	Dependencies  []string          // Runtime dependency names from JSON API
	HasRecipe     bool              // Already exists in tsuku registry
	NeedsGenerate bool              // Needs LLM generation
	Children      []*DependencyNode // Resolved child dependency nodes
}

// ToGenerationOrder returns formulas in topological order (leaves first).
// Only includes formulas that need generation (NeedsGenerate=true).
// Diamond dependencies are handled correctly (each formula appears once).
func (node *DependencyNode) ToGenerationOrder() []string {
	var result []string
	visited := make(map[string]bool)

	var visit func(*DependencyNode)
	visit = func(n *DependencyNode) {
		if visited[n.Formula] {
			return
		}
		visited[n.Formula] = true

		// Visit children first (leaves before parents)
		for _, child := range n.Children {
			visit(child)
		}

		if n.NeedsGenerate {
			result = append(result, n.Formula)
		}
	}

	visit(node)
	return result
}

// CountNeedingGeneration returns the number of formulas that need generation.
func (node *DependencyNode) CountNeedingGeneration() int {
	return len(node.ToGenerationOrder())
}

// HomebrewBuilder generates recipes from Homebrew formulas using LLM analysis.
type HomebrewBuilder struct {
	httpClient      *http.Client
	factory         *llm.Factory
	executor        *validate.Executor
	sanitizer       *validate.Sanitizer
	homebrewAPIURL  string
	telemetryClient *telemetry.Client
	progress        ProgressReporter
	registry        RegistryChecker
}

// HomebrewBuilderOption configures a HomebrewBuilder.
type HomebrewBuilderOption func(*HomebrewBuilder)

// WithHomebrewHTTPClient sets a custom HTTP client.
func WithHomebrewHTTPClient(c *http.Client) HomebrewBuilderOption {
	return func(b *HomebrewBuilder) {
		b.httpClient = c
	}
}

// WithHomebrewFactory sets the LLM provider factory.
func WithHomebrewFactory(f *llm.Factory) HomebrewBuilderOption {
	return func(b *HomebrewBuilder) {
		b.factory = f
	}
}

// WithHomebrewExecutor sets the validation executor.
func WithHomebrewExecutor(e *validate.Executor) HomebrewBuilderOption {
	return func(b *HomebrewBuilder) {
		b.executor = e
	}
}

// WithHomebrewSanitizer sets the error sanitizer.
func WithHomebrewSanitizer(s *validate.Sanitizer) HomebrewBuilderOption {
	return func(b *HomebrewBuilder) {
		b.sanitizer = s
	}
}

// WithHomebrewAPIURL sets a custom Homebrew API base URL (for testing).
func WithHomebrewAPIURL(url string) HomebrewBuilderOption {
	return func(b *HomebrewBuilder) {
		b.homebrewAPIURL = url
	}
}

// WithHomebrewTelemetryClient sets the telemetry client for emitting LLM events.
func WithHomebrewTelemetryClient(c *telemetry.Client) HomebrewBuilderOption {
	return func(b *HomebrewBuilder) {
		b.telemetryClient = c
	}
}

// WithHomebrewProgressReporter sets the progress reporter for stage updates.
func WithHomebrewProgressReporter(p ProgressReporter) HomebrewBuilderOption {
	return func(b *HomebrewBuilder) {
		b.progress = p
	}
}

// WithRegistryChecker sets the registry checker for dependency lookups.
func WithRegistryChecker(r RegistryChecker) HomebrewBuilderOption {
	return func(b *HomebrewBuilder) {
		b.registry = r
	}
}

// NewHomebrewBuilder creates a new HomebrewBuilder.
// If no options are provided, defaults are used:
// - HTTP client with 60s timeout
// - Factory auto-detected from environment (ANTHROPIC_API_KEY, GOOGLE_API_KEY)
// - Sanitizer with default patterns
// - No executor (validation skipped)
// - Default telemetry client (respects TSUKU_NO_TELEMETRY)
func NewHomebrewBuilder(ctx context.Context, opts ...HomebrewBuilderOption) (*HomebrewBuilder, error) {
	b := &HomebrewBuilder{
		homebrewAPIURL: defaultHomebrewAPIURL,
	}

	for _, opt := range opts {
		opt(b)
	}

	// Set defaults for unset options
	if b.httpClient == nil {
		b.httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}

	if b.factory == nil {
		factory, err := llm.NewFactory(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create LLM factory: %w", err)
		}
		b.factory = factory
	}

	if b.sanitizer == nil {
		b.sanitizer = validate.NewSanitizer()
	}

	if b.telemetryClient == nil {
		b.telemetryClient = telemetry.NewClient()
	}

	// Set up factory callback for circuit breaker telemetry
	b.factory.SetOnBreakerTrip(func(provider string, failures int) {
		b.telemetryClient.SendLLM(telemetry.NewLLMCircuitBreakerTripEvent(provider, failures))
	})

	// executor is optional - validation skipped if nil

	return b, nil
}

// Name returns the builder identifier.
func (b *HomebrewBuilder) Name() string {
	return "homebrew"
}

// reportStart reports a stage starting, if progress reporter is set.
func (b *HomebrewBuilder) reportStart(stage string) {
	if b.progress != nil {
		b.progress.OnStageStart(stage)
	}
}

// reportDone reports a stage completed successfully, if progress reporter is set.
func (b *HomebrewBuilder) reportDone(detail string) {
	if b.progress != nil {
		b.progress.OnStageDone(detail)
	}
}

// reportFailed reports a stage failed, if progress reporter is set.
func (b *HomebrewBuilder) reportFailed() {
	if b.progress != nil {
		b.progress.OnStageFailed()
	}
}

// homebrewFormulaInfo represents Homebrew formula metadata from the JSON API
type homebrewFormulaInfo struct {
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Tap         string `json:"tap"`
	Description string `json:"desc"`
	Homepage    string `json:"homepage"`
	Versions    struct {
		Stable string `json:"stable"`
		Head   string `json:"head,omitempty"`
		Bottle bool   `json:"bottle"`
	} `json:"versions"`
	Deprecated bool `json:"deprecated"`
	Disabled   bool `json:"disabled"`
	// Bottle info for checking availability
	Bottle map[string]struct {
		Files map[string]struct {
			URL    string `json:"url"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	} `json:"bottle"`
	// Dependencies
	Dependencies         []string `json:"dependencies"`
	BuildDependencies    []string `json:"build_dependencies"`
	TestDependencies     []string `json:"test_dependencies"`
	OptionalDependencies []string `json:"optional_dependencies"`
	// Source URLs for building from source
	URLs struct {
		Stable struct {
			URL      string `json:"url"`
			Checksum string `json:"checksum"`
		} `json:"stable"`
	} `json:"urls"`
}

// HomebrewFormulaNotFoundError indicates a formula doesn't exist.
type HomebrewFormulaNotFoundError struct {
	Formula string
}

func (e *HomebrewFormulaNotFoundError) Error() string {
	return fmt.Sprintf("Homebrew formula '%s' not found", e.Formula)
}

// HomebrewNoBottlesError indicates a formula has no bottles available.
type HomebrewNoBottlesError struct {
	Formula string
}

func (e *HomebrewNoBottlesError) Error() string {
	return fmt.Sprintf("Homebrew formula '%s' has no bottles available", e.Formula)
}

// CanBuild checks if the formula exists in Homebrew and has bottles available.
func (b *HomebrewBuilder) CanBuild(ctx context.Context, packageName string) (bool, error) {
	// Validate formula name
	if !isValidHomebrewFormula(packageName) {
		return false, nil
	}

	// Query Homebrew API
	formulaInfo, err := b.fetchFormulaInfo(ctx, packageName)
	if err != nil {
		// Not found or disabled means we can't build
		if _, ok := err.(*HomebrewFormulaNotFoundError); ok {
			return false, nil
		}
		return false, err
	}

	// Check if formula has bottles
	if !formulaInfo.Versions.Bottle {
		return false, nil
	}

	return true, nil
}

// DiscoverDependencyTree traverses Homebrew API to build the full dependency tree.
// It queries each formula's runtime dependencies recursively and checks the registry
// to determine which formulas already have recipes.
//
// The returned tree contains:
// - Formula name and dependencies
// - Whether each formula has an existing recipe (HasRecipe)
// - Whether each formula needs generation (NeedsGenerate = !HasRecipe)
// - Child nodes for all runtime dependencies
//
// Diamond dependencies (shared deps) are handled correctly - each formula is
// queried once and shared in the tree structure.
func (b *HomebrewBuilder) DiscoverDependencyTree(ctx context.Context, formula string) (*DependencyNode, error) {
	visited := make(map[string]*DependencyNode)
	return b.discoverDependencyTreeRecursive(ctx, formula, visited)
}

func (b *HomebrewBuilder) discoverDependencyTreeRecursive(
	ctx context.Context,
	formula string,
	visited map[string]*DependencyNode,
) (*DependencyNode, error) {
	// Check if already visited (diamond dependency)
	if node, ok := visited[formula]; ok {
		return node, nil
	}

	// Validate formula name
	if !isValidHomebrewFormula(formula) {
		return nil, fmt.Errorf("invalid formula name: %s", formula)
	}

	// Report progress
	b.reportStart(fmt.Sprintf("Discovering %s", formula))

	// Fetch formula metadata
	info, err := b.fetchFormulaInfo(ctx, formula)
	if err != nil {
		b.reportFailed()
		return nil, fmt.Errorf("failed to fetch formula %s: %w", formula, err)
	}

	// Check if recipe exists in registry
	hasRecipe := false
	if b.registry != nil {
		hasRecipe = b.registry.HasRecipe(formula)
	}

	node := &DependencyNode{
		Formula:       formula,
		Dependencies:  info.Dependencies,
		HasRecipe:     hasRecipe,
		NeedsGenerate: !hasRecipe,
	}

	// Mark as visited before recursing (handles cycles, though Homebrew shouldn't have them)
	visited[formula] = node

	// Recursively resolve children
	for _, dep := range info.Dependencies {
		child, err := b.discoverDependencyTreeRecursive(ctx, dep, visited)
		if err != nil {
			return nil, err
		}
		node.Children = append(node.Children, child)
	}

	b.reportDone("")
	return node, nil
}

// EstimatedCostPerRecipe is the approximate LLM cost for generating one recipe.
const EstimatedCostPerRecipe = 0.05

// FormatTree returns a human-readable representation of the dependency tree.
func (node *DependencyNode) FormatTree() string {
	var sb strings.Builder
	node.formatTreeRecursive(&sb, "", true, make(map[string]bool))
	return sb.String()
}

func (node *DependencyNode) formatTreeRecursive(sb *strings.Builder, prefix string, isLast bool, visited map[string]bool) {
	// Print connector
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	if prefix != "" {
		sb.WriteString(prefix)
		sb.WriteString(connector)
	}

	// Print formula name with status
	sb.WriteString(node.Formula)
	if node.HasRecipe {
		sb.WriteString(" (has recipe)")
	} else {
		sb.WriteString(" (needs recipe)")
	}

	// Mark duplicates in diamond dependencies
	if visited[node.Formula] {
		sb.WriteString(" [duplicate]")
		sb.WriteString("\n")
		return
	}
	visited[node.Formula] = true
	sb.WriteString("\n")

	// Prepare prefix for children
	childPrefix := prefix
	if prefix != "" {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	// Recurse to children
	for i, child := range node.Children {
		isChildLast := i == len(node.Children)-1
		child.formatTreeRecursive(sb, childPrefix, isChildLast, visited)
	}
}

// EstimatedCost returns the estimated LLM cost for generating all needed recipes.
func (node *DependencyNode) EstimatedCost() float64 {
	return float64(node.CountNeedingGeneration()) * EstimatedCostPerRecipe
}

// ConfirmationRequest holds information for user confirmation before generation.
type ConfirmationRequest struct {
	Tree          *DependencyNode
	ToGenerate    []string
	AlreadyHave   []string
	EstimatedCost float64
	FormattedTree string
}

// NewConfirmationRequest creates a confirmation request from a dependency tree.
func NewConfirmationRequest(tree *DependencyNode) *ConfirmationRequest {
	toGenerate := tree.ToGenerationOrder()

	// Collect formulas that already have recipes
	var alreadyHave []string
	var collectExisting func(*DependencyNode, map[string]bool)
	collectExisting = func(n *DependencyNode, visited map[string]bool) {
		if visited[n.Formula] {
			return
		}
		visited[n.Formula] = true
		if n.HasRecipe {
			alreadyHave = append(alreadyHave, n.Formula)
		}
		for _, child := range n.Children {
			collectExisting(child, visited)
		}
	}
	collectExisting(tree, make(map[string]bool))

	return &ConfirmationRequest{
		Tree:          tree,
		ToGenerate:    toGenerate,
		AlreadyHave:   alreadyHave,
		EstimatedCost: tree.EstimatedCost(),
		FormattedTree: tree.FormatTree(),
	}
}

// ConfirmFunc is called to get user confirmation before generation.
// Returns true if the user confirms, false to cancel.
type ConfirmFunc func(req *ConfirmationRequest) bool

// BuildWithDependencies discovers the dependency tree and generates all needed recipes.
// It shows the user the full tree and cost estimate, then requires confirmation before
// proceeding with LLM calls.
func (b *HomebrewBuilder) BuildWithDependencies(
	ctx context.Context,
	req BuildRequest,
	confirm ConfirmFunc,
) ([]*BuildResult, error) {
	// Parse SourceArg to extract formula name and source build flag
	// Fall back to Package if SourceArg is empty
	sourceArg := req.SourceArg
	if sourceArg == "" {
		sourceArg = req.Package
	}
	formulaName, forceSource, err := parseSourceArg(sourceArg)
	if err != nil {
		return nil, fmt.Errorf("invalid source argument: %w", err)
	}

	// 1. Discover full dependency tree (no LLM, just API calls)
	b.reportStart("Discovering dependencies")
	tree, err := b.DiscoverDependencyTree(ctx, formulaName)
	if err != nil {
		b.reportFailed()
		return nil, fmt.Errorf("failed to discover dependencies: %w", err)
	}
	b.reportDone("")

	// 2. Get formulas needing generation
	toGenerate := tree.ToGenerationOrder()
	if len(toGenerate) == 0 {
		// All recipes exist - nothing to generate
		return nil, nil
	}

	// 3. Request user confirmation
	confirmReq := NewConfirmationRequest(tree)
	if confirm != nil && !confirm(confirmReq) {
		return nil, ErrUserCanceled
	}

	// 4. Generate in topological order (leaves first)
	var results []*BuildResult
	for i, formula := range toGenerate {
		b.reportStart(fmt.Sprintf("Generating recipe %d/%d: %s", i+1, len(toGenerate), formula))

		// Only apply forceSource to the root package, not dependencies
		sourceArg := formula
		if forceSource && formula == formulaName {
			sourceArg = formula + ":source"
		}

		result, err := b.Build(ctx, BuildRequest{
			Package:   formula,
			SourceArg: sourceArg,
		})
		if err != nil {
			b.reportFailed()
			return results, fmt.Errorf("failed to generate recipe for %s: %w", formula, err)
		}

		b.reportDone("")
		results = append(results, result)
	}

	return results, nil
}

// ErrUserCanceled is returned when the user cancels the operation.
var ErrUserCanceled = fmt.Errorf("operation canceled by user")

// isValidHomebrewFormula validates Homebrew formula names.
//
// Homebrew formula names:
// - Lowercase letters, numbers, hyphens, underscores
// - May contain @ for versioned formulae (e.g., openssl@1.1)
// - No path separators or shell metacharacters
//
// Security: Prevents command injection and path traversal
func isValidHomebrewFormula(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}

	// Reject dangerous patterns
	if strings.Contains(name, "..") || strings.Contains(name, "/") ||
		strings.Contains(name, "\\") || strings.HasPrefix(name, "-") {
		return false
	}

	// Check allowed characters
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '@' || c == '.') {
			return false
		}
	}

	return true
}

// isValidVerifyCommand checks if a verify command is safe to execute.
// It rejects commands containing shell metacharacters that could enable injection.
func isValidVerifyCommand(cmd string) error {
	if cmd == "" {
		return fmt.Errorf("verify command cannot be empty")
	}

	// Reject shell metacharacters that could enable command injection
	dangerousChars := []string{";", "&&", "||", "|", "`", "$", "(", ")", "{", "}", "<", ">", "\n", "\r"}
	for _, c := range dangerousChars {
		if strings.Contains(cmd, c) {
			return fmt.Errorf("verify command contains dangerous character %q", c)
		}
	}

	// Reject commands that don't look like version checks
	// Valid patterns: "tool --version", "tool -v", "tool -V", "tool version"
	cmdLower := strings.ToLower(cmd)
	hasVersionFlag := strings.Contains(cmdLower, "--version") ||
		strings.Contains(cmdLower, "-v") ||
		strings.Contains(cmdLower, "version")
	if !hasVersionFlag {
		return fmt.Errorf("verify command should check version (use --version, -v, or version subcommand)")
	}

	return nil
}

// getStringArg extracts a string argument from LLM tool call arguments.
// If the key is missing, returns defaultVal. If the value is not a string, returns an error.
func getStringArg(args map[string]interface{}, key string, defaultVal string) (string, error) {
	val, ok := args[key]
	if !ok {
		return defaultVal, nil
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("argument %q must be a string, got %T", key, val)
	}
	if str == "" {
		return defaultVal, nil
	}
	return str, nil
}

// parseSourceArg parses the builder-specific SourceArg for Homebrew.
// It extracts the formula name and whether source build is requested.
// Examples:
//   - "jq" → ("jq", false, nil)
//   - "jq:source" → ("jq", true, nil)
//   - "openssl@1.1:source" → ("openssl@1.1", true, nil)
//   - "" → ("", false, error)
func parseSourceArg(sourceArg string) (formula string, forceSource bool, err error) {
	if sourceArg == "" {
		return "", false, fmt.Errorf("source argument is required (use --from homebrew:formula)")
	}

	if strings.HasSuffix(strings.ToLower(sourceArg), ":source") {
		formula = sourceArg[:len(sourceArg)-7]
		forceSource = true
	} else {
		formula = sourceArg
		forceSource = false
	}

	if !isValidHomebrewFormula(formula) {
		return "", false, fmt.Errorf("invalid Homebrew formula name: %s", formula)
	}

	return formula, forceSource, nil
}

// fetchFormulaInfo fetches formula metadata from Homebrew API.
func (b *HomebrewBuilder) fetchFormulaInfo(ctx context.Context, formula string) (*homebrewFormulaInfo, error) {
	baseURL, err := url.Parse(b.homebrewAPIURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	apiURL := baseURL.JoinPath("api", "formula", formula+".json")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch formula info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, &HomebrewFormulaNotFoundError{Formula: formula}
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Homebrew API returned status %d", resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, maxHomebrewResponseSize)

	var formulaInfo homebrewFormulaInfo
	if err := json.NewDecoder(limitedReader).Decode(&formulaInfo); err != nil {
		return nil, fmt.Errorf("failed to parse formula info: %w", err)
	}

	// Check if formula is disabled
	if formulaInfo.Disabled {
		return nil, &HomebrewFormulaNotFoundError{Formula: formula}
	}

	return &formulaInfo, nil
}

// Build generates a recipe from a Homebrew formula.
// It first attempts bottle-based generation, falling back to source-based
// generation if bottles are not available.
func (b *HomebrewBuilder) Build(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	// Parse SourceArg to extract formula name and source build flag
	sourceArg := req.SourceArg
	if sourceArg == "" {
		sourceArg = req.Package
	}
	formula, forceSource, err := parseSourceArg(sourceArg)
	if err != nil {
		return nil, fmt.Errorf("invalid source argument: %w", err)
	}

	// Fetch formula metadata
	b.reportStart("Fetching formula metadata")
	formulaInfo, err := b.fetchFormulaInfo(ctx, formula)
	if err != nil {
		b.reportFailed()
		return nil, fmt.Errorf("failed to fetch formula: %w", err)
	}

	// Check for bottles - if not available or source forced, switch to source build mode
	if !formulaInfo.Versions.Bottle || forceSource {
		suffix := "source only"
		if forceSource && formulaInfo.Versions.Bottle {
			suffix = "source requested"
		}
		b.reportDone(fmt.Sprintf("v%s (%s)", formulaInfo.Versions.Stable, suffix))
		return b.buildFromSource(ctx, req, formula, formulaInfo)
	}

	b.reportDone(fmt.Sprintf("v%s", formulaInfo.Versions.Stable))

	// Check bottle availability across platforms
	b.reportStart("Checking bottle availability")
	availability, err := b.checkBottleAvailability(ctx, formula, formulaInfo.Versions.Stable)
	if err != nil {
		// Non-fatal: continue with generation but log the error
		b.reportDone("check skipped")
	} else {
		b.reportDone(fmt.Sprintf("%d/%d platforms", len(availability.Available), len(targetPlatforms)))
	}

	// Build generation context
	genCtx := &homebrewGenContext{
		formula:     formula,
		formulaInfo: formulaInfo,
		httpClient:  b.httpClient,
		apiURL:      b.homebrewAPIURL,
	}

	// Get provider from factory
	provider, err := b.factory.GetProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("no LLM provider available: %w", err)
	}

	// Emit generation started event
	b.telemetryClient.SendLLM(telemetry.NewLLMGenerationStartedEvent(provider.Name(), req.Package, "homebrew:"+formula))
	startTime := time.Now()

	// Generate recipe with repair loop
	recipeData, usage, repairAttempts, validationSkipped, err := b.generateWithRepair(ctx, provider, genCtx, req.Package)

	// Calculate duration for completed event
	durationMs := time.Since(startTime).Milliseconds()

	if err != nil {
		b.factory.ReportFailure(provider.Name())
		// Emit generation completed (failure) event
		b.telemetryClient.SendLLM(telemetry.NewLLMGenerationCompletedEvent(provider.Name(), req.Package, false, durationMs, repairAttempts+1))
		return nil, err
	}
	b.factory.ReportSuccess(provider.Name())

	// Emit generation completed (success) event
	b.telemetryClient.SendLLM(telemetry.NewLLMGenerationCompletedEvent(provider.Name(), req.Package, true, durationMs, repairAttempts+1))

	// Generate recipe from extracted data
	r, err := b.generateRecipe(req.Package, formulaInfo, recipeData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate recipe: %w", err)
	}

	result := &BuildResult{
		Recipe: r,
		Source: fmt.Sprintf("homebrew:%s", formula),
		Warnings: []string{
			fmt.Sprintf("LLM usage: %s", usage.String()),
		},
		RepairAttempts:    repairAttempts,
		Provider:          provider.Name(),
		ValidationSkipped: validationSkipped,
		Cost:              usage.Cost(),
	}

	if repairAttempts > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Recipe repaired after %d attempt(s)", repairAttempts))
	}

	// Add warnings for missing platform bottles
	if availability != nil && len(availability.Unavailable) > 0 {
		for _, platform := range availability.Unavailable {
			displayName := platformDisplayNames[platform]
			if displayName == "" {
				displayName = platform
			}
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("No bottle available for %s - recipe may not work on this platform", displayName))
		}
	}

	return result, nil
}

// buildFromSource generates a source-based recipe when bottles are not available.
func (b *HomebrewBuilder) buildFromSource(ctx context.Context, req BuildRequest, formula string, formulaInfo *homebrewFormulaInfo) (*BuildResult, error) {
	b.reportStart("Generating source build recipe")

	// Build generation context
	genCtx := &homebrewGenContext{
		formula:     formula,
		formulaInfo: formulaInfo,
		httpClient:  b.httpClient,
		apiURL:      b.homebrewAPIURL,
	}

	// Get provider from factory
	provider, err := b.factory.GetProvider(ctx)
	if err != nil {
		b.reportFailed()
		return nil, fmt.Errorf("no LLM provider available: %w", err)
	}

	// Emit generation started event
	b.telemetryClient.SendLLM(telemetry.NewLLMGenerationStartedEvent(provider.Name(), req.Package, "homebrew-source:"+formula))
	startTime := time.Now()

	// Generate source recipe with LLM
	srcData, usage, repairAttempts, err := b.generateSourceRecipe(ctx, provider, genCtx)

	// Calculate duration for completed event
	durationMs := time.Since(startTime).Milliseconds()

	if err != nil {
		b.factory.ReportFailure(provider.Name())
		b.reportFailed()
		b.telemetryClient.SendLLM(telemetry.NewLLMGenerationCompletedEvent(provider.Name(), req.Package, false, durationMs, repairAttempts+1))
		return nil, err
	}
	b.factory.ReportSuccess(provider.Name())
	b.reportDone("")

	// Emit generation completed (success) event
	b.telemetryClient.SendLLM(telemetry.NewLLMGenerationCompletedEvent(provider.Name(), req.Package, true, durationMs, repairAttempts+1))

	// Generate recipe from source data
	r, err := b.generateSourceRecipeOutput(req.Package, formulaInfo, srcData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate source recipe: %w", err)
	}

	// Validate source build in container if executor is available
	validationSkipped := true
	if b.executor != nil {
		b.reportStart("Validating source build in container")

		validationResult, err := b.executor.ValidateSourceBuild(ctx, r)
		if err != nil {
			b.reportFailed()
			return nil, fmt.Errorf("source build validation error: %w", err)
		}

		if validationResult.Skipped {
			b.telemetryClient.SendLLM(telemetry.NewLLMValidationResultEvent(true, "skipped", 1))
			b.reportDone("skipped")
		} else if validationResult.Passed {
			validationSkipped = false
			b.telemetryClient.SendLLM(telemetry.NewLLMValidationResultEvent(true, "", 1))
			b.reportDone("")
		} else {
			validationSkipped = false
			// Parse error for telemetry
			parsed := validate.ParseValidationError(validationResult.Stdout, validationResult.Stderr, validationResult.ExitCode)
			b.telemetryClient.SendLLM(telemetry.NewLLMValidationResultEvent(false, string(parsed.Category), 1))
			b.reportFailed()
			// Don't fail the build - source validation is informational for now
			// Future: implement repair loop for source builds
		}
	}

	result := &BuildResult{
		Recipe: r,
		Source: fmt.Sprintf("homebrew-source:%s", formula),
		Warnings: []string{
			fmt.Sprintf("LLM usage: %s", usage.String()),
			"Source build recipe - requires build tools on target system",
		},
		RepairAttempts:    repairAttempts,
		Provider:          provider.Name(),
		ValidationSkipped: validationSkipped,
		Cost:              usage.Cost(),
	}

	if repairAttempts > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Recipe repaired after %d attempt(s)", repairAttempts))
	}

	return result, nil
}

// homebrewGenContext holds context needed during recipe generation.
type homebrewGenContext struct {
	formula     string
	formulaInfo *homebrewFormulaInfo
	httpClient  *http.Client
	apiURL      string
}

// homebrewRecipeData holds the extracted recipe information from LLM.
type homebrewRecipeData struct {
	Executables   []string `json:"executables"`
	Dependencies  []string `json:"dependencies,omitempty"`
	VerifyCommand string   `json:"verify_command"`
}

// BuildSystem represents the detected build system from a formula.
type BuildSystem string

// Supported build systems
const (
	BuildSystemAutotools BuildSystem = "autotools" // ./configure && make install
	BuildSystemCMake     BuildSystem = "cmake"     // cmake && make
	BuildSystemCargo     BuildSystem = "cargo"     // cargo build
	BuildSystemGo        BuildSystem = "go"        // go build
	BuildSystemMake      BuildSystem = "make"      // make only (no configure)
	BuildSystemCustom    BuildSystem = "custom"    // custom install method
)

// platformStep represents a platform-conditional step from the LLM.
type platformStep struct {
	Action string                 `json:"action"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// sourceRecipeData holds the extracted source build recipe information from LLM.
type sourceRecipeData struct {
	BuildSystem          BuildSystem               `json:"build_system"`
	ConfigureArgs        []string                  `json:"configure_args,omitempty"`
	CMakeArgs            []string                  `json:"cmake_args,omitempty"`
	BuildDependencies    []string                  `json:"build_dependencies,omitempty"`
	Executables          []string                  `json:"executables"`
	VerifyCommand        string                    `json:"verify_command"`
	PlatformSteps        map[string][]platformStep `json:"platform_steps,omitempty"`
	PlatformDependencies map[string][]string       `json:"platform_dependencies,omitempty"`
	Resources            []sourceResourceData      `json:"resources,omitempty"`
	Patches              []sourcePatchData         `json:"patches,omitempty"`
	Inreplace            []sourceInreplaceData     `json:"inreplace,omitempty"`
}

// validPlatformKeys are the valid keys for platform conditionals.
var validPlatformKeys = map[string]bool{
	"macos":  true, // maps to os: darwin
	"linux":  true, // maps to os: linux
	"arm64":  true, // maps to arch: arm64
	"amd64":  true, // maps to arch: amd64
	"x86_64": true, // alias for amd64
}

// sourceResourceData holds resource information extracted from the formula.
type sourceResourceData struct {
	Name     string `json:"name"`     // Resource identifier (e.g., "tree-sitter-c")
	URL      string `json:"url"`      // Download URL
	Checksum string `json:"checksum"` // SHA256 checksum
	Dest     string `json:"dest"`     // Destination directory relative to source root
}

// sourcePatchData holds patch information extracted from the formula.
type sourcePatchData struct {
	URL    string `json:"url,omitempty"`    // URL to download patch (for external patches)
	Data   string `json:"data,omitempty"`   // Inline patch content (for DATA sections)
	Strip  int    `json:"strip,omitempty"`  // Strip level for patch -p (default 1)
	Subdir string `json:"subdir,omitempty"` // Subdirectory to apply patch in
}

// sourceInreplaceData holds inreplace (text replacement) information.
type sourceInreplaceData struct {
	File        string `json:"file"`            // File path relative to source root
	Pattern     string `json:"pattern"`         // Text to find
	Replacement string `json:"replacement"`     // Text to replace with
	IsRegex     bool   `json:"regex,omitempty"` // If true, pattern is a regex
}

// validBuildSystems is the set of supported build systems.
var validBuildSystems = map[BuildSystem]bool{
	BuildSystemAutotools: true,
	BuildSystemCMake:     true,
	BuildSystemCargo:     true,
	BuildSystemGo:        true,
	BuildSystemMake:      true,
	BuildSystemCustom:    true,
}

// validateSourceRecipeData validates the source recipe data from the LLM.
func validateSourceRecipeData(data *sourceRecipeData) error {
	// Validate build system
	if data.BuildSystem == "" {
		return fmt.Errorf("extract_source_recipe requires 'build_system' parameter")
	}
	if !validBuildSystems[data.BuildSystem] {
		return fmt.Errorf("extract_source_recipe: invalid build_system '%s'; must be one of: autotools, cmake, cargo, go, make, custom", data.BuildSystem)
	}

	// Validate executables
	if len(data.Executables) == 0 {
		return fmt.Errorf("extract_source_recipe requires at least one executable")
	}
	for _, exe := range data.Executables {
		if exe == "" {
			return fmt.Errorf("extract_source_recipe: executable name cannot be empty")
		}
		// Security: disallow path traversal
		if strings.Contains(exe, "..") || strings.HasPrefix(exe, "/") {
			return fmt.Errorf("extract_source_recipe: invalid executable path '%s'", exe)
		}
	}

	// Validate verify command
	if err := isValidVerifyCommand(data.VerifyCommand); err != nil {
		return fmt.Errorf("extract_source_recipe: %w", err)
	}

	// Validate configure args (no shell metacharacters)
	for _, arg := range data.ConfigureArgs {
		if !isValidConfigureArg(arg) {
			return fmt.Errorf("extract_source_recipe: invalid configure_arg '%s'", arg)
		}
	}

	// Validate cmake args (reuse existing validation)
	for _, arg := range data.CMakeArgs {
		if !isValidCMakeArg(arg) {
			return fmt.Errorf("extract_source_recipe: invalid cmake_arg '%s'", arg)
		}
	}

	// Validate platform steps
	for platform, steps := range data.PlatformSteps {
		if !validPlatformKeys[platform] {
			return fmt.Errorf("extract_source_recipe: invalid platform key '%s'; must be one of: macos, linux, arm64, amd64", platform)
		}
		for i, step := range steps {
			if step.Action == "" {
				return fmt.Errorf("extract_source_recipe: platform_steps[%s][%d] missing action", platform, i)
			}
		}
	}

	// Validate platform dependencies
	for platform := range data.PlatformDependencies {
		if !validPlatformKeys[platform] {
			return fmt.Errorf("extract_source_recipe: invalid platform_dependencies key '%s'; must be one of: macos, linux, arm64, amd64", platform)
		}
	}

	// Validate resources
	for i, res := range data.Resources {
		if err := validateSourceResource(&res, i); err != nil {
			return err
		}
	}

	// Validate patches
	for i, patch := range data.Patches {
		if err := validateSourcePatch(&patch, i); err != nil {
			return err
		}
	}

	// Validate inreplace operations
	for i, ir := range data.Inreplace {
		if err := validateSourceInreplace(&ir, i); err != nil {
			return err
		}
	}

	return nil
}

// validateSourceResource validates a single resource entry.
func validateSourceResource(res *sourceResourceData, index int) error {
	if res.Name == "" {
		return fmt.Errorf("resource[%d]: name is required", index)
	}
	if res.URL == "" {
		return fmt.Errorf("resource[%d]: url is required", index)
	}
	// Validate URL (basic sanity check - must be https)
	if !strings.HasPrefix(res.URL, "https://") {
		return fmt.Errorf("resource[%d]: url must use https", index)
	}
	if res.Dest == "" {
		return fmt.Errorf("resource[%d]: dest is required", index)
	}
	// Security: disallow path traversal
	if strings.Contains(res.Dest, "..") || strings.HasPrefix(res.Dest, "/") {
		return fmt.Errorf("resource[%d]: invalid dest path '%s'", index, res.Dest)
	}
	return nil
}

// validateSourcePatch validates a single patch entry.
func validateSourcePatch(patch *sourcePatchData, index int) error {
	// Must have either URL or Data, not both
	hasURL := patch.URL != ""
	hasData := patch.Data != ""
	if !hasURL && !hasData {
		return fmt.Errorf("patch[%d]: either url or data is required", index)
	}
	if hasURL && hasData {
		return fmt.Errorf("patch[%d]: cannot specify both url and data", index)
	}
	// Validate URL if present
	if hasURL && !strings.HasPrefix(patch.URL, "https://") {
		return fmt.Errorf("patch[%d]: url must use https", index)
	}
	// Validate subdir if present
	if patch.Subdir != "" {
		if strings.Contains(patch.Subdir, "..") || strings.HasPrefix(patch.Subdir, "/") {
			return fmt.Errorf("patch[%d]: invalid subdir path '%s'", index, patch.Subdir)
		}
	}
	// Validate strip level (must be non-negative)
	if patch.Strip < 0 {
		return fmt.Errorf("patch[%d]: strip must be non-negative", index)
	}
	return nil
}

// validateSourceInreplace validates a single inreplace entry.
func validateSourceInreplace(ir *sourceInreplaceData, index int) error {
	if ir.File == "" {
		return fmt.Errorf("inreplace[%d]: file is required", index)
	}
	// Security: disallow path traversal
	if strings.Contains(ir.File, "..") || strings.HasPrefix(ir.File, "/") {
		return fmt.Errorf("inreplace[%d]: invalid file path '%s'", index, ir.File)
	}
	if ir.Pattern == "" {
		return fmt.Errorf("inreplace[%d]: pattern is required", index)
	}
	// Replacement can be empty (for deletion)
	return nil
}

// isValidConfigureArg validates configure arguments for security.
func isValidConfigureArg(arg string) bool {
	if arg == "" || len(arg) > 500 {
		return false
	}
	// Disallow shell metacharacters
	dangerousChars := []string{";", "&&", "||", "|", "`", "$", "\n", "\r"}
	for _, c := range dangerousChars {
		if strings.Contains(arg, c) {
			return false
		}
	}
	return true
}

// isValidCMakeArg validates CMake arguments for security.
func isValidCMakeArg(arg string) bool {
	if arg == "" || len(arg) > 500 {
		return false
	}
	// Disallow shell metacharacters
	dangerousChars := []string{";", "&&", "||", "|", "`", "$", "\n", "\r"}
	for _, c := range dangerousChars {
		if strings.Contains(arg, c) {
			return false
		}
	}
	return true
}

// generateWithRepair runs the conversation loop with validation and repair.
func (b *HomebrewBuilder) generateWithRepair(
	ctx context.Context,
	provider llm.Provider,
	genCtx *homebrewGenContext,
	packageName string,
) (*homebrewRecipeData, *llm.Usage, int, bool, error) {
	// Build initial conversation
	systemPrompt := b.buildSystemPrompt()
	userMessage := b.buildUserMessage(genCtx)
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: userMessage},
	}
	tools := b.buildToolDefs()

	var totalUsage llm.Usage
	var repairAttempts int
	var validationSkipped bool
	var lastErrorCategory string

	for attempt := 0; attempt <= MaxRepairAttempts; attempt++ {
		// Emit repair attempt event for retries (not the first attempt)
		if attempt > 0 {
			b.telemetryClient.SendLLM(telemetry.NewLLMRepairAttemptEvent(provider.Name(), attempt, lastErrorCategory))
			// Report repair progress
			b.reportStart(fmt.Sprintf("Repairing recipe (attempt %d/%d)", attempt, MaxRepairAttempts+1))
		} else {
			// Report LLM analysis starting (first attempt only)
			b.reportStart(fmt.Sprintf("Analyzing formula with %s", provider.Name()))
		}

		// Run conversation loop to get recipe data
		recipeData, turnUsage, err := b.runConversationLoop(ctx, provider, systemPrompt, messages, tools, genCtx)
		if err != nil {
			b.reportFailed()
			return nil, &totalUsage, repairAttempts, validationSkipped, err
		}
		totalUsage.Add(*turnUsage)

		// Report LLM analysis done
		b.reportDone("")

		// If no executor, skip validation
		if b.executor == nil {
			validationSkipped = true
			// Emit validation skipped event
			b.telemetryClient.SendLLM(telemetry.NewLLMValidationResultEvent(true, "skipped", attempt+1))
			return recipeData, &totalUsage, repairAttempts, validationSkipped, nil
		}

		// Generate recipe for validation
		r, err := b.generateRecipe(packageName, genCtx.formulaInfo, recipeData)
		if err != nil {
			return nil, &totalUsage, repairAttempts, validationSkipped, fmt.Errorf("failed to generate recipe for validation: %w", err)
		}

		// Report validation starting
		b.reportStart("Validating in container")

		// Validate in container (homebrew_bottle action handles downloads internally)
		result, err := b.executor.Validate(ctx, r, "")
		if err != nil {
			b.reportFailed()
			return nil, &totalUsage, repairAttempts, validationSkipped, fmt.Errorf("validation error: %w", err)
		}

		// Check validation result
		if result.Skipped {
			validationSkipped = true
			// Emit validation skipped event
			b.telemetryClient.SendLLM(telemetry.NewLLMValidationResultEvent(true, "skipped", attempt+1))
			b.reportDone("skipped")
			return recipeData, &totalUsage, repairAttempts, validationSkipped, nil
		}

		if result.Passed {
			// Emit validation passed event
			b.telemetryClient.SendLLM(telemetry.NewLLMValidationResultEvent(true, "", attempt+1))
			b.reportDone("")
			return recipeData, &totalUsage, repairAttempts, validationSkipped, nil
		}

		// Validation failed
		b.reportFailed()

		// Parse error category for telemetry
		parsed := validate.ParseValidationError(result.Stdout, result.Stderr, result.ExitCode)
		lastErrorCategory = string(parsed.Category)

		// Emit validation failed event
		b.telemetryClient.SendLLM(telemetry.NewLLMValidationResultEvent(false, lastErrorCategory, attempt+1))

		// Validation failed - prepare repair if we have attempts left
		if attempt >= MaxRepairAttempts {
			return nil, &totalUsage, repairAttempts, validationSkipped, fmt.Errorf("recipe validation failed after %d repair attempts: %s", repairAttempts, b.formatValidationError(result))
		}

		// Continue conversation with error feedback
		repairAttempts++
		repairMessage := b.buildRepairMessage(result)
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: repairMessage})
	}

	return nil, &totalUsage, repairAttempts, validationSkipped, fmt.Errorf("unexpected end of repair loop")
}

// runConversationLoop executes the multi-turn conversation until extract_recipe is called.
func (b *HomebrewBuilder) runConversationLoop(
	ctx context.Context,
	provider llm.Provider,
	systemPrompt string,
	messages []llm.Message,
	tools []llm.ToolDef,
	genCtx *homebrewGenContext,
) (*homebrewRecipeData, *llm.Usage, error) {
	var totalUsage llm.Usage

	for turn := 0; turn < MaxTurns; turn++ {
		resp, err := provider.Complete(ctx, &llm.CompletionRequest{
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        tools,
			MaxTokens:    4096,
		})
		if err != nil {
			return nil, &totalUsage, err
		}

		totalUsage.Add(resp.Usage)

		// Add assistant response to conversation
		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Process tool calls
		var toolResults []llm.Message
		var recipeData *homebrewRecipeData

		for _, tc := range resp.ToolCalls {
			result, extracted, err := b.executeToolCall(ctx, genCtx, tc)
			if err != nil {
				// Return error as tool result so LLM can try again
				toolResults = append(toolResults, llm.Message{
					Role: llm.RoleUser,
					ToolResult: &llm.ToolResult{
						CallID:  tc.ID,
						Content: fmt.Sprintf("Error: %v", err),
						IsError: true,
					},
				})
				continue
			}

			if extracted != nil {
				recipeData = extracted
			} else {
				toolResults = append(toolResults, llm.Message{
					Role: llm.RoleUser,
					ToolResult: &llm.ToolResult{
						CallID:  tc.ID,
						Content: result,
						IsError: false,
					},
				})
			}
		}

		// If extract_recipe was called, return the data
		if recipeData != nil {
			return recipeData, &totalUsage, nil
		}

		// If there were tool calls, add results and continue
		if len(toolResults) > 0 {
			messages = append(messages, toolResults...)
			continue
		}

		// No tool calls and no extract_recipe - LLM is done but didn't call the tool
		if resp.StopReason == "end_turn" {
			return nil, &totalUsage, fmt.Errorf("conversation ended without extract_recipe being called")
		}
	}

	return nil, &totalUsage, fmt.Errorf("max turns (%d) exceeded without completing recipe generation", MaxTurns)
}

// executeToolCall executes a tool call and returns the result.
func (b *HomebrewBuilder) executeToolCall(ctx context.Context, genCtx *homebrewGenContext, tc llm.ToolCall) (string, *homebrewRecipeData, error) {
	switch tc.Name {
	case ToolFetchFormulaJSON:
		formula, err := getStringArg(tc.Arguments, "formula", genCtx.formula)
		if err != nil {
			return "", nil, fmt.Errorf("fetch_formula_json: %w", err)
		}
		// Validate formula name for security
		if !isValidHomebrewFormula(formula) {
			return "", nil, fmt.Errorf("invalid formula name: %s", formula)
		}
		content, err := b.fetchFormulaJSON(ctx, genCtx, formula)
		if err != nil {
			return "", nil, err
		}
		return content, nil, nil

	case ToolInspectBottle:
		formula, err := getStringArg(tc.Arguments, "formula", genCtx.formula)
		if err != nil {
			return "", nil, fmt.Errorf("inspect_bottle: %w", err)
		}
		platform, err := getStringArg(tc.Arguments, "platform", "x86_64_linux")
		if err != nil {
			return "", nil, fmt.Errorf("inspect_bottle: %w", err)
		}
		// Validate inputs
		if !isValidHomebrewFormula(formula) {
			return "", nil, fmt.Errorf("invalid formula name: %s", formula)
		}
		if !isValidPlatformTag(platform) {
			return "", nil, fmt.Errorf("invalid platform tag: %s", platform)
		}
		listing, err := b.inspectBottle(ctx, genCtx, formula, platform)
		if err != nil {
			return "", nil, err
		}
		return listing, nil, nil

	case ToolFetchFormulaRuby:
		formula, err := getStringArg(tc.Arguments, "formula", genCtx.formula)
		if err != nil {
			return "", nil, fmt.Errorf("fetch_formula_ruby: %w", err)
		}
		if !isValidHomebrewFormula(formula) {
			return "", nil, fmt.Errorf("invalid formula name: %s", formula)
		}
		content, err := b.fetchFormulaRuby(ctx, formula)
		if err != nil {
			return "", nil, err
		}
		return content, nil, nil

	case ToolExtractRecipe:
		argsJSON, err := json.Marshal(tc.Arguments)
		if err != nil {
			return "", nil, fmt.Errorf("invalid extract_recipe input: %w", err)
		}
		var recipeData homebrewRecipeData
		if err := json.Unmarshal(argsJSON, &recipeData); err != nil {
			return "", nil, fmt.Errorf("invalid extract_recipe input: %w", err)
		}
		// Validate executables
		if len(recipeData.Executables) == 0 {
			return "", nil, fmt.Errorf("extract_recipe requires at least one executable")
		}
		// Validate verify command
		if err := isValidVerifyCommand(recipeData.VerifyCommand); err != nil {
			return "", nil, fmt.Errorf("extract_recipe: %w", err)
		}
		return "", &recipeData, nil

	case ToolExtractSourceRecipe:
		argsJSON, err := json.Marshal(tc.Arguments)
		if err != nil {
			return "", nil, fmt.Errorf("invalid extract_source_recipe input: %w", err)
		}
		var srcData sourceRecipeData
		if err := json.Unmarshal(argsJSON, &srcData); err != nil {
			return "", nil, fmt.Errorf("invalid extract_source_recipe input: %w", err)
		}
		// Validate required fields
		if err := validateSourceRecipeData(&srcData); err != nil {
			return "", nil, err
		}
		// Return as JSON string - the caller will need to detect and handle this
		resultJSON, _ := json.Marshal(srcData)
		return string(resultJSON), nil, nil

	default:
		return "", nil, fmt.Errorf("unknown tool: %s", tc.Name)
	}
}

// isValidPlatformTag validates Homebrew platform tags.
func isValidPlatformTag(tag string) bool {
	validTags := map[string]bool{
		"arm64_sonoma": true,
		"sonoma":       true,
		"arm64_linux":  true,
		"x86_64_linux": true,
		// Also support older macOS versions
		"arm64_ventura":  true,
		"ventura":        true,
		"arm64_monterey": true,
		"monterey":       true,
	}
	return validTags[tag]
}

// targetPlatforms lists all platforms tsuku supports for Homebrew bottles.
var targetPlatforms = []string{
	"arm64_sonoma", // macOS ARM64
	"sonoma",       // macOS x86_64
	"x86_64_linux", // Linux x86_64
	"arm64_linux",  // Linux ARM64
}

// platformDisplayNames provides human-readable names for platform tags.
var platformDisplayNames = map[string]string{
	"arm64_sonoma": "macOS ARM64",
	"sonoma":       "macOS x86_64",
	"x86_64_linux": "Linux x86_64",
	"arm64_linux":  "Linux ARM64",
}

// BottleAvailability tracks which platforms have bottles available.
type BottleAvailability struct {
	Available   []string // Platforms with bottles
	Unavailable []string // Platforms without bottles
}

// checkBottleAvailability queries GHCR to check bottle availability for all platforms.
// It returns availability info and any platforms that are missing bottles.
func (b *HomebrewBuilder) checkBottleAvailability(ctx context.Context, formula, version string) (*BottleAvailability, error) {
	result := &BottleAvailability{
		Available:   make([]string, 0),
		Unavailable: make([]string, 0),
	}

	// Get GHCR token for the formula
	token, err := b.getGHCRToken(formula)
	if err != nil {
		return nil, fmt.Errorf("failed to get GHCR token: %w", err)
	}

	// Fetch the manifest to check available platforms
	manifest, err := b.fetchGHCRManifest(ctx, formula, version, token)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}

	// Build a set of available platform tags from manifest
	availableTags := make(map[string]bool)
	for _, entry := range manifest.Manifests {
		if refName, ok := entry.Annotations["org.opencontainers.image.ref.name"]; ok {
			// ref.name format is "{version}.{platform_tag}" e.g., "1.0.0.arm64_sonoma"
			// Version may contain dots, so find the platform tag at the end
			for _, platform := range targetPlatforms {
				if strings.HasSuffix(refName, "."+platform) {
					availableTags[platform] = true
					break
				}
			}
		}
	}

	// Check each target platform
	for _, platform := range targetPlatforms {
		if availableTags[platform] {
			result.Available = append(result.Available, platform)
		} else {
			result.Unavailable = append(result.Unavailable, platform)
		}
	}

	return result, nil
}

// ghcrManifest represents the GHCR manifest index structure.
type ghcrManifest struct {
	Manifests []ghcrManifestEntry `json:"manifests"`
}

// ghcrManifestEntry represents a single manifest entry.
type ghcrManifestEntry struct {
	Digest      string            `json:"digest"`
	Annotations map[string]string `json:"annotations"`
}

// getGHCRToken obtains an anonymous token for GHCR access.
func (b *HomebrewBuilder) getGHCRToken(formula string) (string, error) {
	tokenURL := fmt.Sprintf("https://ghcr.io/token?service=ghcr.io&scope=repository:homebrew/core/%s:pull", formula)

	resp, err := b.httpClient.Get(tokenURL)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request returned %d", resp.StatusCode)
	}

	var tokenResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.Token == "" {
		return "", fmt.Errorf("empty token in response")
	}

	return tokenResp.Token, nil
}

// fetchGHCRManifest fetches the GHCR manifest for a formula version.
func (b *HomebrewBuilder) fetchGHCRManifest(ctx context.Context, formula, version, token string) (*ghcrManifest, error) {
	manifestURL := fmt.Sprintf("https://ghcr.io/v2/homebrew/core/%s/manifests/%s", formula, version)

	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("manifest request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest request returned %d", resp.StatusCode)
	}

	var manifest ghcrManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &manifest, nil
}

// fetchFormulaJSON fetches formula JSON for the LLM.
func (b *HomebrewBuilder) fetchFormulaJSON(ctx context.Context, genCtx *homebrewGenContext, formula string) (string, error) {
	baseURL, err := url.Parse(genCtx.apiURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	apiURL := baseURL.JoinPath("api", "formula", formula+".json")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")

	resp, err := genCtx.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch formula: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", fmt.Errorf("formula '%s' not found", formula)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, maxHomebrewResponseSize)
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Sanitize the JSON before sending to LLM
	return b.sanitizeFormulaJSON(string(content)), nil
}

// sanitizeFormulaJSON removes potentially dangerous content from formula JSON.
func (b *HomebrewBuilder) sanitizeFormulaJSON(jsonStr string) string {
	// Remove any embedded control characters
	var sanitized strings.Builder
	for _, r := range jsonStr {
		if r >= 32 || r == '\n' || r == '\r' || r == '\t' {
			sanitized.WriteRune(r)
		}
	}
	return sanitized.String()
}

// fetchFormulaRuby fetches the raw Ruby formula source from GitHub.
// This allows the LLM to analyze the install method for source-based builds.
func (b *HomebrewBuilder) fetchFormulaRuby(ctx context.Context, formula string) (string, error) {
	// Homebrew formulas are organized by first letter:
	// https://raw.githubusercontent.com/Homebrew/homebrew-core/HEAD/Formula/{first_letter}/{formula}.rb
	firstLetter := strings.ToLower(string(formula[0]))
	rubyURL := fmt.Sprintf("https://raw.githubusercontent.com/Homebrew/homebrew-core/HEAD/Formula/%s/%s.rb", firstLetter, formula)

	req, err := http.NewRequestWithContext(ctx, "GET", rubyURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch Ruby formula: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", fmt.Errorf("Ruby formula '%s' not found at %s", formula, rubyURL)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d fetching Ruby formula", resp.StatusCode)
	}

	// Limit response size (Ruby formulas are typically small, but cap at 256KB)
	const maxRubyFormulaSize = 256 * 1024
	limitedReader := io.LimitReader(resp.Body, maxRubyFormulaSize)
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read Ruby formula: %w", err)
	}

	// Sanitize content (remove control characters)
	return b.sanitizeRubyFormula(string(content)), nil
}

// sanitizeRubyFormula removes control characters from Ruby formula content.
func (b *HomebrewBuilder) sanitizeRubyFormula(content string) string {
	var sanitized strings.Builder
	for _, r := range content {
		// Allow printable characters, newlines, tabs
		if r >= 32 || r == '\n' || r == '\r' || r == '\t' {
			sanitized.WriteRune(r)
		}
	}
	return sanitized.String()
}

// inspectBottle downloads and lists bottle contents.
func (b *HomebrewBuilder) inspectBottle(ctx context.Context, genCtx *homebrewGenContext, formula, platform string) (string, error) {
	// For now, return a placeholder - actual bottle inspection requires downloading
	// and extracting the bottle, which is complex and potentially slow.
	// The LLM should be able to make reasonable guesses from the formula JSON.
	return fmt.Sprintf(`Bottle inspection for %s (%s):

Note: Full bottle inspection is not implemented yet. Please analyze the formula JSON to determine:
1. The main executable name (often matches formula name, but check for aliases like ripgrep->rg, fd-find->fd)
2. Look at the formula name and description for hints about the executable
3. Common patterns: CLI tools typically install to bin/

For CLI tools, the executable is usually in bin/<name> where <name> matches the formula name or is derived from it.`, formula, platform), nil
}

// buildRepairMessage constructs the error feedback message for the LLM.
func (b *HomebrewBuilder) buildRepairMessage(result *validate.ValidationResult) string {
	// Combine stdout and stderr
	output := result.Stdout + "\n" + result.Stderr

	// Sanitize the output
	sanitizedOutput := b.sanitizer.Sanitize(output)

	// Parse the error for structured feedback
	parsed := validate.ParseValidationError(result.Stdout, result.Stderr, result.ExitCode)

	var sb strings.Builder
	sb.WriteString("The recipe you generated failed validation. Here is the error:\n\n")
	sb.WriteString("---\n")
	sb.WriteString(sanitizedOutput)
	sb.WriteString("\n---\n\n")
	sb.WriteString(fmt.Sprintf("Exit code: %d\n", result.ExitCode))
	sb.WriteString(fmt.Sprintf("Error category: %s\n", parsed.Category))

	if len(parsed.Suggestions) > 0 {
		sb.WriteString("\nSuggested fixes:\n")
		for _, suggestion := range parsed.Suggestions {
			sb.WriteString(fmt.Sprintf("- %s\n", suggestion))
		}
	}

	sb.WriteString("\nPlease analyze what went wrong and call extract_recipe again with corrected values.")

	return sb.String()
}

// formatValidationError creates a human-readable validation error message.
func (b *HomebrewBuilder) formatValidationError(result *validate.ValidationResult) string {
	output := result.Stdout + "\n" + result.Stderr
	sanitized := b.sanitizer.Sanitize(output)
	if len(sanitized) > 500 {
		sanitized = sanitized[:500] + "..."
	}
	return fmt.Sprintf("exit code %d: %s", result.ExitCode, sanitized)
}

// buildSystemPrompt creates the system prompt for Homebrew recipe generation.
func (b *HomebrewBuilder) buildSystemPrompt() string {
	return `You are an expert at analyzing Homebrew formulas to create installation recipes for tsuku, a package manager.

Your task is to analyze the provided Homebrew formula information and determine:
1. The executable binary names (often different from formula name, e.g., ripgrep installs "rg")
2. Runtime dependencies that tsuku should signal
3. A verification command to test the installation

You have three tools available:
1. fetch_formula_json: Fetch the full formula JSON metadata
2. inspect_bottle: Inspect the contents of a Homebrew bottle (limited)
3. extract_recipe: Call this when you've determined the executables and verification command

IMPORTANT: The generated recipe uses the homebrew_bottle action, which:
- Handles platform detection automatically (macOS ARM64/x86_64, Linux ARM64/x86_64)
- Downloads bottles from GHCR with SHA256 verification
- Does NOT require checksums in the recipe (they come from GHCR manifests)

Common executable naming patterns:
- ripgrep -> rg
- fd-find -> fd
- bat -> bat
- Most tools: same as formula name

When calling extract_recipe:
- executables: List of paths relative to bottle root, e.g., ["bin/rg"]
- verify_command: Command to verify installation, e.g., "rg --version"
- dependencies: Runtime dependencies (optional), e.g., ["pcre2"]

Analyze the formula and call extract_recipe with the correct information.`
}

// buildUserMessage creates the initial user message with formula context.
func (b *HomebrewBuilder) buildUserMessage(genCtx *homebrewGenContext) string {
	info := genCtx.formulaInfo

	// Build a concise summary of the formula
	var sb strings.Builder
	sb.WriteString("Please analyze this Homebrew formula and create a recipe.\n\n")
	sb.WriteString(fmt.Sprintf("Formula: %s\n", info.Name))
	sb.WriteString(fmt.Sprintf("Description: %s\n", info.Description))
	sb.WriteString(fmt.Sprintf("Homepage: %s\n", info.Homepage))
	sb.WriteString(fmt.Sprintf("Version: %s\n", info.Versions.Stable))

	if len(info.Dependencies) > 0 {
		sb.WriteString(fmt.Sprintf("Runtime Dependencies: %s\n", strings.Join(info.Dependencies, ", ")))
	}

	sb.WriteString("\nAnalyze the formula name and description to determine the executable name(s).\n")
	sb.WriteString("Then call extract_recipe with the executables, verify_command, and any runtime dependencies.")

	return sb.String()
}

// buildToolDefs creates the tool definitions for Homebrew recipe generation.
func (b *HomebrewBuilder) buildToolDefs() []llm.ToolDef {
	return b.buildBottleToolDefs()
}

// buildBottleToolDefs returns tools for bottle-based recipe generation.
func (b *HomebrewBuilder) buildBottleToolDefs() []llm.ToolDef {
	return []llm.ToolDef{
		{
			Name:        ToolFetchFormulaJSON,
			Description: "Fetch the Homebrew formula JSON metadata including version, dependencies, and bottle availability.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"formula": map[string]any{
						"type":        "string",
						"description": "Formula name (e.g., 'libyaml', 'ripgrep'). Defaults to the current formula if not specified.",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        ToolInspectBottle,
			Description: "Inspect the contents of a Homebrew bottle to discover executables. Returns a listing of files in the bottle.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"formula": map[string]any{
						"type":        "string",
						"description": "Formula name. Defaults to current formula if not specified.",
					},
					"platform": map[string]any{
						"type":        "string",
						"description": "Platform tag (arm64_sonoma, sonoma, x86_64_linux, arm64_linux). Defaults to x86_64_linux.",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        ToolExtractRecipe,
			Description: "Signal completion and output the recipe structure. Call this when you have determined the executables and verification command.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"executables": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "List of executable paths relative to bottle root (e.g., ['bin/rg'])",
					},
					"dependencies": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "List of runtime dependency formula names",
					},
					"verify_command": map[string]any{
						"type":        "string",
						"description": "Command to verify installation (e.g., 'rg --version')",
					},
				},
				"required": []string{"executables", "verify_command"},
			},
		},
	}
}

// buildSourceToolDefs returns tools for source-based recipe generation.
func (b *HomebrewBuilder) buildSourceToolDefs() []llm.ToolDef {
	return []llm.ToolDef{
		{
			Name:        ToolFetchFormulaJSON,
			Description: "Fetch the Homebrew formula JSON metadata including version, dependencies, and source URL.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"formula": map[string]any{
						"type":        "string",
						"description": "Formula name (e.g., 'jq', 'ripgrep'). Defaults to the current formula if not specified.",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        ToolFetchFormulaRuby,
			Description: "Fetch the raw Ruby formula source to analyze the install method and build system.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"formula": map[string]any{
						"type":        "string",
						"description": "Formula name. Defaults to current formula if not specified.",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        ToolExtractSourceRecipe,
			Description: "Signal completion and output the source build recipe structure. Call this when you have analyzed the build system, resources, patches, and determined the build configuration.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"build_system": map[string]any{
						"type":        "string",
						"enum":        []string{"autotools", "cmake", "cargo", "go", "make", "custom"},
						"description": "The build system used by the formula (autotools for ./configure && make, cmake for CMake, cargo for Rust, go for Go, make for plain Makefile, custom for other)",
					},
					"configure_args": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Arguments to pass to ./configure (for autotools) or cmake (as -D flags). Do not include --prefix as it's set automatically.",
					},
					"cmake_args": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "CMake arguments (e.g., ['-DBUILD_SHARED_LIBS=OFF']). Only for cmake build system.",
					},
					"build_dependencies": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Build-time dependencies that must be installed before building (e.g., ['autoconf', 'automake'])",
					},
					"executables": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Names of the executable binaries that will be produced (e.g., ['jq'])",
					},
					"verify_command": map[string]any{
						"type":        "string",
						"description": "Command to verify installation (e.g., 'jq --version')",
					},
					"platform_steps": map[string]any{
						"type":        "object",
						"description": "Platform-conditional steps. Keys are platform names (macos, linux, arm64, amd64), values are arrays of step objects.",
						"additionalProperties": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"action": map[string]any{
										"type":        "string",
										"description": "Action name (e.g., run_command, set_rpath)",
									},
									"params": map[string]any{
										"type":        "object",
										"description": "Action parameters",
									},
								},
								"required": []string{"action"},
							},
						},
					},
					"platform_dependencies": map[string]any{
						"type":        "object",
						"description": "Platform-conditional dependencies. Keys are platform names (macos, linux, arm64, amd64), values are arrays of dependency names.",
						"additionalProperties": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
					},
					"resources": map[string]any{
						"type":        "array",
						"description": "Additional downloads required before building (e.g., tree-sitter grammars for neovim)",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"name":     map[string]any{"type": "string", "description": "Unique resource identifier"},
								"url":      map[string]any{"type": "string", "description": "Download URL (must be https)"},
								"checksum": map[string]any{"type": "string", "description": "SHA256 checksum"},
								"dest":     map[string]any{"type": "string", "description": "Destination directory relative to source root"},
							},
							"required": []string{"name", "url", "dest"},
						},
					},
					"patches": map[string]any{
						"type":        "array",
						"description": "Patches to apply before building (from Homebrew formula-patches or inline DATA)",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"url":    map[string]any{"type": "string", "description": "URL to download patch (for external patches)"},
								"data":   map[string]any{"type": "string", "description": "Inline patch content (for __END__ DATA sections)"},
								"strip":  map[string]any{"type": "integer", "description": "Strip level for patch -p (default 1)"},
								"subdir": map[string]any{"type": "string", "description": "Subdirectory to apply patch in"},
							},
						},
					},
					"inreplace": map[string]any{
						"type":        "array",
						"description": "Text replacements to apply (from Homebrew inreplace calls)",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"file":        map[string]any{"type": "string", "description": "File path relative to source root"},
								"pattern":     map[string]any{"type": "string", "description": "Text to find"},
								"replacement": map[string]any{"type": "string", "description": "Text to replace with"},
								"regex":       map[string]any{"type": "boolean", "description": "If true, pattern is a regex"},
							},
							"required": []string{"file", "pattern", "replacement"},
						},
					},
				},
				"required": []string{"build_system", "executables", "verify_command"},
			},
		},
	}
}

// generateRecipe creates a recipe.Recipe from the extracted data.
func (b *HomebrewBuilder) generateRecipe(packageName string, info *homebrewFormulaInfo, data *homebrewRecipeData) (*recipe.Recipe, error) {
	if len(data.Executables) == 0 {
		return nil, fmt.Errorf("no executables specified")
	}

	// Extract just the binary names for install_binaries
	binaries := make([]string, len(data.Executables))
	copy(binaries, data.Executables)

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        packageName,
			Description: info.Description,
			Homepage:    info.Homepage,
		},
		Version: recipe.VersionSection{
			Source:  "homebrew",
			Formula: info.Name,
		},
		Verify: recipe.VerifySection{
			Command: data.VerifyCommand,
		},
	}

	// Add homebrew_bottle action
	r.Steps = []recipe.Step{
		{
			Action: "homebrew_bottle",
			Params: map[string]interface{}{
				"formula": info.Name,
			},
		},
		{
			Action: "install_binaries",
			Params: map[string]interface{}{
				"binaries": binaries,
			},
		},
	}

	// Add runtime dependencies if present
	if len(data.Dependencies) > 0 {
		r.Metadata.RuntimeDependencies = data.Dependencies
	}

	return r, nil
}

// generateSourceRecipe runs the LLM conversation loop for source-based recipe generation.
func (b *HomebrewBuilder) generateSourceRecipe(
	ctx context.Context,
	provider llm.Provider,
	genCtx *homebrewGenContext,
) (*sourceRecipeData, *llm.Usage, int, error) {
	// Build initial conversation
	systemPrompt := b.buildSourceSystemPrompt()
	userMessage := b.buildSourceUserMessage(genCtx)
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: userMessage},
	}
	tools := b.buildSourceToolDefs()

	var totalUsage llm.Usage

	// Report LLM analysis starting
	b.reportStart(fmt.Sprintf("Analyzing source formula with %s", provider.Name()))

	// Run conversation loop
	for turn := 0; turn < MaxTurns; turn++ {
		resp, err := provider.Complete(ctx, &llm.CompletionRequest{
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        tools,
			MaxTokens:    4096,
		})
		if err != nil {
			return nil, &totalUsage, 0, err
		}

		totalUsage.Add(resp.Usage)

		// Add assistant response to conversation
		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Process tool calls
		var toolResults []llm.Message
		var srcData *sourceRecipeData

		for _, tc := range resp.ToolCalls {
			result, extracted, err := b.executeToolCall(ctx, genCtx, tc)
			if err != nil {
				// Return error as tool result so LLM can try again
				toolResults = append(toolResults, llm.Message{
					Role: llm.RoleUser,
					ToolResult: &llm.ToolResult{
						CallID:  tc.ID,
						Content: fmt.Sprintf("Error: %v", err),
						IsError: true,
					},
				})
				continue
			}

			// Check if this is extract_source_recipe
			if tc.Name == ToolExtractSourceRecipe && result != "" {
				// Parse the JSON result back into sourceRecipeData
				var data sourceRecipeData
				if err := json.Unmarshal([]byte(result), &data); err != nil {
					toolResults = append(toolResults, llm.Message{
						Role: llm.RoleUser,
						ToolResult: &llm.ToolResult{
							CallID:  tc.ID,
							Content: fmt.Sprintf("Error parsing source recipe: %v", err),
							IsError: true,
						},
					})
					continue
				}
				srcData = &data
			} else if extracted != nil {
				// bottle-based extract_recipe was called - shouldn't happen in source mode
				toolResults = append(toolResults, llm.Message{
					Role: llm.RoleUser,
					ToolResult: &llm.ToolResult{
						CallID:  tc.ID,
						Content: "Error: use extract_source_recipe for source builds, not extract_recipe",
						IsError: true,
					},
				})
				continue
			} else {
				toolResults = append(toolResults, llm.Message{
					Role: llm.RoleUser,
					ToolResult: &llm.ToolResult{
						CallID:  tc.ID,
						Content: result,
						IsError: false,
					},
				})
			}
		}

		// If extract_source_recipe was called, return the data
		if srcData != nil {
			b.reportDone("")
			return srcData, &totalUsage, 0, nil
		}

		// If there were tool calls, add results and continue
		if len(toolResults) > 0 {
			messages = append(messages, toolResults...)
			continue
		}

		// No tool calls and no extract_source_recipe - LLM is done but didn't call the tool
		if resp.StopReason == "end_turn" {
			return nil, &totalUsage, 0, fmt.Errorf("conversation ended without extract_source_recipe being called")
		}
	}

	return nil, &totalUsage, 0, fmt.Errorf("max turns (%d) exceeded without completing source recipe generation", MaxTurns)
}

// buildSourceSystemPrompt creates the system prompt for source-based recipe generation.
func (b *HomebrewBuilder) buildSourceSystemPrompt() string {
	return `You are an expert at analyzing Homebrew Ruby formulas to create source build recipes for tsuku, a package manager.

This formula does NOT have pre-built bottles available, so you need to analyze the Ruby source code to understand how to build it from source.

Your task is to:
1. Fetch and analyze the Ruby formula source code
2. Identify the build system (autotools, cmake, cargo, go, make, or custom)
3. Extract any configure/cmake arguments needed
4. Identify platform-specific steps (on_macos, on_linux, on_arm, on_intel blocks)
5. Identify resources (additional downloads), patches, and inreplace operations
6. Determine the executable names that will be produced
7. Call extract_source_recipe with the complete build configuration

You have these tools available:
1. fetch_formula_json: Fetch the formula JSON metadata (version, dependencies, source URL)
2. fetch_formula_ruby: Fetch the raw Ruby formula source to analyze the install method
3. extract_source_recipe: Call this when you've determined the build system and configuration

Build system detection hints:
- autotools: Look for "system "./configure"" or "./autogen.sh" in the install method
- cmake: Look for "cmake" calls or "std_cmake_args"
- cargo: Look for "cargo" calls or Rust-related build commands
- go: Look for "go build" or Go-related commands
- make: Look for plain "make" without ./configure
- custom: Other build patterns that don't fit the above

Platform conditional detection:
Look for these Ruby blocks in the formula source:
- on_macos do ... end: Steps that only run on macOS
- on_linux do ... end: Steps that only run on Linux
- on_arm do ... end: Steps that only run on ARM64 architecture
- on_intel do ... end: Steps that only run on x86_64 architecture

These blocks may contain:
- Additional dependencies (depends_on inside the block)
- Platform-specific build commands or patches
- Post-install fixups (install_name_tool on macOS, patchelf on Linux)

RESOURCES: Look for "resource" blocks in the formula. Each resource has:
- A name (the string after "resource")
- A url (in the "url" line)
- A sha256 checksum (in the "sha256" line)
- A destination (from "stage" or path in resource.stage block)

Example Ruby:
  resource "tree-sitter-c" do
    url "https://github.com/tree-sitter/tree-sitter-c/archive/refs/tags/v0.24.1.tar.gz"
    sha256 "25dd4bb3..."
  end
  # Later in install:
  resources.each { |r| r.stage(buildpath/"deps"/r.name) }

PATCHES: Look for "patch" blocks or "__END__" DATA sections:
- URL patches: patch do ... url "https://..." ... end
- Inline patches: patch :DATA followed by __END__ section
- For URL patches, extract the URL and strip level (:p1 is default)
- For DATA patches, extract the content after __END__

INREPLACE: Look for "inreplace" calls that modify files:
- inreplace "file.txt", "old", "new"
- inreplace "CMakeLists.txt", "STATIC", "SHARED"

When calling extract_source_recipe:
- build_system: One of autotools, cmake, cargo, go, make, custom
- configure_args: Arguments for ./configure (autotools) - do NOT include --prefix
- cmake_args: CMake definition flags like -DBUILD_SHARED_LIBS=OFF
- executables: List of binary names that will be built (e.g., ["jq"])
- verify_command: Command to verify installation (e.g., "jq --version")
- build_dependencies: Any build-time dependencies (optional)
- platform_steps: Object mapping platform keys to arrays of steps, e.g.:
  {"macos": [{"action": "run_command", "params": {"command": "..."}}]}
  Valid platform keys: macos, linux, arm64, amd64
- platform_dependencies: Object mapping platform keys to dependency arrays, e.g.:
  {"linux": ["libffi", "patchelf"]}
- resources: Array of resource objects with name, url, checksum, dest
- patches: Array of patch objects with url OR data, and optional strip/subdir
- inreplace: Array of text replacement objects with file, pattern, replacement

Analyze the formula and call extract_source_recipe with the correct build configuration.`
}

// buildSourceUserMessage creates the initial user message for source-based generation.
func (b *HomebrewBuilder) buildSourceUserMessage(genCtx *homebrewGenContext) string {
	info := genCtx.formulaInfo

	var sb strings.Builder
	sb.WriteString("Please analyze this Homebrew formula and create a source build recipe.\n\n")
	sb.WriteString(fmt.Sprintf("Formula: %s\n", info.Name))
	sb.WriteString(fmt.Sprintf("Description: %s\n", info.Description))
	sb.WriteString(fmt.Sprintf("Homepage: %s\n", info.Homepage))
	sb.WriteString(fmt.Sprintf("Version: %s\n", info.Versions.Stable))
	sb.WriteString("\nThis formula does NOT have pre-built bottles, so we need to build from source.\n")
	sb.WriteString("\nPlease:\n")
	sb.WriteString("1. Fetch the Ruby formula source using fetch_formula_ruby\n")
	sb.WriteString("2. Analyze the install method to identify the build system\n")
	sb.WriteString("3. Extract any configure/cmake arguments\n")
	sb.WriteString("4. Look for resources, patches, and inreplace operations\n")
	sb.WriteString("5. Call extract_source_recipe with the complete build configuration\n")

	if len(info.BuildDependencies) > 0 {
		sb.WriteString(fmt.Sprintf("\nBuild Dependencies: %s\n", strings.Join(info.BuildDependencies, ", ")))
	}
	if len(info.Dependencies) > 0 {
		sb.WriteString(fmt.Sprintf("Runtime Dependencies: %s\n", strings.Join(info.Dependencies, ", ")))
	}

	return sb.String()
}

// generateSourceRecipeOutput creates a recipe.Recipe from the source build data.
func (b *HomebrewBuilder) generateSourceRecipeOutput(packageName string, info *homebrewFormulaInfo, data *sourceRecipeData) (*recipe.Recipe, error) {
	if len(data.Executables) == 0 {
		return nil, fmt.Errorf("no executables specified")
	}

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        packageName,
			Description: info.Description,
			Homepage:    info.Homepage,
		},
		Version: recipe.VersionSection{
			Source:  "homebrew",
			Formula: info.Name,
		},
		Verify: recipe.VerifySection{
			Command: data.VerifyCommand,
		},
	}

	// Convert source resources to recipe resources
	for _, res := range data.Resources {
		r.Resources = append(r.Resources, recipe.Resource{
			Name:     res.Name,
			URL:      res.URL,
			Checksum: res.Checksum,
			Dest:     res.Dest,
		})
	}

	// Convert source patches to recipe patches
	for _, patch := range data.Patches {
		r.Patches = append(r.Patches, recipe.Patch{
			URL:    patch.URL,
			Data:   patch.Data,
			Strip:  patch.Strip,
			Subdir: patch.Subdir,
		})
	}

	// Build the steps based on build system (includes inreplace as text_replace steps)
	steps, err := b.buildSourceSteps(data, info.Name)
	if err != nil {
		return nil, err
	}
	r.Steps = steps

	// Add dependencies from formula info (deterministic, not LLM-derived)
	// Combine build and runtime dependencies since source builds need both
	var allDeps []string
	allDeps = append(allDeps, info.BuildDependencies...)
	allDeps = append(allDeps, info.Dependencies...)
	if len(allDeps) > 0 {
		r.Metadata.Dependencies = allDeps
	}

	return r, nil
}

// buildSourceSteps generates the recipe steps for a source build.
// Resources and patches are stored in the recipe's Resources/Patches fields.
// Inreplace operations are emitted as text_replace steps before the build.
func (b *HomebrewBuilder) buildSourceSteps(data *sourceRecipeData, formula string) ([]recipe.Step, error) {
	var steps []recipe.Step

	// Use homebrew_source action which fetches URL/checksum from Homebrew API at plan time.
	// This enables version-aware source builds where the URL and checksum are resolved
	// dynamically when the recipe is installed, not when it's generated.
	steps = append(steps, recipe.Step{
		Action: "homebrew_source",
		Params: map[string]interface{}{
			"formula": formula,
		},
	})

	// Add text_replace steps for inreplace operations (before the build)
	for i, ir := range data.Inreplace {
		params := map[string]interface{}{
			"file":        ir.File,
			"pattern":     ir.Pattern,
			"replacement": ir.Replacement,
		}
		if ir.IsRegex {
			params["regex"] = true
		}
		steps = append(steps, recipe.Step{
			Action: "text_replace",
			Params: params,
		})
		_ = i // avoid unused variable warning
	}

	// Add build step based on build system
	// source_dir is "." because we use strip_dirs=1 during extraction
	switch data.BuildSystem {
	case BuildSystemAutotools:
		params := map[string]interface{}{
			"source_dir":  ".",
			"executables": data.Executables,
		}
		if len(data.ConfigureArgs) > 0 {
			params["configure_args"] = data.ConfigureArgs
		}
		steps = append(steps, recipe.Step{
			Action: "configure_make",
			Params: params,
		})

	case BuildSystemCMake:
		params := map[string]interface{}{
			"source_dir":  ".",
			"executables": data.Executables,
		}
		if len(data.CMakeArgs) > 0 {
			params["cmake_args"] = data.CMakeArgs
		}
		steps = append(steps, recipe.Step{
			Action: "cmake_build",
			Params: params,
		})

	case BuildSystemCargo:
		steps = append(steps, recipe.Step{
			Action: "cargo_build",
			Params: map[string]interface{}{
				"source_dir":  ".",
				"executables": data.Executables,
			},
		})

	case BuildSystemGo:
		steps = append(steps, recipe.Step{
			Action: "go_build",
			Params: map[string]interface{}{
				"source_dir":  ".",
				"executables": data.Executables,
			},
		})

	case BuildSystemMake:
		// Plain make without configure - use configure_make with empty configure_args
		steps = append(steps, recipe.Step{
			Action: "configure_make",
			Params: map[string]interface{}{
				"source_dir":     ".",
				"executables":    data.Executables,
				"skip_configure": true,
			},
		})

	case BuildSystemCustom:
		return nil, fmt.Errorf("custom build systems are not yet supported; formula requires manual recipe creation")

	default:
		return nil, fmt.Errorf("unknown build system: %s", data.BuildSystem)
	}

	// Add install_binaries step
	steps = append(steps, recipe.Step{
		Action: "install_binaries",
		Params: map[string]interface{}{
			"binaries": data.Executables,
		},
	})

	// Add platform-conditional steps
	for platform, platformSteps := range data.PlatformSteps {
		whenClause := platformKeyToWhen(platform)
		for _, ps := range platformSteps {
			step := recipe.Step{
				Action: ps.Action,
				When:   whenClause,
				Params: ps.Params,
			}
			steps = append(steps, step)
		}
	}

	return steps, nil
}

// platformKeyToWhen converts a platform key to a recipe.Step.When clause.
func platformKeyToWhen(platform string) map[string]string {
	switch platform {
	case "macos":
		return map[string]string{"os": "darwin"}
	case "linux":
		return map[string]string{"os": "linux"}
	case "arm64":
		return map[string]string{"arch": "arm64"}
	case "amd64", "x86_64":
		return map[string]string{"arch": "amd64"}
	default:
		return nil
	}
}
