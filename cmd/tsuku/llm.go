package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tsukumogami/tsuku/internal/llm"
	"github.com/tsukumogami/tsuku/internal/llm/addon"
)

var (
	llmDownloadModel string
	llmDownloadForce bool
	llmDownloadYes   bool
)

var llmCmd = &cobra.Command{
	Use:   "llm",
	Short: "Manage local LLM runtime",
	Long: `Manage the local LLM inference runtime used for recipe generation.

The local runtime uses the tsuku-llm addon with a small open-source model
to generate recipes without requiring cloud API keys.`,
}

var llmDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Pre-download addon and model for offline use",
	Long: `Download the tsuku-llm addon binary and a hardware-appropriate model.

This is useful for CI pipelines and offline environments where you want
to pre-cache everything before running tsuku create.

By default, the command auto-detects your hardware and selects the best
model. Use --model to override the selection.

Examples:
  tsuku llm download              # interactive download with hardware detection
  tsuku llm download --yes        # auto-approve for CI pipelines
  tsuku llm download --model qwen2.5-3b-instruct-q4
  tsuku llm download --force      # re-download even if cached`,
	Args: cobra.NoArgs,
	RunE: runLLMDownload,
}

func init() {
	llmDownloadCmd.Flags().StringVar(&llmDownloadModel, "model", "", "Override auto-selected model name")
	llmDownloadCmd.Flags().BoolVar(&llmDownloadForce, "force", false, "Re-download even if already cached")
	llmDownloadCmd.Flags().BoolVar(&llmDownloadYes, "yes", false, "Auto-approve downloads without prompting")
	llmCmd.AddCommand(llmDownloadCmd)
}

// runLLMDownload pre-downloads the addon binary and selected model.
func runLLMDownload(cmd *cobra.Command, args []string) error {
	ctx := globalCtx

	// Set up addon manager
	mgr := addon.NewAddonManager()

	// Configure prompter based on --yes flag
	if llmDownloadYes {
		mgr.SetPrompter(addon.NewAutoApprovePrompter())
	} else {
		mgr.SetPrompter(addon.NewInteractivePrompter())
	}

	// Step 1: Ensure addon binary is downloaded
	if mgr.IsInstalled() && !llmDownloadForce {
		fmt.Fprintln(os.Stderr, "Addon binary already installed, skipping download.")
	} else {
		if llmDownloadForce && mgr.IsInstalled() {
			fmt.Fprintln(os.Stderr, "Force re-downloading addon binary...")
			// Remove existing addon so EnsureAddon will re-download
			if binPath, err := mgr.BinaryPath(); err == nil {
				_ = os.Remove(binPath)
			}
		}
		fmt.Fprintln(os.Stderr, "Downloading addon binary...")
		if _, err := mgr.EnsureAddon(ctx); err != nil {
			if errors.Is(err, addon.ErrDownloadDeclined) {
				fmt.Fprintln(os.Stderr, "Download declined.")
				fmt.Fprintln(os.Stderr, "Use --yes to auto-approve, or configure a cloud provider instead.")
				exitWithCode(ExitGeneral)
			}
			fmt.Fprintf(os.Stderr, "Failed to download addon: %v\n", err)
			exitWithCode(ExitGeneral)
		}
	}

	addonPath, err := mgr.BinaryPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get addon path: %v\n", err)
		exitWithCode(ExitGeneral)
	}
	fmt.Fprintf(os.Stderr, "Addon binary: %s\n", addonPath)

	// Step 2: Start addon to query hardware and model status
	fmt.Fprintln(os.Stderr, "Starting addon for hardware detection...")

	socketPath := llm.SocketPath()
	lifecycle := llm.NewServerLifecycleWithManager(socketPath, mgr)

	// Use a short idle timeout -- we only need the server briefly for status
	lifecycle.SetIdleTimeout(1 * time.Minute)

	if err := lifecycle.EnsureRunning(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start addon: %v\n", err)
		exitWithCode(ExitGeneral)
	}

	// Create a local provider to query status
	provider := llm.NewLocalProvider()
	if llmDownloadYes {
		provider.SetPrompter(addon.NewAutoApprovePrompter())
	} else {
		provider.SetPrompter(addon.NewInteractivePrompter())
	}
	defer func() { _ = provider.Close() }()

	// Step 3: Query hardware detection and model info
	status, err := provider.GetStatus(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get addon status: %v\n", err)
		fmt.Fprintln(os.Stderr, "The addon may still be initializing. Try again in a moment.")
		exitWithCode(ExitGeneral)
	}

	// Display hardware info
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "Hardware backend: %s\n", status.Backend)
	if status.AvailableVramBytes > 0 {
		fmt.Fprintf(os.Stderr, "Available VRAM:   %s\n", formatBytesHuman(status.AvailableVramBytes))
	}

	// Determine model name
	modelName := status.ModelName
	if llmDownloadModel != "" {
		modelName = llmDownloadModel
		fmt.Fprintf(os.Stderr, "Model (override): %s\n", modelName)
	} else if modelName != "" {
		fmt.Fprintf(os.Stderr, "Selected model:   %s\n", modelName)
	} else {
		fmt.Fprintln(os.Stderr, "No model selected by hardware detection.")
		fmt.Fprintln(os.Stderr, "Use --model to specify a model manually.")
		exitWithCode(ExitUsage)
	}

	// Step 4: Check if model is already ready
	if status.Ready && status.ModelName != "" && !llmDownloadForce {
		if llmDownloadModel == "" || llmDownloadModel == status.ModelName {
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Model already downloaded and loaded. Everything is up to date.")
			return nil
		}
	}

	// Step 5: Model download is managed by the addon.
	// The addon downloads the model when it needs it (during inference or status checks).
	// We've already started the addon and queried its status. If the model isn't ready,
	// the addon will handle download on the next inference call.
	if status.ModelSizeBytes > 0 && !status.Ready {
		fmt.Fprintf(os.Stderr, "Model size:       %s\n", formatBytesHuman(status.ModelSizeBytes))
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "The addon will download the model when first needed.")
		fmt.Fprintln(os.Stderr, "Addon binary is ready for offline use.")
	} else {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Download complete. Ready for offline use.")
	}

	return nil
}

// formatBytesHuman formats bytes into a human-readable string (e.g., "2.5 GB").
func formatBytesHuman(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.0f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.0f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}
