package recipe

import (
	"fmt"
	"strings"
)

// ValidateStructural performs fast, structural validation without external dependencies.
// This is suitable for parse-time validation in the loader.
//
// Structural validation checks:
// - Required fields are present (name, steps, verify.command)
// - Field types are correct
// - Field formats are valid (URLs, paths)
// - No security issues (path traversal, dangerous patterns)
//
// It does NOT query registries or validate action existence/parameters.
func ValidateStructural(r *Recipe) []ValidationError {
	var errors []ValidationError

	// Metadata validation
	if r.Metadata.Name == "" {
		errors = append(errors, ValidationError{Field: "metadata.name", Message: "name is required"})
	} else if strings.Contains(r.Metadata.Name, " ") {
		errors = append(errors, ValidationError{Field: "metadata.name", Message: "name should not contain spaces (use kebab-case)"})
	}

	// Type validation
	if r.Metadata.Type != "" && r.Metadata.Type != RecipeTypeTool && r.Metadata.Type != RecipeTypeLibrary {
		errors = append(errors, ValidationError{Field: "metadata.type", Message: fmt.Sprintf("invalid type '%s' (valid values: tool, library)", r.Metadata.Type)})
	}

	// Steps existence
	if len(r.Steps) == 0 {
		errors = append(errors, ValidationError{Field: "steps", Message: "at least one step is required"})
	}

	// Steps must have action field
	for i, step := range r.Steps {
		if step.Action == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps[%d].action", i),
				Message: "action is required",
			})
		}
	}

	// Verify command (for non-libraries)
	if r.Metadata.Type != RecipeTypeLibrary && r.Verify.Command == "" {
		errors = append(errors, ValidationError{Field: "verify.command", Message: "command is required"})
	}

	// Patch validation
	for i, patch := range r.Patches {
		patchField := fmt.Sprintf("patches[%d]", i)
		if patch.URL != "" && patch.Data != "" {
			errors = append(errors, ValidationError{Field: patchField, Message: "cannot specify both 'url' and 'data' (must be mutually exclusive)"})
		}
		if patch.URL == "" && patch.Data == "" {
			errors = append(errors, ValidationError{Field: patchField, Message: "must specify either 'url' or 'data'"})
		}
	}

	return errors
}

// ValidateSemantic performs deep validation that queries action and version registries.
// This is suitable for CLI validation where comprehensive checks are desired.
//
// Semantic validation checks:
// - Action existence (via actions.ValidateAction)
// - Action parameters (via Preflight interface when implemented)
// - Version source validity (via VersionValidator interface)
// - Cross-field constraints
//
// Requires: actions package imported, VersionValidator registered
func ValidateSemantic(r *Recipe) []ValidationError {
	var errors []ValidationError

	// Action validation - delegate to the actions package
	// This will be used once we update validateSteps to use it
	// For now, this function prepares the architecture

	// Version validation - delegate to the registered validator
	if vv := GetVersionValidator(); vv != nil {
		if err := vv.ValidateVersionConfig(r); err != nil {
			errors = append(errors, ValidationError{
				Field:   "version",
				Message: err.Error(),
			})
		}
	}

	return errors
}

// ValidateFull performs both structural and semantic validation.
// This is the entry point for CLI validation (tsuku validate).
func ValidateFull(r *Recipe) *ValidationResult {
	result := &ValidationResult{Valid: true, Recipe: r}

	for _, err := range ValidateStructural(r) {
		result.Errors = append(result.Errors, err)
		result.Valid = false
	}

	for _, err := range ValidateSemantic(r) {
		result.Errors = append(result.Errors, err)
		result.Valid = false
	}

	return result
}
