package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/hook"
)

var hookShellFlag string
var hookInstallActivateFlag bool
var hookUninstallActivateFlag bool

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage shell command-not-found hooks",
	Long: `Manage shell command-not-found hooks for bash, zsh, and fish.

When installed, the hook intercepts a command your shell cannot resolve and
runs 'tsuku run <command> -- <args>'. If a recipe provides that command,
tsuku installs the tool and then executes the command you typed. Hook files
are written to $TSUKU_HOME/share/hooks/ and sourced from the shell's rc file.

Whether it installs is governed by the consent mode. From a hook there is
nowhere to pass --mode, so the two reachable sources are the
TSUKU_AUTO_INSTALL_MODE environment variable and the auto_install_mode key in
$TSUKU_HOME/config.toml; without either, the default is confirm, which prompts
and needs a terminal. Set auto_install_mode = "suggest" for a hook that prints
an install instruction and installs nothing.

A .tsuku.toml declaring the tool raises an unset default to auto, so a declared
tool installs without a prompt. A mode you set yourself is honored as given,
suggest included.

Use 'tsuku hook install' to register the hook for your current shell.
Use 'tsuku hook uninstall' to remove it.
Use 'tsuku hook status' to check current registration state.

See 'tsuku run --help' for the full resolution order and exit codes.`,
}

var hookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install shell hooks",
	Long: `Install shell hooks for command-not-found handling or environment activation.

By default, installs the command-not-found hook, which routes an unresolvable
command to 'tsuku run' and so can install and execute it -- see 'tsuku hook'
for the setting that governs that. With --activate, installs
the activation hook that calls 'tsuku hook-env' on each prompt to manage
per-project tool versions automatically.

For bash and zsh, appends a two-line source block to the shell's rc file
(~/.bashrc or ~/.zshrc). The hook file itself is written to
$TSUKU_HOME/share/hooks/. Running install twice is safe -- the block is
only appended once. The command-not-found and activation hooks use separate
marker blocks and can be installed independently.

For fish, writes the hook file to ~/.config/fish/conf.d/.

Without --shell, the shell is detected from the $SHELL environment variable.

Examples:
  tsuku hook install
  tsuku hook install --shell=bash
  tsuku hook install --activate
  tsuku hook install --activate --shell=zsh`,
	RunE: func(cmd *cobra.Command, args []string) error {
		shell, err := resolveShell(hookShellFlag)
		if err != nil {
			return err
		}

		cfg, err := config.DefaultConfig()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home directory: %w", err)
		}

		shareHooksDir := filepath.Join(cfg.ShareDir, "hooks")
		if err := os.MkdirAll(shareHooksDir, 0755); err != nil {
			return fmt.Errorf("create hooks directory: %w", err)
		}

		if hookInstallActivateFlag {
			if err := hook.InstallActivate(shell, homeDir, shareHooksDir); err != nil {
				return fmt.Errorf("install activation hook: %w", err)
			}
			fmt.Fprintf(os.Stdout, "Registered activation hook for %s.\n", shell)
		} else {
			if err := hook.Install(shell, homeDir, shareHooksDir); err != nil {
				return fmt.Errorf("install hook: %w", err)
			}
			// The second line is the account the install flow otherwise
			// never gives. install.sh registers the hook by delegating
			// here, so a user who took the installer's default has read
			// nothing about what the hook does; a line saying only that
			// something was registered leaves them to assume it suggests.
			fmt.Fprintf(os.Stdout, "Registered command-not-found hook for %s.\n", shell)
			fmt.Fprintf(os.Stdout, "An unresolvable command now goes to 'tsuku run', which installs the tool and runs it.\n")
			fmt.Fprintf(os.Stdout, "Run 'tsuku hook --help' for the setting that governs whether it installs.\n")
		}

		return nil
	},
}

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove shell hooks",
	Long: `Remove shell hooks for command-not-found handling or environment activation.

By default, removes the command-not-found hook. With --activate, removes
the activation hook instead.

With the command-not-found hook removed, an unresolvable command goes straight
to the shell's own handler and tsuku installs nothing.

For bash and zsh, removes the marker block from ~/.bashrc or ~/.zshrc.
For fish, deletes the corresponding file from ~/.config/fish/conf.d/.

Running uninstall when the hook is not installed is safe -- it does nothing.

Without --shell, the shell is detected from the $SHELL environment variable.

Examples:
  tsuku hook uninstall
  tsuku hook uninstall --shell=zsh
  tsuku hook uninstall --activate`,
	RunE: func(cmd *cobra.Command, args []string) error {
		shell, err := resolveShell(hookShellFlag)
		if err != nil {
			return err
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home directory: %w", err)
		}

		if hookUninstallActivateFlag {
			if err := hook.UninstallActivate(shell, homeDir); err != nil {
				return fmt.Errorf("uninstall activation hook: %w", err)
			}
			fmt.Fprintf(os.Stdout, "Removed activation hook for %s.\n", shell)
		} else {
			if err := hook.Uninstall(shell, homeDir); err != nil {
				return fmt.Errorf("uninstall hook: %w", err)
			}
			fmt.Fprintf(os.Stdout, "Removed command-not-found hook for %s.\n", shell)
		}

		return nil
	},
}

var hookStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report shell hook installation status",
	Long: `Report shell hook installation status.

Checks each supported shell and reports two lines for it: whether the
command-not-found hook is installed, and whether the activation hook is.
Without --shell, reports status for all supported shells.

Examples:
  tsuku hook status
  tsuku hook status --shell=bash`,
	RunE: func(cmd *cobra.Command, args []string) error {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home directory: %w", err)
		}

		shells := []string{"bash", "zsh", "fish"}
		if hookShellFlag != "" {
			shells = []string{hookShellFlag}
		}

		for _, shell := range shells {
			installed, err := hook.Status(shell, homeDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: error checking status: %v\n", shell, err)
				continue
			}
			state := "not installed"
			if installed {
				state = "installed"
			}
			fmt.Fprintf(os.Stdout, "%s: command-not-found %s\n", shell, state)

			activateInstalled, err := hook.ActivateStatus(shell, homeDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: error checking activation status: %v\n", shell, err)
				continue
			}
			activateState := "not installed"
			if activateInstalled {
				activateState = "installed"
			}
			fmt.Fprintf(os.Stdout, "%s: activate %s\n", shell, activateState)
		}
		return nil
	},
}

// resolveShell returns the shell name from the flag or from $SHELL.
// Returns an error if neither is set or if the value is unsupported.
func resolveShell(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}

	shellEnv := os.Getenv("SHELL")
	if shellEnv == "" {
		return "", fmt.Errorf("--shell flag not set and $SHELL is not set; specify a shell with --shell=<shell>")
	}

	// Extract base name: /bin/bash -> bash
	base := filepath.Base(shellEnv)
	switch base {
	case "bash", "zsh", "fish":
		return base, nil
	default:
		return "", fmt.Errorf("unsupported shell %q; supported shells are bash, zsh, fish", base)
	}
}

func init() {
	hookCmd.PersistentFlags().StringVar(&hookShellFlag, "shell", "", "Shell to target (bash, zsh, or fish); defaults to $SHELL")
	hookInstallCmd.Flags().BoolVar(&hookInstallActivateFlag, "activate", false, "Install the activation hook instead of command-not-found")
	hookUninstallCmd.Flags().BoolVar(&hookUninstallActivateFlag, "activate", false, "Remove the activation hook instead of command-not-found")
	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
	hookCmd.AddCommand(hookStatusCmd)
}
