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
	"github.com/tsukumogami/tsuku/internal/installevents"
	"github.com/tsukumogami/tsuku/internal/project"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/updates"
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

Project configuration:
  When a .tsuku.toml file exists in the current directory (or a parent),
  tsuku run checks it for the tool being executed. If the tool is declared,
  the project-pinned version is installed and used automatically -- no
  confirmation prompt needed. The project config is treated as consent.

  Tools not declared in .tsuku.toml fall through to the normal consent mode.

Consent modes (for tools not in .tsuku.toml):
  suggest   Print install instructions and exit (no install)
  confirm   Prompt before installing (default, requires TTY)
  auto      Install silently with audit logging (requires opt-in)

Mode resolution order:
  1. .tsuku.toml declares the tool -> auto (project config is consent)
  2. --mode flag
  3. TSUKU_AUTO_INSTALL_MODE environment variable
  4. auto_install_mode config key ($TSUKU_HOME/config.toml)
  5. Default: confirm

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

		// Trigger background update check (best-effort)
		updates.CheckAndSpawnUpdateCheck(cfg, userCfg)

		mode, err := resolveMode(runModeFlag, userCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tsuku run: %v\n", err)
			exitWithCode(ExitUsage)
		}

		cwd, _ := os.Getwd()
		wiring := newRunWiring(cfg, cwd)

		// TTY gate: confirm mode requires an interactive terminal.
		//
		// The skip is keyed on the configuration declaring *any* tool, not on
		// it declaring this command, and both halves of that are wrong. A
		// command nothing declares still skips the gate in a repository that
		// declares something else, and then meets the prompt at a closed stdin
		// and exits 13 rather than 12. A command that is declared can be
		// lowered back to confirm by a gate inside Run -- an unverified recipe
		// is the ordinary way -- so "no prompt is shown" is not something this
		// check can know from here. Both follow from the check running before
		// the declaration is resolved and before the gates, which is why
		// moving it inside Run is what fixes it, and a separate unit's work.
		hasProjectTools := wiring.projectCfg != nil && len(wiring.projectCfg.Config.Tools) > 0
		if mode == autoinstall.ModeConfirm && !hasProjectTools && !term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprintln(os.Stderr, "tsuku: confirm mode requires a TTY; set TSUKU_AUTO_INSTALL_MODE=auto or use --mode=auto for non-interactive use")
			exitWithCode(ExitNotInteractive)
		}

		runner := autoinstall.NewRunner(cfg, os.Stdout, os.Stderr)
		runner.Lookup = wiring.lookup
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

		runErr := runner.Run(globalCtx, command, commandArgs, mode, wiring.resolver)
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

// runWiring is everything `tsuku run` builds between the working directory and
// the runner: the project configuration discovered by walking up from cwd, the
// declaration resolver over it, and the lookup the runner resolves commands
// with.
//
// It is a named function rather than four lines inside the command because the
// properties that can go wrong here live in the joining and in neither package
// it joins -- whether the resolver the runner is handed is the one built from
// the discovered file, and whether the index is opened once per run or twice.
// Inline, the only way to exercise those was to run the command, which ends in
// exitWithCode or syscall.Exec and so cannot be driven in process. A test that
// reconstructed the same four lines would pass while this function was wrong,
// which is the whole reason it is a function.
type runWiring struct {
	// projectCfg is nil when no .tsuku.toml was found on the walk.
	projectCfg *project.ConfigResult
	resolver   *project.Resolver
	lookup     autoinstall.LookupFunc
}

// newRunWiring discovers the project configuration from cwd and builds what
// the runner needs from it. A config that fails to load is treated as no
// config: the run continues under the consent mode alone.
func newRunWiring(cfg *config.Config, cwd string) runWiring {
	// The load error stays discarded: this path falls back to a non-project
	// install when there is no usable config. The diagnostics do not -- a
	// declaration refused at parse time has to be visible from every command
	// that reads the file, and this route previously said nothing at all.
	//
	// This call arrived on main inline in the RunE body. It belongs here now
	// because that body's project wiring was extracted into this function, and
	// splitting the two would leave the diagnostics reporting on one path and
	// the resolver on another.
	projectCfg, _ := loadProjectConfigReporting(cwd)
	return runWiring{
		projectCfg: projectCfg,
		resolver:   project.NewResolver(projectCfg),
		lookup: func(ctx context.Context, command string) ([]index.BinaryMatch, error) {
			return binaryCommandLookup(ctx, cfg, command)
		},
	}
}

// runInstaller wraps the existing install pipeline for use by autoinstall.Runner.
type runInstaller struct{}

func (i *runInstaller) Install(ctx context.Context, recipeName, version string) error {
	// `tsuku run` triggers auto-installation when the project config (or the
	// user-elected consent mode) approves it. Tag every event published by
	// the install pipeline with SourceProjectAuto.
	return runInstall(installevents.WithSource(ctx, installevents.SourceProjectAuto), installArgs{
		Tool:              recipeName,
		ReqVersion:        version,
		VersionConstraint: version,
		IsExplicit:        true,
	})
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
