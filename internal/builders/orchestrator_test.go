package builders

import "testing"

func TestExtractBinaryName(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{
			name:    "simple command",
			command: "tool --version",
			want:    "tool",
		},
		{
			name:    "command without args",
			command: "tool",
			want:    "tool",
		},
		{
			name:    "relative path",
			command: "./tool --version",
			want:    "./tool",
		},
		{
			name:    "absolute path",
			command: "/usr/bin/tool --version",
			want:    "/usr/bin/tool",
		},
		{
			name:    "multiple args",
			command: "tool -v --help --extra",
			want:    "tool",
		},
		{
			name:    "empty command",
			command: "",
			want:    "",
		},
		{
			name:    "whitespace only",
			command: "   ",
			want:    "",
		},
		{
			name:    "leading whitespace",
			command: "  tool --version",
			want:    "tool",
		},
		{
			name:    "command with equals arg",
			command: "tool --config=value",
			want:    "tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBinaryName(tt.command)
			if got != tt.want {
				t.Errorf("extractBinaryName(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}
