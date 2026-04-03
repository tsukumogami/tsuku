package project

import (
	"testing"
)

func TestSplitOrgKey(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		wantSource string
		wantBare   string
		wantOrg    bool
		wantErr    bool
	}{
		{
			name:       "bare key",
			key:        "node",
			wantSource: "",
			wantBare:   "node",
			wantOrg:    false,
		},
		{
			name:       "bare key with hyphen",
			key:        "cargo-audit",
			wantSource: "",
			wantBare:   "cargo-audit",
			wantOrg:    false,
		},
		{
			name:       "org scoped simple",
			key:        "tsukumogami/koto",
			wantSource: "tsukumogami/koto",
			wantBare:   "koto",
			wantOrg:    true,
		},
		{
			name:       "org scoped with explicit recipe",
			key:        "tsukumogami/registry:mytool",
			wantSource: "tsukumogami/registry",
			wantBare:   "mytool",
			wantOrg:    true,
		},
		{
			name:       "org scoped with version",
			key:        "myorg/repo@1.2.3",
			wantSource: "myorg/repo",
			wantBare:   "repo",
			wantOrg:    true,
		},
		{
			name:       "org scoped with recipe and version",
			key:        "myorg/repo:tool@2.0.0",
			wantSource: "myorg/repo",
			wantBare:   "tool",
			wantOrg:    true,
		},
		{
			name:    "path traversal rejected",
			key:     "../etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal in middle",
			key:     "myorg/../other",
			wantErr: true,
		},
		{
			name:    "triple slash rejected",
			key:     "a/b/c",
			wantErr: true,
		},
		{
			name:    "empty owner rejected",
			key:     "/repo",
			wantErr: true,
		},
		{
			name:    "empty repo rejected",
			key:     "owner/",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, bare, isOrg, err := SplitOrgKey(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Errorf("SplitOrgKey(%q) expected error, got nil", tt.key)
				}
				return
			}
			if err != nil {
				t.Fatalf("SplitOrgKey(%q) unexpected error: %v", tt.key, err)
			}
			if source != tt.wantSource {
				t.Errorf("source = %q, want %q", source, tt.wantSource)
			}
			if bare != tt.wantBare {
				t.Errorf("bare = %q, want %q", bare, tt.wantBare)
			}
			if isOrg != tt.wantOrg {
				t.Errorf("isOrgScoped = %v, want %v", isOrg, tt.wantOrg)
			}
		})
	}
}
