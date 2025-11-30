package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tsuku-dev/tsuku/internal/config"
	"github.com/tsuku-dev/tsuku/internal/install"
	"github.com/tsuku-dev/tsuku/internal/recipe"
)

// verifyWithAbsolutePath verifies a hidden tool using absolute paths
func verifyWithAbsolutePath(r *recipe.Recipe, toolName, version, installDir string) {
	command := r.Verify.Command
	command = strings.ReplaceAll(command, "{version}", version)
	command = strings.ReplaceAll(command, "{install_dir}", installDir)

	pattern := r.Verify.Pattern
	pattern = strings.ReplaceAll(pattern, "{version}", version)
	pattern = strings.ReplaceAll(pattern, "{install_dir}", installDir)

	printInfof("  Running: %s\n", command)

	cmdExec := exec.Command("sh", "-c", command)
	output, err := cmdExec.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Verification failed: %v\nOutput: %s\n", err, string(output))
		exitWithCode(ExitVerifyFailed)
	}

	outputStr := strings.TrimSpace(string(output))
	printInfof("  Output: %s\n", outputStr)

	if pattern != "" {
		if !strings.Contains(outputStr, pattern) {
			fmt.Fprintf(os.Stderr, "Output does not match expected pattern\n  Expected: %s\n  Got: %s\n", pattern, outputStr)
			exitWithCode(ExitVerifyFailed)
		}
		printInfof("  Pattern matched: %s\n", pattern)
	}
}

// verifyVisibleTool performs comprehensive verification for visible tools
func verifyVisibleTool(r *recipe.Recipe, toolName string, toolState *install.ToolState, installDir string, cfg *config.Config) {
	// Step 1: Verify installation via current/ symlink
	printInfo("  Step 1: Verifying installation via symlink...")

	command := r.Verify.Command
	pattern := r.Verify.Pattern

	// For visible tools, use the binary name directly (will resolve via current/)
	// But first verify the symlink works by using absolute path
	version := toolState.Version
	command = strings.ReplaceAll(command, "{version}", version)
	command = strings.ReplaceAll(command, "{install_dir}", installDir)
	pattern = strings.ReplaceAll(pattern, "{version}", version)
	pattern = strings.ReplaceAll(pattern, "{install_dir}", installDir)

	printInfof("    Running: %s\n", command)
	cmdExec := exec.Command("sh", "-c", command)

	// For Step 1, add install directory bin/ to PATH so binaries can be found
	// This is needed for binary-only installs where verify command doesn't use {install_dir}
	env := os.Environ()
	binDir := filepath.Join(installDir, "bin")
	env = append(env, "PATH="+binDir+":"+os.Getenv("PATH"))
	cmdExec.Env = env

	output, err := cmdExec.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "    Installation verification failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "    Output: %s\n", string(output))
		fmt.Fprintf(os.Stderr, "\nThe tool is installed but not working correctly.\n")
		exitWithCode(ExitVerifyFailed)
	}
	outputStr := strings.TrimSpace(string(output))
	printInfof("    Output: %s\n", outputStr)

	if pattern != "" && !strings.Contains(outputStr, pattern) {
		fmt.Fprintf(os.Stderr, "    Pattern mismatch\n")
		fmt.Fprintf(os.Stderr, "    Expected: %s\n", pattern)
		fmt.Fprintf(os.Stderr, "    Got: %s\n", outputStr)
		exitWithCode(ExitVerifyFailed)
	}
	printInfo("    Installation verified\n")

	// Step 2: Check if current/ is in PATH
	printInfof("  Step 2: Checking if %s is in PATH...\n", cfg.CurrentDir)
	pathEnv := os.Getenv("PATH")
	pathInPATH := false
	for _, dir := range strings.Split(pathEnv, ":") {
		if dir == cfg.CurrentDir {
			pathInPATH = true
			break
		}
	}

	if !pathInPATH {
		fmt.Fprintf(os.Stderr, "    %s is not in your PATH\n", cfg.CurrentDir)
		fmt.Fprintf(os.Stderr, "\nThe tool is installed correctly, but you need to add this to your shell profile:\n")
		fmt.Fprintf(os.Stderr, "  export PATH=\"%s:$PATH\"\n", cfg.CurrentDir)
		exitWithCode(ExitVerifyFailed)
	}
	printInfof("    %s is in PATH\n\n", cfg.CurrentDir)

	// Step 3: Verify tool binaries are accessible from PATH and check for conflicts
	printInfo("  Step 3: Checking PATH resolution for binaries...")

	// Check each binary provided by this tool
	binariesToCheck := toolState.Binaries
	if len(binariesToCheck) == 0 {
		// Fallback: assume tool name is the binary
		binariesToCheck = []string{toolName}
	}

	for _, binaryPath := range binariesToCheck {
		// Extract just the binary name (e.g., "cargo/bin/cargo" -> "cargo")
		binaryName := filepath.Base(binaryPath)

		whichPath, err := exec.Command("which", binaryName).Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "    Binary '%s' not found in PATH\n", binaryName)
			fmt.Fprintf(os.Stderr, "\nThe tool is installed and current/ is in PATH, but '%s' cannot be found.\n", binaryName)
			fmt.Fprintf(os.Stderr, "This may indicate a broken symlink in %s\n", cfg.CurrentDir)
			exitWithCode(ExitVerifyFailed)
		}

		resolvedPath := strings.TrimSpace(string(whichPath))
		expectedPath := cfg.CurrentSymlink(binaryName)

		printInfof("    Binary '%s':\n", binaryName)
		printInfof("      Found: %s\n", resolvedPath)
		printInfof("      Expected: %s\n", expectedPath)

		if resolvedPath != expectedPath {
			fmt.Fprintf(os.Stderr, "      PATH conflict detected!\n")
			fmt.Fprintf(os.Stderr, "\nThe tool is installed, but another '%s' is earlier in your PATH:\n", binaryName)
			fmt.Fprintf(os.Stderr, "  Using: %s\n", resolvedPath)
			fmt.Fprintf(os.Stderr, "  Expected: %s\n", expectedPath)

			// Try to get version info from the conflicting tool
			versionCmd := exec.Command(binaryName, "--version")
			if versionOut, err := versionCmd.CombinedOutput(); err == nil {
				fmt.Fprintf(os.Stderr, "  Conflicting version output: %s\n", strings.TrimSpace(string(versionOut)))
			}
			exitWithCode(ExitVerifyFailed)
		}
		printInfo("      Correct binary is being used from PATH")
	}
}

var verifyCmd = &cobra.Command{
	Use:   "verify <tool>",
	Short: "Verify an installed tool",
	Long:  `Verify that an installed tool is working correctly using the recipe's verification command.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		toolName := args[0]

		// Get config and manager
		cfg, err := config.DefaultConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get config: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		mgr := install.New(cfg)

		// Check if tool is installed
		state, err := mgr.GetState().Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load state: %v\n", err)
			exitWithCode(ExitGeneral)
		}

		toolState, ok := state.Installed[toolName]
		if !ok {
			fmt.Fprintf(os.Stderr, "Tool '%s' is not installed\n", toolName)
			exitWithCode(ExitGeneral)
		}

		// Load recipe
		r, err := loader.Get(toolName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load recipe: %v\n", err)
			exitWithCode(ExitRecipeNotFound)
		}

		// Check if recipe has verification
		if r.Verify.Command == "" {
			fmt.Fprintf(os.Stderr, "Recipe for '%s' does not define verification\n", toolName)
			exitWithCode(ExitGeneral)
		}

		installDir := filepath.Join(cfg.ToolsDir, fmt.Sprintf("%s-%s", toolName, toolState.Version))
		printInfof("Verifying %s (version %s)...\n", toolName, toolState.Version)

		// Determine verification strategy based on tool visibility
		if toolState.IsHidden {
			// Hidden tools: verify with absolute path
			printInfo("  Tool is hidden (not in PATH)")
			verifyWithAbsolutePath(r, toolName, toolState.Version, installDir)
		} else {
			// Visible tools: comprehensive verification
			verifyVisibleTool(r, toolName, &toolState, installDir, cfg)
		}

		printInfof("%s is working correctly\n", toolName)
	},
}
