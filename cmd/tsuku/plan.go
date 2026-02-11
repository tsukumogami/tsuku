package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Manage installation plans",
	Long:  `Manage installation plans for tools. Plans capture the exact steps and checksums used during installation.`,
}

var planShowCmd = &cobra.Command{
	Use:   "show <tool>",
	Short: "Display the installation plan for a tool",
	Long: `Display the stored installation plan for an installed tool.

The plan shows the exact URLs, checksums, and steps that were used
during installation. This enables verification and reproducibility.

Examples:
  tsuku plan show gh
  tsuku plan show kubectl --json`,
	Args: cobra.ExactArgs(1),
	Run:  runPlanShow,
}

var planShowJSON bool

var planExportCmd = &cobra.Command{
	Use:   "export <tool>",
	Short: "Export the installation plan for a tool to a file",
	Long: `Export the stored installation plan for an installed tool as a JSON file.

The exported plan can be shared, version-controlled, or used for offline
installation (future milestone). The JSON format matches 'tsuku eval' output.

Default output filename: <tool>-<version>-<os>-<arch>.plan.json

Examples:
  tsuku plan export gh
  tsuku plan export gh -o my-plan.json
  tsuku plan export gh -o -              # output to stdout`,
	Args: cobra.ExactArgs(1),
	Run:  runPlanExport,
}

var planExportOutput string

func init() {
	planCmd.AddCommand(planShowCmd)
	planCmd.AddCommand(planExportCmd)
	planShowCmd.Flags().BoolVar(&planShowJSON, "json", false, "Output in JSON format")
	planExportCmd.Flags().StringVarP(&planExportOutput, "output", "o", "", "Output file path (use '-' for stdout)")
}

func runPlanShow(cmd *cobra.Command, args []string) {
	toolName := args[0]

	// Load config
	cfg, err := config.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Load state
	mgr := install.New(cfg)
	toolState, err := mgr.GetState().GetToolState(toolName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load state: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Check if tool is installed
	if toolState == nil {
		fmt.Fprintf(os.Stderr, "Error: tool '%s' is not installed\n", toolName)
		fmt.Fprintf(os.Stderr, "\nTo install it:\n")
		fmt.Fprintf(os.Stderr, "  tsuku install %s\n", toolName)
		exitWithCode(ExitRecipeNotFound)
	}

	// Get active version
	version := toolState.ActiveVersion
	if version == "" {
		version = toolState.Version // Fallback for legacy state
	}

	// Get version state
	versionState, exists := toolState.Versions[version]
	if !exists {
		fmt.Fprintf(os.Stderr, "Error: no version state found for %s@%s\n", toolName, version)
		exitWithCode(ExitGeneral)
	}

	// Check if plan exists
	if versionState.Plan == nil {
		fmt.Fprintf(os.Stderr, "Error: no installation plan stored for %s@%s\n", toolName, version)
		fmt.Fprintf(os.Stderr, "\nThis tool was installed before plan storage was implemented.\n")
		fmt.Fprintf(os.Stderr, "To generate a plan, reinstall the tool:\n")
		fmt.Fprintf(os.Stderr, "  tsuku install %s --force\n", toolName)
		exitWithCode(ExitGeneral)
	}

	plan := versionState.Plan

	// JSON output
	if planShowJSON {
		printJSON(plan)
		return
	}

	// Human-readable output
	printPlanHuman(plan)
}

// printPlanHuman formats and prints a plan in human-readable format.
func printPlanHuman(plan *install.Plan) {
	// Header
	fmt.Printf("Plan for %s@%s\n", plan.Tool, plan.Version)
	fmt.Println()

	// Metadata
	fmt.Printf("Platform:      %s/%s\n", plan.Platform.OS, plan.Platform.Arch)
	fmt.Printf("Generated:     %s\n", plan.GeneratedAt.Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("Recipe:        %s\n", plan.RecipeSource)
	if plan.Deterministic {
		fmt.Printf("Deterministic: yes\n")
	} else {
		fmt.Printf("Deterministic: no (contains ecosystem primitives with residual non-determinism)\n")
	}
	fmt.Println()

	// Steps
	fmt.Printf("Steps (%d):\n", len(plan.Steps))
	for i, step := range plan.Steps {
		printStep(i+1, step)
	}
}

// printStep formats and prints a single plan step.
func printStep(num int, step install.PlanStep) {
	// Step header with determinism/evaluability indicator
	mark := ""
	if !step.Evaluable {
		mark = " (non-evaluable)"
	} else if !step.Deterministic {
		mark = " (non-deterministic)"
	}
	fmt.Printf("  %d. [%s]%s\n", num, step.Action, mark)

	// URL and checksum for download steps
	if step.URL != "" {
		fmt.Printf("     URL: %s\n", step.URL)
	}
	if step.Checksum != "" {
		fmt.Printf("     Checksum: %s\n", step.Checksum)
	}
	if step.Size > 0 {
		fmt.Printf("     Size: %s\n", formatBytes(step.Size))
	}

	// Key parameters (skip empty and already-shown fields)
	params := formatParams(step.Params)
	if params != "" {
		fmt.Printf("     Params: %s\n", params)
	}
}

// formatParams formats step parameters for display.
func formatParams(params map[string]interface{}) string {
	if len(params) == 0 {
		return ""
	}

	var parts []string
	for k, v := range params {
		// Skip url since it's shown separately
		if k == "url" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", k, formatValue(v)))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, ", ")
}

// formatValue formats a parameter value for display.
func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		if len(val) > 50 {
			return val[:47] + "..."
		}
		return val
	case []interface{}:
		if len(val) == 0 {
			return "[]"
		}
		// Show first few items
		var items []string
		for i, item := range val {
			if i >= 3 {
				items = append(items, fmt.Sprintf("...+%d more", len(val)-3))
				break
			}
			items = append(items, fmt.Sprintf("%v", item))
		}
		return "[" + strings.Join(items, ", ") + "]"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// getPlanForTool retrieves the installation plan for a tool.
// Returns the plan or exits with an appropriate error message.
func getPlanForTool(toolName string) *install.Plan {
	// Load config
	cfg, err := config.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Load state
	mgr := install.New(cfg)
	toolState, err := mgr.GetState().GetToolState(toolName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load state: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Check if tool is installed
	if toolState == nil {
		fmt.Fprintf(os.Stderr, "Error: tool '%s' is not installed\n", toolName)
		fmt.Fprintf(os.Stderr, "\nTo install it:\n")
		fmt.Fprintf(os.Stderr, "  tsuku install %s\n", toolName)
		exitWithCode(ExitRecipeNotFound)
	}

	// Get active version
	version := toolState.ActiveVersion
	if version == "" {
		version = toolState.Version // Fallback for legacy state
	}

	// Get version state
	versionState, exists := toolState.Versions[version]
	if !exists {
		fmt.Fprintf(os.Stderr, "Error: no version state found for %s@%s\n", toolName, version)
		exitWithCode(ExitGeneral)
	}

	// Check if plan exists
	if versionState.Plan == nil {
		fmt.Fprintf(os.Stderr, "Error: no installation plan stored for %s@%s\n", toolName, version)
		fmt.Fprintf(os.Stderr, "\nThis tool was installed before plan storage was implemented.\n")
		fmt.Fprintf(os.Stderr, "To generate a plan, reinstall the tool:\n")
		fmt.Fprintf(os.Stderr, "  tsuku install %s --force\n", toolName)
		exitWithCode(ExitGeneral)
	}

	return versionState.Plan
}

func runPlanExport(cmd *cobra.Command, args []string) {
	toolName := args[0]
	plan := getPlanForTool(toolName)

	// Determine output destination
	outputPath := planExportOutput
	if outputPath == "" {
		// Default filename: <tool>-<version>-<os>-<arch>.plan.json
		outputPath = defaultPlanFilename(plan)
	}

	// Handle stdout output
	if outputPath == "-" {
		printJSON(plan)
		return
	}

	// Write to file
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to encode plan: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to write file: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	fmt.Printf("Exported plan to %s\n", outputPath)
}

// defaultPlanFilename generates the default output filename for a plan.
func defaultPlanFilename(plan *install.Plan) string {
	return fmt.Sprintf("%s-%s-%s-%s.plan.json",
		plan.Tool, plan.Version, plan.Platform.OS, plan.Platform.Arch)
}
