package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TextReplaceAction performs text replacement in a file.
// This is used to implement Homebrew's inreplace functionality.
type TextReplaceAction struct{ BaseAction }

// IsDeterministic returns true because text_replace produces identical results.
func (TextReplaceAction) IsDeterministic() bool { return true }

// Name returns the action name.
func (a *TextReplaceAction) Name() string {
	return "text_replace"
}

// Execute performs the text replacement.
//
// Parameters:
//   - file (required): File path relative to work directory
//   - pattern (required): Pattern to find (literal or regex)
//   - replacement (optional): Text to replace with (empty for deletion)
//   - regex (optional): If true, pattern is treated as regex
func (a *TextReplaceAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get file path (required)
	file, ok := GetString(params, "file")
	if !ok || file == "" {
		return fmt.Errorf("text_replace action requires 'file' parameter")
	}

	// Security: disallow path traversal
	if strings.Contains(file, "..") || filepath.IsAbs(file) {
		return fmt.Errorf("text_replace: invalid file path '%s'", file)
	}

	// Get pattern (required)
	pattern, ok := GetString(params, "pattern")
	if !ok || pattern == "" {
		return fmt.Errorf("text_replace action requires 'pattern' parameter")
	}

	// Get replacement (optional, can be empty for deletion)
	replacement, _ := GetString(params, "replacement")

	// Get regex flag (optional)
	isRegex := false
	if r, ok := params["regex"].(bool); ok {
		isRegex = r
	}

	// Build vars for variable substitution
	vars := GetStandardVars(ctx.Version, ctx.InstallDir, ctx.WorkDir)
	file = ExpandVars(file, vars)
	pattern = ExpandVars(pattern, vars)
	replacement = ExpandVars(replacement, vars)

	// Construct absolute path
	filePath := filepath.Join(ctx.WorkDir, file)

	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("text_replace: failed to read file '%s': %w", file, err)
	}

	var newContent string
	if isRegex {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("text_replace: invalid regex pattern: %w", err)
		}
		newContent = re.ReplaceAllString(string(content), replacement)
	} else {
		newContent = strings.ReplaceAll(string(content), pattern, replacement)
	}

	// Check if any replacement was made
	if newContent == string(content) {
		// Pattern not found - log but don't fail
		ctx.Log().Debug("text_replace: pattern not found in file", "file", file, "pattern", pattern)
	}

	// Write file back with same permissions
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("text_replace: failed to stat file '%s': %w", file, err)
	}

	if err := os.WriteFile(filePath, []byte(newContent), info.Mode()); err != nil {
		return fmt.Errorf("text_replace: failed to write file '%s': %w", file, err)
	}

	ctx.Log().Debug("text_replace: replaced pattern in file", "file", file, "pattern", pattern)
	return nil
}

func init() {
	Register(&TextReplaceAction{})
}
