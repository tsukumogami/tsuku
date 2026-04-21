package actions

import (
	"strings"
	"testing"
)

// TestActionDescriber_TypeAssertion verifies that all 10 expected action types
// implement ActionDescriber and that StatusMessage returns a non-empty string
// for representative param sets.
func TestActionDescriber_TypeAssertion(t *testing.T) {
	tests := []struct {
		actionName string
		params     map[string]interface{}
		wantPrefix string // expected prefix in the returned message
	}{
		{
			actionName: "download_file",
			params:     map[string]interface{}{"url": "https://example.com/tool_1.0_linux.tar.gz"},
			wantPrefix: "Downloading ",
		},
		{
			actionName: "extract",
			params:     map[string]interface{}{"archive": "tool_1.0_linux.tar.gz", "format": "tar.gz"},
			wantPrefix: "Extracting ",
		},
		{
			actionName: "install_binaries",
			params: map[string]interface{}{
				"outputs": []interface{}{
					map[string]interface{}{"src": "tool", "dest": "bin/tool"},
				},
			},
			wantPrefix: "Installing ",
		},
		{
			actionName: "configure_make",
			params:     map[string]interface{}{"source_dir": "openssl-3.0.0"},
			wantPrefix: "Building ",
		},
		{
			actionName: "cargo_build",
			params:     map[string]interface{}{"crate": "ripgrep", "version": "14.0.0", "executables": []interface{}{"rg"}, "lock_data": "x", "lock_checksum": "y"},
			wantPrefix: "Building ",
		},
		{
			actionName: "go_build",
			params:     map[string]interface{}{"module": "github.com/cli/cli", "version": "v2.0.0", "executables": []interface{}{"gh"}, "go_sum": "x"},
			wantPrefix: "Building ",
		},
		{
			actionName: "cargo_install",
			params:     map[string]interface{}{"crate": "ripgrep", "executables": []interface{}{"rg"}},
			wantPrefix: "cargo install ",
		},
		{
			actionName: "npm_install",
			params:     map[string]interface{}{"package": "typescript", "executables": []interface{}{"tsc"}},
			wantPrefix: "npm install ",
		},
		{
			actionName: "pipx_install",
			params:     map[string]interface{}{"package": "black", "executables": []interface{}{"black"}},
			wantPrefix: "pipx install ",
		},
		{
			actionName: "gem_install",
			params:     map[string]interface{}{"gem": "bundler", "executables": []interface{}{"bundle"}},
			wantPrefix: "gem install ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.actionName, func(t *testing.T) {
			action := Get(tt.actionName)
			if action == nil {
				t.Fatalf("Get(%q) = nil, action not registered", tt.actionName)
			}

			describer, ok := action.(ActionDescriber)
			if !ok {
				t.Fatalf("action %q does not implement ActionDescriber", tt.actionName)
			}

			msg := describer.StatusMessage(tt.params)
			if msg == "" {
				t.Errorf("action %q: StatusMessage returned empty string, want non-empty", tt.actionName)
				return
			}
			if !strings.HasPrefix(msg, tt.wantPrefix) {
				t.Errorf("action %q: StatusMessage = %q, want prefix %q", tt.actionName, msg, tt.wantPrefix)
			}
		})
	}
}

// TestActionDescriber_FallbackForNonImplementing verifies that a non-implementing
// action (mockAction) does not satisfy ActionDescriber, so callers fall back to
// the action name.
func TestActionDescriber_FallbackForNonImplementing(t *testing.T) {
	// mockAction (defined in action_test.go) does not implement ActionDescriber.
	// Use an Action interface variable so that the type assertion compiles.
	var action Action = &mockAction{name: "test_no_describer"}

	_, ok := action.(ActionDescriber)
	if ok {
		t.Error("mockAction unexpectedly implements ActionDescriber; fallback test is invalid")
	}

	// Simulate the executor fallback: if not ActionDescriber, use action name.
	var msg string
	if d, ok := action.(ActionDescriber); ok {
		msg = d.StatusMessage(nil)
	}
	if msg == "" {
		msg = action.Name()
	}

	if msg != "test_no_describer" {
		t.Errorf("fallback message = %q, want %q", msg, "test_no_describer")
	}
}

// TestActionDescriber_EmptyReturnFallback verifies that when StatusMessage returns
// "", the executor falls back to the action name.
func TestActionDescriber_EmptyReturnFallback(t *testing.T) {
	// download_file with no url returns "".
	action := Get("download_file")
	if action == nil {
		t.Fatal("download_file action not registered")
	}

	describer, ok := action.(ActionDescriber)
	if !ok {
		t.Fatal("download_file does not implement ActionDescriber")
	}

	// Empty params -> no url -> StatusMessage returns "".
	msg := describer.StatusMessage(map[string]interface{}{})
	if msg != "" {
		t.Errorf("StatusMessage with no url = %q, want empty string", msg)
	}

	// Simulate executor fallback.
	actionMsg := msg
	if actionMsg == "" {
		actionMsg = action.Name()
	}
	if actionMsg != "download_file" {
		t.Errorf("fallback message = %q, want %q", actionMsg, "download_file")
	}
}

// TestDownloadFile_StatusMessage_WithSize verifies the size suffix is appended.
func TestDownloadFile_StatusMessage_WithSize(t *testing.T) {
	action := Get("download_file")
	describer, ok := action.(ActionDescriber)
	if !ok {
		t.Fatal("download_file does not implement ActionDescriber")
	}

	params := map[string]interface{}{
		"url":  "https://example.com/tool.tar.gz",
		"size": float64(1536),
	}

	msg := describer.StatusMessage(params)
	if !strings.HasPrefix(msg, "Downloading tool.tar.gz") {
		t.Errorf("StatusMessage = %q, want prefix %q", msg, "Downloading tool.tar.gz")
	}
	if !strings.Contains(msg, "(") {
		t.Errorf("StatusMessage = %q, expected size suffix in parentheses", msg)
	}
}

// TestActionDescriber_ANSIStripped verifies ANSI escape sequences are sanitized.
func TestActionDescriber_ANSIStripped(t *testing.T) {
	action := Get("extract")
	describer, ok := action.(ActionDescriber)
	if !ok {
		t.Fatal("extract does not implement ActionDescriber")
	}

	params := map[string]interface{}{
		"archive": "\x1b[31mmalicious\x1b[0m.tar.gz",
		"format":  "tar.gz",
	}

	msg := describer.StatusMessage(params)
	if strings.Contains(msg, "\x1b") {
		t.Errorf("StatusMessage contains ANSI escape: %q", msg)
	}
	if !strings.Contains(msg, "malicious.tar.gz") {
		t.Errorf("StatusMessage = %q, expected sanitized archive name", msg)
	}
}
