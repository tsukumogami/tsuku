package telemetry

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestShowNoticeIfNeeded_FirstRun(t *testing.T) {
	// Setup temp directory as TSUKU_HOME
	tmpDir := t.TempDir()
	t.Setenv("TSUKU_HOME", tmpDir)
	_ = os.Unsetenv(EnvNoTelemetry)

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	ShowNoticeIfNeeded()

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Verify notice was shown
	if output != NoticeText {
		t.Errorf("notice text mismatch:\ngot:  %q\nwant: %q", output, NoticeText)
	}

	// Verify marker file was created
	markerPath := filepath.Join(tmpDir, NoticeMarkerFile)
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Error("marker file was not created")
	}
}

func TestShowNoticeIfNeeded_Internal_FirstRun(t *testing.T) {
	// Test the internal function directly with a buffer
	tmpDir := t.TempDir()
	var buf bytes.Buffer

	showNoticeIfNeeded(tmpDir, &buf)

	// Verify notice was shown
	if buf.String() != NoticeText {
		t.Errorf("notice text mismatch:\ngot:  %q\nwant: %q", buf.String(), NoticeText)
	}

	// Verify marker file was created
	markerPath := filepath.Join(tmpDir, NoticeMarkerFile)
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Error("marker file was not created")
	}
}

func TestShowNoticeIfNeeded_Internal_AlreadyShown(t *testing.T) {
	// Test the internal function with marker already present
	tmpDir := t.TempDir()

	// Create marker file
	markerPath := filepath.Join(tmpDir, NoticeMarkerFile)
	f, err := os.Create(markerPath)
	if err != nil {
		t.Fatalf("failed to create marker file: %v", err)
	}
	f.Close()

	var buf bytes.Buffer
	showNoticeIfNeeded(tmpDir, &buf)

	// Verify notice was NOT shown
	if buf.String() != "" {
		t.Errorf("notice was shown when marker file exists: %q", buf.String())
	}
}

func TestShowNoticeIfNeeded_Internal_MkdirAllFails(t *testing.T) {
	// Test that MkdirAll failure is handled silently
	// Use a path that cannot be created (file exists where directory should be)
	tmpDir := t.TempDir()

	// Create a file where we'd want to create the home directory
	blockingFile := filepath.Join(tmpDir, "blocked")
	f, err := os.Create(blockingFile)
	if err != nil {
		t.Fatalf("failed to create blocking file: %v", err)
	}
	f.Close()

	// Try to use the file path as the home directory (MkdirAll will fail)
	invalidHomeDir := filepath.Join(blockingFile, "subdir")

	var buf bytes.Buffer
	// Should not panic, should write notice but fail silently on marker creation
	showNoticeIfNeeded(invalidHomeDir, &buf)

	// Notice is still shown even though marker creation fails
	if buf.String() != NoticeText {
		t.Errorf("notice should still be shown even when mkdir fails:\ngot:  %q\nwant: %q", buf.String(), NoticeText)
	}

	// Marker should NOT exist
	markerPath := filepath.Join(invalidHomeDir, NoticeMarkerFile)
	if _, err := os.Stat(markerPath); err == nil {
		t.Error("marker file should not exist when mkdir fails")
	}
}

func TestShowNoticeIfNeeded_Internal_CreateFails(t *testing.T) {
	// Test that os.Create failure is handled silently
	// Make the home directory read-only so we can't create files
	tmpDir := t.TempDir()

	// Make the directory read-only (no write permission)
	if err := os.Chmod(tmpDir, 0555); err != nil {
		t.Fatalf("failed to make directory read-only: %v", err)
	}
	// Restore permissions on cleanup so TempDir can be removed
	t.Cleanup(func() {
		_ = os.Chmod(tmpDir, 0755)
	})

	var buf bytes.Buffer
	// Should not panic, should write notice but fail silently on file creation
	showNoticeIfNeeded(tmpDir, &buf)

	// Notice is still shown even though file creation fails
	if buf.String() != NoticeText {
		t.Errorf("notice should still be shown even when file create fails:\ngot:  %q\nwant: %q", buf.String(), NoticeText)
	}

	// Marker should NOT exist
	markerPath := filepath.Join(tmpDir, NoticeMarkerFile)
	if _, err := os.Stat(markerPath); err == nil {
		t.Error("marker file should not exist when create fails")
	}
}

func TestShowNoticeIfNeeded_AlreadyShown(t *testing.T) {
	// Setup temp directory with marker file
	tmpDir := t.TempDir()
	t.Setenv("TSUKU_HOME", tmpDir)
	_ = os.Unsetenv(EnvNoTelemetry)

	// Create marker file
	markerPath := filepath.Join(tmpDir, NoticeMarkerFile)
	f, err := os.Create(markerPath)
	if err != nil {
		t.Fatalf("failed to create marker file: %v", err)
	}
	f.Close()

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	ShowNoticeIfNeeded()

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Verify notice was NOT shown
	if output != "" {
		t.Errorf("notice was shown when marker file exists: %q", output)
	}
}

func TestShowNoticeIfNeeded_TelemetryDisabled(t *testing.T) {
	// Setup temp directory
	tmpDir := t.TempDir()
	t.Setenv("TSUKU_HOME", tmpDir)
	t.Setenv(EnvNoTelemetry, "1")

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	ShowNoticeIfNeeded()

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Verify notice was NOT shown
	if output != "" {
		t.Errorf("notice was shown when telemetry disabled: %q", output)
	}

	// Verify marker file was NOT created
	markerPath := filepath.Join(tmpDir, NoticeMarkerFile)
	if _, err := os.Stat(markerPath); err == nil {
		t.Error("marker file was created when telemetry disabled")
	}
}

func TestShowNoticeIfNeeded_RespectsHome(t *testing.T) {
	// Setup custom TSUKU_HOME
	tmpDir := t.TempDir()
	customHome := filepath.Join(tmpDir, "custom", "tsuku")
	t.Setenv("TSUKU_HOME", customHome)
	_ = os.Unsetenv(EnvNoTelemetry)

	// Capture stderr (ignore output)
	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w

	ShowNoticeIfNeeded()

	w.Close()
	os.Stderr = oldStderr

	// Verify marker file was created in custom location
	markerPath := filepath.Join(customHome, NoticeMarkerFile)
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Errorf("marker file not created at custom TSUKU_HOME: %s", markerPath)
	}
}

func TestNoticeText_Content(t *testing.T) {
	// Verify expected content per issue requirements
	expectedSubstrings := []string{
		"tsuku collects anonymous usage statistics",
		"No personal information is collected",
		"https://tsuku.dev/telemetry",
		"TSUKU_NO_TELEMETRY=1",
	}

	for _, expected := range expectedSubstrings {
		if !bytes.Contains([]byte(NoticeText), []byte(expected)) {
			t.Errorf("NoticeText missing expected content: %q", expected)
		}
	}
}
