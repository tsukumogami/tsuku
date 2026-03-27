package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tsukumogami/tsuku/internal/project"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

// projectToolResult tracks the outcome of installing a single tool.
type projectToolResult struct {
	Name   string
	Status string // "installed", "current", "failed"
	Error  error
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

	// Build sorted tool list
	type toolEntry struct {
		Name    string
		Version string
	}
	var tools []toolEntry
	for name, req := range result.Config.Tools {
		tools = append(tools, toolEntry{Name: name, Version: req.Version})
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})

	// Display config path and tool list
	fmt.Printf("Using: %s\n", result.Path)

	var displayParts []string
	for _, t := range tools {
		if t.Version != "" {
			displayParts = append(displayParts, t.Name+"@"+t.Version)
		} else {
			displayParts = append(displayParts, t.Name)
		}
	}
	fmt.Printf("Tools: %s\n", strings.Join(displayParts, ", "))

	// Warn about unpinned versions
	var unpinned []string
	for _, t := range tools {
		if t.Version == "" || t.Version == "latest" {
			unpinned = append(unpinned, t.Name)
		}
	}
	if len(unpinned) > 0 {
		printWarning(fmt.Sprintf("Warning: %s %s unpinned (no version or \"latest\"). Pin versions for reproducibility.",
			strings.Join(unpinned, ", "), pluralVerb(len(unpinned))))
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
		resolveVersion := t.Version
		if resolveVersion == "latest" {
			resolveVersion = ""
		}

		err := runInstallWithTelemetry(t.Name, resolveVersion, t.Version, true, "", telemetryClient)
		if err != nil {
			results = append(results, projectToolResult{Name: t.Name, Status: "failed", Error: err})
		} else {
			// runInstallWithTelemetry returns nil for both new installs and
			// already-current tools. We classify as "installed" since we can't
			// distinguish here without deeper state inspection.
			results = append(results, projectToolResult{Name: t.Name, Status: "installed"})
		}
	}

	// Print summary
	printProjectSummary(results)

	// Determine exit code
	failCount := 0
	for _, r := range results {
		if r.Status == "failed" {
			failCount++
		}
	}
	if failCount == len(results) {
		exitWithCode(ExitInstallFailed)
	}
	if failCount > 0 {
		exitWithCode(ExitPartialFailure)
	}
	exitWithCode(ExitSuccess)
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
