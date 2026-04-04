package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/project"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

// projectToolResult tracks the outcome of installing a single tool.
type projectToolResult struct {
	Name   string
	Status string // "installed", "current", "failed", "dry-run"
	Error  error
}

// toolEntry represents a tool declared in the project config, with parsed
// metadata for version and distributed source information.
type toolEntry struct {
	Name         string
	Version      string
	Distributed  *distributedInstallArgs // non-nil for org-scoped tools
	SourceFailed bool                    // true if source bootstrap failed
}

// runProjectInstall handles the no-args install path: discover the nearest
// .tsuku.toml, display the tool list, confirm with the user, and batch-install
// all declared tools with lenient error handling.
func runProjectInstall(cmd *cobra.Command) {
	// Incompatible flags check
	incompatible := []string{"plan", "recipe", "from", "sandbox"}
	for _, name := range incompatible {
		if cmd.Flags().Changed(name) {
			printError(fmt.Errorf("--%s cannot be combined with project install (no-args mode)", name))
			exitWithCode(ExitUsage)
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		printError(fmt.Errorf("failed to get working directory: %w", err))
		exitWithCode(ExitGeneral)
	}

	result, err := project.LoadProjectConfig(cwd)
	if err != nil {
		printError(err)
		exitWithCode(ExitGeneral)
	}
	if result == nil {
		printError(fmt.Errorf("no %s found (run 'tsuku init' to create one)", project.ConfigFileName))
		exitWithCode(ExitUsage)
	}

	if len(result.Config.Tools) == 0 {
		fmt.Printf("No tools declared in %s\n", result.Path)
		exitWithCode(ExitSuccess)
	}

	// Build sorted tool list with distributed-name parsing
	var tools []toolEntry
	for name, req := range result.Config.Tools {
		entry := toolEntry{Name: name, Version: req.Version}
		if dArgs := parseDistributedName(name); dArgs != nil {
			// Override version from dArgs if present (e.g., "org/repo@1.0")
			if dArgs.Version != "" && entry.Version == "" {
				entry.Version = dArgs.Version
			}
			entry.Distributed = dArgs
		}
		tools = append(tools, entry)
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})

	// Pre-scan: batch-bootstrap distributed sources
	var sysCfg *config.Config
	uniqueSources := make(map[string]bool)
	failedSources := make(map[string]error)

	for _, t := range tools {
		if t.Distributed != nil {
			uniqueSources[t.Distributed.Source] = true
		}
	}

	if len(uniqueSources) > 0 {
		var cfgErr error
		sysCfg, cfgErr = config.DefaultConfig()
		if cfgErr != nil {
			printError(fmt.Errorf("failed to load config: %w", cfgErr))
			exitWithCode(ExitGeneral)
		}

		for source := range uniqueSources {
			if err := ensureDistributedSource(source, installYes || installForce, sysCfg); err != nil {
				failedSources[source] = err
				printWarning(fmt.Sprintf("Warning: failed to register source %q: %v", source, err))
			}
		}

		// Mark tools from failed sources
		for i := range tools {
			if tools[i].Distributed != nil {
				if _, failed := failedSources[tools[i].Distributed.Source]; failed {
					tools[i].SourceFailed = true
				}
			}
		}
	}

	// Display config path and tool list
	fmt.Printf("Using: %s\n", result.Path)

	var displayParts []string
	for _, t := range tools {
		displayName := t.Name
		// For org-scoped tools, show the bare recipe name for cleaner output
		if t.Distributed != nil {
			displayName = t.Distributed.RecipeName
		}
		if t.Version != "" {
			displayParts = append(displayParts, displayName+"@"+t.Version)
		} else {
			displayParts = append(displayParts, displayName)
		}
	}
	fmt.Printf("Tools: %s\n", strings.Join(displayParts, ", "))

	// Warn about unpinned versions
	var unpinned []string
	for _, t := range tools {
		if t.Version == "" || t.Version == "latest" {
			name := t.Name
			if t.Distributed != nil {
				name = t.Distributed.RecipeName
			}
			unpinned = append(unpinned, name)
		}
	}
	if len(unpinned) > 0 {
		printWarning(fmt.Sprintf("Warning: %s %s unpinned (no version or \"latest\"). Pin versions for reproducibility.",
			strings.Join(unpinned, ", "), pluralVerb(len(unpinned))))
	}

	// Dry-run mode: show what would be installed without making changes
	if installDryRun {
		runProjectDryRun(tools)
		return
	}

	// Interactive confirmation unless --yes or non-TTY
	if !installYes && isInteractive() {
		fmt.Print("Proceed? [Y/n] ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(strings.ToLower(line))
		if line != "" && line != "y" && line != "yes" {
			exitWithCode(ExitUserDeclined)
		}
	}

	// Install each tool
	telemetryClient := telemetry.NewClient()
	telemetry.ShowNoticeIfNeeded()

	var results []projectToolResult
	for _, t := range tools {
		// Skip tools from failed sources
		if t.SourceFailed {
			results = append(results, projectToolResult{
				Name:   t.Name,
				Status: "failed",
				Error:  fmt.Errorf("source %q failed to register: %v", t.Distributed.Source, failedSources[t.Distributed.Source]),
			})
			continue
		}

		resolveVersion := t.Version
		constraint := t.Version
		if resolveVersion == "latest" {
			resolveVersion = ""
			constraint = ""
		}

		if t.Distributed != nil {
			// Distributed install path
			dArgs := t.Distributed

			// Check for source collision
			if sysCfg != nil {
				if collErr := checkSourceCollision(dArgs.RecipeName, dArgs.Source, installForce, sysCfg); collErr != nil {
					results = append(results, projectToolResult{Name: t.Name, Status: "failed", Error: collErr})
					continue
				}
			}

			// Build qualified name and pre-load recipe through distributed provider
			qualifiedName := dArgs.Source + ":" + dArgs.RecipeName

			// Fetch recipe bytes for hash computation
			recipeBytes, bytesErr := fetchRecipeBytes(dArgs.Source, dArgs.RecipeName)

			// Load recipe via qualified name to route through distributed provider
			r, loadErr := loader.GetWithContext(globalCtx, qualifiedName, recipe.LoaderOptions{})
			if loadErr != nil {
				results = append(results, projectToolResult{
					Name:   t.Name,
					Status: "failed",
					Error:  fmt.Errorf("recipe %q not found in %s: %w", dArgs.RecipeName, dArgs.Source, loadErr),
				})
				continue
			}

			// Cache under bare name for dependency resolution
			loader.CacheRecipe(dArgs.RecipeName, r)

			// Install using bare recipe name
			installErr := runInstallWithTelemetry(dArgs.RecipeName, resolveVersion, constraint, true, "", telemetryClient)
			if installErr != nil {
				results = append(results, projectToolResult{Name: t.Name, Status: "failed", Error: installErr})
				continue
			}

			// Record source and recipe hash
			var recipeHash string
			if bytesErr == nil && recipeBytes != nil {
				recipeHash = computeRecipeHash(recipeBytes)
			}
			if sysCfg != nil {
				if recordErr := recordDistributedSource(dArgs.RecipeName, dArgs.Source, recipeHash, sysCfg); recordErr != nil {
					printInfof("Warning: failed to record source for %s: %v\n", dArgs.RecipeName, recordErr)
				}
			}

			results = append(results, projectToolResult{Name: t.Name, Status: "installed"})
		} else {
			// Standard install path (unchanged)
			err := runInstallWithTelemetry(t.Name, resolveVersion, constraint, true, "", telemetryClient)
			if err != nil {
				results = append(results, projectToolResult{Name: t.Name, Status: "failed", Error: err})
			} else {
				results = append(results, projectToolResult{Name: t.Name, Status: "installed"})
			}
		}
	}

	// Determine exit code
	failCount := 0
	for _, r := range results {
		if r.Status == "failed" {
			failCount++
		}
	}

	exitCode := ExitSuccess
	if failCount == len(results) {
		exitCode = ExitInstallFailed
	} else if failCount > 0 {
		exitCode = ExitPartialFailure
	}

	// Print summary
	if installJSON {
		printProjectSummaryJSON(results, exitCode)
	} else {
		printProjectSummary(results)
	}

	exitWithCode(exitCode)
}

// printProjectSummary prints the batch install summary.
func printProjectSummary(results []projectToolResult) {
	var installed, failed []projectToolResult
	for _, r := range results {
		switch r.Status {
		case "failed":
			failed = append(failed, r)
		default:
			installed = append(installed, r)
		}
	}

	if len(installed) > 0 {
		names := make([]string, len(installed))
		for i, r := range installed {
			names[i] = r.Name
		}
		fmt.Printf("\nInstalled: %d %s (%s)\n", len(installed), pluralTool(len(installed)), strings.Join(names, ", "))
	}

	if len(failed) > 0 {
		fmt.Printf("Failed: %d %s\n", len(failed), pluralTool(len(failed)))
		for _, r := range failed {
			fmt.Printf("  %s: %v\n", r.Name, r.Error)
		}
	}
}

// pluralTool returns "tool" or "tools" based on count.
func pluralTool(n int) string {
	if n == 1 {
		return "tool"
	}
	return "tools"
}

// pluralVerb returns "is" or "are" based on count.
func pluralVerb(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

// runProjectDryRun runs dry-run for each tool in the project config.
// It calls runDryRun for each tool and collects results, then prints
// a summary. For distributed tools, it uses the qualified name so the
// recipe can be resolved through the distributed provider.
func runProjectDryRun(tools []toolEntry) {
	var results []projectToolResult
	for _, t := range tools {
		if t.SourceFailed {
			results = append(results, projectToolResult{
				Name:   t.Name,
				Status: "failed",
				Error:  fmt.Errorf("source failed to register (skipped)"),
			})
			continue
		}

		resolveVersion := t.Version
		if resolveVersion == "latest" {
			resolveVersion = ""
		}

		toolName := t.Name
		if t.Distributed != nil {
			toolName = t.Distributed.Source + ":" + t.Distributed.RecipeName
		}

		if err := runDryRun(toolName, resolveVersion); err != nil {
			results = append(results, projectToolResult{
				Name:   t.Name,
				Status: "failed",
				Error:  err,
			})
		} else {
			results = append(results, projectToolResult{
				Name:   t.Name,
				Status: "dry-run",
			})
		}
	}

	// Print summary
	failCount := 0
	for _, r := range results {
		if r.Status == "failed" {
			failCount++
		}
	}

	if failCount > 0 {
		fmt.Fprintf(os.Stderr, "\nDry-run completed with %d %s failing to resolve.\n", failCount, pluralTool(failCount))
		for _, r := range results {
			if r.Status == "failed" {
				fmt.Fprintf(os.Stderr, "  %s: %v\n", r.Name, r.Error)
			}
		}
		if failCount == len(results) {
			exitWithCode(ExitInstallFailed)
		}
		exitWithCode(ExitPartialFailure)
	}
	exitWithCode(ExitSuccess)
}

// projectToolJSON is the JSON representation of a single tool result
// in the project install summary.
type projectToolJSON struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// projectSummaryJSON is the structured JSON output for project install.
type projectSummaryJSON struct {
	Status   string            `json:"status"`
	Tools    []projectToolJSON `json:"tools"`
	ExitCode int               `json:"exit_code"`
}

// buildProjectSummaryJSON constructs the structured JSON summary for project
// install results. Separated from printProjectSummaryJSON for testability.
func buildProjectSummaryJSON(results []projectToolResult, exitCode int) projectSummaryJSON {
	status := "success"
	if exitCode == ExitInstallFailed {
		status = "error"
	} else if exitCode == ExitPartialFailure {
		status = "partial"
	}

	tools := make([]projectToolJSON, len(results))
	for i, r := range results {
		tools[i] = projectToolJSON{
			Name:   r.Name,
			Status: r.Status,
		}
		if r.Error != nil {
			tools[i].Error = r.Error.Error()
		}
	}

	return projectSummaryJSON{
		Status:   status,
		Tools:    tools,
		ExitCode: exitCode,
	}
}

// printProjectSummaryJSON prints the project install summary as JSON.
func printProjectSummaryJSON(results []projectToolResult, exitCode int) {
	summary := buildProjectSummaryJSON(results, exitCode)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(summary); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		exitWithCode(ExitGeneral)
	}
}
