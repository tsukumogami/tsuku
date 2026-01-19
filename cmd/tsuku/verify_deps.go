package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// CommandCheck represents the result of a require_command check.
type CommandCheck struct {
	Command    string `json:"command"`
	Status     string `json:"status"` // "pass", "fail", "version_mismatch"
	Path       string `json:"path,omitempty"`
	Version    string `json:"version,omitempty"`
	MinVersion string `json:"min_version,omitempty"`
	Error      string `json:"error,omitempty"`
}

// VerifyDepsOutput represents the JSON output structure.
type VerifyDepsOutput struct {
	Recipe   string         `json:"recipe"`
	Platform string         `json:"platform"`
	Checks   []CommandCheck `json:"checks"`
	AllPass  bool           `json:"all_pass"`
}

var verifyDepsCmd = &cobra.Command{
	Use:   "verify-deps <recipe>",
	Short: "Verify system dependencies for a recipe",
	Long: `Verify that all system dependencies (require_command actions) are satisfied.

This command loads a recipe, filters it for the current platform, and checks
all require_command steps. Each required command is checked for existence in
PATH, and optionally for version requirements.

Exit codes:
  0 - All required commands are present and meet version requirements
  1 - One or more required commands are missing or fail version checks

Examples:
  tsuku verify-deps docker
  tsuku verify-deps --json gh`,
	Args: cobra.ExactArgs(1),
	Run:  runVerifyDeps,
}

func init() {
	verifyDepsCmd.Flags().Bool("json", false, "Output in JSON format")
	rootCmd.AddCommand(verifyDepsCmd)
}

func runVerifyDeps(cmd *cobra.Command, args []string) {
	recipeName := args[0]
	jsonOutput, _ := cmd.Flags().GetBool("json")

	// Load recipe
	r, err := loader.Get(recipeName, recipe.LoaderOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Recipe '%s' not found in registry.\n", recipeName)
		exitWithCode(ExitRecipeNotFound)
	}

	// Detect current target
	target, err := platform.DetectTarget()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to detect platform: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Filter steps for current target
	filteredSteps := executor.FilterStepsByTarget(r.Steps, target)

	// Find require_command steps
	var checks []CommandCheck
	allPass := true

	for _, step := range filteredSteps {
		if step.Action != "require_command" {
			continue
		}

		check := verifyCommand(globalCtx, step.Params)
		checks = append(checks, check)
		if check.Status != "pass" {
			allPass = false
		}
	}

	// Output results
	if jsonOutput {
		output := VerifyDepsOutput{
			Recipe:   recipeName,
			Platform: target.Platform,
			Checks:   checks,
			AllPass:  allPass,
		}
		printVerifyDepsJSON(output)
	} else {
		printVerifyDepsReport(recipeName, target.Platform, checks, allPass)
	}

	if !allPass {
		exitWithCode(ExitDependencyFailed)
	}
}

// verifyCommand checks if a required command exists and optionally verifies its version.
func verifyCommand(ctx context.Context, params map[string]interface{}) CommandCheck {
	command, _ := actions.GetString(params, "command")
	if command == "" {
		return CommandCheck{
			Command: "(empty)",
			Status:  "fail",
			Error:   "missing command parameter",
		}
	}

	check := CommandCheck{
		Command: command,
	}

	// Check if command exists
	path, err := exec.LookPath(command)
	if err != nil {
		check.Status = "fail"
		check.Error = "command not found in PATH"
		return check
	}
	check.Path = path

	// Check version if min_version is specified
	minVersion, hasMinVersion := actions.GetString(params, "min_version")
	if hasMinVersion && minVersion != "" {
		check.MinVersion = minVersion

		versionFlag, _ := actions.GetString(params, "version_flag")
		versionRegex, _ := actions.GetString(params, "version_regex")

		if versionFlag == "" || versionRegex == "" {
			check.Status = "pass"
			check.Error = "min_version specified but version_flag or version_regex missing"
			return check
		}

		// Run command with version flag
		cmdExec := exec.CommandContext(ctx, command, versionFlag)
		output, err := cmdExec.CombinedOutput()
		if err != nil {
			check.Status = "fail"
			check.Error = fmt.Sprintf("failed to get version: %v", err)
			return check
		}

		// Extract version using regex
		re, err := regexp.Compile(versionRegex)
		if err != nil {
			check.Status = "fail"
			check.Error = fmt.Sprintf("invalid version_regex: %v", err)
			return check
		}

		matches := re.FindStringSubmatch(string(output))
		if len(matches) < 2 {
			check.Status = "fail"
			check.Error = "could not extract version from output"
			return check
		}

		check.Version = strings.TrimSpace(matches[1])

		// Compare versions
		if !versionSatisfiesMinimum(check.Version, minVersion) {
			check.Status = "version_mismatch"
			check.Error = fmt.Sprintf("version %s does not meet minimum %s", check.Version, minVersion)
			return check
		}
	}

	check.Status = "pass"
	return check
}

// versionSatisfiesMinimum checks if detected version meets the minimum requirement.
// Uses simple numeric comparison of version parts.
func versionSatisfiesMinimum(detected, minimum string) bool {
	// Strip common prefixes
	detected = strings.TrimPrefix(detected, "v")
	minimum = strings.TrimPrefix(minimum, "v")

	detectedParts := strings.Split(detected, ".")
	minimumParts := strings.Split(minimum, ".")

	for i := 0; i < len(minimumParts); i++ {
		if i >= len(detectedParts) {
			return false
		}

		var detNum, minNum int
		if _, err := fmt.Sscanf(detectedParts[i], "%d", &detNum); err == nil {
			if _, err := fmt.Sscanf(minimumParts[i], "%d", &minNum); err == nil {
				if detNum < minNum {
					return false
				}
				if detNum > minNum {
					return true
				}
				continue
			}
		}

		// Fall back to string comparison
		if detectedParts[i] < minimumParts[i] {
			return false
		}
		if detectedParts[i] > minimumParts[i] {
			return true
		}
	}

	return true
}

// printVerifyDepsJSON outputs results in JSON format.
func printVerifyDepsJSON(output VerifyDepsOutput) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(output)
}

// printVerifyDepsReport outputs a human-readable report.
func printVerifyDepsReport(recipeName, platform string, checks []CommandCheck, allPass bool) {
	if len(checks) == 0 {
		fmt.Printf("Recipe '%s' has no require_command steps for %s\n", recipeName, platform)
		return
	}

	fmt.Printf("%sVerifying system dependencies for %s%s (%s)\n\n", colorBold, recipeName, colorReset, platform)

	for _, check := range checks {
		var statusColor, statusText string
		switch check.Status {
		case "pass":
			statusColor = colorGreen
			if check.Version != "" {
				statusText = fmt.Sprintf("OK (%s)", check.Version)
			} else {
				statusText = "OK"
			}
		case "fail":
			statusColor = colorRed
			statusText = "MISSING"
		case "version_mismatch":
			statusColor = colorYellow
			statusText = fmt.Sprintf("VERSION MISMATCH (found %s, need %s)", check.Version, check.MinVersion)
		}

		fmt.Printf("  %s%-20s%s %s%s%s", colorBold, check.Command, colorReset, statusColor, statusText, colorReset)
		if check.Path != "" && check.Status == "pass" {
			fmt.Printf(" (%s)", check.Path)
		}
		fmt.Println()

		if check.Error != "" && check.Status != "pass" {
			fmt.Printf("    %s\n", check.Error)
		}
	}

	fmt.Println()
	if allPass {
		fmt.Printf("%sAll system dependencies satisfied%s\n", colorGreen, colorReset)
	} else {
		fmt.Printf("%sSome system dependencies are missing or have version issues%s\n", colorRed, colorReset)
	}
}
