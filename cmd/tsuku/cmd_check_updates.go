package main

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/updates"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

// cmdRecipeLoader adapts the cmd package's loadRecipeForTool into the
// updates.RecipeLoader interface.
type cmdRecipeLoader struct{}

func (l *cmdRecipeLoader) LoadRecipe(ctx context.Context, toolName string, state *install.State, cfg *config.Config) (*recipe.Recipe, error) {
	return loadRecipeForTool(ctx, toolName, state, cfg)
}

var checkUpdatesCmd = &cobra.Command{
	Use:    "check-updates",
	Short:  "Check for tool updates in background",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Suppress all output (background process)
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		cfg, err := config.DefaultConfig()
		if err != nil {
			return err
		}

		userCfg, err := userconfig.Load()
		if err != nil {
			return err
		}

		if !userCfg.UpdatesEnabled() {
			return nil
		}

		// 10-second timeout per PRD R19
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Redirect stdout/stderr to devnull for truly silent operation
		devNull, err := os.Open(os.DevNull)
		if err == nil {
			defer devNull.Close()
			os.Stdout = devNull
			os.Stderr = devNull
		}

		return updates.RunUpdateCheck(ctx, cfg, userCfg, &cmdRecipeLoader{})
	},
}

func init() {
	rootCmd.AddCommand(checkUpdatesCmd)
}
