package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/discover"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/userconfig"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage distributed recipe registries",
	Long: `Manage distributed recipe registries stored in config.toml.

Registries are GitHub repositories containing a .tsuku-recipes/ directory.
Each registry provides additional recipes beyond the central tsuku registry.

When strict_registries is enabled (tsuku config set strict_registries true),
tsuku will only install from explicitly registered sources. This is recommended
for CI environments and shared machines.

Examples:
  tsuku registry list
  tsuku registry add myorg/recipes
  tsuku registry remove myorg/recipes`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var registryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered recipe registries",
	Long:  `Display all registered distributed recipe sources and their configuration.`,
	Run:   runRegistryList,
}

var registryAddCmd = &cobra.Command{
	Use:   "add <owner/repo>",
	Short: "Register a distributed recipe source",
	Long: `Register a GitHub repository as a distributed recipe source.

The repository must contain a .tsuku-recipes/ directory with TOML recipe files.
The owner/repo format is validated to prevent path traversal and other attacks.

If the source is already registered, this command is a no-op.

Examples:
  tsuku registry add myorg/recipes
  tsuku registry add github-user/my-tools`,
	Args: cobra.ExactArgs(1),
	Run:  runRegistryAdd,
}

var registryRemoveCmd = &cobra.Command{
	Use:   "remove <owner/repo>",
	Short: "Remove a registered recipe source",
	Long: `Remove a distributed recipe source from the configuration.

This does NOT uninstall any tools that were installed from the removed source.
Tools already installed will continue to work but won't receive updates from
this source until it is re-added.

Examples:
  tsuku registry remove myorg/recipes`,
	Args: cobra.ExactArgs(1),
	Run:  runRegistryRemove,
}

func init() {
	registryCmd.AddCommand(registryListCmd)
	registryCmd.AddCommand(registryAddCmd)
	registryCmd.AddCommand(registryRemoveCmd)
}

func runRegistryList(cmd *cobra.Command, args []string) {
	cfg, err := userconfig.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	if len(cfg.Registries) == 0 {
		fmt.Println("No registries configured.")
		if cfg.StrictRegistries {
			fmt.Println("\nstrict_registries: enabled")
		}
		return
	}

	// Sort registry names for deterministic output
	names := make([]string, 0, len(cfg.Registries))
	for name := range cfg.Registries {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Printf("Registered registries (%d):\n\n", len(cfg.Registries))
	for _, name := range names {
		entry := cfg.Registries[name]
		annotation := ""
		if entry.AutoRegistered {
			annotation = " (auto-registered)"
		}
		url := entry.URL
		if url == "" {
			url = fmt.Sprintf("https://github.com/%s", name)
		}
		fmt.Printf("  %-30s  %s%s\n", name, url, annotation)
	}

	fmt.Println()
	if cfg.StrictRegistries {
		fmt.Println("strict_registries: enabled")
	} else {
		fmt.Println("strict_registries: disabled")
	}
}

func runRegistryAdd(cmd *cobra.Command, args []string) {
	source := args[0]

	// Validate the owner/repo format
	if err := discover.ValidateGitHubURL(source); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid registry source %q: %v\n", source, err)
		exitWithCode(ExitUsage)
	}

	cfg, err := userconfig.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Check if already registered (idempotent)
	if _, exists := cfg.Registries[source]; exists {
		fmt.Printf("Registry %q is already registered.\n", source)
		return
	}

	// Add the entry
	if cfg.Registries == nil {
		cfg.Registries = make(map[string]userconfig.RegistryEntry)
	}
	cfg.Registries[source] = userconfig.RegistryEntry{
		URL:            fmt.Sprintf("https://github.com/%s", source),
		AutoRegistered: false,
	}

	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	fmt.Printf("Added registry %q.\n", source)
}

func runRegistryRemove(cmd *cobra.Command, args []string) {
	source := args[0]

	cfg, err := userconfig.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Check if the registry exists
	if _, exists := cfg.Registries[source]; !exists {
		fmt.Printf("Registry %q is not registered.\n", source)
		return
	}

	// Remove the entry
	delete(cfg.Registries, source)

	// Clean up empty map to keep config.toml tidy
	if len(cfg.Registries) == 0 {
		cfg.Registries = nil
	}

	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	fmt.Printf("Removed registry %q.\n", source)

	// List tools still installed from this source
	printToolsFromSource(source)
}

// printToolsFromSource prints an informational message listing tools that were
// installed from the given source. This helps users understand that removing a
// registry doesn't uninstall tools.
func printToolsFromSource(source string) {
	sysCfg, err := config.DefaultConfig()
	if err != nil {
		return // Best-effort; don't fail the remove operation
	}

	mgr := install.New(sysCfg)
	state, err := mgr.GetState().Load()
	if err != nil {
		return // Best-effort
	}

	var toolNames []string
	for name, tool := range state.Installed {
		if tool.Source == source {
			toolNames = append(toolNames, name)
		}
	}

	if len(toolNames) == 0 {
		return
	}

	sort.Strings(toolNames)
	fmt.Printf("\nNote: the following tools were installed from %q and remain installed:\n", source)
	for _, name := range toolNames {
		fmt.Printf("  - %s\n", name)
	}
	fmt.Println("\nTo remove them, run: tsuku remove <tool>")
}
