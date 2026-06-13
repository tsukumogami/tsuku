package actions

import (
	"testing"
)

func TestDownloadArchiveAction_Decompose(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &EvalContext{
		Version:    "1.2.3",
		VersionTag: "v1.2.3",
		OS:         "linux",
		Arch:       "amd64",
	}

	tests := []struct {
		name          string
		params        map[string]interface{}
		wantSteps     int
		wantActions   []string
		wantErr       bool
		checkDownload func(t *testing.T, params map[string]interface{})
		checkBinaries func(t *testing.T, params map[string]interface{})
	}{
		{
			name: "basic tar.gz archive",
			params: map[string]interface{}{
				"url":            "https://example.com/tool-{version}-{os}-{arch}.tar.gz",
				"archive_format": "tar.gz",
				"binaries":       []interface{}{"tool"},
			},
			wantSteps:   4,
			wantActions: []string{"download_file", "extract", "chmod", "install_binaries"},
			checkDownload: func(t *testing.T, params map[string]interface{}) {
				url := params["url"].(string)
				if url != "https://example.com/tool-1.2.3-linux-amd64.tar.gz" {
					t.Errorf("url = %q, want variables expanded", url)
				}
			},
		},
		{
			name: "zip archive with strip_prefix",
			params: map[string]interface{}{
				"url":            "https://example.com/tool.zip",
				"archive_format": "zip",
				"strip_prefix":   "tool-1.0/",
				"binaries":       []interface{}{"bin/tool"},
			},
			wantSteps:   4,
			wantActions: []string{"download_file", "extract", "chmod", "install_binaries"},
		},
		{
			name: "with version_tag variable",
			params: map[string]interface{}{
				"url":            "https://example.com/{version_tag}/tool.tar.gz",
				"archive_format": "tar.gz",
				"binaries":       []interface{}{"tool"},
			},
			wantSteps:   4,
			wantActions: []string{"download_file", "extract", "chmod", "install_binaries"},
			checkDownload: func(t *testing.T, params map[string]interface{}) {
				url := params["url"].(string)
				if url != "https://example.com/v1.2.3/tool.tar.gz" {
					t.Errorf("url = %q, want version_tag expanded", url)
				}
			},
		},
		{
			name: "missing url",
			params: map[string]interface{}{
				"archive_format": "tar.gz",
				"binaries":       []interface{}{"tool"},
			},
			wantErr: true,
		},
		{
			name: "missing format with undetectable URL",
			params: map[string]interface{}{
				"url":      "https://example.com/download?file=tool",
				"binaries": []interface{}{"tool"},
			},
			wantErr: true,
		},
		{
			name: "missing binaries",
			params: map[string]interface{}{
				"url":            "https://example.com/tool.tar.gz",
				"archive_format": "tar.gz",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps, err := action.Decompose(ctx, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Decompose() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if len(steps) != tt.wantSteps {
				t.Errorf("len(steps) = %d, want %d", len(steps), tt.wantSteps)
			}

			for i, wantAction := range tt.wantActions {
				if i < len(steps) && steps[i].Action != wantAction {
					t.Errorf("steps[%d].Action = %q, want %q", i, steps[i].Action, wantAction)
				}
			}

			if tt.checkDownload != nil && len(steps) > 0 {
				tt.checkDownload(t, steps[0].Params)
			}
			if tt.checkBinaries != nil && len(steps) > 3 {
				tt.checkBinaries(t, steps[3].Params)
			}
		})
	}
}

// TestDownloadArchiveAction_Decompose_ChecksumURL exercises the checksum_url
// forwarding wired in by Issue 2. Without a Downloader configured, no plan-
// time fetch happens, so the test only confirms the field plumbing (the
// composite reads checksum_url and the helper accepts it without error);
// the install-time validation path is exercised by integration tests.
func TestDownloadArchiveAction_Decompose_ChecksumURL(t *testing.T) {
	t.Parallel()
	action := &DownloadArchiveAction{}
	ctx := &EvalContext{
		Version: "1.2.3", VersionTag: "v1.2.3", OS: "linux", Arch: "amd64",
	}

	// No checksum_url: shape byte-identical to pre-change behavior.
	stepsNo, err := action.Decompose(ctx, map[string]interface{}{
		"url":            "https://example.com/tool-{version}.tar.gz",
		"archive_format": "tar.gz",
		"binaries":       []interface{}{"tool"},
	})
	if err != nil {
		t.Fatalf("control case error: %v", err)
	}
	if _, ok := stepsNo[0].Params["checksum_url"]; ok {
		t.Errorf("control case should NOT carry checksum_url; got: %v", stepsNo[0].Params)
	}

	// With checksum_url: the parameter is accepted (Decompose does not error).
	// The composite itself does not propagate checksum_url to the decomposed
	// download_file step — DownloadAction.Decompose consumes it at plan time
	// when a Downloader is available. Here we just confirm no error.
	_, err = action.Decompose(ctx, map[string]interface{}{
		"url":            "https://example.com/tool-{version}.tar.gz",
		"archive_format": "tar.gz",
		"binaries":       []interface{}{"tool"},
		"checksum_url":   "https://example.com/v{version}/SHA256SUMS",
	})
	if err != nil {
		t.Fatalf("checksum_url case error: %v", err)
	}
}

// TestGitHubArchiveAction_Decompose_ChecksumFields exercises Issue 2 and
// Issue 3 plumbing: both checksum_url (escape hatch) and checksum_asset
// (ergonomic default) are accepted on github_archive without error. With no
// Downloader configured, the plan-time validation path no-ops; the test
// confirms the parameter handling.
func TestGitHubArchiveAction_Decompose_ChecksumFields(t *testing.T) {
	t.Parallel()
	action := &GitHubArchiveAction{}
	ctx := &EvalContext{
		Version: "1.2.3", VersionTag: "v1.2.3", OS: "linux", Arch: "amd64",
	}

	base := map[string]interface{}{
		"repo":          "example/tool",
		"asset_pattern": "tool-{version}-{os}-{arch}.tar.gz",
		"binaries":      []interface{}{"tool"},
	}

	// Control: no checksum field, existing behavior.
	stepsNo, err := action.Decompose(ctx, base)
	if err != nil {
		t.Fatalf("control case error: %v", err)
	}
	if _, ok := stepsNo[0].Params["checksum_url"]; ok {
		t.Errorf("control case should NOT carry checksum_url; got: %v", stepsNo[0].Params)
	}

	// With checksum_url: parameter accepted.
	paramsURL := map[string]interface{}{
		"checksum_url": "https://example.com/v{version}/checksums.txt",
	}
	for k, v := range base {
		paramsURL[k] = v
	}
	_, err = action.Decompose(ctx, paramsURL)
	if err != nil {
		t.Fatalf("checksum_url case error: %v", err)
	}

	// With checksum_asset: parameter accepted; sibling-URL construction
	// happens inside resolveGitHubArchiveChecksumURL.
	paramsAsset := map[string]interface{}{
		"checksum_asset": "tool-checksums-{version}.txt",
	}
	for k, v := range base {
		paramsAsset[k] = v
	}
	_, err = action.Decompose(ctx, paramsAsset)
	if err != nil {
		t.Fatalf("checksum_asset case error: %v", err)
	}
}

// TestResolveGitHubArchiveChecksumURL covers the helper introduced by Issue 3
// for sibling-URL construction at the composite layer (keeping the primitive
// download action GitHub-agnostic).
func TestResolveGitHubArchiveChecksumURL(t *testing.T) {
	t.Parallel()
	ctx := &EvalContext{Version: "1.2.3", OS: "linux", Arch: "amd64"}

	tests := []struct {
		name    string
		params  map[string]interface{}
		repo    string
		tag     string
		wantURL string
	}{
		{
			name:    "neither field set returns empty",
			params:  map[string]interface{}{},
			repo:    "example/tool",
			tag:     "v1.2.3",
			wantURL: "",
		},
		{
			name:    "checksum_asset constructs sibling URL on same release",
			params:  map[string]interface{}{"checksum_asset": "SHA256SUMS"},
			repo:    "example/tool",
			tag:     "v1.2.3",
			wantURL: "https://github.com/example/tool/releases/download/v1.2.3/SHA256SUMS",
		},
		{
			name:    "checksum_asset with placeholders expands against ctx",
			params:  map[string]interface{}{"checksum_asset": "tool-{version}.sha256"},
			repo:    "example/tool",
			tag:     "v1.2.3",
			wantURL: "https://github.com/example/tool/releases/download/v1.2.3/tool-1.2.3.sha256",
		},
		{
			name:    "checksum_url with placeholders expands",
			params:  map[string]interface{}{"checksum_url": "https://example.com/v{version}/checksums-{os}-{arch}.txt"},
			repo:    "example/tool",
			tag:     "v1.2.3",
			wantURL: "https://example.com/v1.2.3/checksums-linux-amd64.txt",
		},
		{
			name: "checksum_asset takes precedence over checksum_url (Preflight enforces mutual exclusion separately)",
			params: map[string]interface{}{
				"checksum_asset": "SHA256SUMS",
				"checksum_url":   "https://example.com/ignored.txt",
			},
			repo:    "example/tool",
			tag:     "v1.2.3",
			wantURL: "https://github.com/example/tool/releases/download/v1.2.3/SHA256SUMS",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveGitHubArchiveChecksumURL(ctx, tt.params, tt.repo, tt.tag)
			if got != tt.wantURL {
				t.Errorf("resolveGitHubArchiveChecksumURL = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

func TestGitHubFileAction_Decompose(t *testing.T) {
	t.Parallel()
	action := &GitHubFileAction{}
	ctx := &EvalContext{
		Version:    "1.2.3",
		VersionTag: "v1.2.3",
		OS:         "linux",
		Arch:       "amd64",
	}

	tests := []struct {
		name          string
		params        map[string]interface{}
		wantSteps     int
		wantActions   []string
		wantErr       bool
		checkDownload func(t *testing.T, params map[string]interface{})
	}{
		{
			name: "basic github file download with binary param",
			params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "tool-{os}-{arch}",
				"binary":        "tool",
			},
			wantSteps:   3,
			wantActions: []string{"download_file", "chmod", "install_binaries"},
			checkDownload: func(t *testing.T, params map[string]interface{}) {
				url := params["url"].(string)
				expected := "https://github.com/owner/repo/releases/download/v1.2.3/tool-linux-amd64"
				if url != expected {
					t.Errorf("url = %q, want %q", url, expected)
				}
			},
		},
		{
			name: "with binaries map format",
			params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "tool-{version}",
				"binaries": []interface{}{
					map[string]interface{}{
						"src":  "tool",
						"dest": "tool-bin",
					},
				},
			},
			wantSteps:   3,
			wantActions: []string{"download_file", "chmod", "install_binaries"},
			checkDownload: func(t *testing.T, params map[string]interface{}) {
				url := params["url"].(string)
				expected := "https://github.com/owner/repo/releases/download/v1.2.3/tool-1.2.3"
				if url != expected {
					t.Errorf("url = %q, want %q", url, expected)
				}
			},
		},
		{
			name: "wildcard pattern requires resolver",
			params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "tool-*",
				"binary":        "tool",
			},
			wantErr: true, // No resolver available
		},
		{
			name: "missing repo",
			params: map[string]interface{}{
				"asset_pattern": "tool",
				"binary":        "tool",
			},
			wantErr: true,
		},
		{
			name: "missing asset_pattern",
			params: map[string]interface{}{
				"repo":   "owner/repo",
				"binary": "tool",
			},
			wantErr: true,
		},
		{
			name: "missing binaries and binary",
			params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "tool",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps, err := action.Decompose(ctx, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Decompose() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if len(steps) != tt.wantSteps {
				t.Errorf("len(steps) = %d, want %d", len(steps), tt.wantSteps)
			}

			for i, wantAction := range tt.wantActions {
				if i < len(steps) && steps[i].Action != wantAction {
					t.Errorf("steps[%d].Action = %q, want %q", i, steps[i].Action, wantAction)
				}
			}

			if tt.checkDownload != nil && len(steps) > 0 {
				tt.checkDownload(t, steps[0].Params)
			}
		})
	}
}

func TestDecompose_AllReturnPrimitives(t *testing.T) {
	t.Parallel()
	// Verify all decompose methods return only primitive actions
	ctx := &EvalContext{
		Version:    "1.0.0",
		VersionTag: "v1.0.0",
		OS:         "linux",
		Arch:       "amd64",
	}

	tests := []struct {
		name   string
		action Decomposable
		params map[string]interface{}
	}{
		{
			name:   "DownloadArchiveAction",
			action: &DownloadArchiveAction{},
			params: map[string]interface{}{
				"url":            "https://example.com/tool.tar.gz",
				"archive_format": "tar.gz",
				"binaries":       []interface{}{"tool"},
			},
		},
		{
			name:   "GitHubFileAction",
			action: &GitHubFileAction{},
			params: map[string]interface{}{
				"repo":          "owner/repo",
				"asset_pattern": "tool",
				"binary":        "tool",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps, err := tt.action.Decompose(ctx, tt.params)
			if err != nil {
				t.Fatalf("Decompose() error = %v", err)
			}

			for i, step := range steps {
				if !IsPrimitive(step.Action) {
					t.Errorf("steps[%d].Action = %q is not a primitive", i, step.Action)
				}
			}
		})
	}
}
