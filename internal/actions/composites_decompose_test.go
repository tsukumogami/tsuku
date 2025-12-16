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
			name: "missing format",
			params: map[string]interface{}{
				"url":      "https://example.com/tool.tar.gz",
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
