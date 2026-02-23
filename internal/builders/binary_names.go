package builders

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

// BinaryNameProvider is an optional interface that builders implement to expose
// authoritative binary names from registry metadata. The orchestrator uses this
// to cross-check recipe executables before sandbox validation.
//
// Builders that have access to registry-authoritative binary name data (Cargo
// via crates.io bin_names, npm via the bin field) implement this interface.
// Builders without such data (Go, LLM builders) skip this validation step.
type BinaryNameProvider interface {
	// AuthoritativeBinaryNames returns the executable names that the package
	// actually installs, as reported by the registry. An empty slice means
	// the provider has no data (skip validation). Names must pass
	// isValidExecutableName() before being used.
	AuthoritativeBinaryNames() []string
}

// BinaryNameRepairMetadata tracks when the orchestrator corrected recipe
// binary names using registry metadata from a BinaryNameProvider.
type BinaryNameRepairMetadata struct {
	// OldNames is the executable list from the recipe before correction.
	OldNames []string

	// NewNames is the corrected executable list from the provider.
	NewNames []string

	// Builder is the name of the builder that provided the authoritative names.
	Builder string
}

// validateBinaryNames compares the recipe's executable list against authoritative
// binary names from a BinaryNameProvider. If the lists differ, the recipe is
// corrected in-place and a warning is appended to the BuildResult.
//
// The method is a no-op when:
//   - the builder does not implement BinaryNameProvider
//   - the provider returns an empty slice (no registry data)
//   - the recipe has no steps with an "executables" parameter
//   - the names already match
func (o *Orchestrator) validateBinaryNames(
	provider BinaryNameProvider,
	result *BuildResult,
	builderName string,
) *BinaryNameRepairMetadata {
	authoritative := provider.AuthoritativeBinaryNames()
	if len(authoritative) == 0 {
		return nil
	}

	// Filter through executable name validation
	var validNames []string
	for _, name := range authoritative {
		if isValidExecutableName(name) {
			validNames = append(validNames, name)
		}
	}
	if len(validNames) == 0 {
		return nil
	}

	// Find the step with an "executables" parameter
	r := result.Recipe
	stepIdx := -1
	for i, step := range r.Steps {
		if _, ok := step.Params["executables"]; ok {
			stepIdx = i
			break
		}
	}
	if stepIdx < 0 {
		return nil
	}

	// Extract current executables from the recipe
	currentNames := extractExecutablesFromStep(r.Steps[stepIdx])
	if len(currentNames) == 0 {
		return nil
	}

	// Compare (order-independent)
	if executableSetsEqual(currentNames, validNames) {
		return nil
	}

	// Mismatch detected -- correct the recipe
	oldNames := make([]string, len(currentNames))
	copy(oldNames, currentNames)

	r.Steps[stepIdx].Params["executables"] = validNames

	// Update verify command if it references the first executable
	if r.Verify != nil && len(currentNames) > 0 && len(validNames) > 0 {
		oldFirst := currentNames[0]
		newFirst := validNames[0]
		if oldFirst != newFirst && strings.Contains(r.Verify.Command, oldFirst) {
			r.Verify.Command = strings.Replace(r.Verify.Command, oldFirst, newFirst, 1)
		}
	}

	warning := fmt.Sprintf(
		"Binary name mismatch corrected by %s registry: %v -> %v",
		builderName, oldNames, validNames,
	)
	result.Warnings = append(result.Warnings, warning)

	// Emit telemetry
	if o.telemetryClient != nil {
		event := telemetry.NewBinaryNameRepairEvent(
			r.Metadata.Name,
			builderName,
			true,
		)
		o.telemetryClient.SendBinaryNameRepair(event)
	}

	return &BinaryNameRepairMetadata{
		OldNames: oldNames,
		NewNames: validNames,
		Builder:  builderName,
	}
}

// extractExecutablesFromStep extracts the executable names from a step's
// "executables" parameter. It handles both []string and []interface{} types
// since TOML deserialization may produce either.
func extractExecutablesFromStep(step recipe.Step) []string {
	raw, ok := step.Params["executables"]
	if !ok {
		return nil
	}

	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		var names []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				names = append(names, s)
			}
		}
		return names
	}

	return nil
}

// executableSetsEqual returns true if two executable name slices contain the
// same names, regardless of order.
func executableSetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	aSorted := make([]string, len(a))
	copy(aSorted, a)
	sort.Strings(aSorted)

	bSorted := make([]string, len(b))
	copy(bSorted, b)
	sort.Strings(bSorted)

	for i := range aSorted {
		if aSorted[i] != bSorted[i] {
			return false
		}
	}
	return true
}
