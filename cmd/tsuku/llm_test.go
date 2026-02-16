package main

import (
	"slices"
	"testing"
)

func TestLLMCommandStructure(t *testing.T) {
	t.Run("llm command exists", func(t *testing.T) {
		if llmCmd == nil {
			t.Fatal("llmCmd should not be nil")
		}
		if llmCmd.Use != "llm" {
			t.Errorf("llmCmd.Use = %q, want %q", llmCmd.Use, "llm")
		}
		if llmCmd.Short == "" {
			t.Error("llmCmd.Short should not be empty")
		}
	})

	t.Run("download subcommand exists", func(t *testing.T) {
		if llmDownloadCmd == nil {
			t.Fatal("llmDownloadCmd should not be nil")
		}
		if llmDownloadCmd.Use != "download" {
			t.Errorf("llmDownloadCmd.Use = %q, want %q", llmDownloadCmd.Use, "download")
		}
		if llmDownloadCmd.Short == "" {
			t.Error("llmDownloadCmd.Short should not be empty")
		}
	})

	t.Run("download is subcommand of llm", func(t *testing.T) {
		if !slices.Contains(llmCmd.Commands(), llmDownloadCmd) {
			t.Error("llmDownloadCmd should be a subcommand of llmCmd")
		}
	})

	t.Run("llm is subcommand of root", func(t *testing.T) {
		if !slices.Contains(rootCmd.Commands(), llmCmd) {
			t.Error("llmCmd should be a subcommand of rootCmd")
		}
	})
}

func TestLLMDownloadFlags(t *testing.T) {
	t.Run("model flag", func(t *testing.T) {
		flag := llmDownloadCmd.Flags().Lookup("model")
		if flag == nil {
			t.Fatal("--model flag should exist")
		}
		if flag.DefValue != "" {
			t.Errorf("--model default should be empty, got %q", flag.DefValue)
		}
	})

	t.Run("force flag", func(t *testing.T) {
		flag := llmDownloadCmd.Flags().Lookup("force")
		if flag == nil {
			t.Fatal("--force flag should exist")
		}
		if flag.DefValue != "false" {
			t.Errorf("--force default should be false, got %q", flag.DefValue)
		}
	})

	t.Run("yes flag", func(t *testing.T) {
		flag := llmDownloadCmd.Flags().Lookup("yes")
		if flag == nil {
			t.Fatal("--yes flag should exist")
		}
		if flag.DefValue != "false" {
			t.Errorf("--yes default should be false, got %q", flag.DefValue)
		}
	})
}

func TestLLMDownloadNoArgs(t *testing.T) {
	// The command should accept no arguments
	if llmDownloadCmd.Args == nil {
		t.Fatal("llmDownloadCmd.Args should not be nil")
	}

	// Test that passing arguments is rejected
	err := llmDownloadCmd.Args(llmDownloadCmd, []string{"extra-arg"})
	if err == nil {
		t.Error("llmDownloadCmd should reject extra arguments")
	}
}

func TestFormatBytesHuman(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero bytes", 0, "0 bytes"},
		{"small bytes", 500, "500 bytes"},
		{"kilobytes", 2048, "2 KB"},
		{"megabytes", 50 * 1024 * 1024, "50 MB"},
		{"gigabytes", int64(2.5 * 1024 * 1024 * 1024), "2.5 GB"},
		{"one GB", 1024 * 1024 * 1024, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBytesHuman(tt.bytes)
			if got != tt.want {
				t.Errorf("formatBytesHuman(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
