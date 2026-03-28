package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/shellenv"
)

var shellFlag string

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Print shell exports to activate project tools in PATH",
	Long: `Print shell export statements that activate the tools declared in the
nearest .tsuku.toml. Intended for use with eval:

  eval $(tsuku shell)

This sets PATH, _TSUKU_DIR, and _TSUKU_PREV_PATH so that project-specific
tool versions are available in the current shell.

Re-running in the same shell reuses _TSUKU_PREV_PATH as the base PATH,
so activations don't stack.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}

		cfg, err := config.DefaultConfig()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		prevPath := os.Getenv("_TSUKU_PREV_PATH")
		shell := detectShell(shellFlag)

		output, err := runShell(cwd, prevPath, shell, cfg)
		if err != nil {
			return err
		}

		if output == "" {
			fmt.Fprintln(os.Stderr, "tsuku shell: no .tsuku.toml found in current directory or parents")
			exitWithCode(ExitGeneral)
		}

		fmt.Print(output)
		return nil
	},
}

func init() {
	shellCmd.Flags().StringVar(&shellFlag, "shell", "", "Shell format for output (bash, zsh, fish). Auto-detected from $SHELL if omitted.")
}

// detectShell returns the shell name to use for formatting exports.
// Priority: explicit flag > $SHELL env var > "bash" default.
func detectShell(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if s := os.Getenv("SHELL"); s != "" {
		return filepath.Base(s)
	}
	return "bash"
}

// runShell computes the activation for cwd and returns formatted shell export
// statements. Returns ("", nil) when no .tsuku.toml is found, allowing the
// caller to decide how to handle that case.
func runShell(cwd, prevPath, shell string, cfg *config.Config) (string, error) {
	// Pass empty curDir to force activation (no early-exit on same directory).
	result, err := shellenv.ComputeActivation(cwd, prevPath, "", cfg)
	if err != nil {
		return "", err
	}

	if result == nil {
		return "", nil
	}

	return shellenv.FormatExports(result, shell), nil
}
