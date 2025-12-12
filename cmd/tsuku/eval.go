package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/executor"
)

// Platform flag validation whitelists per DESIGN-installation-plans-eval.md
var validOSValues = map[string]bool{
	"linux":   true,
	"darwin":  true,
	"windows": true,
	"freebsd": true,
}

var validArchValues = map[string]bool{
	"amd64": true,
	"arm64": true,
	"386":   true,
	"arm":   true,
}

var evalOS string
var evalArch string

var evalCmd = &cobra.Command{
	Use:   "eval <tool>[@version]",
	Short: "Generate an installation plan for a tool",
	Long: `Generate a deterministic installation plan for a tool and output it as JSON.

The plan captures fully-resolved URLs, computed checksums, and all steps
needed to reproduce the installation. This enables:
  - Recipe testing via JSON comparison
  - Audit trails of what would be downloaded
  - Cross-platform plan generation

By default, plans are generated for the current platform. Use --os and --arch
to generate plans for other platforms.

Examples:
  tsuku eval kubectl
  tsuku eval kubectl@v1.29.0
  tsuku eval ripgrep --os linux --arch arm64`,
	Args: cobra.ExactArgs(1),
	Run:  runEval,
}

func init() {
	evalCmd.Flags().StringVar(&evalOS, "os", "", "Target operating system (linux, darwin, windows, freebsd)")
	evalCmd.Flags().StringVar(&evalArch, "arch", "", "Target architecture (amd64, arm64, 386, arm)")
}

// ValidateOS validates an OS value against the whitelist.
// Returns an error if the value is invalid.
func ValidateOS(os string) error {
	if os == "" {
		return nil // Empty is valid (uses runtime default)
	}
	if !validOSValues[os] {
		return fmt.Errorf("invalid OS value %q: must be one of linux, darwin, windows, freebsd", os)
	}
	return nil
}

// ValidateArch validates an architecture value against the whitelist.
// Returns an error if the value is invalid.
func ValidateArch(arch string) error {
	if arch == "" {
		return nil // Empty is valid (uses runtime default)
	}
	if !validArchValues[arch] {
		return fmt.Errorf("invalid arch value %q: must be one of amd64, arm64, 386, arm", arch)
	}
	return nil
}

func runEval(cmd *cobra.Command, args []string) {
	// Parse tool name and version
	toolName := args[0]
	reqVersion := ""
	if strings.Contains(toolName, "@") {
		parts := strings.SplitN(toolName, "@", 2)
		toolName = parts[0]
		reqVersion = parts[1]
	}

	// Convert "latest" to empty for resolution
	if reqVersion == "latest" {
		reqVersion = ""
	}

	// Validate platform flags
	if err := ValidateOS(evalOS); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitWithCode(ExitUsage)
	}
	if err := ValidateArch(evalArch); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitWithCode(ExitUsage)
	}

	// Load recipe
	r, err := loader.Get(toolName)
	if err != nil {
		printError(err)
		fmt.Fprintf(os.Stderr, "\nTo create a recipe from a package ecosystem:\n")
		fmt.Fprintf(os.Stderr, "  tsuku create %s --from <ecosystem>\n", toolName)
		fmt.Fprintf(os.Stderr, "\nAvailable ecosystems: crates.io, rubygems, pypi, npm\n")
		exitWithCode(ExitRecipeNotFound)
	}

	// Create executor
	var exec *executor.Executor
	if reqVersion != "" {
		exec, err = executor.NewWithVersion(r, reqVersion)
	} else {
		exec, err = executor.New(r)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create executor: %v\n", err)
		exitWithCode(ExitGeneral)
	}
	defer exec.Cleanup()

	// Configure plan generation
	cfg := executor.PlanConfig{
		OS:           evalOS,
		Arch:         evalArch,
		RecipeSource: "registry",
		OnWarning: func(action, message string) {
			// Output warnings to stderr so they don't mix with JSON
			fmt.Fprintf(os.Stderr, "Warning: %s\n", message)
		},
	}

	// Generate plan
	plan, err := exec.GeneratePlan(globalCtx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to generate plan: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Output JSON to stdout
	printJSON(plan)
}
