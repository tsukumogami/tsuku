package progress

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSpinner_Start(t *testing.T) {
	// Save and restore the original IsTerminalFunc
	origFunc := IsTerminalFunc
	defer func() { IsTerminalFunc = origFunc }()

	t.Run("prints message once in non-TTY", func(t *testing.T) {
		IsTerminalFunc = func(fd int) bool { return false }

		output := &bytes.Buffer{}
		s := NewSpinner(output)

		s.Start("Generating...")
		// Give a moment for any goroutines to settle
		time.Sleep(50 * time.Millisecond)
		s.Stop()

		result := output.String()
		if !strings.Contains(result, "Generating...") {
			t.Errorf("output should contain message, got %q", result)
		}
		// In non-TTY, should print exactly once with newline
		count := strings.Count(result, "Generating...")
		if count != 1 {
			t.Errorf("non-TTY should print message once, got %d times", count)
		}
	})

	t.Run("animates in TTY mode", func(t *testing.T) {
		IsTerminalFunc = func(fd int) bool { return true }

		output := &bytes.Buffer{}
		s := NewSpinner(output)

		s.Start("Analyzing...")
		// Let a few frames render
		time.Sleep(350 * time.Millisecond)
		s.Stop()

		result := output.String()
		// Should have carriage returns from animation
		if !strings.Contains(result, "\r") {
			t.Error("TTY output should contain carriage returns from animation")
		}
		if !strings.Contains(result, "Analyzing...") {
			t.Errorf("output should contain message, got %q", result)
		}
	})
}

func TestSpinner_SetMessage(t *testing.T) {
	origFunc := IsTerminalFunc
	defer func() { IsTerminalFunc = origFunc }()
	IsTerminalFunc = func(fd int) bool { return true }

	output := &bytes.Buffer{}
	s := NewSpinner(output)

	s.Start("Step 1...")
	time.Sleep(200 * time.Millisecond)
	s.SetMessage("Step 2...")
	time.Sleep(200 * time.Millisecond)
	s.Stop()

	result := output.String()
	if !strings.Contains(result, "Step 1...") {
		t.Error("output should contain first message")
	}
	if !strings.Contains(result, "Step 2...") {
		t.Error("output should contain second message")
	}
}

func TestSpinner_StopWithMessage(t *testing.T) {
	origFunc := IsTerminalFunc
	defer func() { IsTerminalFunc = origFunc }()

	t.Run("TTY mode clears and prints final", func(t *testing.T) {
		IsTerminalFunc = func(fd int) bool { return true }

		output := &bytes.Buffer{}
		s := NewSpinner(output)

		s.Start("Working...")
		time.Sleep(200 * time.Millisecond)
		s.StopWithMessage("Done!")

		result := output.String()
		if !strings.Contains(result, "Done!") {
			t.Errorf("output should contain final message, got %q", result)
		}
	})

	t.Run("non-TTY prints both messages", func(t *testing.T) {
		IsTerminalFunc = func(fd int) bool { return false }

		output := &bytes.Buffer{}
		s := NewSpinner(output)

		s.Start("Working...")
		time.Sleep(50 * time.Millisecond)
		s.StopWithMessage("Done!")

		result := output.String()
		if !strings.Contains(result, "Working...") {
			t.Error("output should contain start message")
		}
		if !strings.Contains(result, "Done!") {
			t.Error("output should contain final message")
		}
	})
}

func TestSpinner_DoubleStop(t *testing.T) {
	origFunc := IsTerminalFunc
	defer func() { IsTerminalFunc = origFunc }()
	IsTerminalFunc = func(fd int) bool { return true }

	output := &bytes.Buffer{}
	s := NewSpinner(output)

	s.Start("Working...")
	time.Sleep(100 * time.Millisecond)
	s.Stop()
	// Second stop should not panic
	s.Stop()
}

func TestSpinner_StopWithoutStart(t *testing.T) {
	output := &bytes.Buffer{}
	s := NewSpinner(output)

	// Stop without start should not panic
	s.Stop()
}

func TestSpinner_NilOutput(t *testing.T) {
	// NewSpinner with nil should default to os.Stderr
	s := NewSpinner(nil)
	if s.output == nil {
		t.Error("output should not be nil after NewSpinner(nil)")
	}
}

func TestSpinner_UpdateMessageWhileRunning(t *testing.T) {
	origFunc := IsTerminalFunc
	defer func() { IsTerminalFunc = origFunc }()
	IsTerminalFunc = func(fd int) bool { return true }

	output := &bytes.Buffer{}
	s := NewSpinner(output)

	s.Start("Phase 1")
	time.Sleep(150 * time.Millisecond)
	// Calling Start again should update message, not start a new goroutine
	s.Start("Phase 2")
	time.Sleep(150 * time.Millisecond)
	s.Stop()

	result := output.String()
	if !strings.Contains(result, "Phase 2") {
		t.Error("output should contain updated message")
	}
}
