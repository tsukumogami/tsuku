package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/version"
)

// ANSI color codes for terminal output
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// DepStatus represents the status of a single dependency
type DepStatus struct {
	Name     string `json:"name"`
	Type     string `json:"type"`   // "provisionable" or "system-required"
	Status   string `json:"status"` // "installed", "missing", "version_mismatch"
	Version  string `json:"version,omitempty"`
	Required string `json:"required,omitempty"`
}

// CheckDepsOutput represents the JSON output structure
type CheckDepsOutput struct {
	Tool         string      `json:"tool"`
	Dependencies []DepStatus `json:"dependencies"`
	AllSatisfied bool        `json:"all_satisfied"`
}

var checkDepsCmd = &cobra.Command{
	Use:   "check-deps <recipe>",
	Short: "Check dependency status for a recipe",
	Long: `Check all dependencies (direct and transitive) for a recipe.

Shows each dependency's type (provisionable or system-required) and status
(installed, missing, or version mismatch). Exits with code 1 if any system
dependency is missing or has a version mismatch.`,
	Args: cobra.ExactArgs(1),
	Run:  runCheckDeps,
}

func init() {
	checkDepsCmd.Flags().Bool("json", false, "Output in JSON format")
	rootCmd.AddCommand(checkDepsCmd)
}

func runCheckDeps(cmd *cobra.Command, args []string) {
	toolName := args[0]
	jsonOutput, _ := cmd.Flags().GetBool("json")

	// Load recipe
	r, err := loader.Get(toolName, recipe.LoaderOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Recipe '%s' not found in registry.\n", toolName)
		exitWithCode(ExitRecipeNotFound)
	}

	// Resolve direct dependencies
	directDeps := actions.ResolveDependencies(r)

	// Resolve transitive dependencies
	resolvedDeps, err := actions.ResolveTransitive(globalCtx, loader, directDeps, toolName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve dependencies: %v\n", err)
		exitWithCode(ExitDependencyFailed)
	}

	// Merge install-time and runtime deps for checking
	allDeps := mergeDeps(resolvedDeps)

	// Get installation manager for checking provisionable deps
	cfg, err := config.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		exitWithCode(ExitGeneral)
	}
	mgr := install.New(cfg)

	// Check each dependency
	var statuses []DepStatus
	hasSystemIssue := false

	for depName := range allDeps {
		status := checkDependency(globalCtx, loader, mgr, depName)
		statuses = append(statuses, status)

		// Track if any system dependency has issues
		if status.Type == "system-required" && status.Status != "installed" {
			hasSystemIssue = true
		}
	}

	// Sort by name for consistent output
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})

	// Output results
	if jsonOutput {
		output := CheckDepsOutput{
			Tool:         toolName,
			Dependencies: statuses,
			AllSatisfied: !hasSystemIssue,
		}
		printJSON(output)
	} else {
		printDepsReport(toolName, statuses)
	}

	// Exit non-zero if any system dependency is missing
	if hasSystemIssue {
		exitWithCode(ExitDependencyFailed)
	}
}

// mergeDeps combines install-time and runtime deps into a single map
func mergeDeps(deps actions.ResolvedDeps) map[string]string {
	result := make(map[string]string)
	for name, ver := range deps.InstallTime {
		result[name] = ver
	}
	for name, ver := range deps.Runtime {
		result[name] = ver
	}
	return result
}

// checkDependency determines the type and status of a dependency
func checkDependency(ctx context.Context, recipeLoader *recipe.Loader, mgr *install.Manager, depName string) DepStatus {
	status := DepStatus{
		Name: depName,
	}

	// Load the dependency's recipe to classify it
	depRecipe, err := recipeLoader.GetWithContext(ctx, depName, recipe.LoaderOptions{})
	if err != nil {
		// Recipe not found - mark as unknown
		status.Type = "unknown"
		status.Status = "missing"
		return status
	}

	// Classify: system-required if all steps are require_system
	if isSystemRequiredRecipe(depRecipe) {
		status.Type = "system-required"
		status = checkSystemDependency(depRecipe, status)
	} else {
		status.Type = "provisionable"
		status = checkProvisionableDependency(mgr, depName, status)
	}

	return status
}

// isSystemRequiredRecipe returns true if all steps are require_system actions
func isSystemRequiredRecipe(r *recipe.Recipe) bool {
	if r == nil || len(r.Steps) == 0 {
		return false
	}
	for _, step := range r.Steps {
		if step.Action != "require_system" {
			return false
		}
	}
	return true
}

// checkSystemDependency checks if a system dependency is installed and meets version requirements
func checkSystemDependency(r *recipe.Recipe, status DepStatus) DepStatus {
	// Process each require_system step
	for _, step := range r.Steps {
		command, _ := actions.GetString(step.Params, "command")
		if command == "" {
			continue
		}

		// Check if command exists
		cmdPath, err := exec.LookPath(command)
		if err != nil {
			status.Status = "missing"
			return status
		}

		// Check version if version_flag and version_regex provided
		versionFlag, hasVersionFlag := actions.GetString(step.Params, "version_flag")
		versionRegex, hasVersionRegex := actions.GetString(step.Params, "version_regex")
		minVersion, hasMinVersion := actions.GetString(step.Params, "min_version")

		if hasVersionFlag && hasVersionRegex {
			detectedVersion, err := detectSystemVersion(cmdPath, versionFlag, versionRegex)
			if err != nil {
				status.Status = "installed"
				status.Version = "unknown"
				return status
			}

			status.Version = detectedVersion

			// Check minimum version if specified
			if hasMinVersion {
				status.Required = minVersion
				if version.CompareVersions(detectedVersion, minVersion) < 0 {
					status.Status = "version_mismatch"
					return status
				}
			}
		}
	}

	status.Status = "installed"
	return status
}

// detectSystemVersion runs a command with version flag and extracts version using regex
func detectSystemVersion(command, versionFlag, versionRegex string) (string, error) {
	cmd := exec.Command(command, versionFlag)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run '%s %s': %w", command, versionFlag, err)
	}

	re, err := regexp.Compile(versionRegex)
	if err != nil {
		return "", fmt.Errorf("invalid version regex: %w", err)
	}

	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("version regex did not match output")
	}

	return strings.TrimSpace(matches[1]), nil
}

// checkProvisionableDependency checks if a provisionable dependency is installed
func checkProvisionableDependency(mgr *install.Manager, depName string, status DepStatus) DepStatus {
	tools, err := mgr.List()
	if err != nil {
		status.Status = "missing"
		return status
	}

	for _, t := range tools {
		if t.Name == depName {
			status.Status = "installed"
			status.Version = t.Version
			return status
		}
	}

	status.Status = "missing"
	return status
}

// printDepsReport prints a colorized dependency report
func printDepsReport(toolName string, statuses []DepStatus) {
	if len(statuses) == 0 {
		fmt.Printf("No dependencies for %s\n", toolName)
		return
	}

	fmt.Printf("%sDependencies for %s%s\n\n", colorBold, toolName, colorReset)

	// Group by type
	var systemDeps, provisionableDeps, unknownDeps []DepStatus
	for _, s := range statuses {
		switch s.Type {
		case "system-required":
			systemDeps = append(systemDeps, s)
		case "provisionable":
			provisionableDeps = append(provisionableDeps, s)
		default:
			unknownDeps = append(unknownDeps, s)
		}
	}

	// Print system-required deps
	if len(systemDeps) > 0 {
		fmt.Printf("%sSystem Dependencies%s (require external installation):\n", colorCyan, colorReset)
		for _, s := range systemDeps {
			printDepLine(s)
		}
		fmt.Println()
	}

	// Print provisionable deps
	if len(provisionableDeps) > 0 {
		fmt.Printf("%sProvisionable Dependencies%s (managed by tsuku):\n", colorCyan, colorReset)
		for _, s := range provisionableDeps {
			printDepLine(s)
		}
		fmt.Println()
	}

	// Print unknown deps
	if len(unknownDeps) > 0 {
		fmt.Printf("%sUnknown Dependencies%s (recipe not found):\n", colorCyan, colorReset)
		for _, s := range unknownDeps {
			printDepLine(s)
		}
		fmt.Println()
	}

	// Print summary
	printSummary(statuses)
}

// printDepLine prints a single dependency status line
func printDepLine(s DepStatus) {
	var statusColor, statusText string

	switch s.Status {
	case "installed":
		statusColor = colorGreen
		statusText = "installed"
		if s.Version != "" {
			statusText = fmt.Sprintf("installed (%s)", s.Version)
		}
	case "missing":
		statusColor = colorRed
		statusText = "missing"
	case "version_mismatch":
		statusColor = colorYellow
		statusText = fmt.Sprintf("version mismatch (found %s, need %s)", s.Version, s.Required)
	default:
		statusColor = colorRed
		statusText = s.Status
	}

	fmt.Printf("  %s%-20s%s %s%s%s\n", colorBold, s.Name, colorReset, statusColor, statusText, colorReset)
}

// printSummary prints a summary of dependency status
func printSummary(statuses []DepStatus) {
	total := len(statuses)
	installed := 0
	missing := 0
	versionMismatch := 0
	systemIssues := 0

	for _, s := range statuses {
		switch s.Status {
		case "installed":
			installed++
		case "missing":
			missing++
			if s.Type == "system-required" {
				systemIssues++
			}
		case "version_mismatch":
			versionMismatch++
			if s.Type == "system-required" {
				systemIssues++
			}
		}
	}

	fmt.Printf("Summary: %d total, ", total)
	if installed > 0 {
		fmt.Printf("%s%d installed%s, ", colorGreen, installed, colorReset)
	}
	if missing > 0 {
		fmt.Printf("%s%d missing%s, ", colorRed, missing, colorReset)
	}
	if versionMismatch > 0 {
		fmt.Printf("%s%d version mismatch%s, ", colorYellow, versionMismatch, colorReset)
	}
	fmt.Println()

	if systemIssues > 0 {
		fmt.Printf("\n%sNote:%s %d system dependency issue(s) must be resolved before installation.\n",
			colorYellow, colorReset, systemIssues)
	}
}
