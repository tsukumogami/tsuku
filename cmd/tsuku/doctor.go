package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/config"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check that the tsuku environment is configured correctly",
	Long: `Verify that the tsuku environment is healthy: home directory exists,
tools/current is in PATH, and state file is accessible.

Exits with a non-zero status if any check fails, making it suitable
for use as a gate in scripts and CI:

  tsuku doctor || exit 1`,
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

		fmt.Println("Checking tsuku environment...")
		failed := false

		// Check 1: Home directory
		fmt.Fprintf(os.Stdout, "  Home directory: %s", homeDir)
		if info, err := os.Stat(homeDir); err != nil {
			fmt.Println(" ... FAIL")
			fmt.Fprintf(os.Stderr, "    Directory does not exist\n")
			fmt.Fprintf(os.Stderr, "    Run: tsuku install <tool> to create it\n")
			failed = true
		} else if !info.IsDir() {
			fmt.Println(" ... FAIL")
			fmt.Fprintf(os.Stderr, "    Path exists but is not a directory\n")
			failed = true
		} else {
			fmt.Println(" ... ok")
		}

		// Check 2: tools/current in PATH
		currentDir := filepath.Join(homeDir, "tools", "current")
		fmt.Fprintf(os.Stdout, "  tools/current in PATH")
		pathDirs := filepath.SplitList(os.Getenv("PATH"))
		found := false
		for _, dir := range pathDirs {
			absDir, _ := filepath.Abs(dir)
			if absDir == currentDir {
				found = true
				break
			}
		}
		if found {
			fmt.Println(" ... ok")
		} else {
			fmt.Println(" ... FAIL")
			fmt.Fprintf(os.Stderr, "    %s is not in your PATH\n", currentDir)
			fmt.Fprintf(os.Stderr, "    Run: eval $(tsuku shellenv)\n")
			failed = true
		}

		// Check 3: bin directory in PATH
		binDir := filepath.Join(homeDir, "bin")
		fmt.Fprintf(os.Stdout, "  bin in PATH")
		foundBin := false
		for _, dir := range pathDirs {
			absDir, _ := filepath.Abs(dir)
			if absDir == binDir {
				foundBin = true
				break
			}
		}
		if foundBin {
			fmt.Println(" ... ok")
		} else {
			fmt.Println(" ... FAIL")
			fmt.Fprintf(os.Stderr, "    %s is not in your PATH\n", binDir)
			fmt.Fprintf(os.Stderr, "    Run: eval $(tsuku shellenv)\n")
			failed = true
		}

		// Check 4: State file
		stateFile := filepath.Join(homeDir, "state.json")
		fmt.Fprintf(os.Stdout, "  State file")
		if _, err := os.Stat(stateFile); err != nil {
			if os.IsNotExist(err) {
				fmt.Println(" ... ok (no tools installed yet)")
			} else {
				fmt.Println(" ... FAIL")
				fmt.Fprintf(os.Stderr, "    Cannot access state file: %v\n", err)
				failed = true
			}
		} else {
			fmt.Println(" ... ok")
		}

		// Summary
		if failed {
			fmt.Println()
			return fmt.Errorf("environment check failed")
		}

		fmt.Println()
		fmt.Println("Everything looks good!")
		return nil
	},
}
