package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/shellquote"
)

var shellenvCmd = &cobra.Command{
	Use:   "shellenv",
	Short: "Print shell commands to configure PATH for tsuku",
	Long: `Print shell commands that configure PATH to include tsuku's bin and
tools/current directories. Useful for users who install tsuku without
the install script, or for development builds that use a non-default
home directory.

Usage in shell profile:
  eval $(tsuku shellenv)

Usage for one-off sessions:
  eval $(./tsuku shellenv)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.DefaultConfig()
		if err != nil {
			return fmt.Errorf("failed to get config: %w", err)
		}

		homeDir, err := filepath.Abs(cfg.HomeDir)
		if err != nil {
			return fmt.Errorf("failed to resolve home directory: %w", err)
		}

		// Source the shell init cache if it exists.
		// Detect the current shell to pick the right cache file.
		shell := detectShellForEnv()
		cachePath := filepath.Join(homeDir, "share", "shell.d", ".init-cache."+shell)
		if _, err := os.Stat(cachePath); err != nil {
			cachePath = ""
		}

		_, err = fmt.Fprint(os.Stdout, shellenvScript(homeDir, cachePath))
		return err
	},
}

// shellenvScript renders exactly what `tsuku shellenv` writes to stdout, for a
// home directory and an optional init-cache path. An empty cachePath omits the
// source line.
//
// This is a function rather than inline command body for one reason: the test
// that proves the quoting works has to run against *this* text. It previously
// ran against a copy of it that lived in the test file, which meant reverting
// the command to hand-written double quotes -- the original vulnerability, in
// full -- left the entire suite green. Verified, not assumed. A second
// implementation cannot fail when the first one changes, so there is now only
// one.
//
// Each interpolated component is quoted; the trailing $PATH is left live on
// purpose. Quoting the whole statement would round-trip both components
// perfectly and silently discard the user's existing PATH, which is the one
// place in this change where the safe transformation and the correct one
// diverge.
func shellenvScript(homeDir, cachePath string) string {
	binDir := filepath.Join(homeDir, "bin")
	currentDir := filepath.Join(homeDir, "tools", "current")

	var b strings.Builder
	fmt.Fprintf(&b, "export PATH=%s:%s:\"$PATH\"\n",
		shellquote.POSIX(binDir), shellquote.POSIX(currentDir))
	if cachePath != "" {
		fmt.Fprintf(&b, ". %s\n", shellquote.POSIX(cachePath))
	}
	return b.String()
}

// detectShellForEnv returns the shell name to use for shellenv output.
// Uses $SHELL to determine the current shell, defaulting to "bash".
func detectShellForEnv() string {
	if s := os.Getenv("SHELL"); s != "" {
		base := filepath.Base(s)
		switch base {
		case "bash", "zsh", "fish":
			return base
		}
	}
	return "bash"
}
