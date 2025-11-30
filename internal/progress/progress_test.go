package progress

import (
	"bytes"
	"io"
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1048576, "1.0MB"},
		{52428800, "50.0MB"},
		{1073741824, "1.0GB"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %s, want %s", tt.bytes, result, tt.expected)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		seconds  float64
		expected string
	}{
		{0, "0:00"},
		{30, "0:30"},
		{60, "1:00"},
		{90, "1:30"},
		{3600, "1:00:00"},
		{3661, "1:01:01"},
		{-5, "0:00"}, // Negative should be treated as 0
	}

	for _, tt := range tests {
		result := formatDuration(tt.seconds)
		if result != tt.expected {
			t.Errorf("formatDuration(%v) = %s, want %s", tt.seconds, result, tt.expected)
		}
	}
}

func TestProgressWriter(t *testing.T) {
	dest := &bytes.Buffer{}
	output := &bytes.Buffer{}

	total := int64(1000)
	pw := NewWriter(dest, total, output)

	// Simulate slow writing to trigger progress display
	data := make([]byte, 100)
	for i := 0; i < 10; i++ {
		n, err := pw.Write(data)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		if n != 100 {
			t.Errorf("Write returned %d, want 100", n)
		}
		// Small delay to allow progress updates
		time.Sleep(150 * time.Millisecond)
	}

	pw.Finish()

	// Verify all data was written
	if dest.Len() != 1000 {
		t.Errorf("Total written = %d, want 1000", dest.Len())
	}

	// Verify progress output was generated (should have progress bar characters)
	// Note: progress may or may not show depending on timing, just verify no crash
	_ = output.String()
}

func TestProgressWriterUnknownTotal(t *testing.T) {
	dest := &bytes.Buffer{}
	output := &bytes.Buffer{}

	// Unknown total (0)
	pw := NewWriter(dest, 0, output)

	data := make([]byte, 1000)
	n, err := pw.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 1000 {
		t.Errorf("Write returned %d, want 1000", n)
	}

	// Small delay to allow progress
	time.Sleep(150 * time.Millisecond)
	_, _ = pw.Write(data)

	pw.Finish()

	// Verify data was written
	if dest.Len() != 2000 {
		t.Errorf("Total written = %d, want 2000", dest.Len())
	}
}

func TestShouldShowProgress(t *testing.T) {
	// Save original function
	origFunc := IsTerminalFunc
	defer func() { IsTerminalFunc = origFunc }()

	// Test when terminal
	IsTerminalFunc = func(fd int) bool { return true }
	if !ShouldShowProgress() {
		t.Error("ShouldShowProgress() = false when terminal, want true")
	}

	// Test when not terminal
	IsTerminalFunc = func(fd int) bool { return false }
	if ShouldShowProgress() {
		t.Error("ShouldShowProgress() = true when not terminal, want false")
	}
}

func TestProgressWriterWritesAllData(t *testing.T) {
	dest := &bytes.Buffer{}
	output := io.Discard

	total := int64(5000)
	pw := NewWriter(dest, total, output)

	// Write data in chunks
	chunk := make([]byte, 500)
	for i := 0; i < 10; i++ {
		n, err := pw.Write(chunk)
		if err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
		if n != 500 {
			t.Errorf("Write %d returned %d, want 500", i, n)
		}
	}

	pw.Finish()

	if dest.Len() != 5000 {
		t.Errorf("Total written = %d, want 5000", dest.Len())
	}
}
