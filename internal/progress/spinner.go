package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// spinnerFrames defines the animation characters for the spinner.
var spinnerFrames = []string{"|", "/", "-", "\\"}

// spinnerInterval is the time between spinner frame updates.
const spinnerInterval = 100 * time.Millisecond

// Spinner displays an animated spinner with a message during long operations.
// In non-TTY environments, it prints the message once without animation.
type Spinner struct {
	mu      sync.Mutex
	output  io.Writer
	message string
	done    chan struct{}
	stopped bool
	isTTY   bool
}

// NewSpinner creates a new spinner that writes to the given output.
// If output is nil, os.Stderr is used.
func NewSpinner(output io.Writer) *Spinner {
	if output == nil {
		output = os.Stderr
	}
	return &Spinner{
		output: output,
		done:   make(chan struct{}),
		isTTY:  ShouldShowProgress(),
	}
}

// Start begins the spinner animation with the given message.
// In TTY mode, it animates the spinner. In non-TTY mode, it prints
// the message once and returns.
func (s *Spinner) Start(message string) {
	s.mu.Lock()
	s.message = message
	s.stopped = false
	s.mu.Unlock()

	if !s.isTTY {
		// Non-TTY: print message once, no animation
		fmt.Fprintf(s.output, "%s\n", message)
		return
	}

	go s.animate()
}

// SetMessage updates the spinner message while it's running.
func (s *Spinner) SetMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
}

// Stop halts the spinner animation and clears the line.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()

	close(s.done)

	if s.isTTY {
		// Clear the spinner line
		fmt.Fprintf(s.output, "\r%s\r", strings.Repeat(" ", 80))
	}
}

// StopWithMessage halts the spinner and prints a final message.
func (s *Spinner) StopWithMessage(message string) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()

	close(s.done)

	if s.isTTY {
		// Clear spinner line and print the final message
		fmt.Fprintf(s.output, "\r%s\r%s\n", strings.Repeat(" ", 80), message)
	} else {
		fmt.Fprintf(s.output, "%s\n", message)
	}
}

// animate runs the spinner animation loop.
func (s *Spinner) animate() {
	frame := 0
	ticker := time.NewTicker(spinnerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			msg := s.message
			s.mu.Unlock()

			char := spinnerFrames[frame%len(spinnerFrames)]
			line := fmt.Sprintf("\r%s %s", char, msg)
			// Pad to clear previous content
			if len(line) < 80 {
				line += strings.Repeat(" ", 80-len(line))
			}
			fmt.Fprint(s.output, line)

			frame++
		}
	}
}
