package index

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// recipeMinimal is a minimal TOML struct used only during index rebuild to
// extract binary names without importing the full internal/recipe package.
type recipeMinimal struct {
	Metadata struct {
		Binaries []string `toml:"binaries"`
	} `toml:"metadata"`
	Steps []recipeStep `toml:"steps"`
}

// recipeStep mirrors the flat step encoding used in recipe TOML files.
type recipeStep struct {
	Action  string                 `toml:"action"`
	Params  map[string]interface{} `toml:"-"`
	rawData map[string]interface{}
}

// UnmarshalTOML implements the toml.Unmarshaler interface so that step params
// are decoded into the flat map alongside the known fields.
func (s *recipeStep) UnmarshalTOML(data interface{}) error {
	m, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("expected map for step, got %T", data)
	}
	s.rawData = m
	if action, ok := m["action"].(string); ok {
		s.Action = action
	}
	s.Params = make(map[string]interface{}, len(m))
	for k, v := range m {
		if k != "action" {
			s.Params[k] = v
		}
	}
	return nil
}

// extractBinariesFromMinimal extracts binary destination paths from a
// recipeMinimal, mirroring the logic in internal/recipe.(*Recipe).ExtractBinaries.
// It returns paths like "bin/jq" that callers can use for both binary_path and
// to derive the command name (filepath.Base).
func extractBinariesFromMinimal(r *recipeMinimal) []string {
	// Explicit metadata.binaries takes precedence (homebrew recipes).
	if len(r.Metadata.Binaries) > 0 {
		return r.Metadata.Binaries
	}

	installActions := map[string]bool{
		"install_binaries": true,
		"download_archive": true,
		"github_archive":   true,
		"github_file":      true,
		"npm_install":      true,
		"gem_install":      true,
		"cargo_install":    true,
		"cargo_build":      true,
		"configure_make":   true,
		"cmake_build":      true,
		"meson_build":      true,
	}

	var binaries []string
	seen := make(map[string]bool)

	add := func(destPath string) {
		name := filepath.Base(destPath)
		if !seen[name] {
			binaries = append(binaries, destPath)
			seen[name] = true
		}
	}

	for _, step := range r.Steps {
		if !installActions[step.Action] {
			continue
		}

		// Singular "binary" param (github_file action).
		if binaryRaw, ok := step.Params["binary"]; ok {
			if binaryStr, ok := binaryRaw.(string); ok {
				add(filepath.Join("bin", filepath.Base(binaryStr)))
			}
		}

		// "outputs" (preferred) or "binaries" (deprecated) param.
		outputsRaw, hasOutputs := step.Params["outputs"]
		if !hasOutputs {
			outputsRaw = step.Params["binaries"]
		}
		if outputsRaw == nil {
			// Try "executables" (gem_install, cargo_install, configure_make, etc.).
			outputsRaw = step.Params["executables"]
		}
		if outputsRaw == nil {
			continue
		}

		installMode, _ := step.Params["install_mode"].(string)
		isDirectoryMode := (installMode == "directory" || installMode == "directory_wrapped")

		outputsList, ok := outputsRaw.([]interface{})
		if !ok {
			continue
		}
		for _, b := range outputsList {
			switch v := b.(type) {
			case string:
				if isDirectoryMode {
					add(v)
				} else {
					add(filepath.Join("bin", filepath.Base(v)))
				}
			case map[string]interface{}:
				if dest, ok := v["dest"].(string); ok {
					add(dest)
				}
			}
		}
	}

	return binaries
}

// Rebuild regenerates the index from the recipe registry and installed state.
//
// It runs entirely inside a single transaction: a mid-run error causes a full
// rollback, leaving the previous index intact. A second call produces identical
// rows (DELETE + re-insert is idempotent).
func (idx *sqliteBinaryIndex) Rebuild(ctx context.Context, reg Registry, state StateReader) error {
	tools, err := state.AllTools()
	if err != nil {
		return fmt.Errorf("index rebuild: read installed state: %w", err)
	}

	names, err := reg.ListCached()
	if err != nil {
		return fmt.Errorf("index rebuild: list cached recipes: %w", err)
	}

	tx, err := idx.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("index rebuild: begin transaction: %w", err)
	}
	defer func() {
		// Only reaches here on error path; happy path commits below.
		_ = tx.Rollback()
	}()

	// Clear all existing rows so the rebuild is idempotent.
	if _, err := tx.ExecContext(ctx, `DELETE FROM binaries`); err != nil {
		return fmt.Errorf("index rebuild: clear binaries table: %w", err)
	}

	const insertSQL = `INSERT OR REPLACE INTO binaries (command, recipe, binary_path, source, installed)
	VALUES (?, ?, ?, ?, ?)`

	insertedRecipes := make(map[string]bool)

	// Process all registry recipes.
	for _, name := range names {
		data, err := reg.GetCached(name)
		if err != nil {
			return fmt.Errorf("index rebuild: get cached recipe %q: %w", name, err)
		}

		var r recipeMinimal
		if err := toml.Unmarshal(data, &r); err != nil {
			// Skip malformed recipes rather than aborting the whole rebuild.
			continue
		}

		binPaths := extractBinariesFromMinimal(&r)
		if len(binPaths) == 0 {
			// No binary information — record a placeholder so the recipe is
			// still findable by name. Use the recipe name as the command.
			command := name
			installed := boolToInt(isInstalled(tools, name))
			if _, err := tx.ExecContext(ctx, insertSQL, command, name, "", "registry", installed); err != nil {
				return fmt.Errorf("index rebuild: insert placeholder for %q: %w", name, err)
			}
			insertedRecipes[name] = true
			continue
		}

		installed := boolToInt(isInstalled(tools, name))
		for _, binPath := range binPaths {
			command := filepath.Base(binPath)
			command = strings.TrimSuffix(command, ".exe")
			if _, err := tx.ExecContext(ctx, insertSQL, command, name, binPath, "registry", installed); err != nil {
				return fmt.Errorf("index rebuild: insert binary %q for recipe %q: %w", command, name, err)
			}
		}
		insertedRecipes[name] = true
	}

	// Add rows for installed tools that did not come from the registry
	// (local/distributed sources). These get source = 'installed'.
	for toolName, info := range tools {
		if insertedRecipes[toolName] {
			continue // already covered by the registry pass
		}
		if info.ActiveVersion == "" {
			continue // not currently active
		}

		// Collect binary names from the active version state if available.
		var binPaths []string
		if vInfo, ok := info.Versions[info.ActiveVersion]; ok {
			binPaths = append(binPaths, vInfo.Binaries...)
		}

		if len(binPaths) == 0 {
			// Fallback: use the tool name itself as the command.
			command := toolName
			if _, err := tx.ExecContext(ctx, insertSQL, command, toolName, "", "installed", 1); err != nil {
				return fmt.Errorf("index rebuild: insert installed tool %q: %w", toolName, err)
			}
			continue
		}

		for _, binPath := range binPaths {
			command := filepath.Base(binPath)
			command = strings.TrimSuffix(command, ".exe")
			if _, err := tx.ExecContext(ctx, insertSQL, command, toolName, binPath, "installed", 1); err != nil {
				return fmt.Errorf("index rebuild: insert installed binary %q for %q: %w", command, toolName, err)
			}
		}
	}

	// Stamp the meta table with a build timestamp.
	builtAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx,
		`INSERT OR REPLACE INTO meta (key, value) VALUES ('built_at', ?)`,
		builtAt,
	); err != nil {
		return fmt.Errorf("index rebuild: update built_at: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("index rebuild: commit: %w", err)
	}

	return nil
}

// isInstalled returns true when the named tool has a non-empty ActiveVersion.
func isInstalled(tools map[string]ToolInfo, name string) bool {
	if info, ok := tools[name]; ok {
		return info.ActiveVersion != ""
	}
	return false
}

// boolToInt converts a bool to the SQLite integer representation (0 or 1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
