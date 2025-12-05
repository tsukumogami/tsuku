package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage tsuku configuration",
	Long: `Manage tsuku configuration settings.

Configuration is stored in ~/.tsuku/config.toml.

Available settings:
  telemetry    Enable anonymous usage statistics (true/false)

Examples:
  tsuku config get telemetry
  tsuku config set telemetry false`,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Get the current value of a configuration setting.

Available keys:
  telemetry    Enable anonymous usage statistics (true/false)`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		cfg, err := userconfig.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		value, ok := cfg.Get(key)
		if !ok {
			fmt.Fprintf(os.Stderr, "Unknown config key: %s\n", key)
			fmt.Fprintf(os.Stderr, "\nAvailable keys:\n")
			printAvailableKeys()
			exitWithCode(ExitUsage)
		}

		fmt.Println(value)
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value.

Available keys:
  telemetry    Enable anonymous usage statistics (true/false)

Examples:
  tsuku config set telemetry false
  tsuku config set telemetry true`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		value := args[1]

		cfg, err := userconfig.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		if err := cfg.Set(key, value); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			fmt.Fprintf(os.Stderr, "\nAvailable keys:\n")
			printAvailableKeys()
			exitWithCode(ExitUsage)
		}

		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		fmt.Printf("%s = %s\n", key, value)
	},
}

func printAvailableKeys() {
	keys := userconfig.AvailableKeys()
	// Sort keys for consistent output
	var sortedKeys []string
	for k := range keys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	for _, k := range sortedKeys {
		fmt.Fprintf(os.Stderr, "  %s - %s\n", k, keys[k])
	}
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
}
