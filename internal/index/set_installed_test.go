package index

import (
	"context"
	"testing"
)

// TestSetInstalled_TogglesFlag verifies that SetInstalled correctly sets and
// clears the installed flag for a given recipe.
func TestSetInstalled_TogglesFlag(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	// Build index: jq installed, ripgrep not.
	recipes := map[string][]byte{
		"jq":      minimalRecipeTOML("bin/jq"),
		"ripgrep": minimalRecipeTOML("bin/rg"),
	}
	tools := map[string]ToolInfo{
		"jq": {ActiveVersion: "1.7.0"},
	}
	buildIndexWithRows(t, idx, recipes, tools)

	// Pre-condition: jq is installed, rg is not.
	matches, err := idx.Lookup(ctx, "jq")
	if err != nil {
		t.Fatalf("Lookup(jq) error = %v", err)
	}
	if len(matches) != 1 || !matches[0].Installed {
		t.Fatalf("pre-condition: jq should be installed")
	}

	// Mark jq as not installed.
	if err := idx.SetInstalled(ctx, "jq", false); err != nil {
		t.Fatalf("SetInstalled(jq, false) error = %v", err)
	}

	matches, err = idx.Lookup(ctx, "jq")
	if err != nil {
		t.Fatalf("Lookup(jq) after SetInstalled(false) error = %v", err)
	}
	if len(matches) != 1 || matches[0].Installed {
		t.Errorf("after SetInstalled(false): jq.Installed = %v, want false", matches[0].Installed)
	}

	// Mark ripgrep as installed.
	if err := idx.SetInstalled(ctx, "ripgrep", true); err != nil {
		t.Fatalf("SetInstalled(ripgrep, true) error = %v", err)
	}

	matches, err = idx.Lookup(ctx, "rg")
	if err != nil {
		t.Fatalf("Lookup(rg) after SetInstalled(true) error = %v", err)
	}
	if len(matches) != 1 || !matches[0].Installed {
		t.Errorf("after SetInstalled(true): rg.Installed = %v, want true", matches[0].Installed)
	}
}

// TestSetInstalled_NoopForUnknownRecipe verifies that SetInstalled does not
// return an error when the recipe name matches no rows (UPDATE affects 0 rows).
func TestSetInstalled_NoopForUnknownRecipe(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	buildIndexWithRows(t, idx, map[string][]byte{
		"jq": minimalRecipeTOML("bin/jq"),
	}, map[string]ToolInfo{})

	// Recipe "nonexistent" has no rows — UPDATE should be a no-op, not an error.
	if err := idx.SetInstalled(ctx, "nonexistent", true); err != nil {
		t.Errorf("SetInstalled for unknown recipe: error = %v, want nil", err)
	}
}
