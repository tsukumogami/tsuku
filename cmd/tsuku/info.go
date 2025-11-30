package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tsuku-dev/tsuku/internal/config"
	"github.com/tsuku-dev/tsuku/internal/install"
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

		// Check installation status
		var installedVersion, location string
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
		}

		// JSON output mode
		if jsonOutput {
			type infoOutput struct {
				Name             string `json:"name"`
				Description      string `json:"description"`
				Homepage         string `json:"homepage,omitempty"`
				VersionFormat    string `json:"version_format"`
				Status           string `json:"status"`
				InstalledVersion string `json:"installed_version,omitempty"`
				Location         string `json:"location,omitempty"`
				VerifyCommand    string `json:"verify_command,omitempty"`
			}
			output := infoOutput{
				Name:             r.Metadata.Name,
				Description:      r.Metadata.Description,
				Homepage:         r.Metadata.Homepage,
				VersionFormat:    r.Metadata.VersionFormat,
				Status:           status,
				InstalledVersion: installedVersion,
				Location:         location,
				VerifyCommand:    r.Verify.Command,
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
	},
}

func init() {
	infoCmd.Flags().Bool("json", false, "Output in JSON format")
}
