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
  tsuku run checks it for the tool being executed. If the tool is declared
  and you have not chosen a consent mode yourself, the project-pinned
  version is installed without a prompt: with nothing else to go on, the
  declaration is taken as consent.

  A mode you set is honored as given. Declaring a tool does not override a
  --mode flag, a TSUKU_AUTO_INSTALL_MODE value or an auto_install_mode
  config key -- including suggest, which installs nothing.

  Tools not declared in .tsuku.toml fall through to the normal consent mode.

Consent modes (for tools not in .tsuku.toml):
  suggest   Print install instructions and exit (no install)
  confirm   Prompt before installing (default, requires a terminal)
  auto      Install without prompting (requires opt-in)

Every install tsuku run performs is recorded in $TSUKU_HOME/audit.log,
whichever mode governed it, along with where that mode came from. Installs
started any other way, and dependencies pulled in by one, are not recorded.

Mode resolution order:
  1. --mode flag
  2. TSUKU_AUTO_INSTALL_MODE environment variable
  3. auto_install_mode config key ($TSUKU_HOME/config.toml)
  4. Default: confirm -- raised to auto for a tool .tsuku.toml declares

  The declaration acts on the default only, which is why it is step 4 and
  not step 1. A mode set at steps 1 through 3 reaches the install unchanged.

  A raised mode can still be lowered back to confirm before installing:
  permissive permissions on config.toml, a recipe without checksums, or
  several recipes providing the command each do so, and each says which one
  it was.

Exit codes:
  0   Command executed successfully
  1   No match found or other error
  10  The project declares more than one recipe providing the command
  11  Binary index not built
  12  Confirm mode requires a terminal
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

		mode, modeOrigin, err := resolveMode(runModeFlag, userCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tsuku run: %v\n", err)
			exitWithCode(ExitUsage)
		}

		cwd, _ := os.Getwd()
		wiring := newRunWiring(cfg, cwd)

		runner := autoinstall.NewRunner(cfg, os.Stdout, os.Stderr)
		runner.Lookup = wiring.lookup
		// The terminal is an input to the check rather than the site of it.
		// The check itself is inside Run, below the declaration lookup and the
		// gates, which is the only place it can ask whether *this* command
		// needs a prompt; here it could only ask whether the configuration
		// declared anything at all.
		runner.IsTerminal = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
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

		runErr := runner.Run(globalCtx, command, commandArgs, mode, modeOrigin, wiring.resolver)
		if runErr == nil {
			return
		}

		// The refusal for a project that declares two providers of one
		// command. The runner has printed it already, the way it prints
		// suggest mode's instructions, so this case adds only the exit code --
		// printing here too would put the one-line Error() under the message.
		//
		// ExitAmbiguous is the code the install path uses for a name it cannot
		// narrow to one recipe, which is the same condition arriving by
		// another route, so a script that already distinguishes it needs no
		// new handling.
		var ambiguous *autoinstall.AmbiguousDeclarationError

		switch {
		case errors.As(runErr, &ambiguous):
			exitWithCode(ExitAmbiguous)
		case errors.Is(runErr, autoinstall.ErrIndexNotBuilt):
			exitWithCode(ExitIndexNotBuilt)
		case errors.Is(runErr, autoinstall.ErrForbidden):
			fmt.Fprintf(os.Stderr, "tsuku run: %v\n", runErr)
			exitWithCode(ExitForbidden)
		case errors.Is(runErr, autoinstall.ErrNotInteractive):
			// The runner has printed the message, so this case adds only the
			// code -- the same ExitNotInteractive the check exited with when
			// it lived here, which is what keeps a script that already
			// distinguishes 12 working across the move.
			exitWithCode(ExitNotInteractive)
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
	resolver *project.Resolver
	lookup   autoinstall.LookupFunc
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
		resolver: project.NewResolver(projectCfg),
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
// It returns the origin alongside the mode, because the mode alone does not
// say whether anyone chose it and the runner's project elevation turns on
// exactly that: only a mode nobody set is raised by a declaration. The origin
// is the highest-precedence source that supplied a value, which is this
// function's own step order.
//
// The env var escalation restriction prevents TSUKU_AUTO_INSTALL_MODE=auto
// from taking effect unless the config also has auto_install_mode = "auto".
// This blocks malicious .envrc files from silently enabling auto mode. Its
// output is an environment-origin confirm rather than a default one -- the
// environment did supply a value, and recording it as a default would hand the
// runner a mode a declaration could raise straight back to auto.
func resolveMode(flagMode string, cfg *userconfig.Config) (autoinstall.Mode, autoinstall.Origin, error) {
	// Step 1: explicit flag wins unconditionally.
	if flagMode != "" {
		m, ok := autoinstall.ParseMode(flagMode)
		if !ok {
			return 0, autoinstall.OriginUnset, fmt.Errorf("invalid mode %q: must be suggest, confirm, or auto", flagMode)
		}
		return m, autoinstall.OriginFlag, nil
	}

	// Step 2: environment variable.
	envMode := os.Getenv("TSUKU_AUTO_INSTALL_MODE")
	if envMode != "" {
		m, ok := autoinstall.ParseMode(envMode)
		if !ok {
			return 0, autoinstall.OriginUnset, fmt.Errorf("invalid TSUKU_AUTO_INSTALL_MODE %q: must be suggest, confirm, or auto", envMode)
		}

		// Escalation restriction: env var cannot escalate to auto unless
		// the persistent config also has auto_install_mode = "auto".
		if m == autoinstall.ModeAuto && cfg.AutoInstallMode != "auto" {
			return autoinstall.ModeConfirm, autoinstall.OriginEnvironment, nil
		}

		return m, autoinstall.OriginEnvironment, nil
	}

	// Step 3: config file value.
	if cfg.AutoInstallMode != "" {
		m, ok := autoinstall.ParseMode(cfg.AutoInstallMode)
		if !ok {
			return 0, autoinstall.OriginUnset, fmt.Errorf("invalid auto_install_mode config %q: must be suggest, confirm, or auto", cfg.AutoInstallMode)
		}
		return m, autoinstall.OriginConfig, nil
	}

	// Step 4: default.
	return autoinstall.ModeConfirm, autoinstall.OriginDefault, nil
}

func init() {
	runCmd.Flags().StringVar(&runModeFlag, "mode", "", "Consent mode: suggest, confirm, or auto")
}
