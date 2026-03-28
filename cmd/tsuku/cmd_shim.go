package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/shim"
)

var shimCmd = &cobra.Command{
	Use:   "shim",
	Short: "Manage shim scripts for transparent tool invocation",
	Long: `Manage shim scripts in $TSUKU_HOME/bin/ that delegate to tsuku run.

Shims let tools work transparently in CI, Makefiles, and shell scripts
without requiring shell hooks. Each shim is a small script that calls
tsuku run, which handles version resolution (including .tsuku.toml
project pins) and auto-install.

Use 'tsuku shim install <tool>' to create shims for a tool's binaries.
Use 'tsuku shim uninstall <tool>' to remove them.
Use 'tsuku shim list' to see all installed shims.`,
}

var shimInstallCmd = &cobra.Command{
	Use:   "install <tool>",
	Short: "Create shims for a tool's binaries",
	Long: `Create shim scripts in $TSUKU_HOME/bin/ for every binary provided by
the given recipe.

Each shim is a static shell script that calls 'tsuku run', deferring
version resolution and installation to runtime. This means shims never
need to be regenerated when tool versions change.

Refuses to overwrite existing non-shim files in $TSUKU_HOME/bin/.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		recipeName := args[0]

		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		mgr := shim.NewManager(cfg, loader)
		paths, err := mgr.Install(recipeName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tsuku shim install: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		for _, p := range paths {
			fmt.Println(p)
		}
	},
}

var shimUninstallCmd = &cobra.Command{
	Use:   "uninstall <tool>",
	Short: "Remove shims for a tool",
	Long: `Remove shim scripts owned by the given recipe from $TSUKU_HOME/bin/.

Only files that are identified as shims (by content) are removed.
Non-shim files are left untouched even if they share a name with
a binary the recipe provides.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		recipeName := args[0]

		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		mgr := shim.NewManager(cfg, loader)
		if err := mgr.Uninstall(recipeName); err != nil {
			fmt.Fprintf(os.Stderr, "tsuku shim uninstall: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		printInfof("Removed shims for %s\n", recipeName)
	},
}

var shimListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed shims",
	Long: `List all shim scripts in $TSUKU_HOME/bin/ and their owning recipes.

Output format: one shim per line, showing the binary name and recipe.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		mgr := shim.NewManager(cfg, loader)
		entries, err := mgr.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "tsuku shim list: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		if len(entries) == 0 {
			printInfo("No shims installed")
			return
		}

		for _, e := range entries {
			fmt.Printf("%-20s %s\n", e.Name, e.Recipe)
		}
	},
}

func init() {
	shimCmd.AddCommand(shimInstallCmd)
	shimCmd.AddCommand(shimUninstallCmd)
	shimCmd.AddCommand(shimListCmd)
}
