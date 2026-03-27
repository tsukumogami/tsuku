package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/project"
)

// configTemplate is the default content for a new .tsuku.toml file.
const configTemplate = `# Project tools managed by tsuku.
# See: https://tsuku.dev/docs/project-config
[tools]
`

var initForceFlag bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a project configuration file",
	Long: `Create a .tsuku.toml file in the current directory.

This file declares which tools and versions a project requires,
enabling reproducible development environments across machines.

Use --force to overwrite an existing configuration file.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		return runInit(dir, initForceFlag)
	},
}

func init() {
	initCmd.Flags().BoolVarP(&initForceFlag, "force", "f", false, "Overwrite existing configuration file")
}

// runInit creates a project configuration file in the given directory.
func runInit(dir string, force bool) error {
	target := filepath.Join(dir, project.ConfigFileName)

	if !force {
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", project.ConfigFileName)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("checking %s: %w", project.ConfigFileName, err)
		}
	}

	if err := os.WriteFile(target, []byte(configTemplate), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", project.ConfigFileName, err)
	}

	fmt.Printf("Created %s\n", project.ConfigFileName)
	return nil
}
