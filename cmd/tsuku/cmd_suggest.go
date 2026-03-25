package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
)

// suggestJSONOutput is the machine-readable output for tsuku suggest --json.
type suggestJSONOutput struct {
	Command string             `json:"command"`
	Matches []suggestJSONMatch `json:"matches"`
}

// suggestJSONMatch represents a single recipe match in JSON output.
type suggestJSONMatch struct {
	Recipe     string `json:"recipe"`
	BinaryPath string `json:"binary_path"`
	Installed  bool   `json:"installed"`
}

var suggestJSONFlag bool

var suggestCmd = &cobra.Command{
	Use:   "suggest <command>",
	Short: "Suggest recipes that provide a command",
	Long: `Suggest recipes that provide a command.

Looks up the binary index to find which recipe(s) install the given command
and prints install suggestions. This command is network-free and reads only
the local binary index.

Used by shell command-not-found hooks to suggest install actions when a
command is not found. The index must be built first by running
'tsuku update-registry'.

Exit codes:
  0  One or more matches found
  1  No match found or other error
  11 Binary index not built

Examples:
  tsuku suggest jq
  tsuku suggest kubectl --json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		command := args[0]

		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		code := runSuggest(globalCtx, os.Stdout, os.Stderr, cfg, command, suggestJSONFlag)
		if code != ExitSuccess {
			exitWithCode(code)
		}
	},
}

// runSuggest implements the suggest command logic with injectable writers.
// It returns an exit code: ExitSuccess (0), ExitGeneral (1), or ExitIndexNotBuilt (11).
func runSuggest(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config, command string, jsonOutput bool) int {
	matches, err := lookupBinaryCommand(ctx, cfg, command)
	if err != nil {
		if errors.Is(err, index.ErrIndexNotBuilt) {
			if jsonOutput {
				printSuggestJSON(stdout, command, nil)
			}
			return ExitIndexNotBuilt
		}
		// StaleIndexWarning: results are still valid; continue with a warning.
		var stale index.StaleIndexWarning
		if !errors.As(err, &stale) {
			return ExitGeneral
		}
		fmt.Fprintf(stderr, "Warning: %v\n", err)
	}

	if jsonOutput {
		printSuggestJSON(stdout, command, matches)
		if len(matches) == 0 {
			return ExitGeneral
		}
		return ExitSuccess
	}

	if len(matches) == 0 {
		return ExitGeneral
	}

	if len(matches) == 1 {
		fmt.Fprintf(stdout, "Command '%s' not found. Install with: tsuku install %s\n", command, matches[0].Recipe)
		return ExitSuccess
	}

	// Multiple matches: print a list with installed status.
	fmt.Fprintf(stdout, "Command '%s' not found. Provided by:\n", command)
	for _, m := range matches {
		if m.Installed {
			fmt.Fprintf(stdout, "  %s (installed)   tsuku install %s\n", m.Recipe, m.Recipe)
		} else {
			fmt.Fprintf(stdout, "  %s   tsuku install %s\n", m.Recipe, m.Recipe)
		}
	}
	return ExitSuccess
}

// printSuggestJSON writes the JSON output for tsuku suggest --json.
// matches may be nil; the output will have an empty array, never null.
func printSuggestJSON(w io.Writer, command string, matches []index.BinaryMatch) {
	out := suggestJSONOutput{
		Command: command,
		Matches: make([]suggestJSONMatch, 0, len(matches)),
	}
	for _, m := range matches {
		out.Matches = append(out.Matches, suggestJSONMatch{
			Recipe:     m.Recipe,
			BinaryPath: m.BinaryPath,
			Installed:  m.Installed,
		})
	}
	data, err := json.Marshal(out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal JSON: %v\n", err)
		return
	}
	_, _ = fmt.Fprintln(w, string(data))
}

func init() {
	suggestCmd.Flags().BoolVar(&suggestJSONFlag, "json", false, "Output machine-readable JSON")
}
