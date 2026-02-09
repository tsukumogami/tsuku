package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/executor"
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// DepsOutput represents the JSON output for the deps command.
type DepsOutput struct {
	Packages []string `json:"packages"`
	Family   string   `json:"family,omitempty"`
}

var depsCmd = &cobra.Command{
	Use:   "deps <tool> | --recipe <path>",
	Short: "Show dependencies for a tool",
	Long: `Show dependencies for a tool, optionally filtered by type and platform.

By default, shows all tsuku-managed dependencies. Use --system to show
only system package dependencies (apt, apk, dnf, etc.).

Examples:
  # Show all dependencies
  tsuku deps zlib

  # Show system packages for Alpine Linux
  tsuku deps --system --family alpine zlib

  # Output as JSON
  tsuku deps --system --family alpine --format json zlib

  # Use with package managers
  apk add $(tsuku deps --system --family alpine zlib)`,
	Args: cobra.MaximumNArgs(1),
	Run:  runDeps,
}

func init() {
	depsCmd.Flags().Bool("system", false, "Show only system package dependencies (apt, apk, dnf, etc.)")
	depsCmd.Flags().String("family", "", "Target Linux family (alpine, debian, rhel, arch, suse)")
	depsCmd.Flags().String("format", "text", "Output format (text, json)")
	depsCmd.Flags().String("recipe", "", "Path to local recipe file")
}

func runDeps(cmd *cobra.Command, args []string) {
	systemOnly, _ := cmd.Flags().GetBool("system")
	family, _ := cmd.Flags().GetString("family")
	format, _ := cmd.Flags().GetString("format")
	recipePath, _ := cmd.Flags().GetString("recipe")

	// Validate arguments: tool name XOR --recipe
	if recipePath != "" && len(args) > 0 {
		printError(fmt.Errorf("cannot specify both --recipe and a tool name"))
		exitWithCode(ExitUsage)
	}
	if recipePath == "" && len(args) == 0 {
		printError(fmt.Errorf("must specify either a tool name or --recipe flag"))
		exitWithCode(ExitUsage)
	}

	// Validate format
	if format != "text" && format != "json" {
		printError(fmt.Errorf("invalid format %q, must be 'text' or 'json'", format))
		exitWithCode(ExitUsage)
	}

	// Validate family if provided
	validFamilies := []string{"alpine", "debian", "rhel", "arch", "suse"}
	if family != "" {
		valid := false
		for _, f := range validFamilies {
			if f == family {
				valid = true
				break
			}
		}
		if !valid {
			printError(fmt.Errorf("invalid family %q, must be one of: %s", family, strings.Join(validFamilies, ", ")))
			exitWithCode(ExitUsage)
		}
	}

	// Load recipe from registry or file
	var r *recipe.Recipe
	var err error

	if recipePath != "" {
		r, err = loadLocalRecipe(recipePath)
		if err != nil {
			printError(fmt.Errorf("failed to load recipe from %s: %w", recipePath, err))
			exitWithCode(ExitGeneral)
		}
	} else {
		toolName := args[0]
		r, err = loader.Get(toolName, recipe.LoaderOptions{})
		if err != nil {
			printError(fmt.Errorf("tool '%s' not found in registry", toolName))
			exitWithCode(ExitRecipeNotFound)
		}
	}

	// Build target from flags
	target := buildTargetFromFlags(family)

	// Get packages based on mode
	var packages []string
	if systemOnly {
		packages = extractSystemPackages(r, target)
	} else {
		// For non-system mode, get tsuku-managed dependencies
		deps := actions.ResolveDependencies(r)
		for name := range deps.InstallTime {
			packages = append(packages, name)
		}
		for name := range deps.Runtime {
			// Avoid duplicates
			found := false
			for _, p := range packages {
				if p == name {
					found = true
					break
				}
			}
			if !found {
				packages = append(packages, name)
			}
		}
	}

	// Output
	if format == "json" {
		output := DepsOutput{
			Packages: packages,
		}
		if family != "" {
			output.Family = family
		}
		jsonBytes, err := json.Marshal(output)
		if err != nil {
			printError(fmt.Errorf("failed to marshal JSON: %w", err))
			exitWithCode(ExitGeneral)
		}
		fmt.Println(string(jsonBytes))
	} else {
		// Text output: one package per line for easy shell consumption
		for _, pkg := range packages {
			fmt.Println(pkg)
		}
	}
}

// buildTargetFromFlags creates a platform.Target from command flags.
// If family is empty, uses the current platform.
func buildTargetFromFlags(family string) platform.Target {
	os := runtime.GOOS
	arch := runtime.GOARCH
	platformStr := os + "/" + arch

	// If family is specified, assume Linux
	if family != "" {
		os = "linux"
		platformStr = "linux/" + arch
	}

	// Derive libc from family
	libc := ""
	if os == "linux" {
		if family != "" {
			libc = platform.LibcForFamily(family)
		} else {
			libc = platform.DetectLibc()
		}
	}

	return platform.NewTarget(platformStr, family, libc)
}

// extractSystemPackages extracts system package names from a recipe for a target.
// Returns package names from system dependency actions (apk_install, apt_install, etc.)
// that match the target platform.
func extractSystemPackages(r *recipe.Recipe, target platform.Target) []string {
	// Filter steps for target
	filtered := executor.FilterStepsByTarget(r.Steps, target)

	// Extract packages from system dependency steps
	var packages []string
	for _, step := range filtered {
		if !systemActionNames[step.Action] {
			continue
		}

		// Extract packages from step params
		pkgs, ok := actions.GetStringSlice(step.Params, "packages")
		if ok {
			packages = append(packages, pkgs...)
		}
	}

	return packages
}
