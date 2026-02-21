package progress

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSpinner_TTY_StartStop(t *testing.T) {
	origFunc := IsTerminalFunc
	IsTerminalFunc = func(fd int) bool { return true }
	defer func() { IsTerminalFunc = origFunc }()

	output := &bytes.Buffer{}
	s := NewSpinner(output)
	s.Start("Loading...")

	// Let the spinner animate for a bit
	time.Sleep(350 * time.Millisecond)

	s.Stop()

	content := output.String()
	// Should have animated with the message
	if !strings.Contains(content, "Loading...") {
		t.Error("spinner output should contain the message")
	}
}

func TestSpinner_TTY_StopWithMessage(t *testing.T) {
	origFunc := IsTerminalFunc
	IsTerminalFunc = func(fd int) bool { return true }
	defer func() { IsTerminalFunc = origFunc }()

	output := &bytes.Buffer{}
	s := NewSpinner(output)
	s.Start("Processing...")

	time.Sleep(250 * time.Millisecond)

	s.StopWithMessage("Done.")

	content := output.String()
	if !strings.Contains(content, "Done.") {
		t.Error("spinner output should contain the final message")
	}
}

func TestSpinner_TTY_SetMessage(t *testing.T) {
	origFunc := IsTerminalFunc
	IsTerminalFunc = func(fd int) bool { return true }
	defer func() { IsTerminalFunc = origFunc }()

	output := &bytes.Buffer{}
	s := NewSpinner(output)
	s.Start("First message")

	time.Sleep(250 * time.Millisecond)
	s.SetMessage("Second message")
	time.Sleep(250 * time.Millisecond)

	s.Stop()

	content := output.String()
	if !strings.Contains(content, "First message") {
		t.Error("spinner output should contain the first message")
	}
	if !strings.Contains(content, "Second message") {
		t.Error("spinner output should contain the updated message")
	}
}

func TestSpinner_NonTTY_Start(t *testing.T) {
	origFunc := IsTerminalFunc
	IsTerminalFunc = func(fd int) bool { return false }
	defer func() { IsTerminalFunc = origFunc }()

	output := &bytes.Buffer{}
	s := NewSpinner(output)
	s.Start("Processing...")

	// In non-TTY mode, message is printed once
	content := output.String()
	if !strings.Contains(content, "Processing...") {
		t.Error("non-TTY spinner should print message")
	}

	// Should have a newline, not animation characters
	if !strings.HasSuffix(content, "\n") {
		t.Error("non-TTY spinner should end with newline")
	}

	s.Stop()
}

func TestSpinner_NonTTY_StopWithMessage(t *testing.T) {
	origFunc := IsTerminalFunc
	IsTerminalFunc = func(fd int) bool { return false }
	defer func() { IsTerminalFunc = origFunc }()

	output := &bytes.Buffer{}
	s := NewSpinner(output)
	s.Start("Processing...")
	s.StopWithMessage("Done.")

	content := output.String()
	if !strings.Contains(content, "Processing...") {
		t.Error("non-TTY spinner should print initial message")
	}
	if !strings.Contains(content, "Done.") {
		t.Error("non-TTY spinner should print final message")
	}
}

func TestSpinner_DoubleStop(t *testing.T) {
	origFunc := IsTerminalFunc
	IsTerminalFunc = func(fd int) bool { return true }
	defer func() { IsTerminalFunc = origFunc }()

	output := &bytes.Buffer{}
	s := NewSpinner(output)
	s.Start("Test")
	time.Sleep(150 * time.Millisecond)

	// Should not panic on double stop
	s.Stop()
	s.Stop()
}

func TestSpinner_DoubleStopWithMessage(t *testing.T) {
	origFunc := IsTerminalFunc
	IsTerminalFunc = func(fd int) bool { return true }
	defer func() { IsTerminalFunc = origFunc }()

	output := &bytes.Buffer{}
	s := NewSpinner(output)
	s.Start("Test")
	time.Sleep(150 * time.Millisecond)

	// First StopWithMessage should print, second should be no-op
	s.StopWithMessage("Done.")
	s.StopWithMessage("Also done.")

	content := output.String()
	if strings.Count(content, "Done.") != 1 {
		t.Error("second StopWithMessage should be a no-op")
	}
}

func TestSpinner_NilOutput(t *testing.T) {
	origFunc := IsTerminalFunc
	IsTerminalFunc = func(fd int) bool { return false }
	defer func() { IsTerminalFunc = origFunc }()

	// nil output should default to os.Stderr, not panic
	s := NewSpinner(nil)
	s.Start("Test")
	s.Stop()
}

func TestSpinnerFrames(t *testing.T) {
	// Verify spinner animation frames are defined
	if len(spinnerFrames) == 0 {
		t.Error("spinnerFrames should not be empty")
	}
	if len(spinnerFrames) != 4 {
		t.Errorf("expected 4 spinner frames, got %d", len(spinnerFrames))
	}
}
