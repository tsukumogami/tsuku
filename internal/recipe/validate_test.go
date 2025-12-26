package recipe

import (
	"errors"
	"testing"
)

func TestValidateStructural_MissingName(t *testing.T) {
	r := &Recipe{}
	errs := ValidateStructural(r)

	found := false
	for _, err := range errs {
		if err.Field == "metadata.name" && err.Message == "name is required" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error for missing name")
	}
}

func TestValidateStructural_NameWithSpaces(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "my tool"},
	}
	errs := ValidateStructural(r)

	found := false
	for _, err := range errs {
		if err.Field == "metadata.name" && err.Message == "name should not contain spaces (use kebab-case)" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error for name with spaces")
	}
}

func TestValidateStructural_InvalidType(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{
			Name: "test",
			Type: "invalid",
		},
		Steps:  []Step{{Action: "download"}},
		Verify: VerifySection{Command: "test --version"},
	}
	errs := ValidateStructural(r)

	found := false
	for _, err := range errs {
		if err.Field == "metadata.type" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error for invalid type")
	}
}

func TestValidateStructural_NoSteps(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "test"},
		Verify:   VerifySection{Command: "test --version"},
	}
	errs := ValidateStructural(r)

	found := false
	for _, err := range errs {
		if err.Field == "steps" && err.Message == "at least one step is required" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error for missing steps")
	}
}

func TestValidateStructural_StepWithoutAction(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "test"},
		Steps:    []Step{{Params: map[string]interface{}{"url": "http://example.com"}}},
		Verify:   VerifySection{Command: "test --version"},
	}
	errs := ValidateStructural(r)

	found := false
	for _, err := range errs {
		if err.Field == "steps[0].action" && err.Message == "action is required" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error for step without action")
	}
}

func TestValidateStructural_NoVerifyCommand(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "test"},
		Steps:    []Step{{Action: "download"}},
	}
	errs := ValidateStructural(r)

	found := false
	for _, err := range errs {
		if err.Field == "verify.command" && err.Message == "command is required" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error for missing verify command")
	}
}

func TestValidateStructural_LibraryNoVerifyCommand(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{
			Name: "test-lib",
			Type: RecipeTypeLibrary,
		},
		Steps: []Step{{Action: "download"}},
	}
	errs := ValidateStructural(r)

	// Libraries don't require verify command
	for _, err := range errs {
		if err.Field == "verify.command" {
			t.Error("libraries should not require verify command")
		}
	}
}

func TestValidateStructural_ValidRecipe(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "test-tool"},
		Steps:    []Step{{Action: "download", Params: map[string]interface{}{"url": "http://example.com"}}},
		Verify:   VerifySection{Command: "test --version"},
	}
	errs := ValidateStructural(r)

	if len(errs) != 0 {
		t.Errorf("expected no errors for valid recipe, got: %v", errs)
	}
}

func TestValidateStructural_PatchMutualExclusivity(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "test"},
		Steps:    []Step{{Action: "download"}},
		Verify:   VerifySection{Command: "test --version"},
		Patches: []Patch{
			{URL: "http://example.com/patch", Data: "some data"},
		},
	}
	errs := ValidateStructural(r)

	found := false
	for _, err := range errs {
		if err.Field == "patches[0]" && err.Message == "cannot specify both 'url' and 'data' (must be mutually exclusive)" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error for patch with both url and data")
	}
}

func TestValidateSemantic_WithVersionValidator(t *testing.T) {
	// Save and restore original validator
	origValidator := GetVersionValidator()
	defer SetVersionValidator(origValidator)

	// Set a mock validator that returns an error
	mock := &mockVersionValidator{
		validateErr: errors.New("no version source configured"),
	}
	SetVersionValidator(mock)

	r := &Recipe{
		Metadata: MetadataSection{Name: "test"},
		Steps:    []Step{{Action: "download"}},
	}
	errs := ValidateSemantic(r)

	found := false
	for _, err := range errs {
		if err.Field == "version" && err.Message == "no version source configured" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error from version validator")
	}
}

func TestValidateFull_CombinesBothLayers(t *testing.T) {
	// Save and restore original validator
	origValidator := GetVersionValidator()
	defer SetVersionValidator(origValidator)

	// Set a mock validator that returns an error
	mock := &mockVersionValidator{
		validateErr: errors.New("version error"),
	}
	SetVersionValidator(mock)

	// Recipe with structural error (no name) and semantic error (version)
	r := &Recipe{
		Steps:  []Step{{Action: "download"}},
		Verify: VerifySection{Command: "test --version"},
	}
	result := ValidateFull(r)

	if result.Valid {
		t.Error("expected result to be invalid")
	}

	// Should have structural error for name
	foundNameError := false
	foundVersionError := false
	for _, err := range result.Errors {
		if err.Field == "metadata.name" {
			foundNameError = true
		}
		if err.Field == "version" {
			foundVersionError = true
		}
	}

	if !foundNameError {
		t.Error("expected structural error for name")
	}
	if !foundVersionError {
		t.Error("expected semantic error for version")
	}
}
