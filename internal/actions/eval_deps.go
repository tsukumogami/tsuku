package actions

import (
	"os"
	"path/filepath"
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

	toolsDir := getToolsDir()
	if toolsDir == "" {
		// Can't determine tools directory, assume all deps are missing
		return deps
	}

	var missing []string
	for _, dep := range deps {
		if !isToolInstalled(toolsDir, dep) {
			missing = append(missing, dep)
		}
	}
	return missing
}

// getToolsDir returns the tools directory path.
// Uses $TSUKU_HOME/tools or defaults to ~/.tsuku/tools.
func getToolsDir() string {
	// Check TSUKU_HOME env var first
	if tsukuHome := os.Getenv("TSUKU_HOME"); tsukuHome != "" {
		return filepath.Join(tsukuHome, "tools")
	}

	// Default to ~/.tsuku/tools
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".tsuku", "tools")
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
