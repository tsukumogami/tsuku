package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/version"
)

var validateCmd = &cobra.Command{
	Use:   "validate <recipe-file>",
	Short: "Validate a recipe file",
	Long: `Validate a recipe file without attempting to install it.

Checks include:
  - TOML syntax
  - Required fields (metadata.name, steps, verify.command)
  - Action type validation
  - Action parameter requirements
  - Security checks (URL schemes, path traversal)

Use --check-libc-coverage to validate that recipes have proper glibc/musl coverage:
  - Library recipes without musl support (and no explicit constraint) generate errors
  - Tool recipes with library deps but no musl path generate warnings

Examples:
  tsuku validate recipes/mytool.toml
  tsuku validate ~/.tsuku/recipes/custom-tool.toml --json
  tsuku validate recipes/mytool.toml --strict
  tsuku validate recipes/zlib.toml --check-libc-coverage`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		jsonOutput, _ := cmd.Flags().GetBool("json")
		strictMode, _ := cmd.Flags().GetBool("strict")
		checkLibcCoverage, _ := cmd.Flags().GetBool("check-libc-coverage")

		// Check file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			if jsonOutput {
				printJSON(recipe.ValidationResult{
					Valid: false,
					Errors: []recipe.ValidationError{
						{Message: fmt.Sprintf("file not found: %s", filePath)},
					},
				})
			} else {
				fmt.Printf("Error: file not found: %s\n", filePath)
			}
			exitWithCode(ExitGeneral)
			return
		}

		// Validate the recipe
		result := recipe.ValidateFile(filePath)

		// Check for shadowed dependencies
		if result.Recipe != nil {
			shadowed := actions.DetectShadowedDeps(result.Recipe)
			for _, dep := range shadowed {
				msg := fmt.Sprintf("dependency '%s' is already inherited from action '%s' (remove this redundant declaration)",
					dep.Name, dep.Source)
				result.Warnings = append(result.Warnings, recipe.ValidationWarning{
					Field:   "dependencies",
					Message: msg,
				})
			}
		}

		// Check for redundant version configuration
		if result.Recipe != nil {
			redundant := version.DetectRedundantVersion(result.Recipe)
			for _, r := range redundant {
				result.Warnings = append(result.Warnings, recipe.ValidationWarning{
					Field:   "version",
					Message: r.Message,
				})
			}
		}

		// Check for hardcoded versions in action parameters
		if result.Recipe != nil {
			hardcoded := recipe.DetectHardcodedVersions(result.Recipe)
			for _, h := range hardcoded {
				result.Warnings = append(result.Warnings, recipe.ValidationWarning{
					Field:   fmt.Sprintf("steps[%d].%s", h.Step-1, h.Field),
					Message: h.String(),
				})
			}
		}

		// Check for download_file with version when dynamic source is configured
		if result.Recipe != nil {
			mismatches := recipe.DetectDownloadFileVersionMismatch(result.Recipe)
			for _, m := range mismatches {
				result.Warnings = append(result.Warnings, recipe.ValidationWarning{
					Field:   fmt.Sprintf("steps[%d].url", m.Step-1),
					Message: m.String(),
				})
			}
		}

		// Check for libc coverage if requested
		if checkLibcCoverage && result.Recipe != nil {
			coverage := recipe.AnalyzeRecipeCoverage(result.Recipe)
			for _, err := range coverage.Errors {
				result.Errors = append(result.Errors, recipe.ValidationError{
					Field:   "coverage",
					Message: err,
				})
				result.Valid = false
			}
			for _, warn := range coverage.Warnings {
				result.Warnings = append(result.Warnings, recipe.ValidationWarning{
					Field:   "coverage",
					Message: warn,
				})
			}
		}

		// In strict mode, warnings are treated as errors
		if strictMode && len(result.Warnings) > 0 {
			result.Valid = false
		}

		// JSON output mode
		if jsonOutput {
			printJSON(result)
			if !result.Valid {
				exitWithCode(ExitGeneral)
			}
			return
		}

		// Human-readable output
		recipeName := filepath.Base(filePath)
		if result.Recipe != nil && result.Recipe.Metadata.Name != "" {
			recipeName = result.Recipe.Metadata.Name
		}

		if result.Valid {
			fmt.Printf("Valid recipe: %s\n", recipeName)

			// Show summary info
			if result.Recipe != nil {
				r := result.Recipe

				// Show actions
				if len(r.Steps) > 0 {
					actions := make([]string, 0, len(r.Steps))
					for _, step := range r.Steps {
						actions = append(actions, step.Action)
					}
					fmt.Printf("  Actions: %s\n", formatList(actions))
				}

				// Show binaries
				binaries := r.ExtractBinaries()
				if len(binaries) > 0 {
					fmt.Printf("  Binaries: %s\n", formatList(binaries))
				}

				// Show verification
				if r.Verify.Command != "" {
					fmt.Printf("  Verification: %s\n", r.Verify.Command)
				}

				// Show dependencies
				if len(r.Metadata.Dependencies) > 0 {
					fmt.Printf("  Dependencies: %s\n", formatList(r.Metadata.Dependencies))
				}
			}

			// Show warnings if any
			if len(result.Warnings) > 0 {
				fmt.Println()
				fmt.Println("Warnings:")
				for _, w := range result.Warnings {
					fmt.Printf("  - %s\n", w)
				}
			}
		} else {
			fmt.Printf("Invalid recipe: %s\n", recipeName)
			fmt.Println()

			// Show errors if any
			if len(result.Errors) > 0 {
				fmt.Println("Errors:")
				for _, e := range result.Errors {
					fmt.Printf("  - %s\n", e)
				}
			}

			// Show warnings (as errors if in strict mode)
			if len(result.Warnings) > 0 {
				if strictMode && len(result.Errors) == 0 {
					fmt.Println("Warnings (treated as errors in strict mode):")
				} else if len(result.Errors) > 0 {
					fmt.Println()
					fmt.Println("Warnings:")
				} else {
					fmt.Println("Warnings:")
				}
				for _, w := range result.Warnings {
					fmt.Printf("  - %s\n", w)
				}
			}

			exitWithCode(ExitGeneral)
		}
	},
}

// formatList joins a slice with commas
func formatList(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	if len(items) == 1 {
		return items[0]
	}
	result := items[0]
	for i := 1; i < len(items); i++ {
		result += ", " + items[i]
	}
	return result
}

func init() {
	validateCmd.Flags().Bool("json", false, "Output in JSON format")
	validateCmd.Flags().Bool("strict", false, "Treat warnings as errors")
	validateCmd.Flags().Bool("check-libc-coverage", false, "Check for glibc/musl platform coverage")
}
