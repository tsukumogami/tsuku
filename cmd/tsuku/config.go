package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/secrets"
	"github.com/tsukumogami/tsuku/internal/userconfig"
	"golang.org/x/term"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage tsuku configuration",
	Long: `Display or manage tsuku configuration settings.

When invoked without a subcommand, displays current configuration values.

Configuration is stored in $TSUKU_HOME/config.toml.

Available settings:
  telemetry           Enable anonymous usage statistics (true/false)
  llm.enabled         Enable LLM features for recipe generation (true/false)
  llm.local_enabled   Enable local LLM inference via tsuku-llm addon (true/false)
  llm.idle_timeout    How long addon stays alive after last request (e.g., 5m, 30s)
  llm.providers       Preferred LLM provider order (comma-separated, e.g., claude,gemini)
  secrets.*           API keys stored securely (use stdin for values)

Examples:
  tsuku config
  tsuku config --json
  tsuku config get telemetry
  tsuku config set telemetry false
  tsuku config set llm.local_enabled true
  tsuku config set llm.idle_timeout 10m
  echo "sk-..." | tsuku config set secrets.anthropic_api_key`,
	Run: runConfig,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Long: `Get the current value of a configuration setting.

Available keys:
  telemetry           Enable anonymous usage statistics (true/false)
  llm.enabled         Enable LLM features for recipe generation (true/false)
  llm.local_enabled   Enable local LLM inference via tsuku-llm addon (true/false)
  llm.idle_timeout    How long addon stays alive after last request (e.g., 5m, 30s)
  llm.providers       Preferred LLM provider order (comma-separated)
  secrets.<name>      Secret status (displays (set) or (not set), never the value)`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		// For secrets.* keys, show status only -- never the actual value.
		if secretName, ok := strings.CutPrefix(strings.ToLower(key), "secrets."); ok {
			if !isKnownSecret(secretName) {
				fmt.Fprintf(os.Stderr, "Unknown secret: %s\n", secretName)
				fmt.Fprintf(os.Stderr, "\nKnown secrets:\n")
				printKnownSecrets()
				exitWithCode(ExitUsage)
			}
			if secrets.IsSet(secretName) {
				fmt.Println("(set)")
			} else {
				fmt.Println("(not set)")
			}
			return
		}

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
	Use:   "set <key> [value]",
	Short: "Set a configuration value",
	Long: `Set a configuration value.

For secrets (keys starting with "secrets."), the value is read from stdin
to avoid exposure in shell history. You can pipe a value or type it interactively.

Available keys:
  telemetry           Enable anonymous usage statistics (true/false)
  llm.enabled         Enable LLM features for recipe generation (true/false)
  llm.local_enabled   Enable local LLM inference via tsuku-llm addon (true/false)
  llm.idle_timeout    How long addon stays alive after last request (e.g., 5m, 30s)
  llm.providers       Preferred LLM provider order (comma-separated)
  secrets.<name>      Set a secret value (read from stdin)

Examples:
  tsuku config set telemetry false
  tsuku config set llm.local_enabled true
  tsuku config set llm.idle_timeout 10m
  echo "sk-ant-..." | tsuku config set secrets.anthropic_api_key`,
	Args: cobra.RangeArgs(1, 2),
	Run:  runConfigSet,
}

// stdinReader is the reader used for stdin. Replaceable for testing.
var stdinReader io.Reader = os.Stdin

// stdinIsTerminal reports whether stdin is a terminal. Replaceable for testing.
var stdinIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func runConfigSet(cmd *cobra.Command, args []string) {
	key := args[0]

	// Detect whether this is a secret key.
	isSecret := strings.HasPrefix(strings.ToLower(key), "secrets.")

	var value string
	if isSecret {
		secretName, _ := strings.CutPrefix(strings.ToLower(key), "secrets.")

		// Validate against known secret names.
		if !isKnownSecret(secretName) {
			fmt.Fprintf(os.Stderr, "Unknown secret: %s\n", secretName)
			fmt.Fprintf(os.Stderr, "\nKnown secrets:\n")
			printKnownSecrets()
			exitWithCode(ExitUsage)
		}

		// Secrets must not accept the value as a CLI argument (shell history risk).
		if len(args) > 1 {
			fmt.Fprintf(os.Stderr, "Error: secret values must be provided via stdin, not as arguments\n")
			fmt.Fprintf(os.Stderr, "Usage: echo \"value\" | tsuku config set %s\n", key)
			exitWithCode(ExitUsage)
		}

		// Read secret value from stdin.
		var err error
		value, err = readSecretFromStdin(secretName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading secret: %v\n", err)
			exitWithCode(ExitGeneral)
		}
	} else {
		// Non-secret keys require the value as an argument.
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: value required for non-secret key %q\n", key)
			fmt.Fprintf(os.Stderr, "Usage: tsuku config set %s <value>\n", key)
			exitWithCode(ExitUsage)
		}
		value = args[1]
	}

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

	if isSecret {
		fmt.Printf("%s = (set)\n", key)
	} else {
		fmt.Printf("%s = %s\n", key, value)
	}
}

// readSecretFromStdin reads a secret value from stdin.
// If stdin is a terminal, it prints a prompt. If piped, it reads silently.
func readSecretFromStdin(secretName string) (string, error) {
	if stdinIsTerminal() {
		fmt.Fprintf(os.Stderr, "Enter value for %s: ", secretName)
	}

	reader := bufio.NewReader(stdinReader)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read from stdin: %w", err)
	}

	value := strings.TrimRight(line, "\r\n")
	if value == "" {
		return "", fmt.Errorf("empty value provided")
	}

	return value, nil
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

// isKnownSecret checks whether a secret name is in the known keys registry.
func isKnownSecret(name string) bool {
	for _, k := range secrets.KnownKeys() {
		if k.Name == name {
			return true
		}
	}
	return false
}

// printKnownSecrets prints the list of known secrets to stderr.
func printKnownSecrets() {
	for _, k := range secrets.KnownKeys() {
		fmt.Fprintf(os.Stderr, "  secrets.%s - %s\n", k.Name, k.Desc)
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

	// Build secrets status for all known keys.
	knownKeys := secrets.KnownKeys()
	secretsStatus := make(map[string]string, len(knownKeys))
	for _, k := range knownKeys {
		if secrets.IsSet(k.Name) {
			secretsStatus[k.Name] = "(set)"
		} else {
			secretsStatus[k.Name] = "(not set)"
		}
	}

	if jsonOutput {
		type configOutput struct {
			TsukuHome       string            `json:"tsuku_home"`
			APITimeout      string            `json:"api_timeout"`
			VersionCacheTTL string            `json:"version_cache_ttl"`
			Telemetry       bool              `json:"telemetry"`
			Secrets         map[string]string `json:"secrets"`
		}

		output := configOutput{
			TsukuHome:       sysCfg.HomeDir,
			APITimeout:      config.GetAPITimeout().String(),
			VersionCacheTTL: config.GetVersionCacheTTL().String(),
			Telemetry:       userCfg.Telemetry,
			Secrets:         secretsStatus,
		}
		printJSON(output)
		return
	}

	// Human-readable output
	fmt.Printf("TSUKU_HOME: %s\n", sysCfg.HomeDir)
	fmt.Printf("TSUKU_API_TIMEOUT: %s\n", config.GetAPITimeout())
	fmt.Printf("TSUKU_VERSION_CACHE_TTL: %s\n", config.GetVersionCacheTTL())
	fmt.Printf("telemetry: %t\n", userCfg.Telemetry)
	fmt.Printf("\nSecrets:\n")
	for _, k := range knownKeys {
		fmt.Printf("  %s: %s\n", k.Name, secretsStatus[k.Name])
	}
}
