package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/tsukumogami/tsuku/internal/install"
)

func TestWarnShellInitChanges_NoWarningWhenHashesMatch(t *testing.T) {
	old := []install.CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "abc123"},
		{Action: "delete_file", Path: "share/shell.d/tool.zsh", ContentHash: "def456"},
	}
	new := []install.CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "abc123"},
		{Action: "delete_file", Path: "share/shell.d/tool.zsh", ContentHash: "def456"},
	}

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	warnShellInitChanges("tool", old, new)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	if buf.Len() != 0 {
		t.Errorf("expected no output when hashes match, got: %s", buf.String())
	}
}

func TestWarnShellInitChanges_WarnsWhenHashChanges(t *testing.T) {
	old := []install.CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "abc123"},
		{Action: "delete_file", Path: "share/shell.d/tool.zsh", ContentHash: "def456"},
	}
	new := []install.CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "changed"},
		{Action: "delete_file", Path: "share/shell.d/tool.zsh", ContentHash: "def456"},
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	warnShellInitChanges("tool", old, new)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if output == "" {
		t.Fatal("expected warning output when hash changes")
	}
	if !bytes.Contains([]byte(output), []byte("shell init changed for tool (bash)")) {
		t.Errorf("expected warning about bash, got: %s", output)
	}
	// zsh hash didn't change, so no warning for it
	if bytes.Contains([]byte(output), []byte("(zsh)")) {
		t.Errorf("did not expect warning about zsh, got: %s", output)
	}
}

func TestWarnShellInitChanges_NoWarningForNewPaths(t *testing.T) {
	// Old version had no shell.d files, new version adds them
	old := []install.CleanupAction{}
	new := []install.CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "abc123"},
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	warnShellInitChanges("tool", old, new)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	if buf.Len() != 0 {
		t.Errorf("expected no output for new paths, got: %s", buf.String())
	}
}

func TestWarnShellInitChanges_SkipsActionsWithoutHash(t *testing.T) {
	old := []install.CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/tool.bash"},
	}
	new := []install.CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/tool.bash"},
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	warnShellInitChanges("tool", old, new)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	if buf.Len() != 0 {
		t.Errorf("expected no output for actions without hash, got: %s", buf.String())
	}
}

func TestWarnShellInitChanges_MultipleShellChanges(t *testing.T) {
	old := []install.CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "hash1"},
		{Action: "delete_file", Path: "share/shell.d/tool.zsh", ContentHash: "hash2"},
		{Action: "delete_file", Path: "share/shell.d/tool.fish", ContentHash: "hash3"},
	}
	new := []install.CleanupAction{
		{Action: "delete_file", Path: "share/shell.d/tool.bash", ContentHash: "changed1"},
		{Action: "delete_file", Path: "share/shell.d/tool.zsh", ContentHash: "changed2"},
		{Action: "delete_file", Path: "share/shell.d/tool.fish", ContentHash: "hash3"},
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	warnShellInitChanges("tool", old, new)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !bytes.Contains([]byte(output), []byte("(bash)")) {
		t.Errorf("expected warning about bash, got: %s", output)
	}
	if !bytes.Contains([]byte(output), []byte("(zsh)")) {
		t.Errorf("expected warning about zsh, got: %s", output)
	}
	// fish hash didn't change
	if bytes.Contains([]byte(output), []byte("(fish)")) {
		t.Errorf("did not expect warning about fish, got: %s", output)
	}
}
