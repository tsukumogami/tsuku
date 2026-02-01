package discover

import (
	"testing"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"kubectl", "kubectl", false},
		{"  kubectl  ", "kubectl", false},
		{"Kubectl", "kubectl", false},
		{"RIPGREP", "ripgrep", false},
		{"my-tool", "my-tool", false},
		{"", "", true},
		{"   ", "", true},
		// Non-ASCII: Cyrillic "е" in "kubеctl"
		{"kub\u0435ctl", "", true},
	}
	for _, tt := range tests {
		got, err := NormalizeName(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("NormalizeName(%q): err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("NormalizeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
