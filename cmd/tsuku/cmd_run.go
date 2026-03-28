package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/tsukumogami/tsuku/internal/autoinstall"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
	"github.com/tsukumogami/tsuku/internal/project"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

var runModeFlag string

var runCmd = &cobra.Command{
	Use:   "run <command> [args...]",
	Short: "Run a command, installing it first if needed",
	Long: `Run a command, installing it first if needed.

Looks up the command in the binary index. If the tool is not installed,
tsuku installs it according to the consent mode and then hands off
execution via process replacement. The tool's exit code propagates directly.

Use -- to separate tsuku flags from the target command's flags:
  tsuku run jq -- --arg foo bar

Consent modes:
  suggest   Print install instructions and exit (no install)
  confirm   Prompt before installing (default, requires TTY)
  auto      Install silently with audit logging (requires opt-in)

Mode resolution order:
  1. --mode flag
  2. TSUKU_AUTO_INSTALL_MODE environment variable
  3. auto_install_mode config key ($TSUKU_HOME/config.toml)
  4. Default: confirm

Exit codes:
  0   Command executed successfully
  1   No match found or other error
  11  Binary index not built
  12  Confirm mode requires a TTY
  13  User declined installation
  14  Forbidden (e.g., running as root)`,
	Args:                  cobra.MinimumNArgs(1),
	DisableFlagParsing:    false,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		command := args[0]
		commandArgs := args[1:]

		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		userCfg, err := userconfig.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load user config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		mode, err := resolveMode(runModeFlag, userCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tsuku run: %v\n", err)
			exitWithCode(ExitUsage)
		}

		// Load project config from working directory. Errors are ignored;
		// a nil result means no .tsuku.toml was found.
		cwd, _ := os.Getwd()
		projectCfg, _ := project.LoadProjectConfig(cwd)

		indexLookup := func(ctx context.Context, cmd string) ([]index.BinaryMatch, error) {
			return lookupBinaryCommand(ctx, cfg, cmd)
		}
		resolver := project.NewResolver(projectCfg, indexLookup)

		// TTY gate: confirm mode requires an interactive terminal.
		// Project-declared tools bypass this gate because the mode override
		// in Runner.Run will escalate to auto before any prompt is shown.
		hasProjectTools := projectCfg != nil && len(projectCfg.Config.Tools) > 0
		if mode == autoinstall.ModeConfirm && !hasProjectTools && !term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprintln(os.Stderr, "tsuku: confirm mode requires a TTY; set TSUKU_AUTO_INSTALL_MODE=auto or use --mode=auto for non-interactive use")
			exitWithCode(ExitNotInteractive)
		}

		runner := autoinstall.NewRunner(cfg, os.Stdout, os.Stderr)
		runner.Lookup = indexLookup
		runner.Installer = &runInstaller{}
		runner.Exec = func(binary string, execArgs []string, env []string) error {
			return syscall.Exec(binary, execArgs, env)
		}
		runner.RecipeHasVerification = func(recipeName string) bool {
			r, loadErr := loader.Get(recipeName, recipe.LoaderOptions{})
			if loadErr != nil {
				return false
			}
			return r.HasChecksumVerification()
		}

		runErr := runner.Run(globalCtx, command, commandArgs, mode, resolver)
		if runErr == nil {
			return
		}

		switch {
		case errors.Is(runErr, autoinstall.ErrIndexNotBuilt):
			exitWithCode(ExitIndexNotBuilt)
		case errors.Is(runErr, autoinstall.ErrForbidden):
			fmt.Fprintf(os.Stderr, "tsuku run: %v\n", runErr)
			exitWithCode(ExitForbidden)
		case errors.Is(runErr, autoinstall.ErrUserDeclined):
			exitWithCode(ExitUserDeclined)
		case errors.Is(runErr, autoinstall.ErrSuggestOnly):
			exitWithCode(ExitGeneral)
		case errors.Is(runErr, autoinstall.ErrNoMatch):
			exitWithCode(ExitGeneral)
		default:
			fmt.Fprintf(os.Stderr, "tsuku run: %v\n", runErr)
			exitWithCode(ExitGeneral)
		}
	},
}

// runInstaller wraps the existing install pipeline for use by autoinstall.Runner.
type runInstaller struct{}

func (i *runInstaller) Install(ctx context.Context, recipeName, version string) error {
	_ = ctx // install pipeline uses globalCtx internally
	return runInstallWithTelemetry(recipeName, version, version, true, "", nil)
}

// resolveMode applies the four-step priority chain to determine the active
// consent mode: flag > env var > config > default (confirm).
//
// The env var escalation restriction prevents TSUKU_AUTO_INSTALL_MODE=auto
// from taking effect unless the config also has auto_install_mode = "auto".
// This blocks malicious .envrc files from silently enabling auto mode.
func resolveMode(flagMode string, cfg *userconfig.Config) (autoinstall.Mode, error) {
	// Step 1: explicit flag wins unconditionally.
	if flagMode != "" {
		m, ok := autoinstall.ParseMode(flagMode)
		if !ok {
			return 0, fmt.Errorf("invalid mode %q: must be suggest, confirm, or auto", flagMode)
		}
		return m, nil
	}

	// Step 2: environment variable.
	envMode := os.Getenv("TSUKU_AUTO_INSTALL_MODE")
	if envMode != "" {
		m, ok := autoinstall.ParseMode(envMode)
		if !ok {
			return 0, fmt.Errorf("invalid TSUKU_AUTO_INSTALL_MODE %q: must be suggest, confirm, or auto", envMode)
		}

		// Escalation restriction: env var cannot escalate to auto unless
		// the persistent config also has auto_install_mode = "auto".
		if m == autoinstall.ModeAuto && cfg.AutoInstallMode != "auto" {
			return autoinstall.ModeConfirm, nil
		}

		return m, nil
	}

	// Step 3: config file value.
	if cfg.AutoInstallMode != "" {
		m, ok := autoinstall.ParseMode(cfg.AutoInstallMode)
		if !ok {
			return 0, fmt.Errorf("invalid auto_install_mode config %q: must be suggest, confirm, or auto", cfg.AutoInstallMode)
		}
		return m, nil
	}

	// Step 4: default.
	return autoinstall.ModeConfirm, nil
}

func init() {
	runCmd.Flags().StringVar(&runModeFlag, "mode", "", "Consent mode: suggest, confirm, or auto")
}
