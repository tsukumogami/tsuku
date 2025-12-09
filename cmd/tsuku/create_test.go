package main

import "testing"

func TestParseFromFlag(t *testing.T) {
	tests := []struct {
		name          string
		from          string
		wantBuilder   string
		wantSourceArg string
		wantIsGitHub  bool
	}{
		{
			name:          "ecosystem crates.io",
			from:          "crates.io",
			wantBuilder:   "crates.io",
			wantSourceArg: "",
			wantIsGitHub:  false,
		},
		{
			name:          "ecosystem pypi",
			from:          "pypi",
			wantBuilder:   "pypi",
			wantSourceArg: "",
			wantIsGitHub:  false,
		},
		{
			name:          "ecosystem npm",
			from:          "npm",
			wantBuilder:   "npm",
			wantSourceArg: "",
			wantIsGitHub:  false,
		},
		{
			name:          "ecosystem rubygems",
			from:          "rubygems",
			wantBuilder:   "rubygems",
			wantSourceArg: "",
			wantIsGitHub:  false,
		},
		{
			name:          "ecosystem cargo alias",
			from:          "cargo",
			wantBuilder:   "crates.io",
			wantSourceArg: "",
			wantIsGitHub:  false,
		},
		{
			name:          "github with lowercase",
			from:          "github:cli/cli",
			wantBuilder:   "github",
			wantSourceArg: "cli/cli",
			wantIsGitHub:  true,
		},
		{
			name:          "github with uppercase",
			from:          "GitHub:FiloSottile/age",
			wantBuilder:   "github",
			wantSourceArg: "FiloSottile/age",
			wantIsGitHub:  true,
		},
		{
			name:          "github with mixed case",
			from:          "GITHUB:stern/stern",
			wantBuilder:   "github",
			wantSourceArg: "stern/stern",
			wantIsGitHub:  true,
		},
		{
			name:          "github preserves sourceArg case",
			from:          "github:BurntSushi/ripgrep",
			wantBuilder:   "github",
			wantSourceArg: "BurntSushi/ripgrep",
			wantIsGitHub:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder, sourceArg, isGitHub := parseFromFlag(tt.from)
			if builder != tt.wantBuilder {
				t.Errorf("parseFromFlag(%q) builder = %q, want %q", tt.from, builder, tt.wantBuilder)
			}
			if sourceArg != tt.wantSourceArg {
				t.Errorf("parseFromFlag(%q) sourceArg = %q, want %q", tt.from, sourceArg, tt.wantSourceArg)
			}
			if isGitHub != tt.wantIsGitHub {
				t.Errorf("parseFromFlag(%q) isGitHub = %v, want %v", tt.from, isGitHub, tt.wantIsGitHub)
			}
		})
	}
}

func TestNormalizeEcosystem(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"crates.io", "crates.io", "crates.io"},
		{"crates_io", "crates_io", "crates.io"},
		{"crates", "crates", "crates.io"},
		{"cargo", "cargo", "crates.io"},
		{"rubygems", "rubygems", "rubygems"},
		{"rubygems.org", "rubygems.org", "rubygems"},
		{"gems", "gems", "rubygems"},
		{"gem", "gem", "rubygems"},
		{"pypi", "pypi", "pypi"},
		{"pypi.org", "pypi.org", "pypi"},
		{"pip", "pip", "pypi"},
		{"python", "python", "pypi"},
		{"npm", "npm", "npm"},
		{"npmjs", "npmjs", "npm"},
		{"npmjs.com", "npmjs.com", "npm"},
		{"node", "node", "npm"},
		{"nodejs", "nodejs", "npm"},
		{"unknown", "unknown", "unknown"},
		{"uppercase", "NPM", "npm"},
		{"mixed case", "PyPI", "pypi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeEcosystem(tt.input)
			if got != tt.want {
				t.Errorf("normalizeEcosystem(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
