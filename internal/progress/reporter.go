package progress

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// Reporter is the interface for writing progress and log output during tool
// installation and other long-running operations.
//
// Security notice: callers must not pass values from internal/secrets/ to any
// Reporter method. For non-literal format strings, always use the safe
// indirection form:
//
//	reporter.Log("%s", value)   // correct
//	reporter.Log(value)         // incorrect — treats value as a format string
//
// All Reporter implementations apply SanitizeDisplayString to string inputs to
// prevent ANSI injection into the terminal from recipe-sourced strings.
type Reporter interface {
	// Status sets the current transient status message. On a TTY this drives a
	// braille spinner that overwrites the same line. On non-TTY output this is
	// a no-op. The message is replaced on the next Status call; it is not
	// preserved in the output.
	Status(msg string)

	// Log writes a permanent line to the output. On TTY output the spinner is
	// stopped and its line is cleared before the line is written. The format
	// string follows fmt.Sprintf conventions; a newline is appended
	// automatically.
	Log(format string, args ...any)

	// Warn writes a permanent warning line. It behaves like Log but prepends
	// "warning: " to the output.
	Warn(format string, args ...any)

	// DeferWarn queues a warning message to be emitted by FlushDeferred. Use
	// this for non-critical warnings that should appear after the operation
	// summary rather than inline during execution.
	DeferWarn(format string, args ...any)

	// FlushDeferred prints all queued DeferWarn messages in the order they were
	// enqueued, then clears the queue.
	FlushDeferred()

	// Stop terminates the background spinner goroutine (if running) and clears
	// the spinner line. It is idempotent: safe to call when no goroutine is
	// running or after a previous Stop call.
	Stop()
}

// NoopReporter is a Reporter that discards all output. Its zero value is ready
// to use without any initialization.
type NoopReporter struct{}

func (NoopReporter) Status(msg string)                    {}
func (NoopReporter) Log(format string, args ...any)       {}
func (NoopReporter) Warn(format string, args ...any)      {}
func (NoopReporter) DeferWarn(format string, args ...any) {}
func (NoopReporter) FlushDeferred()                       {}
func (NoopReporter) Stop()                                {}

// spinFrames is the braille spinner sequence used by the background tick goroutine.
var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ttyReporter is a Reporter that writes to an io.Writer with TTY-aware
// spinner support. Use NewTTYReporter to construct one.
type ttyReporter struct {
	w     io.Writer
	isTTY bool

	mu        sync.Mutex
	spinMsg   string
	spinFrame int
	spinStop  chan struct{} // closed to signal goroutine to exit
	spinDone  chan struct{} // closed by goroutine on exit

	deferred []string // messages queued by DeferWarn
}

// NewTTYReporter constructs a Reporter backed by w. If w is an *os.File and
// its file descriptor is a terminal, spinner animation is enabled. Otherwise
// Status() is a no-op and Log/Warn emit plain lines.
func NewTTYReporter(w io.Writer) Reporter {
	isTTY := false
	if f, ok := w.(*os.File); ok {
		isTTY = term.IsTerminal(int(f.Fd()))
	}
	return &ttyReporter{w: w, isTTY: isTTY}
}

// Status updates the current transient status message and starts the spinner
// goroutine if not already running. On non-TTY output this is a no-op.
func (r *ttyReporter) Status(msg string) {
	if !r.isTTY {
		return
	}
	msg = SanitizeDisplayString(msg)
	r.mu.Lock()
	r.spinMsg = msg
	if r.spinStop == nil {
		stop := make(chan struct{})
		done := make(chan struct{})
		r.spinStop = stop
		r.spinDone = done
		go r.spinLoop(stop, done)
	}
	r.mu.Unlock()
}

// spinLoop is the background goroutine started by Status. stop and done are
// passed as parameters rather than read from r.spinStop/r.spinDone to avoid a
// data race after stopSpinner clears those fields. It ticks immediately on
// startup (so the status appears without waiting 100ms), then every ~100ms
// until stop is closed.
func (r *ttyReporter) spinLoop(stop, done chan struct{}) {
	defer close(done)
	r.doTick()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			r.doTick()
		}
	}
}

// doTick writes one spinner frame for the current status message.
func (r *ttyReporter) doTick() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.spinMsg == "" {
		return
	}
	frame := spinFrames[r.spinFrame%len(spinFrames)]
	r.spinFrame++
	fmt.Fprintf(r.w, "\r\033[K%s %s", frame, r.spinMsg)
}

// stopSpinner stops the background goroutine and clears the spinner line. It
// is a no-op when no goroutine is running.
func (r *ttyReporter) stopSpinner() {
	r.mu.Lock()
	if r.spinStop == nil {
		r.mu.Unlock()
		return
	}
	stop := r.spinStop
	done := r.spinDone
	r.spinStop = nil
	r.spinDone = nil
	r.spinMsg = ""
	r.mu.Unlock()
	close(stop)
	<-done
	_, _ = fmt.Fprint(r.w, "\r\033[K") // clear the last spinner line
}

// Stop terminates the background spinner goroutine (if running) and clears the
// spinner line. Idempotent.
func (r *ttyReporter) Stop() {
	r.stopSpinner()
}

// Log writes a permanent line to the output. On TTY output the spinner is
// stopped and its line cleared first.
func (r *ttyReporter) Log(format string, args ...any) {
	if r.isTTY {
		r.stopSpinner()
	}
	msg := fmt.Sprintf(format, args...)
	msg = SanitizeDisplayString(msg)
	_, _ = fmt.Fprintln(r.w, msg)
}

// Warn writes a permanent warning line. It behaves like Log but prepends
// "warning: ".
func (r *ttyReporter) Warn(format string, args ...any) {
	if r.isTTY {
		r.stopSpinner()
	}
	msg := fmt.Sprintf(format, args...)
	msg = SanitizeDisplayString(msg)
	_, _ = fmt.Fprintln(r.w, "warning: "+msg)
}

// DeferWarn queues a warning message for FlushDeferred.
func (r *ttyReporter) DeferWarn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	msg = SanitizeDisplayString(msg)
	r.mu.Lock()
	r.deferred = append(r.deferred, "warning: "+msg)
	r.mu.Unlock()
}

// FlushDeferred prints all queued DeferWarn messages in order and clears the queue.
func (r *ttyReporter) FlushDeferred() {
	r.mu.Lock()
	msgs := r.deferred
	r.deferred = nil
	r.mu.Unlock()
	for _, msg := range msgs {
		if r.isTTY {
			r.stopSpinner()
		}
		_, _ = fmt.Fprintln(r.w, msg)
	}
}
