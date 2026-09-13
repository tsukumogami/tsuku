package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/installevents"
	"github.com/tsukumogami/tsuku/internal/project"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

// projectToolResult tracks the outcome of installing a single tool.
type projectToolResult struct {
	Name string
	// Status is one of "installed", "current", "failed", "dry-run", or
	// "needs-approval" -- the last meaning the tool was never attempted
	// because nobody approved registering the source that declares it.
	Status string
	Error  error
}

// toolEntry represents a tool declared in the project config, with parsed
// metadata for version and distributed source information.
type toolEntry struct {
	Name        string
	Version     string
	Distributed *distributedInstallArgs // non-nil for org-scoped tools

	// SourceFailed means the source could not be used at all: an invalid name,
	// or strict_registries. The tool failed.
	SourceFailed bool

	// SourceNeedsApproval means the source is usable but nobody approved
	// registering it. The tool was skipped, which is not the same as failing
	// and is reported differently: the user can approve and re-run.
	SourceNeedsApproval bool
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

	result, err := loadProjectConfigReporting(cwd)
	if err != nil {
		if isRefusal(err) {
			// The helper already printed the line, with the reason and the
			// remedy. Printing the error again here would say it twice, and
			// this command's job on this path is only to choose the code.
			exitWithCode(ExitForbidden)
		}
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

	// Pre-scan: classify every source the project named. Nothing is written
	// here -- classification is not an action, and the question of whether to
	// register anything is not asked until the reader has seen the tool list.
	var sysCfg *config.Config
	plan := newProjectSourcePlan(result.Path)
	for _, t := range tools {
		if t.Distributed != nil {
			plan.add(t.Distributed.Source, t.Name)
		}
	}

	if !plan.empty() {
		var cfgErr error
		sysCfg, cfgErr = config.DefaultConfig()
		if cfgErr != nil {
			printError(fmt.Errorf("failed to load config: %w", cfgErr))
			exitWithCode(ExitGeneral)
		}
		plan.classify()

		for i := range tools {
			if tools[i].Distributed == nil {
				continue
			}
			if st, ok := plan.state(tools[i].Distributed.Source); ok && st == sourceFailed {
				tools[i].SourceFailed = true
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

	// Dry-run mode: show what would be installed without making changes.
	//
	// It classifies and reports, and never asks or writes. A dry run
	// deliberately asks nothing, so it cannot have consent, so it must not take
	// the action consent is for -- whatever the terminal and whatever --yes or
	// --force say.
	if installDryRun {
		plan.each(func(s *projectSource) {
			if s.State == sourceNeedsApproval {
				fmt.Fprintf(os.Stderr, "Source %q, declared in %q, is not registered. A dry run does not register it.\n",
					s.Name, plan.DeclaredIn)
			}
		})
		if sysCfg != nil {
			plan.activate(sysCfg)
		}
		runProjectDryRun(tools)
		return
	}

	// Ask about each unregistered source, after the tool list and before
	// "Proceed?". Nothing is written yet: declining either prompt has to leave
	// config.toml untouched, which it cannot if the write already happened.
	plan.decideConsent(consentInputs{
		AutoApprove: installYes,
		Interactive: isInteractive,
		Ask:         askYesNo,
	})

	// When every declared tool belongs to a source nobody approved, there is
	// nothing left to proceed with. Asking would be asking about an empty list.
	if plan.anyNeedsApproval() && allToolsBlocked(tools, plan) {
		plan.reportSkipped()
		exitWithCode(ExitNeedsApproval)
	}

	// Interactive confirmation unless --yes or non-TTY
	if !installYes && isInteractive() {
		fmt.Print("Proceed? [Y/n] ")
		line, _ := readPromptLine()
		line = strings.TrimSpace(strings.ToLower(line))
		if line != "" && line != "y" && line != "yes" {
			exitWithCode(ExitUserDeclined)
		}
	}

	// The user has proceeded. This is the first and only moment anything is
	// written, and it happens before any recipe is fetched.
	if err := plan.commit(); err != nil {
		printError(fmt.Errorf("failed to register approved sources: %w", err))
		exitWithCode(ExitGeneral)
	}
	if sysCfg != nil {
		plan.activate(sysCfg)
	}

	// Tools whose source nobody approved are skipped, not attempted. The rest
	// install.
	for i := range tools {
		if tools[i].Distributed == nil {
			continue
		}
		if st, ok := plan.state(tools[i].Distributed.Source); ok {
			switch st {
			case sourceFailed:
				tools[i].SourceFailed = true
			case sourceNeedsApproval:
				tools[i].SourceNeedsApproval = true
			}
		}
	}

	// Install each tool
	telemetryClient := telemetry.NewClient()
	telemetry.ShowNoticeIfNeeded()

	var results []projectToolResult
	for _, t := range tools {
		// Skip tools from failed sources
		if t.SourceFailed {
			var cause error
			if s, ok := plan.sources[t.Distributed.Source]; ok {
				cause = s.Err
			}
			results = append(results, projectToolResult{
				Name:   t.Name,
				Status: "failed",
				Error:  fmt.Errorf("source %q failed to register: %v", t.Distributed.Source, cause),
			})
			continue
		}

		// A source nobody approved is not a failure. Its tools were never
		// attempted, and saying "failed" would send the reader looking for a
		// broken install rather than for the approval they did not give.
		if t.SourceNeedsApproval {
			results = append(results, projectToolResult{
				Name:   t.Name,
				Status: "needs-approval",
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
			installErr := runInstall(installevents.WithSource(globalCtx, installevents.SourceManual), installArgs{
				Tool:              dArgs.RecipeName,
				ReqVersion:        resolveVersion,
				VersionConstraint: constraint,
				IsExplicit:        true,
				TelemetryClient:   telemetryClient,
			})
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
			err := runInstall(installevents.WithSource(globalCtx, installevents.SourceManual), installArgs{
				Tool:              t.Name,
				ReqVersion:        resolveVersion,
				VersionConstraint: constraint,
				IsExplicit:        true,
				TelemetryClient:   telemetryClient,
			})
			if err != nil {
				results = append(results, projectToolResult{Name: t.Name, Status: "failed", Error: err})
			} else {
				results = append(results, projectToolResult{Name: t.Name, Status: "installed"})
			}
		}
	}

	// Determine exit code
	failCount, skipCount := 0, 0
	for _, r := range results {
		switch r.Status {
		case "failed":
			failCount++
		case "needs-approval":
			skipCount++
		}
	}

	exitCode := ExitSuccess
	if failCount == len(results) {
		exitCode = ExitInstallFailed
	} else if failCount > 0 {
		exitCode = ExitPartialFailure
	}

	// A skipped source outranks an install failure. The two say different
	// things to a script: an install failure is "something broke", and this is
	// "approve this and run again". Returning the failure code would send a
	// script to retry logic that cannot succeed, since nothing about the
	// missing approval changes on a retry. The failure is still named on stderr
	// and in the structured output, and the script meets it on the next run.
	if skipCount > 0 {
		exitCode = ExitNeedsApproval
	}

	// Print summary
	if installJSON {
		printProjectSummaryJSON(results, exitCode)
	} else {
		printProjectSummary(results)
	}

	plan.reportSkipped()

	exitWithCode(exitCode)
}

// printProjectSummary prints the batch install summary.
func printProjectSummary(results []projectToolResult) {
	var installed, failed, skipped []projectToolResult
	for _, r := range results {
		switch r.Status {
		case "failed":
			failed = append(failed, r)
		case "needs-approval":
			// Its own bucket. Folding it into installed would report a tool
			// that was never attempted as present, which is the reading that
			// makes a skipped source invisible.
			skipped = append(skipped, r)
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

	if len(skipped) > 0 {
		names := make([]string, len(skipped))
		for i, r := range skipped {
			names[i] = r.Name
		}
		fmt.Printf("Skipped: %d %s awaiting source approval (%s)\n",
			len(skipped), pluralTool(len(skipped)), strings.Join(names, ", "))
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
	switch exitCode {
	case ExitInstallFailed:
		status = "error"
	case ExitPartialFailure:
		status = "partial"
	case ExitNeedsApproval:
		// Its own status, not "error" and not "partial". A consumer reading
		// this has one action available that the other two do not: approve the
		// source and run again.
		status = "needs-approval"
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
