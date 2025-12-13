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
	ToolFetchFormulaJSON = "fetch_formula_json"
	ToolInspectBottle    = "inspect_bottle"
	ToolExtractRecipe    = "extract_recipe"
)

// HomebrewBuilder generates recipes from Homebrew formulas using LLM analysis.
type HomebrewBuilder struct {
	httpClient      *http.Client
	factory         *llm.Factory
	executor        *validate.Executor
	sanitizer       *validate.Sanitizer
	homebrewAPIURL  string
	telemetryClient *telemetry.Client
	progress        ProgressReporter
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
func (b *HomebrewBuilder) Build(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	formula := req.SourceArg
	if formula == "" {
		formula = req.Package
	}

	// Validate formula name
	if !isValidHomebrewFormula(formula) {
		return nil, fmt.Errorf("invalid Homebrew formula name: %s", formula)
	}

	// Fetch formula metadata
	b.reportStart("Fetching formula metadata")
	formulaInfo, err := b.fetchFormulaInfo(ctx, formula)
	if err != nil {
		b.reportFailed()
		return nil, fmt.Errorf("failed to fetch formula: %w", err)
	}

	// Check for bottles
	if !formulaInfo.Versions.Bottle {
		b.reportFailed()
		return nil, &HomebrewNoBottlesError{Formula: formula}
	}

	b.reportDone(fmt.Sprintf("v%s", formulaInfo.Versions.Stable))

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
		formula, _ := tc.Arguments["formula"].(string)
		if formula == "" {
			formula = genCtx.formula // Default to current formula
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
		formula, _ := tc.Arguments["formula"].(string)
		if formula == "" {
			formula = genCtx.formula
		}
		platform, _ := tc.Arguments["platform"].(string)
		if platform == "" {
			platform = "x86_64_linux" // Default to Linux for inspection
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
		if recipeData.VerifyCommand == "" {
			return "", nil, fmt.Errorf("extract_recipe requires verify_command")
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
	sb.WriteString(fmt.Sprintf("Please analyze this Homebrew formula and create a recipe.\n\n"))
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
	for i, exe := range data.Executables {
		// Remove bin/ prefix if present for the binary name
		binaries[i] = exe
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
