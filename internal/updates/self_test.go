package updates

import (
	"testing"
)

func TestIsSelfUpdate(t *testing.T) {
	tests := []struct {
		name string
		tool string
		want bool
	}{
		{"self update entry", SelfToolName, true},
		{"other tool", "ripgrep", false},
		{"empty tool", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &UpdateCheckEntry{Tool: tt.tool}
			if got := IsSelfUpdate(entry); got != tt.want {
				t.Errorf("IsSelfUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseChecksumForAsset(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		asset     string
		wantHash  string
		wantError bool
	}{
		{
			name:     "valid checksum line",
			data:     "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789  tsuku-linux-amd64\n",
			asset:    "tsuku-linux-amd64",
			wantHash: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		},
		{
			name: "multiple assets",
			data: "1111111111111111111111111111111111111111111111111111111111111111  tsuku-darwin-arm64\n" +
				"2222222222222222222222222222222222222222222222222222222222222222  tsuku-linux-amd64\n" +
				"3333333333333333333333333333333333333333333333333333333333333333  tsuku-linux-arm64\n",
			asset:    "tsuku-linux-amd64",
			wantHash: "2222222222222222222222222222222222222222222222222222222222222222",
		},
		{
			name:      "missing asset",
			data:      "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789  tsuku-linux-amd64\n",
			asset:     "tsuku-darwin-arm64",
			wantError: true,
		},
		{
			name:      "malformed hash too short",
			data:      "abcdef  tsuku-linux-amd64\n",
			asset:     "tsuku-linux-amd64",
			wantError: true,
		},
		{
			name:      "malformed hash non-hex",
			data:      "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz  tsuku-linux-amd64\n",
			asset:     "tsuku-linux-amd64",
			wantError: true,
		},
		{
			name:      "empty data",
			data:      "",
			asset:     "tsuku-linux-amd64",
			wantError: true,
		},
		{
			name:      "blank lines only",
			data:      "\n\n\n",
			asset:     "tsuku-linux-amd64",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := parseChecksumForAsset([]byte(tt.data), tt.asset)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error, got hash %q", hash)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hash != tt.wantHash {
				t.Errorf("got hash %q, want %q", hash, tt.wantHash)
			}
		})
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"equal", "1.2.3", "1.2.3", 0},
		{"a newer major", "2.0.0", "1.0.0", 1},
		{"b newer major", "1.0.0", "2.0.0", -1},
		{"a newer minor", "1.3.0", "1.2.0", 1},
		{"b newer minor", "1.2.0", "1.3.0", -1},
		{"a newer patch", "1.2.4", "1.2.3", 1},
		{"b newer patch", "1.2.3", "1.2.4", -1},
		{"different segment counts a shorter", "1.2", "1.2.0", 0},
		{"different segment counts b shorter", "1.2.0", "1.2", 0},
		{"different segment counts with diff", "1.2", "1.2.1", -1},
		{"single segment", "2", "1", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareSemver(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CompareSemver(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
