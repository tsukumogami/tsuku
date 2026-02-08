package validate

import (
	"os/exec"
	"testing"
)

// TestAnalyzeVerifyFailure_RealCLITools tests the analyzer against real CLI tool output.
// These tests require the tools to be installed on the system.
func TestAnalyzeVerifyFailure_RealCLITools(t *testing.T) {
	tests := []struct {
		name       string
		tool       string
		args       []string
		wantRepair bool
		skip       string // reason to skip if tool not available
	}{
		{
			name:       "goimports -h produces repairable help",
			tool:       "goimports",
			args:       []string{"-h"},
			wantRepair: true,
		},
		{
			name:       "git --invalid produces repairable help",
			tool:       "git",
			args:       []string{"--invalid-flag-xyz"},
			wantRepair: true, // exit 129 with usage: pattern
		},
		{
			name:       "go help exits 0 - not repairable (success)",
			tool:       "go",
			args:       []string{"help"},
			wantRepair: false, // exit 0 means the command succeeded
		},
		{
			name:       "go invalid exits non-zero with help suggestion",
			tool:       "go",
			args:       []string{"invalid-command-xyz"},
			wantRepair: true, // has tool name + help suggestion
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check if tool exists
			if _, err := exec.LookPath(tt.tool); err != nil {
				t.Skipf("tool %s not found, skipping", tt.tool)
			}

			// Run the command
			cmd := exec.Command(tt.tool, tt.args...)
			output, _ := cmd.CombinedOutput()
			exitCode := 0
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}

			// Analyze the output
			analysis := AnalyzeVerifyFailure(string(output), "", exitCode, tt.tool)

			t.Logf("Exit code: %d, Output length: %d, HasUsage: %v, HasToolName: %v",
				exitCode, len(output), analysis.HasUsageText, analysis.HasToolName)

			if analysis.Repairable != tt.wantRepair {
				t.Errorf("Repairable = %v, want %v\nOutput: %s", analysis.Repairable, tt.wantRepair, string(output))
			}
		})
	}
}
