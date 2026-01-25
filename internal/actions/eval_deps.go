package actions

import (
	"os"
	"strings"
)

// GetEvalDeps returns the eval-time dependencies for an action.
// Returns nil if the action has no eval-time dependencies.
func GetEvalDeps(action string) []string {
	act := Get(action)
	if act == nil {
		return nil
	}
	deps := act.Dependencies()
	return deps.EvalTime
}

// CheckEvalDeps checks which eval-time dependencies are not installed.
// It looks for tools in $TSUKU_HOME/tools/ directory.
// Returns a list of missing dependency names.
func CheckEvalDeps(deps []string) []string {
	if len(deps) == 0 {
		return nil
	}

	toolsDir := GetToolsDir()
	if toolsDir == "" {
		// Can't determine tools directory, assume all deps are missing
		return deps
	}

	return checkEvalDepsInDir(deps, toolsDir)
}

// checkEvalDepsInDir checks which dependencies are missing from the given tools directory.
func checkEvalDepsInDir(deps []string, toolsDir string) []string {
	var missing []string
	for _, dep := range deps {
		if !isToolInstalled(toolsDir, dep) {
			missing = append(missing, dep)
		}
	}
	return missing
}

// isToolInstalled checks if a tool is installed in the tools directory.
// A tool is considered installed if there's a directory matching the pattern name-*.
func isToolInstalled(toolsDir, name string) bool {
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return false
	}

	prefix := name + "-"
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			return true
		}
	}
	return false
}
