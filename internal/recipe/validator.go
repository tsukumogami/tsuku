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
}

// validateVersion checks the version section
func validateVersion(result *ValidationResult, r *Recipe) {
	// Version source validation
	validSources := map[string]bool{
		"github_releases": true,
		"github_tags":     true,
		"nodejs_dist":     true,
		"npm_registry":    true,
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
		"":                true, // Empty is allowed (defaults to github_releases)
	}

	// Handle sources with parameters like "goproxy:module/path"
	source := r.Version.Source
	if idx := strings.Index(source, ":"); idx != -1 {
		source = source[:idx]
	}

	if !validSources[source] {
		result.addWarning("version.source", fmt.Sprintf("unknown version source '%s'", r.Version.Source))
	}

	// If using github sources, check github_repo
	if (source == "github_releases" || source == "github_tags" || source == "") && r.Version.GitHubRepo == "" {
		// Check if any step has repo parameter that could provide this
		hasRepoInStep := false
		for _, step := range r.Steps {
			if _, ok := step.Params["repo"]; ok {
				hasRepoInStep = true
				break
			}
		}
		if !hasRepoInStep {
			result.addWarning("version.github_repo", "github_repo is recommended when using github version source")
		}
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
		"extract":           true,
		"chmod":             true,
		"install_binaries":  true,
		"set_env":           true,
		"run_command":       true,
		"apt_install":       true,
		"yum_install":       true,
		"brew_install":      true,
		"npm_install":       true,
		"pipx_install":      true,
		"cargo_install":     true,
		"go_install":        true,
		"gem_install":       true,
		"cpan_install":      true,
		"nix_install":       true,
		"github_archive":    true,
		"github_file":       true,
		"hashicorp_release": true,
		"homebrew_bottle":   true,
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

	case "hashicorp_release":
		if _, ok := step.Params["product"]; !ok {
			result.addError(stepField, "hashicorp_release action requires 'product' parameter")
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
	if r.Verify.Command == "" {
		result.addError("verify.command", "command is required")
		return
	}

	// Check for dangerous patterns in verify command
	dangerous := []string{"rm ", "rm\t", "> /", "| sh", "| bash", "curl |", "wget |"}
	for _, pattern := range dangerous {
		if strings.Contains(r.Verify.Command, pattern) {
			result.addWarning("verify.command", fmt.Sprintf("verify command contains potentially dangerous pattern '%s'", strings.TrimSpace(pattern)))
		}
	}

	// Check if pattern contains version placeholder
	if r.Verify.Pattern != "" && !strings.Contains(r.Verify.Pattern, "{version}") {
		result.addWarning("verify.pattern", "pattern does not contain {version} placeholder - version verification may not work correctly")
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
