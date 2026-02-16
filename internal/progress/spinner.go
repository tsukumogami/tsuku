package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// spinnerFrames defines the animation frames for the spinner.
var spinnerFrames = []string{"|", "/", "-", "\\"}

// Spinner displays a status message with an animated spinner.
// It suppresses output in non-TTY environments.
type Spinner struct {
	mu      sync.Mutex
	output  io.Writer
	message string
	done    chan struct{}
	stopped bool
	active  bool
}

// NewSpinner creates a spinner that writes to the given writer.
// If output is nil, it defaults to os.Stderr.
// The spinner does not start until Start() is called.
func NewSpinner(output io.Writer) *Spinner {
	if output == nil {
		output = os.Stderr
	}
	return &Spinner{
		output: output,
		done:   make(chan struct{}),
	}
}

// Start begins the spinner animation with the given message.
// In non-TTY environments, it prints the message once without animation.
// Calling Start on an already active spinner updates the message.
func (s *Spinner) Start(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.message = message

	if s.active {
		// Already running - just update the message
		return
	}

	s.active = true
	s.stopped = false
	s.done = make(chan struct{})

	if !ShouldShowProgress() {
		// Non-TTY: print message once, no animation
		fmt.Fprintf(s.output, "%s\n", message)
		return
	}

	go s.animate()
}

// SetMessage updates the spinner's message while it's running.
func (s *Spinner) SetMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
}

// Stop halts the spinner animation and clears the line.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if s.stopped || !s.active {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.active = false
	s.mu.Unlock()

	close(s.done)

	if ShouldShowProgress() {
		// Clear the spinner line
		fmt.Fprintf(s.output, "\r%s\r", strings.Repeat(" ", 80))
	}
}

// StopWithMessage halts the spinner and prints a final message.
func (s *Spinner) StopWithMessage(message string) {
	s.mu.Lock()
	if s.stopped || !s.active {
		s.mu.Unlock()
		// Still print the message even if spinner wasn't running
		fmt.Fprintf(s.output, "%s\n", message)
		return
	}
	s.stopped = true
	s.active = false
	s.mu.Unlock()

	close(s.done)

	if ShouldShowProgress() {
		// Clear the spinner line and print final message
		fmt.Fprintf(s.output, "\r%s\r%s\n", strings.Repeat(" ", 80), message)
	} else {
		fmt.Fprintf(s.output, "%s\n", message)
	}
}

// animate runs the spinner animation loop.
func (s *Spinner) animate() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			msg := s.message
			s.mu.Unlock()

			line := fmt.Sprintf("\r  %s %s", spinnerFrames[frame%len(spinnerFrames)], msg)
			if len(line) < 80 {
				line += strings.Repeat(" ", 80-len(line))
			}
			_, _ = fmt.Fprint(s.output, line)

			frame++
		}
	}
}
