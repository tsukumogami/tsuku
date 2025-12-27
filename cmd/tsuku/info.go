package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
)

var infoCmd = &cobra.Command{
	Use:   "info <tool>",
	Short: "Show detailed information about a tool",
	Long:  `Show detailed information about a tool, including description, homepage, and installation status.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		toolName := args[0]
		jsonOutput, _ := cmd.Flags().GetBool("json")

		// Load recipe
		r, err := loader.Get(toolName)
		if err != nil {
			fmt.Printf("Tool '%s' not found in registry.\n", toolName)
			return
		}

		// Check installation status and get dependencies
		var installedVersion, location string
		var installDeps, runtimeDeps []string
		status := "not_installed"
		cfg, err := config.DefaultConfig()
		if err == nil {
			mgr := install.New(cfg)
			tools, _ := mgr.List()

			for _, t := range tools {
				if t.Name == toolName {
					status = "installed"
					installedVersion = t.Version
					location = cfg.ToolDir(toolName, t.Version)
					break
				}
			}

			// Get dependencies from state for installed tools
			if status == "installed" {
				stateMgr := install.NewStateManager(cfg)
				toolState, err := stateMgr.GetToolState(toolName)
				if err == nil && toolState != nil {
					installDeps = toolState.InstallDependencies
					runtimeDeps = toolState.RuntimeDependencies
				}
			}
		}

		// For uninstalled tools, resolve dependencies from recipe
		if status == "not_installed" {
			directDeps := actions.ResolveDependencies(r)
			// Resolve transitive dependencies
			resolvedDeps, err := actions.ResolveTransitive(context.Background(), loader, directDeps, toolName)
			if err == nil {
				installDeps = sortedKeys(resolvedDeps.InstallTime)
				runtimeDeps = sortedKeys(resolvedDeps.Runtime)
			} else {
				// Fall back to direct deps if transitive resolution fails
				installDeps = sortedKeys(directDeps.InstallTime)
				runtimeDeps = sortedKeys(directDeps.Runtime)
			}
		}

		// JSON output mode
		if jsonOutput {
			type infoOutput struct {
				Name                 string   `json:"name"`
				Description          string   `json:"description"`
				Homepage             string   `json:"homepage,omitempty"`
				VersionFormat        string   `json:"version_format"`
				SupportedOS          []string `json:"supported_os,omitempty"`
				SupportedArch        []string `json:"supported_arch,omitempty"`
				UnsupportedPlatforms []string `json:"unsupported_platforms,omitempty"`
				Status               string   `json:"status"`
				InstalledVersion     string   `json:"installed_version,omitempty"`
				Location             string   `json:"location,omitempty"`
				VerifyCommand        string   `json:"verify_command,omitempty"`
				InstallDependencies  []string `json:"install_dependencies,omitempty"`
				RuntimeDependencies  []string `json:"runtime_dependencies,omitempty"`
			}
			output := infoOutput{
				Name:                 r.Metadata.Name,
				Description:          r.Metadata.Description,
				Homepage:             r.Metadata.Homepage,
				VersionFormat:        r.Metadata.VersionFormat,
				SupportedOS:          r.Metadata.SupportedOS,
				SupportedArch:        r.Metadata.SupportedArch,
				UnsupportedPlatforms: r.Metadata.UnsupportedPlatforms,
				Status:               status,
				InstalledVersion:     installedVersion,
				Location:             location,
				VerifyCommand:        r.Verify.Command,
				InstallDependencies:  installDeps,
				RuntimeDependencies:  runtimeDeps,
			}
			printJSON(output)
			return
		}

		fmt.Printf("Name:           %s\n", r.Metadata.Name)
		fmt.Printf("Description:    %s\n", r.Metadata.Description)
		if r.Metadata.Homepage != "" {
			fmt.Printf("Homepage:       %s\n", r.Metadata.Homepage)
		}
		fmt.Printf("Version Format: %s\n", r.Metadata.VersionFormat)

		// Show platform constraints if present
		hasConstraints := len(r.Metadata.SupportedOS) > 0 ||
			len(r.Metadata.SupportedArch) > 0 ||
			len(r.Metadata.UnsupportedPlatforms) > 0
		if hasConstraints {
			fmt.Printf("Platforms:      %s\n", r.FormatPlatformConstraints())
		}

		if status == "installed" {
			fmt.Printf("Status:         Installed (v%s)\n", installedVersion)
			fmt.Printf("Location:       %s\n", location)
		} else {
			fmt.Printf("Status:         Not installed\n")
		}

		// Show verification method
		if r.Verify.Command != "" {
			fmt.Printf("Verify Command: %s\n", r.Verify.Command)
		}

		// Show dependencies
		if len(installDeps) > 0 {
			fmt.Printf("\nInstall Dependencies:\n")
			for _, dep := range installDeps {
				fmt.Printf("  - %s\n", dep)
			}
		}
		if len(runtimeDeps) > 0 {
			fmt.Printf("\nRuntime Dependencies:\n")
			for _, dep := range runtimeDeps {
				fmt.Printf("  - %s\n", dep)
			}
		}
	},
}

// sortedKeys returns the keys of a map[string]string as a sorted slice.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func init() {
	infoCmd.Flags().Bool("json", false, "Output in JSON format")
}
