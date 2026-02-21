package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/llm"
	"github.com/tsukumogami/tsuku/internal/llm/addon"
)

var llmCmd = &cobra.Command{
	Use:   "llm",
	Short: "Manage local LLM runtime",
	Long: `Manage the local LLM runtime used for recipe generation.

The local LLM runtime runs inference locally using a small language model,
enabling recipe generation without cloud API keys.`,
}

var (
	llmDownloadForce bool
	llmDownloadYes   bool
)

var llmDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Pre-download addon and model for offline use",
	Long: `Download the tsuku-llm addon binary and the appropriate language model
for local inference. This prepares the local LLM runtime for offline or
CI use without waiting for on-demand downloads during recipe generation.

The command detects available hardware (GPU, VRAM, RAM) and selects the
appropriate model size. To override automatic model selection, set
local_model in $TSUKU_HOME/config.toml.

Examples:
  tsuku llm download          # Auto-detect hardware, download addon + model
  tsuku llm download --force  # Re-download even if files exist
  tsuku llm download --yes    # Skip confirmation prompts (CI)`,
	RunE: runLLMDownload,
}

func init() {
	llmDownloadCmd.Flags().BoolVar(&llmDownloadForce, "force", false, "Re-download even if files already exist")
	llmDownloadCmd.Flags().BoolVar(&llmDownloadYes, "yes", false, "Skip download confirmation prompts")

	llmCmd.AddCommand(llmDownloadCmd)
}

// runLLMDownload implements the 'tsuku llm download' command.
// It ensures the addon binary is installed, starts the addon server to detect
// hardware, and triggers a model download by sending a lightweight inference
// request. The addon downloads the model on first inference call (no dedicated
// DownloadModel RPC exists), so a trivial Complete request forces the download.
func runLLMDownload(cmd *cobra.Command, args []string) error {
	ctx := globalCtx
	if ctx == nil {
		ctx = context.Background()
	}

	// Select prompter: --yes auto-approves, otherwise interactive
	var prompter addon.Prompter
	if llmDownloadYes {
		prompter = &addon.AutoApprovePrompter{}
	} else {
		prompter = &addon.InteractivePrompter{}
	}

	// Step 1: Ensure addon binary is available
	fmt.Fprintln(os.Stderr, "Checking addon binary...")
	addonManager := addon.NewAddonManager("", nil, "")
	addonManager.SetPrompter(prompter)

	addonPath, err := addonManager.EnsureAddon(ctx)
	if err != nil {
		if errors.Is(err, addon.ErrDownloadDeclined) {
			fmt.Fprintln(os.Stderr, "Error: addon download declined")
			fmt.Fprintln(os.Stderr, "Use --yes to skip confirmation prompts.")
			exitWithCode(ExitGeneral)
			return nil
		}
		fmt.Fprintf(os.Stderr, "Error: failed to ensure addon: %v\n", err)
		fmt.Fprintln(os.Stderr, "Install the addon first: tsuku install tsuku-llm")
		exitWithCode(ExitGeneral)
		return nil
	}
	fmt.Fprintf(os.Stderr, "Addon: %s\n", addonPath)

	// Step 2: Start the addon server for hardware detection and status
	fmt.Fprintln(os.Stderr, "\nDetecting hardware...")
	socketPath := llm.SocketPath()
	lifecycle := llm.NewServerLifecycle(socketPath, addonPath)
	lifecycle.SetIdleTimeout(llm.GetIdleTimeout())

	if err := lifecycle.EnsureRunning(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to start addon server: %v\n", err)
		exitWithCode(ExitGeneral)
		return nil
	}

	// Step 3: Connect and query hardware/model status
	provider := llm.NewLocalProvider()
	provider.SetPrompter(prompter)
	defer func() { _ = provider.Close() }()

	status, err := provider.GetStatus(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to query addon status: %v\n", err)
		exitWithCode(ExitGeneral)
		return nil
	}

	// Display hardware info
	if status.Backend != "" {
		fmt.Fprintf(os.Stderr, "Backend: %s\n", status.Backend)
	}
	if status.AvailableVramBytes > 0 {
		fmt.Fprintf(os.Stderr, "VRAM: %s\n", addon.FormatSize(status.AvailableVramBytes))
	}

	// Display selected model from addon's hardware-based selection
	if status.ModelName != "" {
		fmt.Fprintf(os.Stderr, "Selected model: %s\n", status.ModelName)
	}

	// Step 4: Check if everything is already present
	if status.Ready && status.ModelName != "" && !llmDownloadForce {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Addon and model already present.")
		printDownloadSummary(addonPath, status.ModelName, status.ModelSizeBytes)
		return nil
	}

	// Step 5: Model needs downloading -- prompt for confirmation
	modelDesc := "LLM model"
	if status.ModelName != "" {
		modelDesc = fmt.Sprintf("LLM model (%s)", status.ModelName)
	}

	ok, promptErr := prompter.ConfirmDownload(ctx, modelDesc, status.ModelSizeBytes)
	if promptErr != nil {
		if errors.Is(promptErr, addon.ErrDownloadDeclined) {
			fmt.Fprintln(os.Stderr, "Error: model download declined")
			fmt.Fprintln(os.Stderr, "Use --yes to skip confirmation prompts.")
			exitWithCode(ExitGeneral)
			return nil
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", promptErr)
		exitWithCode(ExitGeneral)
		return nil
	}
	if !ok {
		fmt.Fprintln(os.Stderr, "Error: model download declined")
		exitWithCode(ExitGeneral)
		return nil
	}

	// Step 6: Trigger model download by sending a lightweight inference request.
	// The addon downloads and loads the model on the first Complete call.
	fmt.Fprintln(os.Stderr, "\nDownloading model...")
	if err := provider.TriggerModelDownload(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to download model: %v\n", err)
		exitWithCode(ExitGeneral)
		return nil
	}

	// Step 7: Verify the model is now ready
	status, err = provider.GetStatus(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to verify model status: %v\n", err)
		exitWithCode(ExitGeneral)
		return nil
	}

	if !status.Ready || status.ModelName == "" {
		fmt.Fprintln(os.Stderr, "Error: model download completed but model is not ready")
		exitWithCode(ExitGeneral)
		return nil
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Download complete.")
	printDownloadSummary(addonPath, status.ModelName, status.ModelSizeBytes)

	return nil
}

// printDownloadSummary displays addon and model paths/sizes.
func printDownloadSummary(addonPath, modelName string, modelSizeBytes int64) {
	fmt.Fprintf(os.Stderr, "  Addon: %s\n", addonPath)
	if modelName != "" {
		fmt.Fprintf(os.Stderr, "  Model: %s", modelName)
		if modelSizeBytes > 0 {
			fmt.Fprintf(os.Stderr, " (%s)", addon.FormatSize(modelSizeBytes))
		}
		fmt.Fprintln(os.Stderr)
	}
}
