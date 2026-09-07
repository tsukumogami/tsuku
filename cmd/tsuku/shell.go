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

This sets PATH, _TSUKU_DIR, _TSUKU_PREV_PATH and _TSUKU_STATE_STAMP so that
project-specific tool versions are available in the current shell.

Re-running in the same shell reuses _TSUKU_PREV_PATH as the base PATH,
so activations don't stack.

A declaration that cannot be honored is reported on stderr with the reason;
stdout carries only the shell code. Use --quiet to suppress the reasons.`,
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
			// An unparseable .tsuku.toml is a diagnostic, not a command
			// failure: one line, no usage block, exit 0. tsuku shell records
			// nothing on this path -- it is one-shot and does not own the
			// prompt hook's tracking variables -- so it emits no shell code
			// either, and must not fall through to the no-project branch below.
			if line := parseDiagnostic(err); line != "" {
				printWarning(line)
				return nil
			}
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
		// The caller distinguishes a parse failure, which is a diagnostic, from
		// a real error. Reporting it here would put the message on stderr even
		// when the caller has decided to do something else with it.
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
