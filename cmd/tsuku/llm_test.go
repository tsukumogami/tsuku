package main

import (
	"testing"
)

func TestLLMCommandStructure(t *testing.T) {
	t.Run("llm command exists as subcommand of root", func(t *testing.T) {
		found := false
		for _, cmd := range rootCmd.Commands() {
			if cmd.Use == "llm" {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("expected 'llm' command to be registered on rootCmd")
		}
	})

	t.Run("llm has download subcommand", func(t *testing.T) {
		found := false
		for _, cmd := range llmCmd.Commands() {
			if cmd.Use == "download" {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("expected 'download' subcommand under 'llm'")
		}
	})

	t.Run("llm command has correct short description", func(t *testing.T) {
		if llmCmd.Short != "Manage local LLM runtime" {
			t.Errorf("llmCmd.Short = %q, want %q", llmCmd.Short, "Manage local LLM runtime")
		}
	})

	t.Run("download command has correct short description", func(t *testing.T) {
		want := "Pre-download addon and model for offline use"
		if llmDownloadCmd.Short != want {
			t.Errorf("llmDownloadCmd.Short = %q, want %q", llmDownloadCmd.Short, want)
		}
	})
}

func TestLLMDownloadFlags(t *testing.T) {
	t.Run("model flag removed", func(t *testing.T) {
		// The --model flag was removed because the addon selects models based
		// on hardware detection and the gRPC API has no way to request a
		// specific model. Model override is done via config.toml local_model.
		f := llmDownloadCmd.Flags().Lookup("model")
		if f != nil {
			t.Fatal("--model flag should not exist; model override is via config.toml local_model")
		}
	})

	t.Run("force flag exists", func(t *testing.T) {
		f := llmDownloadCmd.Flags().Lookup("force")
		if f == nil {
			t.Fatal("expected --force flag on download command")
		}
		if f.DefValue != "false" {
			t.Errorf("--force default = %q, want %q", f.DefValue, "false")
		}
	})

	t.Run("yes flag exists", func(t *testing.T) {
		f := llmDownloadCmd.Flags().Lookup("yes")
		if f == nil {
			t.Fatal("expected --yes flag on download command")
		}
		if f.DefValue != "false" {
			t.Errorf("--yes default = %q, want %q", f.DefValue, "false")
		}
	})
}

func TestLLMDownloadCommandUsesRunE(t *testing.T) {
	// The download command should use RunE (not Run) for proper error handling
	if llmDownloadCmd.RunE == nil {
		t.Fatal("expected llmDownloadCmd to use RunE, not Run")
	}
}

func TestLLMCommandGroupHasNoRunFunction(t *testing.T) {
	// The parent 'llm' command is a group and should not have Run/RunE.
	// Running it without a subcommand should show help.
	if llmCmd.Run != nil {
		t.Fatal("llmCmd should not have a Run function (it's a command group)")
	}
	if llmCmd.RunE != nil {
		t.Fatal("llmCmd should not have a RunE function (it's a command group)")
	}
}
