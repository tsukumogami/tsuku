package index

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
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
	Action string                 `toml:"action"`
	Params map[string]interface{} `toml:"-"`
}

// UnmarshalTOML implements the toml.Unmarshaler interface so that step params
// are decoded into the flat map alongside the known fields.
func (s *recipeStep) UnmarshalTOML(data interface{}) error {
	m, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("expected map for step, got %T", data)
	}
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
// recipeMinimal. The logic mirrors internal/recipe.(*Recipe).ExtractBinaries:
// metadata.binaries takes precedence; otherwise two independent blocks are
// checked per step — Block 1 for outputs/binaries and Block 2 for executables.
// A step may contribute entries from both blocks.
// It returns paths like "bin/jq" that callers can use for both binary_path and
// to derive the command name (filepath.Base).
func extractBinariesFromMinimal(r *recipeMinimal) []string {
	// Explicit metadata.binaries takes precedence (homebrew recipes).
	if len(r.Metadata.Binaries) > 0 {
		return r.Metadata.Binaries
	}

	// keep in sync with internal/recipe.(*Recipe).ExtractBinaries action list
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
		if destPath == "" {
			return
		}
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

		// Block 1: "outputs" (preferred) or "binaries" (deprecated) param.
		outputsRaw, hasOutputs := step.Params["outputs"]
		if !hasOutputs {
			outputsRaw = step.Params["binaries"]
		}
		if outputsRaw != nil {
			installMode, _ := step.Params["install_mode"].(string)
			isDirectoryMode := (installMode == "directory" || installMode == "directory_wrapped")

			if outputsList, ok := outputsRaw.([]interface{}); ok {
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
		}

		// Block 2: "executables" param (gem_install, cargo_install, configure_make, etc.).
		// Checked independently — a recipe may have both outputs/binaries AND executables.
		if executablesRaw, ok := step.Params["executables"]; ok {
			if executablesList, ok := executablesRaw.([]interface{}); ok {
				for _, e := range executablesList {
					if exeStr, ok := e.(string); ok {
						add(filepath.Join("bin", filepath.Base(exeStr)))
					}
				}
			}
		}
	}

	return binaries
}

// isValidRecipeName returns true if the recipe name is safe to pass to URL or
// path construction functions. Names containing '/', "..", or a null byte are
// rejected to prevent path traversal in local-registry deployments.
func isValidRecipeName(name string) bool {
	if strings.Contains(name, "/") {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	if strings.ContainsRune(name, '\x00') {
		return false
	}
	return true
}

// Rebuild regenerates the index from the recipe registry and installed state.
//
// It enumerates all known recipes via reg.ListAll (which reads the cached
// manifest when available), fetches uncached recipe TOMLs on demand using a
// bounded worker pool of 10 concurrent fetches, and inserts all rows in a
// single transaction. A fetch error on one recipe is logged as a warning and
// that recipe is skipped; other recipes continue normally. A DB write error
// during the insert phase rolls back all inserts atomically.
func (idx *sqliteBinaryIndex) Rebuild(ctx context.Context, reg Registry, state StateReader) error {
	tools, err := state.AllTools()
	if err != nil {
		return fmt.Errorf("index rebuild: read installed state: %w", err)
	}

	names, err := reg.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("index rebuild: list recipes: %w", err)
	}

	// Fetch/cache content for each recipe with bounded concurrency (10 workers).
	type result struct {
		name string
		data []byte
	}

	sem := make(chan struct{}, 10)
	var mu sync.Mutex
	results := make([]result, 0, len(names))

	var wg sync.WaitGroup
	for _, name := range names {
		// Validate name before passing to any URL or path construction.
		if !isValidRecipeName(name) {
			slog.Warn("index rebuild: invalid recipe name", "name", name)
			continue
		}

		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			data, err := reg.GetCached(n)
			if err != nil || len(data) == 0 {
				data, err = reg.FetchRecipe(ctx, n)
				if err != nil {
					slog.Warn("index rebuild: fetch recipe", "recipe", n, "err", err)
					return
				}
				// Cache the fetched TOML so subsequent runs are cache hits.
				if cacheErr := reg.CacheRecipe(n, data); cacheErr != nil {
					slog.Warn("index rebuild: cache recipe", "recipe", n, "err", cacheErr)
				}
			}
			mu.Lock()
			results = append(results, result{n, data})
			mu.Unlock()
		}(name)
	}
	wg.Wait()

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

	// Process all fetched recipes.
	for _, res := range results {
		name := res.name
		data := res.data

		var r recipeMinimal
		if err := toml.Unmarshal(data, &r); err != nil {
			// Skip malformed recipes rather than aborting the whole rebuild.
			slog.Warn("binary index: skipping recipe: failed to parse TOML",
				"recipe", name, "error", err)
			continue
		}

		binPaths := extractBinariesFromMinimal(&r)
		if len(binPaths) == 0 {
			// No binary declarations — skip entirely. Recipes without declared
			// binaries are invisible to the index; inserting a placeholder row
			// with empty binary_path would cause false-positive Lookup matches.
			continue
		}

		installed := 0
		if tools[name].ActiveVersion != "" {
			installed = 1
		}
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
