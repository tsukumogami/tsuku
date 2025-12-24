package recipe

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// ValidationError represents a single validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e ValidationError) String() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

// ValidationWarning represents a non-fatal validation warning
type ValidationWarning struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (w ValidationWarning) String() string {
	if w.Field != "" {
		return fmt.Sprintf("%s: %s", w.Field, w.Message)
	}
	return w.Message
}

// ValidationResult contains the results of recipe validation
type ValidationResult struct {
	Valid    bool                `json:"valid"`
	Recipe   *Recipe             `json:"recipe,omitempty"`
	Errors   []ValidationError   `json:"errors,omitempty"`
	Warnings []ValidationWarning `json:"warnings,omitempty"`
}

// addError adds an error to the result
func (r *ValidationResult) addError(field, message string) {
	r.Errors = append(r.Errors, ValidationError{Field: field, Message: message})
	r.Valid = false
}

// addWarning adds a warning to the result
func (r *ValidationResult) addWarning(field, message string) {
	r.Warnings = append(r.Warnings, ValidationWarning{Field: field, Message: message})
}

// ValidateFile validates a recipe file at the given path
func ValidateFile(path string) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		result.addError("", fmt.Sprintf("failed to read file: %v", err))
		return result
	}

	return ValidateBytes(data)
}

// ValidateBytes validates recipe data from bytes
func ValidateBytes(data []byte) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// Parse TOML
	var recipe Recipe
	if err := toml.Unmarshal(data, &recipe); err != nil {
		result.addError("", fmt.Sprintf("TOML parse error: %v", err))
		return result
	}

	result.Recipe = &recipe

	// Run all validations
	validateMetadata(result, &recipe)
	validateVersion(result, &recipe)
	validatePatches(result, &recipe)
	validateSteps(result, &recipe)
	validateVerify(result, &recipe)

	return result
}

// validateMetadata checks the metadata section
func validateMetadata(result *ValidationResult, r *Recipe) {
	if r.Metadata.Name == "" {
		result.addError("metadata.name", "name is required")
	} else {
		// Check name format (kebab-case)
		if strings.Contains(r.Metadata.Name, " ") {
			result.addError("metadata.name", "name should not contain spaces (use kebab-case)")
		}
		if strings.ToLower(r.Metadata.Name) != r.Metadata.Name {
			result.addWarning("metadata.name", "name should be lowercase (kebab-case)")
		}
	}

	if r.Metadata.Description == "" {
		result.addWarning("metadata.description", "description is recommended")
	}

	// Validate type field
	validTypes := map[string]bool{
		RecipeTypeTool:    true,
		RecipeTypeLibrary: true,
		"":                true, // Empty defaults to "tool"
	}
	if !validTypes[r.Metadata.Type] {
		result.addError("metadata.type", fmt.Sprintf("invalid type '%s' (valid values: tool, library)", r.Metadata.Type))
	}
}

// validateVersion checks the version section
func validateVersion(result *ValidationResult, r *Recipe) {
	// Version source validation
	validSources := map[string]bool{
		"github_releases": true,
		"github_tags":     true,
		"nodejs_dist":     true,
		"npm":             true,
		"pypi":            true,
		"crates_io":       true,
		"rubygems":        true,
		"homebrew":        true,
		"hashicorp":       true,
		"manual":          true,
		"go_toolchain":    true,
		"goproxy":         true,
		"metacpan":        true,
		"nixpkgs":         true,
		"":                true, // Empty is allowed (can be inferred from actions)
	}

	// Handle sources with parameters like "goproxy:module/path"
	source := r.Version.Source
	if idx := strings.Index(source, ":"); idx != -1 {
		source = source[:idx]
	}

	if !validSources[source] {
		result.addWarning("version.source", fmt.Sprintf("unknown version source '%s'", r.Version.Source))
	}

	// Check if version source can be inferred from install actions
	// This matches the inference logic in internal/version/provider_factory.go
	canInferVersionSource := canInferVersionFromActions(r)

	// If using github sources, check github_repo (but only if not inferable from actions)
	if (source == "github_releases" || source == "github_tags" || source == "") && r.Version.GitHubRepo == "" {
		// Check if any step has repo parameter that could provide this
		hasRepoInStep := false
		for _, step := range r.Steps {
			if _, ok := step.Params["repo"]; ok {
				hasRepoInStep = true
				break
			}
		}
		// Only warn if we can't infer the version source from install actions
		if !hasRepoInStep && !canInferVersionSource {
			result.addWarning("version.github_repo", "github_repo is recommended when using github version source")
		}
	}
}

// canInferVersionFromActions checks if version source can be inferred from install actions.
// This mirrors the inference logic in internal/version/provider_factory.go (Inferred*Strategy).
func canInferVersionFromActions(r *Recipe) bool {
	for _, step := range r.Steps {
		switch step.Action {
		case "npm_install":
			if _, ok := step.Params["package"].(string); ok {
				return true // InferredNpmStrategy
			}
		case "pipx_install":
			if _, ok := step.Params["package"].(string); ok {
				return true // InferredPyPIStrategy
			}
		case "cargo_install":
			if _, ok := step.Params["crate"].(string); ok {
				return true // InferredCratesIOStrategy
			}
		case "gem_install":
			if _, ok := step.Params["gem"].(string); ok {
				return true // InferredRubyGemsStrategy
			}
		case "cpan_install":
			if _, ok := step.Params["distribution"].(string); ok {
				return true // InferredMetaCPANStrategy
			}
		case "go_install":
			if _, ok := step.Params["module"].(string); ok {
				return true // Requires explicit goproxy source, but module provides context
			}
		case "github_archive", "github_file":
			if _, ok := step.Params["repo"].(string); ok {
				return true // InferredGitHubStrategy
			}
		}
	}
	return false
}

// validatePatches checks patch configuration
func validatePatches(result *ValidationResult, r *Recipe) {
	for i, patch := range r.Patches {
		patchField := fmt.Sprintf("patches[%d]", i)

		// Check mutual exclusivity of url and data
		if patch.URL != "" && patch.Data != "" {
			result.addError(patchField, "cannot specify both 'url' and 'data' (must be mutually exclusive)")
			continue
		}
		if patch.URL == "" && patch.Data == "" {
			result.addError(patchField, "must specify either 'url' or 'data'")
			continue
		}

		// URL-based patches require checksum for integrity verification
		if patch.URL != "" {
			if patch.Checksum == "" {
				result.addError(patchField+".checksum", "checksum is required for url-based patches")
			} else {
				// Validate checksum format (SHA256 is 64 hex characters)
				if len(patch.Checksum) != 64 {
					result.addError(patchField+".checksum", "checksum must be 64 characters (SHA256 hex)")
				} else {
					// Check if all characters are hex
					for _, c := range patch.Checksum {
						if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
							result.addError(patchField+".checksum", "checksum must be hexadecimal (0-9, a-f)")
							break
						}
					}
				}
			}
		}

		// Inline patches (data field) don't require checksums since they're embedded in the recipe
	}
}

// validateSteps checks all steps
func validateSteps(result *ValidationResult, r *Recipe) {
	if len(r.Steps) == 0 {
		result.addError("steps", "at least one step is required")
		return
	}

	// Known action types
	knownActions := map[string]bool{
		"download":          true,
		"download_archive":  true,
		"download_file":     true,
		"extract":           true,
		"chmod":             true,
		"install_binaries":  true,
		"install_libraries": true,
		"link_dependencies": true,
		"set_env":           true,
		"set_rpath":         true,
		"run_command":       true,
		"apt_install":       true,
		"yum_install":       true,
		"brew_install":      true,
		"npm_install":       true,
		"pipx_install":      true,
		"pip_exec":          true,
		"cargo_install":     true,
		"go_install":        true,
		"gem_install":       true,
		"cpan_install":      true,
		"nix_install":       true,
		"github_archive":    true,
		"github_file":       true,
		"homebrew":          true,
		"setup_build_env":   true,
		"configure_make":    true,
		"cmake_build":       true,
		"meson_build":       true,
		"cargo_build":       true,
	}

	for i, step := range r.Steps {
		stepField := fmt.Sprintf("steps[%d]", i)

		if step.Action == "" {
			result.addError(stepField+".action", "action is required")
			continue
		}

		if !knownActions[step.Action] {
			// Try to suggest similar actions
			suggestion := suggestSimilar(step.Action, knownActions)
			if suggestion != "" {
				result.addError(stepField+".action", fmt.Sprintf("unknown action '%s' (did you mean '%s'?)", step.Action, suggestion))
			} else {
				result.addError(stepField+".action", fmt.Sprintf("unknown action '%s'", step.Action))
			}
			continue
		}

		// Validate action-specific parameters
		validateActionParams(result, stepField, &step)
	}
}

// validateActionParams validates parameters for a specific action
func validateActionParams(result *ValidationResult, stepField string, step *Step) {
	switch step.Action {
	case "download":
		if _, ok := step.Params["url"]; !ok {
			result.addError(stepField, "download action requires 'url' parameter")
		} else {
			validateURLParam(result, stepField+".url", step.Params["url"])
		}

	case "download_archive":
		if _, ok := step.Params["url"]; !ok {
			result.addError(stepField, "download_archive action requires 'url' parameter")
		} else {
			validateURLParam(result, stepField+".url", step.Params["url"])
		}

	case "extract":
		if _, ok := step.Params["archive"]; !ok {
			result.addError(stepField, "extract action requires 'archive' parameter")
		}

	case "install_binaries":
		if _, ok := step.Params["binaries"]; !ok {
			if _, ok := step.Params["binary"]; !ok {
				result.addError(stepField, "install_binaries action requires 'binaries' or 'binary' parameter")
			}
		}

	case "github_archive", "github_file":
		if _, ok := step.Params["repo"]; !ok {
			result.addError(stepField, fmt.Sprintf("%s action requires 'repo' parameter", step.Action))
		}
		if _, ok := step.Params["asset_pattern"]; !ok {
			result.addError(stepField, fmt.Sprintf("%s action requires 'asset_pattern' parameter", step.Action))
		}

	case "npm_install":
		if _, ok := step.Params["package"]; !ok {
			result.addError(stepField, "npm_install action requires 'package' parameter")
		}

	case "pipx_install":
		if _, ok := step.Params["package"]; !ok {
			result.addError(stepField, "pipx_install action requires 'package' parameter")
		}

	case "cargo_install":
		if _, ok := step.Params["crate"]; !ok {
			result.addError(stepField, "cargo_install action requires 'crate' parameter")
		}

	case "go_install":
		if _, ok := step.Params["module"]; !ok {
			result.addError(stepField, "go_install action requires 'module' parameter")
		}

	case "gem_install":
		if _, ok := step.Params["gem"]; !ok {
			result.addError(stepField, "gem_install action requires 'gem' parameter")
		}

	case "cpan_install":
		if _, ok := step.Params["distribution"]; !ok {
			result.addError(stepField, "cpan_install action requires 'distribution' parameter")
		}
		if _, ok := step.Params["executables"]; !ok {
			result.addError(stepField, "cpan_install action requires 'executables' parameter")
		}
		// Check for redundant module parameter
		validateCpanModule(result, stepField, step)

	case "run_command":
		if _, ok := step.Params["command"]; !ok {
			result.addError(stepField, "run_command action requires 'command' parameter")
		}

	case "homebrew":
		if _, ok := step.Params["formula"]; !ok {
			result.addError(stepField, "homebrew action requires 'formula' parameter")
		}

	case "configure_make":
		if _, ok := step.Params["source_dir"]; !ok {
			result.addError(stepField, "configure_make action requires 'source_dir' parameter")
		}
		if _, ok := step.Params["executables"]; !ok {
			result.addError(stepField, "configure_make action requires 'executables' parameter")
		}
	}

	// Check for path traversal in any path-like parameters
	pathParams := []string{"dest", "archive", "binary", "src", "path"}
	for _, param := range pathParams {
		if val, ok := step.Params[param]; ok {
			if str, ok := val.(string); ok {
				validatePathParam(result, stepField+"."+param, str)
			}
		}
	}
}

// validateURLParam validates a URL parameter
func validateURLParam(result *ValidationResult, field string, value interface{}) {
	urlStr, ok := value.(string)
	if !ok {
		return
	}

	// Skip template variables
	if strings.Contains(urlStr, "{") {
		return
	}

	parsed, err := url.Parse(urlStr)
	if err != nil {
		result.addError(field, fmt.Sprintf("invalid URL: %v", err))
		return
	}

	// Check scheme
	if parsed.Scheme != "" && parsed.Scheme != "http" && parsed.Scheme != "https" {
		result.addError(field, fmt.Sprintf("URL scheme must be http or https, got '%s'", parsed.Scheme))
	}
}

// validatePathParam validates a path parameter for security issues
func validatePathParam(result *ValidationResult, field, path string) {
	// Skip template variables
	if strings.Contains(path, "{") {
		return
	}

	// Check for path traversal
	if strings.Contains(path, "..") {
		result.addError(field, "path must not contain '..' (path traversal)")
	}

	// Check for absolute paths
	if strings.HasPrefix(path, "/") {
		result.addWarning(field, "absolute paths may cause issues across different systems")
	}
}

// validateCpanModule checks if the module parameter is redundant
// A module is redundant if it matches what would be inferred from the distribution name
// by replacing hyphens with double colons (e.g., "Perl-Critic" -> "Perl::Critic")
func validateCpanModule(result *ValidationResult, stepField string, step *Step) {
	moduleVal, hasModule := step.Params["module"]
	distVal, hasDist := step.Params["distribution"]

	if !hasModule || !hasDist {
		return
	}

	module, moduleOk := moduleVal.(string)
	dist, distOk := distVal.(string)

	if !moduleOk || !distOk {
		return
	}

	// Convert distribution to expected module name (replace hyphens with ::)
	expectedModule := strings.ReplaceAll(dist, "-", "::")

	if module == expectedModule {
		result.addWarning(stepField+".module",
			fmt.Sprintf("module '%s' is redundant (can be inferred from distribution '%s')", module, dist))
	}
}

// validateVerify checks the verify section
func validateVerify(result *ValidationResult, r *Recipe) {
	// Libraries don't require verification (they are files, not executables)
	if r.Metadata.Type == RecipeTypeLibrary {
		return
	}

	if r.Verify.Command == "" {
		result.addError("verify.command", "command is required")
		return
	}

	// Check for dangerous patterns in verify command
	validateDangerousPatterns(result, r.Verify.Command)

	// Validate verification mode
	validateVerifyMode(result, r)
}

// validateDangerousPatterns checks for potentially dangerous patterns in verify commands
func validateDangerousPatterns(result *ValidationResult, command string) {
	// Patterns with word boundaries to avoid false positives on tool names (e.g., "terraform")
	dangerous := []string{" rm ", "\trm ", "> /", "| sh", "| bash", "curl |", "wget |"}
	for _, pattern := range dangerous {
		if strings.Contains(command, pattern) {
			result.addWarning("verify.command", fmt.Sprintf("verify command contains potentially dangerous pattern '%s'", strings.TrimSpace(pattern)))
		}
	}

	// Check if command starts with rm (word boundary at start)
	if strings.HasPrefix(command, "rm ") || strings.HasPrefix(command, "rm\t") {
		result.addWarning("verify.command", "verify command contains potentially dangerous pattern 'rm'")
	}

	// Expanded dangerous pattern detection (per design doc)
	// Check for conditional execution operators
	if strings.Contains(command, "||") {
		result.addWarning("verify.command", "verify command contains potentially dangerous pattern '||' (conditional execution); use exit_code field instead")
	}
	if strings.Contains(command, "&&") {
		result.addWarning("verify.command", "verify command contains potentially dangerous pattern '&&' (conditional execution)")
	}

	// Check for eval/exec with word boundaries
	evalPatterns := []string{" eval ", "\teval ", " eval\t", ";eval ", " exec ", "\texec ", " exec\t", ";exec "}
	for _, pattern := range evalPatterns {
		if strings.Contains(command, pattern) {
			keyword := strings.TrimSpace(strings.Trim(pattern, ";\t "))
			result.addWarning("verify.command", fmt.Sprintf("verify command contains potentially dangerous pattern '%s' (arbitrary code execution)", keyword))
			break // Only warn once per keyword type
		}
	}
	// Check if command starts with eval or exec
	if strings.HasPrefix(command, "eval ") || strings.HasPrefix(command, "eval\t") {
		result.addWarning("verify.command", "verify command contains potentially dangerous pattern 'eval' (arbitrary code execution)")
	}
	if strings.HasPrefix(command, "exec ") || strings.HasPrefix(command, "exec\t") {
		result.addWarning("verify.command", "verify command contains potentially dangerous pattern 'exec' (process replacement)")
	}

	// Check for command substitution
	if strings.Contains(command, "$(") {
		result.addWarning("verify.command", "verify command contains potentially dangerous pattern '$()' (command substitution)")
	}
	if strings.Contains(command, "`") {
		result.addWarning("verify.command", "verify command contains potentially dangerous pattern '`' (command substitution)")
	}
}

// validateVerifyMode checks mode-specific requirements
func validateVerifyMode(result *ValidationResult, r *Recipe) {
	mode := r.Verify.Mode
	if mode == "" {
		mode = VerifyModeVersion // Default mode
	}

	switch mode {
	case VerifyModeVersion:
		// Version mode should have {version} in pattern for proper verification
		// This is a warning because version_format transforms can normalize versions
		if r.Verify.Pattern != "" && !strings.Contains(r.Verify.Pattern, "{version}") {
			result.addWarning("verify.pattern", "version mode pattern should include {version} for proper verification")
		}

	case VerifyModeOutput:
		// Output mode requires a reason explaining why version verification isn't possible
		if r.Verify.Reason == "" {
			result.addError("verify.reason", "output mode requires a reason explaining why version verification is not possible")
		}

	case "functional":
		// Functional mode is reserved for v2
		result.addError("verify.mode", "functional mode is reserved for future implementation")

	default:
		// Unknown mode - error
		if mode != "" {
			result.addError("verify.mode", fmt.Sprintf("unknown verification mode '%s' (valid: version, output)", mode))
		}
	}
}

// suggestSimilar finds a similar string from the known set
func suggestSimilar(input string, known map[string]bool) string {
	input = strings.ToLower(input)

	// First, check for small edit distances (typos)
	bestMatch := ""
	bestDist := 999
	for k := range known {
		dist := levenshteinDistance(k, input)
		if dist < bestDist && dist <= 3 {
			bestDist = dist
			bestMatch = k
		}
	}
	if bestMatch != "" {
		return bestMatch
	}

	// Then check prefixes
	for k := range known {
		// Check if input is a prefix of a known action
		if strings.HasPrefix(k, input) {
			return k
		}
		// Check if known action is a prefix of input
		if strings.HasPrefix(input, k) && len(input)-len(k) <= 3 {
			return k
		}
	}

	return ""
}

// levenshteinDistance calculates the edit distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}
