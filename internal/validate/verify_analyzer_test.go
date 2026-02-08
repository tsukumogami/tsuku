package validate

import (
	"testing"
)

func TestAnalyzeVerifyFailure_HelpTextDetection(t *testing.T) {
	tests := []struct {
		name       string
		stdout     string
		stderr     string
		exitCode   int
		toolName   string
		wantRepair bool
		wantMode   string
	}{
		{
			name:       "usage pattern in stderr",
			stdout:     "",
			stderr:     "Usage: mytool [options] <file>\nOptions:\n  -h  Show help",
			exitCode:   1,
			toolName:   "mytool",
			wantRepair: true,
			wantMode:   "output",
		},
		{
			name:       "usage pattern in stdout",
			stdout:     "usage: mytool command [flags]\n\nCommands:\n  help  Show help",
			stderr:     "",
			exitCode:   2,
			toolName:   "mytool",
			wantRepair: true,
			wantMode:   "output",
		},
		{
			name:       "command not found - exit 127",
			stdout:     "",
			stderr:     "bash: mytool: command not found",
			exitCode:   127,
			toolName:   "mytool",
			wantRepair: false,
		},
		{
			name:       "exit code 0 - should not reach here",
			stdout:     "mytool version 1.0.0",
			stderr:     "",
			exitCode:   0,
			toolName:   "mytool",
			wantRepair: false,
		},
		{
			name:       "empty output",
			stdout:     "",
			stderr:     "",
			exitCode:   1,
			toolName:   "mytool",
			wantRepair: false,
		},
		{
			name:       "short error without help patterns",
			stdout:     "",
			stderr:     "error: unknown flag",
			exitCode:   1,
			toolName:   "mytool",
			wantRepair: false,
		},
		{
			name:       "options section header",
			stdout:     "mytool - a tool\n\nOptions:\n  --help  Show help\n  --verbose  Verbose output",
			stderr:     "",
			exitCode:   1,
			toolName:   "mytool",
			wantRepair: true,
			wantMode:   "output",
		},
		{
			name:       "case insensitive usage",
			stdout:     "",
			stderr:     "USAGE: MYTOOL [OPTIONS]\n\nOPTIONS:\n  -h  help",
			exitCode:   2,
			toolName:   "mytool",
			wantRepair: true,
			wantMode:   "output",
		},
		{
			name:       "real git help output",
			stdout:     "",
			stderr:     "usage: git [-v | --version] [-h | --help] [-C <path>] [-c <name>=<value>]\n           [--exec-path[=<path>]] [--html-path] [--man-path] [--info-path]",
			exitCode:   1,
			toolName:   "git",
			wantRepair: true,
			wantMode:   "output",
		},
		{
			name:       "real curl help output",
			stdout:     "Usage: curl [options...] <url>\n -d, --data <data>           HTTP POST data\n -f, --fail                  Fail fast with no output on HTTP errors",
			stderr:     "",
			exitCode:   2,
			toolName:   "curl",
			wantRepair: true,
			wantMode:   "output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := AnalyzeVerifyFailure(tt.stdout, tt.stderr, tt.exitCode, tt.toolName)

			if analysis.Repairable != tt.wantRepair {
				t.Errorf("Repairable = %v, want %v", analysis.Repairable, tt.wantRepair)
			}

			if tt.wantRepair && analysis.SuggestedMode != tt.wantMode {
				t.Errorf("SuggestedMode = %q, want %q", analysis.SuggestedMode, tt.wantMode)
			}

			if analysis.ExitCode != tt.exitCode {
				t.Errorf("ExitCode = %d, want %d", analysis.ExitCode, tt.exitCode)
			}
		})
	}
}

func TestAnalyzeVerifyFailure_ToolNameDetection(t *testing.T) {
	analysis := AnalyzeVerifyFailure(
		"",
		"mytool: unrecognized option '--version'\nTry 'mytool --help' for more information.",
		1,
		"mytool",
	)

	if !analysis.HasToolName {
		t.Error("expected HasToolName = true")
	}

	// Should be repairable because:
	// - Exit code is 1
	// - Contains tool name
	// - Contains help suggestion
	// - Long enough output
	if !analysis.Repairable {
		t.Error("expected Repairable = true for output with tool name and help suggestion")
	}
}

func TestAnalyzeVerifyFailure_PatternSuggestion(t *testing.T) {
	tests := []struct {
		name        string
		stdout      string
		stderr      string
		toolName    string
		wantPattern string
	}{
		{
			name:        "usage pattern preferred",
			stdout:      "usage: mytool [options]",
			stderr:      "",
			toolName:    "mytool",
			wantPattern: "usage",
		},
		{
			name:        "tool name fallback when no usage",
			stdout:      "mytool: Options:\n  --help  Show help\n  --verbose  Verbose output",
			stderr:      "",
			toolName:    "mytool",
			wantPattern: "mytool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := AnalyzeVerifyFailure(tt.stdout, tt.stderr, 1, tt.toolName)

			if !analysis.Repairable {
				t.Fatal("expected Repairable = true")
			}

			if analysis.SuggestedPattern != tt.wantPattern {
				t.Errorf("SuggestedPattern = %q, want %q", analysis.SuggestedPattern, tt.wantPattern)
			}
		})
	}
}

func TestIsHelpExitCode(t *testing.T) {
	tests := []struct {
		exitCode int
		want     bool
	}{
		{0, false},
		{1, true},
		{2, true},
		{3, false},
		{126, false},
		{127, false},
	}

	for _, tt := range tests {
		if got := IsHelpExitCode(tt.exitCode); got != tt.want {
			t.Errorf("IsHelpExitCode(%d) = %v, want %v", tt.exitCode, got, tt.want)
		}
	}
}

func TestIsNotFoundExitCode(t *testing.T) {
	tests := []struct {
		exitCode int
		want     bool
	}{
		{0, false},
		{1, false},
		{126, false},
		{127, true},
	}

	for _, tt := range tests {
		if got := IsNotFoundExitCode(tt.exitCode); got != tt.want {
			t.Errorf("IsNotFoundExitCode(%d) = %v, want %v", tt.exitCode, got, tt.want)
		}
	}
}

func TestEscapeForPattern(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mytool", "mytool"},
		{"my.tool", `my\.tool`},
		{"my-tool", "my-tool"},
		{"tool++", `tool\+\+`},
		{"tool(1)", `tool\(1\)`},
	}

	for _, tt := range tests {
		if got := escapeForPattern(tt.input); got != tt.want {
			t.Errorf("escapeForPattern(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
