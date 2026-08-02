package actions

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/config"
)

// DataDirVar is the recipe placeholder naming a tool's stable data directory.
const DataDirVar = "{data_dir}"

// dataDir returns $TSUKU_HOME/data/<tool> for the recipe being executed.
//
// This is the stable half of a tool that keeps user state. {install_dir} is versioned
// and reclaimed; this path is neither. A tool whose "install directory" doubles as its
// data root — nvm, and the same shape in pyenv, rbenv, and asdf — must export this one,
// or upgrading it orphans everything the user put there.
//
// The path is derived from ctx.ToolsDir rather than from a Config because actions do not
// carry one. config.DataDirName keeps the two computations spelled the same.
func dataDir(ctx *ExecutionContext) (string, error) {
	if ctx.ToolsDir == "" {
		return "", fmt.Errorf("tools directory is not available in ExecutionContext")
	}
	name, err := recipePathSegment(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(ctx.ToolsDir), config.DataDirName, name), nil
}

// recipePathSegment returns the recipe name, having checked it is safe to use as a
// single path component.
//
// A recipe name reaches this from a distributed registry, so it is untrusted input that
// is about to be joined onto a path tsuku creates and later reports to the user.
func recipePathSegment(ctx *ExecutionContext) (string, error) {
	if ctx.Recipe == nil || ctx.Recipe.Metadata.Name == "" {
		return "", fmt.Errorf("recipe name is not available in ExecutionContext")
	}
	name := ctx.Recipe.Metadata.Name
	if name != filepath.Base(name) || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("recipe name %q is not a valid path segment", name)
	}
	return name, nil
}

// CheckUnexpandedDataDir reports an unexpanded {data_dir} left in a string.
//
// Only some actions expand this placeholder, the same way only some expand {deps.*}.
// The check lives here, next to the actions that know, rather than as a list of
// expanding actions in the recipe validator: such a list drifts the moment a fourth
// action learns to expand, and the drift surfaces as a hard --strict failure against a
// recipe that is actually correct. This mirrors CheckUnexpandedDepVars.
func CheckUnexpandedDataDir(s, context string) error {
	if strings.Contains(s, DataDirVar) {
		return fmt.Errorf("unexpanded %s in %s\n  Hint: %s is only expanded by set_env and install_program_files", DataDirVar, context, DataDirVar)
	}
	return nil
}
