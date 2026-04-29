package builders

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/llm"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/sandbox"
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
	ToolFetchFormulaJSON = "fetch_formula_json"
	ToolInspectBottle    = "inspect_bottle"
	ToolExtractRecipe    = "extract_recipe"
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
// It implements SessionBuilder for use with the Orchestrator.
type HomebrewBuilder struct {
	httpClient      *http.Client
	factory         *llm.Factory
	sanitizer       *validate.Sanitizer
	homebrewAPIURL  string
	telemetryClient *telemetry.Client
	progress        ProgressReporter
	registry        RegistryChecker
}

// HomebrewSession maintains state for an active Homebrew build session.
// It preserves LLM conversation history for effective repairs.
type HomebrewSession struct {
	builder *HomebrewBuilder
	req     BuildRequest

	// LLM state (provider may be nil for bottle mode until needed)
	provider     llm.Provider
	factory      *llm.Factory // For deferred LLM initialization in bottle mode
	messages     []llm.Message
	systemPrompt string
	tools        []llm.ToolDef
	totalUsage   llm.Usage

	// llmFactoryOptions are forwarded to llm.NewFactory when creating the
	// factory lazily in ensureLLMProvider. This allows the CLI layer to
	// inject a prompter (e.g., AutoApprovePrompter for --yes).
	llmFactoryOptions []llm.FactoryOption

	// Generation context
	genCtx  *homebrewGenContext
	formula string

	// Generated state
	lastRecipeData *homebrewRecipeData
	lastRecipe     *recipe.Recipe

	// Deterministic generation state
	usedDeterministic bool // True if the last recipe was generated deterministically
	deterministicOnly bool // When true, skip LLM fallback

	// Progress reporting
	progress ProgressReporter
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

// validateDependencies checks each dependency against the registry and returns
// the validated list. If the registry checker is nil, dependencies pass through
// unchanged (backward compatibility). When dependencies are missing, returns a
// DeterministicFailedError with FailureCategoryMissingDep listing all missing
// names in a format compatible with extractBlockedByFromOutput().
func (b *HomebrewBuilder) validateDependencies(formula string, deps []string) ([]string, error) {
	if b.registry == nil {
		return deps, nil
	}

	var missing []string
	for _, dep := range deps {
		if !b.registry.HasRecipe(dep) {
			missing = append(missing, dep)
		}
	}

	if len(missing) > 0 {
		var parts []string
		for _, name := range missing {
			parts = append(parts, fmt.Sprintf("recipe %s not found in registry", name))
		}
		return nil, &DeterministicFailedError{
			Formula:  formula,
			Category: FailureCategoryMissingDep,
			Message:  fmt.Sprintf("missing dependencies: %s", strings.Join(parts, "; ")),
		}
	}

	return deps, nil
}

// NewHomebrewBuilder creates a new HomebrewBuilder.
// Options can be passed to pre-configure HTTP client, API URL, etc.
// LLM factory is created during NewSession().
func NewHomebrewBuilder(opts ...HomebrewBuilderOption) *HomebrewBuilder {
	b := &HomebrewBuilder{
		homebrewAPIURL: defaultHomebrewAPIURL,
	}

	for _, opt := range opts {
		opt(b)
	}

	// Set defaults for unset options (non-LLM dependencies)
	if b.httpClient == nil {
		b.httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}

	if b.sanitizer == nil {
		b.sanitizer = validate.NewSanitizer()
	}

	if b.telemetryClient == nil {
		b.telemetryClient = telemetry.NewClient()
	}

	return b
}

// RequiresLLM returns false because the builder can generate recipes
// deterministically from bottle inspection without LLM. LLM is used
// as an optional fallback when deterministic generation fails.
func (b *HomebrewBuilder) RequiresLLM() bool {
	return false
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
	// Analytics: install counts by time period
	Analytics struct {
		Install map[string]map[string]int `json:"install"`
	} `json:"analytics"`
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

// CanBuild checks if this builder can handle the given request.
// Returns true if the formula exists in Homebrew (source build is always possible).
func (b *HomebrewBuilder) CanBuild(ctx context.Context, req BuildRequest) (bool, error) {
	// Parse SourceArg to get formula name
	sourceArg := req.SourceArg
	if sourceArg == "" {
		sourceArg = req.Package
	}
	formula, err := parseSourceArg(sourceArg)
	if err != nil {
		return false, nil
	}

	// Validate formula name
	if !isValidHomebrewFormula(formula) {
		return false, nil
	}

	// Query Homebrew API
	formulaInfo, err := b.fetchFormulaInfo(ctx, formula)
	if err != nil {
		if _, ok := err.(*HomebrewFormulaNotFoundError); ok {
			return false, nil
		}
		return false, err
	}

	// Formula exists (source build is always possible even without bottles)
	_ = formulaInfo
	return true, nil
}

// NewSession creates a new build session for the given request.
// The session fetches Homebrew metadata and prepares for generation.
// LLM is only initialized if deterministic generation fails.
func (b *HomebrewBuilder) NewSession(ctx context.Context, req BuildRequest, opts *SessionOptions) (BuildSession, error) {
	// Parse SourceArg to extract formula name
	sourceArg := req.SourceArg
	if sourceArg == "" {
		sourceArg = req.Package
	}
	formula, err := parseSourceArg(sourceArg)
	if err != nil {
		return nil, fmt.Errorf("invalid source argument: %w", err)
	}

	// Set up progress reporter
	var progress ProgressReporter
	if opts != nil && opts.ProgressReporter != nil {
		progress = opts.ProgressReporter
	} else {
		progress = b.progress
	}

	// Report metadata fetch starting
	if progress != nil {
		progress.OnStageStart("Fetching formula metadata")
	}

	// Fetch formula metadata
	formulaInfo, err := b.fetchFormulaInfo(ctx, formula)
	if err != nil {
		if progress != nil {
			progress.OnStageFailed()
		}
		return nil, fmt.Errorf("failed to fetch formula: %w", err)
	}

	// Require bottles (source builds no longer supported)
	if !formulaInfo.Versions.Bottle {
		if progress != nil {
			progress.OnStageFailed()
		}
		return nil, fmt.Errorf("formula %q has no bottles available; source builds are no longer supported", formula)
	}

	// Report metadata fetch complete
	if progress != nil {
		progress.OnStageDone(fmt.Sprintf("v%s", formulaInfo.Versions.Stable))
	}

	// Store factory for deferred LLM initialization (only used if deterministic fails)
	factory := b.factory

	// Build generation context
	genCtx := &homebrewGenContext{
		formula:     formula,
		formulaInfo: formulaInfo,
		httpClient:  b.httpClient,
		apiURL:      b.homebrewAPIURL,
	}

	// Build initial messages and tools
	systemPrompt := b.buildSystemPrompt()
	userMessage := b.buildUserMessage(genCtx)
	tools := b.buildToolDefs()

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: userMessage},
	}

	// Check if deterministic-only mode is requested
	deterministicOnly := opts != nil && opts.DeterministicOnly

	// Capture factory options for deferred LLM initialization
	var llmFactoryOptions []llm.FactoryOption
	if opts != nil {
		llmFactoryOptions = opts.LLMFactoryOptions
	}

	return &HomebrewSession{
		builder:           b,
		req:               req,
		provider:          nil, // Initialized lazily if deterministic generation fails
		factory:           factory,
		messages:          messages,
		systemPrompt:      systemPrompt,
		tools:             tools,
		llmFactoryOptions: llmFactoryOptions,
		genCtx:            genCtx,
		formula:           formula,
		deterministicOnly: deterministicOnly,
		progress:          progress,
	}, nil
}

// Generate produces an initial recipe from the build request.
// It first tries deterministic generation from bottle inspection,
// falling back to LLM if that fails.
func (s *HomebrewSession) Generate(ctx context.Context) (*BuildResult, error) {
	// Try deterministic generation first
	if s.progress != nil {
		s.progress.OnStageStart("Inspecting bottle contents")
	}

	result, err := s.generateDeterministic(ctx)
	if err == nil {
		// Deterministic generation succeeded
		if s.progress != nil {
			s.progress.OnStageDone("deterministic")
		}
		return result, nil
	}

	// Deterministic failed
	if s.deterministicOnly {
		if s.progress != nil {
			s.progress.OnStageFailed()
		}
		return nil, s.classifyDeterministicFailure(err)
	}

	// Fall back to LLM
	if s.progress != nil {
		s.progress.OnStageDone("falling back to LLM")
		s.progress.OnStageStart("Analyzing formula with LLM")
	}

	return s.generateBottle(ctx)
}

// generateDeterministic attempts to generate a recipe without LLM using bottle inspection.
func (s *HomebrewSession) generateDeterministic(ctx context.Context) (*BuildResult, error) {
	r, err := s.builder.generateDeterministicRecipe(ctx, s.req.Package, s.genCtx)
	if err != nil {
		return nil, err
	}

	s.lastRecipe = r
	s.usedDeterministic = true

	return &BuildResult{
		Recipe:   r,
		Source:   fmt.Sprintf("homebrew:%s", s.formula),
		Provider: "deterministic", // No LLM used
		Cost:     0,               // No cost for deterministic
		Warnings: []string{
			"Generated deterministically from bottle inspection",
		},
	}, nil
}

// classifyDeterministicFailure maps an internal error to a DeterministicFailedError
// with a category matching failure-record.schema.json. If the error is already a
// DeterministicFailedError (e.g., from validateDependencies), it is returned as-is.
func (s *HomebrewSession) classifyDeterministicFailure(err error) *DeterministicFailedError {
	var existing *DeterministicFailedError
	if errors.As(err, &existing) {
		return existing
	}

	msg := err.Error()

	var category DeterministicFailureCategory
	var message string

	switch {
	case strings.Contains(msg, "no bottles available") ||
		strings.Contains(msg, "no bottle found for platform"):
		category = FailureCategoryNoBottles
		message = fmt.Sprintf("formula %s has no bottles available", s.formula)

	case strings.Contains(msg, "library recipe generation failed"):
		category = FailureCategoryComplexArchive
		message = fmt.Sprintf("[library_only] formula %s detected as library but recipe generation failed", s.formula)

	case strings.Contains(msg, "no binaries found") ||
		strings.Contains(msg, "no binaries or library files found"):
		category = FailureCategoryComplexArchive
		message = fmt.Sprintf("formula %s bottle contains no binaries in bin/", s.formula)

	case strings.Contains(msg, "failed to fetch") ||
		strings.Contains(msg, "token request") ||
		strings.Contains(msg, "manifest request") ||
		strings.Contains(msg, "download request"):
		category = FailureCategoryAPIError
		message = fmt.Sprintf("failed to fetch bottle data for formula %s", s.formula)

	case strings.Contains(msg, "sandbox") || strings.Contains(msg, "validation"):
		category = FailureCategoryValidation
		message = fmt.Sprintf("generated recipe for %s failed validation", s.formula)

	default:
		category = FailureCategoryAPIError
		message = fmt.Sprintf("deterministic generation failed for formula %s", s.formula)
	}

	return &DeterministicFailedError{
		Formula:  s.formula,
		Category: category,
		Message:  message,
		Err:      err,
	}
}

// ensureLLMProvider initializes the LLM provider if not already done.
// This is called lazily when we need to fall back to LLM after deterministic fails.
func (s *HomebrewSession) ensureLLMProvider(ctx context.Context) error {
	if s.provider != nil {
		return nil // Already initialized
	}

	// Initialize factory if needed
	factory := s.factory
	if factory == nil {
		var err error
		factory, err = llm.NewFactory(ctx, s.llmFactoryOptions...)
		if err != nil {
			return fmt.Errorf("failed to create LLM factory: %w", err)
		}
		s.factory = factory
	}

	// Get provider
	provider, err := factory.GetProvider(ctx)
	if err != nil {
		return fmt.Errorf("no LLM provider available: %w", err)
	}
	s.provider = provider

	// Emit generation started event now that we're using LLM
	s.builder.telemetryClient.SendLLM(telemetry.NewLLMGenerationStartedEvent(provider.Name(), s.req.Package, "homebrew:"+s.formula))

	return nil
}

// generateBottle generates a bottle-based recipe.
func (s *HomebrewSession) generateBottle(ctx context.Context) (*BuildResult, error) {
	s.usedDeterministic = false

	// Ensure LLM provider is initialized (may have been deferred)
	if err := s.ensureLLMProvider(ctx); err != nil {
		if s.progress != nil {
			s.progress.OnStageFailed()
		}
		return nil, err
	}

	// Run conversation loop to get recipe data
	recipeData, turnUsage, err := s.builder.runConversationLoop(ctx, s.provider, s.systemPrompt, s.messages, s.tools, s.genCtx)
	if err != nil {
		if s.progress != nil {
			s.progress.OnStageFailed()
		}
		return nil, err
	}
	s.totalUsage.Add(*turnUsage)

	if s.progress != nil {
		s.progress.OnStageDone("")
	}

	// Store for potential repairs
	s.lastRecipeData = recipeData

	// Generate recipe from extracted data
	r, err := s.builder.generateRecipe(s.req.Package, s.genCtx.formulaInfo, recipeData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate recipe: %w", err)
	}

	s.lastRecipe = r

	return &BuildResult{
		Recipe:   r,
		Source:   fmt.Sprintf("homebrew:%s", s.formula),
		Provider: s.provider.Name(),
		Cost:     s.totalUsage.Cost(),
		Warnings: []string{
			fmt.Sprintf("LLM usage: %s", s.totalUsage.String()),
		},
	}, nil
}

// Repair attempts to fix the recipe given sandbox failure feedback.
func (s *HomebrewSession) Repair(ctx context.Context, failure *sandbox.SandboxResult) (*BuildResult, error) {
	if s.deterministicOnly {
		return nil, &RepairNotSupportedError{BuilderType: "homebrew-deterministic"}
	}

	// If the failed recipe was generated deterministically, use LLM to generate a new one
	if s.usedDeterministic {
		if s.progress != nil {
			s.progress.OnStageStart("Generating recipe with LLM (deterministic failed)")
		}

		// Include the failure context in the initial message to help LLM avoid same mistake
		failureContext := s.builder.buildRepairMessageFromSandbox(failure)
		s.messages = append(s.messages, llm.Message{
			Role:    llm.RoleUser,
			Content: "The deterministic generation produced a recipe that failed validation:\n\n" + failureContext + "\n\nPlease analyze the formula and generate a correct recipe.",
		})

		return s.generateBottle(ctx)
	}

	if s.progress != nil {
		s.progress.OnStageStart("Repairing recipe")
	}

	// Build repair message from failure
	repairMessage := s.builder.buildRepairMessageFromSandbox(failure)

	// Add repair message to conversation
	s.messages = append(s.messages, llm.Message{Role: llm.RoleUser, Content: repairMessage})

	return s.repairBottle(ctx)
}

// repairBottle repairs a bottle-based recipe.
func (s *HomebrewSession) repairBottle(ctx context.Context) (*BuildResult, error) {
	// Run conversation loop to get new recipe data
	recipeData, turnUsage, err := s.builder.runConversationLoop(ctx, s.provider, s.systemPrompt, s.messages, s.tools, s.genCtx)
	if err != nil {
		if s.progress != nil {
			s.progress.OnStageFailed()
		}
		return nil, err
	}
	s.totalUsage.Add(*turnUsage)

	if s.progress != nil {
		s.progress.OnStageDone("")
	}

	// Store new data
	s.lastRecipeData = recipeData

	// Generate recipe from new data
	r, err := s.builder.generateRecipe(s.req.Package, s.genCtx.formulaInfo, recipeData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate recipe: %w", err)
	}

	s.lastRecipe = r

	return &BuildResult{
		Recipe:   r,
		Source:   fmt.Sprintf("homebrew:%s", s.formula),
		Provider: s.provider.Name(),
		Cost:     s.totalUsage.Cost(),
		Warnings: []string{
			fmt.Sprintf("LLM usage: %s", s.totalUsage.String()),
		},
	}, nil
}

// Close releases resources associated with the session.
func (s *HomebrewSession) Close() error {
	// Currently no resources to release
	return nil
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
//   - "jq:source" → error (source builds no longer supported)
//   - "" → error
func parseSourceArg(sourceArg string) (formula string, err error) {
	if sourceArg == "" {
		return "", fmt.Errorf("source argument is required (use --from homebrew:formula)")
	}

	// Normalize to lowercase
	formula = strings.ToLower(sourceArg)

	// Reject source builds (no longer supported)
	if strings.HasSuffix(formula, ":source") {
		return "", fmt.Errorf("source builds are no longer supported; use primitive actions (download_file, extract, configure_make) instead")
	}

	if !isValidHomebrewFormula(formula) {
		return "", fmt.Errorf("invalid Homebrew formula name: %s", formula)
	}

	return formula, nil
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

// Probe checks if a formula exists on Homebrew and returns quality metadata.
// This is independent of RequiresLLM — probing is a deterministic registry
// lookup, while LLM is only needed for recipe generation.
func (b *HomebrewBuilder) Probe(ctx context.Context, name string) (*ProbeResult, error) {
	info, err := b.fetchFormulaInfo(ctx, name)
	if err != nil {
		return nil, nil
	}
	result := &ProbeResult{
		Source:        name,
		HasRepository: info.Homepage != "",
	}
	// Extract 365-day install count from analytics. The analytics map is
	// keyed by time period ("30d", "90d", "365d"), then by formula name.
	if period, ok := info.Analytics.Install["365d"]; ok {
		if count, ok := period[name]; ok {
			result.Downloads = count
		}
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
		for i, exe := range recipeData.Executables {
			if exe == "" {
				return "", nil, fmt.Errorf("extract_recipe: executable[%d] cannot be empty", i)
			}
			// Security: disallow path traversal
			if strings.Contains(exe, "..") || strings.HasPrefix(exe, "/") {
				return "", nil, fmt.Errorf("extract_recipe: invalid executable path '%s'", exe)
			}
		}
		// Validate verify command
		if err := isValidVerifyCommand(recipeData.VerifyCommand); err != nil {
			return "", nil, fmt.Errorf("extract_recipe: %w", err)
		}
		return "", &recipeData, nil

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

	// Build a set of available platform tags from manifest. ref.name
	// format is "<version>.<platform>" or, for revisioned formulas,
	// "<version>_<revision>.<platform>". The platform tag lives at
	// the end after the last `.`, so a HasSuffix check on
	// "."+platform handles both forms.
	availableTags := make(map[string]bool)
	for _, entry := range manifest.Manifests {
		if refName, ok := entry.Annotations["org.opencontainers.image.ref.name"]; ok {
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
	tokenURL := fmt.Sprintf("https://ghcr.io/token?service=ghcr.io&scope=repository:homebrew/core/%s:pull", url.PathEscape(formula))

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
	// Limit response size to prevent DoS from malicious/misconfigured servers
	limitedReader := io.LimitReader(resp.Body, 64*1024) // 64KB max for token response
	if err := json.NewDecoder(limitedReader).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.Token == "" {
		return "", fmt.Errorf("empty token in response")
	}

	return tokenResp.Token, nil
}

// fetchGHCRManifest fetches the GHCR manifest for a formula version.
func (b *HomebrewBuilder) fetchGHCRManifest(ctx context.Context, formula, version, token string) (*ghcrManifest, error) {
	manifestURL := fmt.Sprintf("https://ghcr.io/v2/homebrew/core/%s/manifests/%s", url.PathEscape(formula), url.PathEscape(version))

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
	// Limit response size to prevent DoS from malicious/misconfigured servers
	limitedReader := io.LimitReader(resp.Body, 10*1024*1024) // 10MB max for manifest
	if err := json.NewDecoder(limitedReader).Decode(&manifest); err != nil {
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

// inspectBottle downloads and lists bottle contents.
func (b *HomebrewBuilder) inspectBottle(ctx context.Context, genCtx *homebrewGenContext, formula, platform string) (string, error) {
	// Check if we have formula info with version
	if genCtx.formulaInfo == nil || genCtx.formulaInfo.Versions.Stable == "" {
		// Fall back to placeholder if no version info (e.g., in tests)
		return fmt.Sprintf(`Bottle inspection for %s (%s):

Note: Full bottle inspection requires version information. Please analyze the formula JSON to determine:
1. The main executable name (often matches formula name, but check for aliases like ripgrep->rg, fd-find->fd)
2. Look at the formula name and description for hints about the executable
3. Common patterns: CLI tools typically install to bin/

For CLI tools, the executable is usually in bin/<name> where <name> matches the formula name or is derived from it.`, formula, platform), nil
	}

	version := genCtx.formulaInfo.Versions.Stable

	// Download and inspect the bottle
	binaries, err := b.listBottleBinaries(ctx, formula, version, platform)
	if err != nil {
		// Fall back to placeholder if inspection fails
		return fmt.Sprintf(`Bottle inspection for %s (%s) failed: %v

Please analyze the formula JSON to determine the main executable name.
Common patterns: CLI tools typically install to bin/<formula-name>.`, formula, platform, err), nil
	}

	if len(binaries) == 0 {
		return fmt.Sprintf(`Bottle inspection for %s (%s):

No binaries found in bin/ directory. Please analyze the formula JSON to determine the main executable name.`, formula, platform), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Bottle inspection for %s (%s):\n\n", formula, platform))
	sb.WriteString("Binaries found in bin/ directory:\n")
	for _, bin := range binaries {
		sb.WriteString(fmt.Sprintf("  - %s\n", bin))
	}
	return sb.String(), nil
}

// listBottleBinaries downloads a bottle and returns the list of binaries in bin/.
func (b *HomebrewBuilder) listBottleBinaries(ctx context.Context, formula, version, platformTag string) ([]string, error) {
	// Get anonymous GHCR token
	token, err := b.getGHCRToken(formula)
	if err != nil {
		return nil, fmt.Errorf("failed to get GHCR token: %w", err)
	}

	// Get manifest and find blob SHA for the platform
	manifest, err := b.fetchGHCRManifest(ctx, formula, version, token)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch GHCR manifest: %w", err)
	}

	blobSHA, err := b.getBlobSHAFromManifest(manifest, version, platformTag)
	if err != nil {
		return nil, err
	}

	// Download bottle to temp file
	tempFile, err := os.CreateTemp("", fmt.Sprintf("tsuku-bottle-%s-*.tar.gz", formula))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)

	if err := b.downloadBottleBlob(ctx, formula, blobSHA, token, tempPath); err != nil {
		return nil, fmt.Errorf("failed to download bottle: %w", err)
	}

	// Extract bottle contents and return only binaries for backward compatibility
	contents, err := b.extractBottleContents(tempPath)
	if err != nil {
		return nil, err
	}
	return contents.Binaries, nil
}

// getBlobSHAFromManifest extracts the blob SHA for a platform from a manifest.
//
// Accepts both unrevised (<version>.<platform>) and revision-suffixed
// (<version>_<N>.<platform>) ref-name forms. When multiple revisions
// match, the highest is preferred. See selectBuilderBottleEntry.
func (b *HomebrewBuilder) getBlobSHAFromManifest(manifest *ghcrManifest, version, platformTag string) (string, error) {
	matched := selectBuilderBottleEntry(manifest.Manifests, version, platformTag)
	if matched != nil {
		if digest, ok := matched.Annotations["sh.brew.bottle.digest"]; ok {
			if strings.HasPrefix(digest, "sha256:") {
				return strings.TrimPrefix(digest, "sha256:"), nil
			}
			return digest, nil
		}
		if strings.HasPrefix(matched.Digest, "sha256:") {
			return strings.TrimPrefix(matched.Digest, "sha256:"), nil
		}
		return matched.Digest, nil
	}

	return "", fmt.Errorf("no bottle found for platform tag: %s (expected ref: %s.%s or %s_<revision>.%s)",
		platformTag, version, platformTag, version, platformTag)
}

// selectBuilderBottleEntry scans manifest entries for the one matching
// the requested platform. Accepts both unrevised (<version>.<platform>)
// and revision-suffixed (<version>_<N>.<platform>) ref-name forms.
// When multiple revisions match, returns the entry with the highest
// revision. Mirrors the action-side helper but operates on this
// package's manifest entry type.
func selectBuilderBottleEntry(entries []ghcrManifestEntry, version, platformTag string) *ghcrManifestEntry {
	suffix := "." + platformTag
	exactRefName := version + suffix
	revisionPrefix := version + "_"

	var (
		best       *ghcrManifestEntry
		bestRevSet bool
		bestRev    int
	)
	for i := range entries {
		e := &entries[i]
		refName, ok := e.Annotations["org.opencontainers.image.ref.name"]
		if !ok {
			continue
		}
		if refName == exactRefName {
			if !bestRevSet || bestRev < 0 {
				best = e
				bestRev = 0
				bestRevSet = true
			}
			continue
		}
		if !strings.HasPrefix(refName, revisionPrefix) || !strings.HasSuffix(refName, suffix) {
			continue
		}
		mid := refName[len(revisionPrefix) : len(refName)-len(suffix)]
		if mid == "" {
			continue
		}
		rev, err := strconv.Atoi(mid)
		if err != nil || rev < 0 {
			continue
		}
		if !bestRevSet || rev > bestRev {
			best = e
			bestRev = rev
			bestRevSet = true
		}
	}
	return best
}

// downloadBottleBlob downloads a bottle blob from GHCR to a local file.
func (b *HomebrewBuilder) downloadBottleBlob(ctx context.Context, formula, blobSHA, token, destPath string) error {
	blobURL := fmt.Sprintf("https://ghcr.io/v2/homebrew/core/%s/blobs/sha256:%s", formula, blobSHA)

	req, err := http.NewRequestWithContext(ctx, "GET", blobURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download request returned %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Also compute SHA256 while downloading
	hasher := sha256.New()
	writer := io.MultiWriter(out, hasher)

	_, err = io.Copy(writer, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Verify SHA256
	actualSHA := hex.EncodeToString(hasher.Sum(nil))
	if actualSHA != blobSHA {
		return fmt.Errorf("SHA256 mismatch: expected %s, got %s", blobSHA, actualSHA)
	}

	return nil
}

// bottleContents holds the files found in a Homebrew bottle tarball, organized
// by directory: bin/ for executables, lib/ for shared/static libraries and
// pkgconfig files, and include/ for header files.
type bottleContents struct {
	Binaries []string // Files in bin/ (e.g., ["jq"]) -- bare names without prefix
	LibFiles []string // Files in lib/ (e.g., ["lib/libgc.so", "lib/pkgconfig/gc.pc"]) -- with lib/ prefix
	Includes []string // Files in include/ (e.g., ["include/gc.h"]) -- with include/ prefix
}

// versionedSoPattern matches versioned shared object names like libgc.so.1.5.0.
// The pattern requires ".so." followed by at least one digit to avoid false
// positives from paths that happen to contain ".so." as a substring.
var versionedSoPattern = regexp.MustCompile(`\.so\.\d`)

// matchesVersionedSo returns true if name looks like a versioned shared object
// (e.g., libgc.so.1.5.0, libreadline.so.8.2).
func matchesVersionedSo(name string) bool {
	return versionedSoPattern.MatchString(name)
}

// isLibraryFile returns true if name has a library file extension: .a, .dylib,
// .pc, .so, or a versioned .so (e.g., .so.1.2.3).
func isLibraryFile(name string) bool {
	switch {
	case strings.HasSuffix(name, ".a"):
		return true
	case strings.HasSuffix(name, ".dylib"):
		return true
	case strings.HasSuffix(name, ".pc"):
		return true
	case strings.HasSuffix(name, ".so"):
		return true
	case matchesVersionedSo(name):
		return true
	default:
		return false
	}
}

// extractBottleContents extracts a bottle tarball and returns all files found
// in bin/, lib/, and include/ directories. Regular files and symlinks are
// collected; directory entries are skipped. Files in lib/ are filtered by
// isLibraryFile to exclude non-library files (e.g., .py, .rb). Files in
// include/ are collected without extension filtering.
func (b *HomebrewBuilder) extractBottleContents(tarballPath string) (*bottleContents, error) {
	f, err := os.Open(tarballPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	contents := &bottleContents{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tarball: %w", err)
		}

		// Skip directory entries -- only collect regular files and symlinks.
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeSymlink {
			continue
		}

		// Homebrew bottles have structure: formula/version/{bin,lib,include}/...
		parts := strings.Split(header.Name, "/")
		if len(parts) < 4 {
			continue
		}

		dirName := parts[2]
		switch dirName {
		case "bin":
			// Binaries: only direct children (formula/ver/bin/name), stored as bare names.
			if len(parts) == 4 && parts[3] != "" {
				contents.Binaries = append(contents.Binaries, parts[3])
			}
		case "lib":
			// Library files: formula/ver/lib/... at any depth (including pkgconfig/).
			// Reconstruct the relative path with lib/ prefix and filter by extension.
			relPath := strings.Join(parts[2:], "/")
			fileName := parts[len(parts)-1]
			if fileName != "" && isLibraryFile(fileName) {
				contents.LibFiles = append(contents.LibFiles, relPath)
			}
		case "include":
			// Header files: formula/ver/include/... at any depth.
			// Reconstruct the relative path with include/ prefix, no extension filter.
			relPath := strings.Join(parts[2:], "/")
			lastPart := parts[len(parts)-1]
			if lastPart != "" {
				contents.Includes = append(contents.Includes, relPath)
			}
		}
	}

	return contents, nil
}

// platformContents pairs a platform with its scanned bottle contents. Used by
// generateLibraryRecipe to produce platform-conditional steps.
type platformContents struct {
	OS       string          // "linux" or "darwin"
	Libc     string          // "glibc" or "" (macOS has no libc distinction)
	Contents *bottleContents // Scanned files from the platform's bottle
}

// inspectBottleContents downloads a bottle from GHCR and returns its full
// contents. Unlike listBottleBinaries, this returns all file categories
// (binaries, library files, and headers) so callers can decide what recipe
// type to generate.
func (b *HomebrewBuilder) inspectBottleContents(ctx context.Context, formula, version, platformTag string) (*bottleContents, error) {
	token, err := b.getGHCRToken(formula)
	if err != nil {
		return nil, fmt.Errorf("failed to get GHCR token: %w", err)
	}

	manifest, err := b.fetchGHCRManifest(ctx, formula, version, token)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch GHCR manifest: %w", err)
	}

	blobSHA, err := b.getBlobSHAFromManifest(manifest, version, platformTag)
	if err != nil {
		return nil, err
	}

	tempFile, err := os.CreateTemp("", fmt.Sprintf("tsuku-bottle-%s-*.tar.gz", formula))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)

	if err := b.downloadBottleBlob(ctx, formula, blobSHA, token, tempPath); err != nil {
		return nil, fmt.Errorf("failed to download bottle: %w", err)
	}

	return b.extractBottleContents(tempPath)
}

// getCurrentPlatformTag returns the platform tag for the current runtime.
func getCurrentPlatformTag() (string, error) {
	os := runtime.GOOS
	arch := runtime.GOARCH
	switch {
	case os == "darwin" && arch == "arm64":
		return "arm64_sonoma", nil
	case os == "darwin" && arch == "amd64":
		return "sonoma", nil
	case os == "linux" && arch == "arm64":
		return "arm64_linux", nil
	case os == "linux" && arch == "amd64":
		return "x86_64_linux", nil
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", os, arch)
	}
}

// platformTagToOSLibc maps a Homebrew platform tag to OS and libc values for
// use in recipe when clauses. Tags like "x86_64_linux" and "arm64_linux" map to
// linux/glibc. Tags like "arm64_sonoma" and "sonoma" map to darwin with no libc.
func platformTagToOSLibc(tag string) (os string, libc string) {
	if strings.Contains(tag, "linux") {
		return "linux", "glibc"
	}
	// All non-linux Homebrew tags are macOS (sonoma, ventura, etc.)
	return "darwin", ""
}

// buildRepairMessageFromSandbox constructs error feedback from sandbox results.
// Used by HomebrewSession.Repair() with sandbox.SandboxResult.
func (b *HomebrewBuilder) buildRepairMessageFromSandbox(result *sandbox.SandboxResult) string {
	// Combine stdout and stderr
	output := result.Stdout + "\n" + result.Stderr

	// Sanitize the output
	sanitizedOutput := b.sanitizer.Sanitize(output)

	// Parse the error for structured feedback
	parsed := validate.ParseValidationError(result.Stdout, result.Stderr, result.ExitCode)

	var sb strings.Builder
	sb.WriteString("The recipe you generated failed sandbox validation. Here is the error:\n\n")
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

	if result.Error != nil {
		sb.WriteString(fmt.Sprintf("\nSandbox error: %v\n", result.Error))
	}

	sb.WriteString("\nPlease analyze what went wrong and call extract_recipe again with corrected values.")

	return sb.String()
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

IMPORTANT: The generated recipe uses the homebrew action, which:
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
		Version: recipe.VersionSection{},
		Verify: &recipe.VerifySection{
			Command: data.VerifyCommand,
		},
	}

	// Add homebrew action
	r.Steps = []recipe.Step{
		{
			Action: "homebrew",
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

	// Add runtime dependencies if present, validating against registry
	if len(data.Dependencies) > 0 {
		deps, err := b.validateDependencies(packageName, data.Dependencies)
		if err != nil {
			return nil, err
		}
		r.Metadata.RuntimeDependencies = deps
	}

	return r, nil
}

// scanMultiplePlatforms downloads bottles for Linux and macOS from GHCR and
// scans each independently. The current platform's already-scanned contents are
// included without re-downloading. Returns a slice ordered Linux-first, then macOS,
// matching existing multi-platform library recipe conventions.
func (b *HomebrewBuilder) scanMultiplePlatforms(
	ctx context.Context,
	info *homebrewFormulaInfo,
	currentContents *bottleContents,
) []platformContents {
	currentTag, _ := getCurrentPlatformTag()
	currentOS, currentLibc := platformTagToOSLibc(currentTag)

	// Target platforms: Linux glibc and macOS. Order matters: Linux first.
	type targetPlatform struct {
		tag  string
		os   string
		libc string
	}
	targets := []targetPlatform{
		{"x86_64_linux", "linux", "glibc"},
		{"arm64_sonoma", "darwin", ""},
	}

	var result []platformContents

	for _, target := range targets {
		// If this target matches the current platform, use already-scanned contents.
		if target.os == currentOS && target.libc == currentLibc {
			result = append(result, platformContents{
				OS:       target.os,
				Libc:     target.libc,
				Contents: currentContents,
			})
			continue
		}

		// Download and scan the other platform's bottle.
		contents, err := b.inspectBottleContents(ctx, info.Name, info.Versions.Stable, target.tag)
		if err != nil {
			// Platform bottle not available -- skip with no fatal error.
			// Callers get a recipe with only the available platform's steps.
			continue
		}

		result = append(result, platformContents{
			OS:       target.os,
			Libc:     target.libc,
			Contents: contents,
		})
	}

	return result
}

// generateLibraryRecipe produces a type = "library" recipe from scanned bottle
// contents. For a single-platform input (len(platforms) == 1), the steps have no
// when clauses. For multi-platform inputs, each step pair gets a when clause
// matching the platform's OS and libc.
func (b *HomebrewBuilder) generateLibraryRecipe(
	ctx context.Context,
	packageName string,
	genCtx *homebrewGenContext,
	platforms []platformContents,
) (*recipe.Recipe, error) {
	if len(platforms) == 0 {
		return nil, fmt.Errorf("no platform contents provided")
	}

	info := genCtx.formulaInfo

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        packageName,
			Description: info.Description,
			Homepage:    info.Homepage,
			Type:        recipe.RecipeTypeLibrary,
		},
		Version: recipe.VersionSection{
			Source:  "homebrew",
			Formula: info.Name,
		},
		// Verify is nil for library recipes -- libraries can't be executed
	}

	// Add runtime dependencies from formula info, validating against registry
	if len(info.Dependencies) > 0 {
		deps, err := b.validateDependencies(packageName, info.Dependencies)
		if err != nil {
			return nil, err
		}
		r.Metadata.RuntimeDependencies = deps
	}

	// Build steps for each platform
	for _, plat := range platforms {
		// Combine LibFiles and Includes into outputs list
		outputs := make([]string, 0, len(plat.Contents.LibFiles)+len(plat.Contents.Includes))
		outputs = append(outputs, plat.Contents.LibFiles...)
		outputs = append(outputs, plat.Contents.Includes...)

		homebrewStep := recipe.Step{
			Action: "homebrew",
			Params: map[string]interface{}{
				"formula": info.Name,
			},
		}

		installStep := recipe.Step{
			Action: "install_binaries",
			Params: map[string]interface{}{
				"install_mode": "directory",
				"outputs":      outputs,
			},
		}

		// For multi-platform (len > 1), add when clauses. For single platform,
		// omit when clauses to match the simpler recipe format.
		if len(platforms) > 1 {
			when := &recipe.WhenClause{
				OS: []string{plat.OS},
			}
			if plat.Libc != "" {
				when.Libc = []string{plat.Libc}
			}
			homebrewStep.When = when
			installStep.When = when
		}

		r.Steps = append(r.Steps, homebrewStep, installStep)
	}

	return r, nil
}

// generateToolRecipe creates a tool recipe from binary inspection results.
// This is the existing tool recipe path, extracted from generateDeterministicRecipe
// for clarity after the library recipe path was added.
func (b *HomebrewBuilder) generateToolRecipe(packageName string, genCtx *homebrewGenContext, binaries []string) (*recipe.Recipe, error) {
	info := genCtx.formulaInfo

	verifyBinary := binaries[0]
	verifyCommand := fmt.Sprintf("%s --version", verifyBinary)

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        packageName,
			Description: info.Description,
			Homepage:    info.Homepage,
		},
		Version: recipe.VersionSection{},
		Verify: &recipe.VerifySection{
			Command: verifyCommand,
		},
	}

	// Prefix binaries with bin/ to match the extracted bottle structure.
	// Homebrew bottles contain formula/version/bin/<name>, and after extract
	// with strip_dirs:2, binaries end up at bin/<name> in the work directory.
	binPaths := make([]string, len(binaries))
	for i, b := range binaries {
		binPaths[i] = "bin/" + b
	}

	r.Steps = []recipe.Step{
		{
			Action: "homebrew",
			Params: map[string]interface{}{
				"formula": info.Name,
			},
		},
		{
			Action: "install_binaries",
			Params: map[string]interface{}{
				"binaries": binPaths,
			},
		},
	}

	if len(info.Dependencies) > 0 {
		deps, err := b.validateDependencies(packageName, info.Dependencies)
		if err != nil {
			return nil, err
		}
		r.Metadata.RuntimeDependencies = deps
	}

	return r, nil
}

// generateDeterministicRecipe attempts to generate a recipe without LLM by inspecting the bottle.
// Returns the recipe and nil error on success, or nil and an error if deterministic generation fails.
// For bottles with binaries, it produces a tool recipe. For library-only bottles (lib/ files but
// no binaries), it produces a library recipe. If neither binaries nor library files are found,
// it returns an error.
func (b *HomebrewBuilder) generateDeterministicRecipe(ctx context.Context, packageName string, genCtx *homebrewGenContext) (*recipe.Recipe, error) {
	info := genCtx.formulaInfo
	if info == nil {
		return nil, fmt.Errorf("formula info not available")
	}
	if info.Versions.Stable == "" {
		return nil, fmt.Errorf("no stable version for formula")
	}

	// Get the current platform tag
	platformTag, err := getCurrentPlatformTag()
	if err != nil {
		return nil, fmt.Errorf("failed to get platform tag: %w", err)
	}

	// Inspect the bottle to get full contents (binaries, libs, headers)
	contents, err := b.inspectBottleContents(ctx, info.Name, info.Versions.Stable, platformTag)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect bottle: %w", err)
	}

	// Branch 1: Tool recipe (has binaries)
	if len(contents.Binaries) > 0 {
		return b.generateToolRecipe(packageName, genCtx, contents.Binaries)
	}

	// Branch 2: Library recipe (no binaries, but has lib files)
	if len(contents.LibFiles) > 0 {
		// Scan multiple platforms (Linux + macOS) for multi-platform recipe
		platforms := b.scanMultiplePlatforms(ctx, info, contents)
		recipe, err := b.generateLibraryRecipe(ctx, packageName, genCtx, platforms)
		if err != nil {
			return nil, fmt.Errorf("library recipe generation failed: %w", err)
		}
		return recipe, nil
	}

	// Branch 3: Neither binaries nor library files
	return nil, fmt.Errorf("no binaries or library files found in bottle")
}
