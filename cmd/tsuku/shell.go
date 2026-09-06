package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/activation"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
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
	// Pass an empty curDir to force activation: tsuku shell is an explicit
	// invocation and must resolve and emit whatever the environment holds.
	//
	// The stamp is passed empty for the same reason, and reading
	// _TSUKU_STATE_STAMP here the way hook-env does would be a bug rather than
	// symmetry: with _TSUKU_DIR already at the current directory and a matching
	// stamp, ComputeActivation would return nil, runShell would turn that into
	// an empty string, and the command would report no .tsuku.toml found and
	// exit non-zero for a project that exists and is perfectly valid.
	result, err := activation.ComputeActivation(cwd, prevPath, "", "", cfg, install.NewStateManager(cfg))
	if err != nil {
		return "", err
	}

	if result == nil {
		return "", nil
	}

	// Reasons to stderr, shell code to stdout. tsuku shell passes an empty
	// curDir above, so Entered is always true here: an explicit invocation
	// reports every time, and only the prompt hook is on a budget.
	reportActivation(result)

	return activation.FormatExports(result, shell), nil
}
