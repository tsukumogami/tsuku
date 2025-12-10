package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage tsuku configuration",
	Long: `Display or manage tsuku configuration settings.

When invoked without a subcommand, displays current configuration values.

Configuration is stored in ~/.tsuku/config.toml.

Available settings:
  telemetry      Enable anonymous usage statistics (true/false)
  llm.enabled    Enable LLM features for recipe generation (true/false)
  llm.providers  Preferred LLM provider order (comma-separated, e.g., claude,gemini)

Examples:
  tsuku config
  tsuku config --json
  tsuku config get telemetry
  tsuku config set telemetry false
  tsuku config set llm.enabled false
  tsuku config set llm.providers gemini,claude`,
	Run: runConfig,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Get the current value of a configuration setting.

Available keys:
  telemetry      Enable anonymous usage statistics (true/false)
  llm.enabled    Enable LLM features for recipe generation (true/false)
  llm.providers  Preferred LLM provider order (comma-separated)`,
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
  telemetry      Enable anonymous usage statistics (true/false)
  llm.enabled    Enable LLM features for recipe generation (true/false)
  llm.providers  Preferred LLM provider order (comma-separated)

Examples:
  tsuku config set telemetry false
  tsuku config set llm.enabled false
  tsuku config set llm.providers gemini,claude`,
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
	configCmd.Flags().Bool("json", false, "Output in JSON format")
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
}

func runConfig(cmd *cobra.Command, args []string) {
	jsonOutput, _ := cmd.Flags().GetBool("json")

	// Load system config
	sysCfg, err := config.DefaultConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Load user config
	userCfg, err := userconfig.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading user config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Get environment variable status
	githubToken := os.Getenv("GITHUB_TOKEN")

	if jsonOutput {
		type configOutput struct {
			TsukuHome       string `json:"tsuku_home"`
			GithubToken     string `json:"github_token"`
			APITimeout      string `json:"api_timeout"`
			VersionCacheTTL string `json:"version_cache_ttl"`
			Telemetry       bool   `json:"telemetry"`
		}

		tokenStatus := "(not set)"
		if githubToken != "" {
			tokenStatus = "(set)"
		}

		output := configOutput{
			TsukuHome:       sysCfg.HomeDir,
			GithubToken:     tokenStatus,
			APITimeout:      config.GetAPITimeout().String(),
			VersionCacheTTL: config.GetVersionCacheTTL().String(),
			Telemetry:       userCfg.Telemetry,
		}
		printJSON(output)
		return
	}

	// Human-readable output
	fmt.Printf("TSUKU_HOME: %s\n", sysCfg.HomeDir)
	if githubToken != "" {
		fmt.Printf("GITHUB_TOKEN: (set)\n")
	} else {
		fmt.Printf("GITHUB_TOKEN: (not set)\n")
	}
	fmt.Printf("TSUKU_API_TIMEOUT: %s\n", config.GetAPITimeout())
	fmt.Printf("TSUKU_VERSION_CACHE_TTL: %s\n", config.GetVersionCacheTTL())
	fmt.Printf("telemetry: %t\n", userCfg.Telemetry)
}
