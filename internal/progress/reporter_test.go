package progress

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- helpers ---

// newNonTTY returns a ttyReporter in non-TTY mode writing to buf.
func newNonTTY(buf *bytes.Buffer) Reporter {
	return newTTYReporterWithFlag(buf, false)
}

// newFakeTTY returns a ttyReporter in TTY mode writing to buf.
func newFakeTTY(buf *bytes.Buffer) Reporter {
	return newTTYReporterWithFlag(buf, true)
}

// --- NoopReporter tests ---

func TestNoopReporter_ZeroValue(t *testing.T) {
	// Zero value must be usable without initialization — no panic.
	var r NoopReporter
	r.Status("hello")
	r.Log("log %s", "msg")
	r.Warn("warn %s", "msg")
	r.DeferWarn("deferred %s", "msg")
	r.FlushDeferred()
	r.Stop()
}

func TestNoopReporter_ImplementsInterface(t *testing.T) {
	var _ Reporter = NoopReporter{}
}

// --- Non-TTY behavior ---

func TestNonTTY_StatusIsNoop(t *testing.T) {
	var buf bytes.Buffer
	r := newNonTTY(&buf)
	r.Status("should not appear")
	r.Stop()
	if buf.Len() != 0 {
		t.Errorf("expected no output in non-TTY mode, got %q", buf.String())
	}
}

func TestNonTTY_LogEmitsPlainLine(t *testing.T) {
	var buf bytes.Buffer
	r := newNonTTY(&buf)
	r.Log("hello %s", "world")
	r.Stop()
	got := buf.String()
	if got != "hello world\n" {
		t.Errorf("expected %q, got %q", "hello world\n", got)
	}
}

func TestNonTTY_WarnEmitsPlainLineWithPrefix(t *testing.T) {
	var buf bytes.Buffer
	r := newNonTTY(&buf)
	r.Warn("something %s", "went wrong")
	r.Stop()
	got := buf.String()
	want := "warning: something went wrong\n"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// --- TTY behavior ---

func TestTTY_StatusStartsSpinner(t *testing.T) {
	var buf bytes.Buffer
	r := newFakeTTY(&buf)
	r.Status("working")
	// Give the goroutine time to render at least one frame.
	time.Sleep(50 * time.Millisecond)
	r.Stop()

	got := buf.String()
	// Expect at least one spinner overwrite sequence.
	if !strings.Contains(got, "\r\033[K") {
		t.Errorf("expected spinner escape sequence in output, got %q", got)
	}
	// Expect the message text.
	if !strings.Contains(got, "working") {
		t.Errorf("expected message 'working' in spinner output, got %q", got)
	}
}

func TestTTY_LogClearsSpinnerBeforePrinting(t *testing.T) {
	var buf bytes.Buffer
	r := newFakeTTY(&buf)
	r.Status("working")
	time.Sleep(50 * time.Millisecond)
	r.Log("done")
	r.Stop()

	got := buf.String()
	// After Log the clear sequence (\r\033[K) must appear before "done\n".
	clearIdx := strings.LastIndex(got, "\r\033[K")
	doneIdx := strings.Index(got, "done\n")
	if clearIdx == -1 {
		t.Fatal("expected clear sequence in output")
	}
	if doneIdx == -1 {
		t.Fatal("expected 'done\\n' in output")
	}
	if clearIdx >= doneIdx {
		t.Errorf("expected clear sequence before 'done\\n'; clearIdx=%d doneIdx=%d", clearIdx, doneIdx)
	}
}

// --- Goroutine lifecycle ---

func TestTTY_StopTerminatesGoroutine(t *testing.T) {
	var buf bytes.Buffer
	r := newFakeTTY(&buf)
	r.Status("running")
	time.Sleep(50 * time.Millisecond)

	// Access the underlying struct to check goroutine has exited after Stop.
	tr := r.(*ttyReporter)

	// Capture the done channel before Stop clears it.
	tr.mu.Lock()
	done := tr.spinDone
	tr.mu.Unlock()

	if done == nil {
		t.Fatal("expected goroutine to be running before Stop")
	}

	r.Stop()

	// The done channel must be closed now.
	select {
	case <-done:
		// goroutine exited — correct
	case <-time.After(500 * time.Millisecond):
		t.Fatal("goroutine did not exit within 500ms after Stop")
	}

	// spinStop should be nil after Stop.
	tr.mu.Lock()
	stopped := tr.spinStop == nil
	tr.mu.Unlock()
	if !stopped {
		t.Error("expected spinStop to be nil after Stop")
	}
}

// --- Stop idempotency ---

func TestTTY_StopIdempotent(t *testing.T) {
	var buf bytes.Buffer
	r := newFakeTTY(&buf)
	r.Status("running")
	time.Sleep(50 * time.Millisecond)

	// Two consecutive Stop calls must not panic or deadlock.
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Stop()
		r.Stop()
	}()
	select {
	case <-done:
		// success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("double Stop deadlocked")
	}
}

func TestNonTTY_StopIdempotentWithNoSpinner(t *testing.T) {
	var buf bytes.Buffer
	r := newNonTTY(&buf)
	// Calling Stop when no spinner ever ran must not panic.
	r.Stop()
	r.Stop()
}

// --- DeferWarn / FlushDeferred ordering ---

func TestDeferWarn_FlushDeferred_Ordering(t *testing.T) {
	var buf bytes.Buffer
	r := newNonTTY(&buf)
	r.DeferWarn("first %s", "warn")
	r.DeferWarn("second %s", "warn")
	r.DeferWarn("third %s", "warn")
	r.FlushDeferred()
	r.Stop()

	got := buf.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	want := []string{
		"warning: first warn",
		"warning: second warn",
		"warning: third warn",
	}
	if len(lines) != len(want) {
		t.Fatalf("expected %d lines, got %d: %q", len(want), len(lines), got)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d: expected %q, got %q", i, w, lines[i])
		}
	}
}

func TestDeferWarn_FlushDeferred_ClearsQueue(t *testing.T) {
	var buf bytes.Buffer
	r := newNonTTY(&buf)
	r.DeferWarn("msg")
	r.FlushDeferred()
	buf.Reset()
	// Second flush should produce no output.
	r.FlushDeferred()
	r.Stop()
	if buf.Len() != 0 {
		t.Errorf("expected no output after second FlushDeferred, got %q", buf.String())
	}
}

// --- ANSI sanitization ---

func TestSanitizeDisplayString_CSICursorMovement(t *testing.T) {
	// Cursor up: \x1b[1A
	input := "before\x1b[1Aafter"
	got := SanitizeDisplayString(input)
	want := "beforeafter"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestSanitizeDisplayString_OSCTitle(t *testing.T) {
	// Terminal title set: \x1b]0;title\x07
	input := "\x1b]0;My Title\x07hello"
	got := SanitizeDisplayString(input)
	want := "hello"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestSanitizeDisplayString_HideCursor(t *testing.T) {
	// Hide cursor: \x1b[?25l
	input := "text\x1b[?25lmore"
	got := SanitizeDisplayString(input)
	want := "textmore"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestSanitizeDisplayString_BareESC(t *testing.T) {
	// Bare ESC not part of a recognized sequence.
	input := "foo\x1bbar"
	got := SanitizeDisplayString(input)
	want := "foobar"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestSanitizeDisplayString_NoSequences(t *testing.T) {
	input := "plain text with no escapes"
	got := SanitizeDisplayString(input)
	if got != input {
		t.Errorf("expected unchanged string, got %q", got)
	}
}

func TestLog_SanitizesInput(t *testing.T) {
	var buf bytes.Buffer
	r := newNonTTY(&buf)
	// Inject a hide-cursor sequence into the message.
	r.Log("msg\x1b[?25l end")
	r.Stop()
	got := buf.String()
	if strings.Contains(got, "\x1b") {
		t.Errorf("expected ANSI sequences stripped from Log output, got %q", got)
	}
	if !strings.Contains(got, "msg end") {
		t.Errorf("expected 'msg end' in output, got %q", got)
	}
}

func TestWarn_SanitizesInput(t *testing.T) {
	var buf bytes.Buffer
	r := newNonTTY(&buf)
	r.Warn("bad\x1b[1Avalue")
	r.Stop()
	got := buf.String()
	if strings.Contains(got, "\x1b") {
		t.Errorf("expected ANSI sequences stripped from Warn output, got %q", got)
	}
	if !strings.Contains(got, "badvalue") {
		t.Errorf("expected 'badvalue' in output, got %q", got)
	}
}

func TestStatus_SanitizesInput(t *testing.T) {
	var buf bytes.Buffer
	r := newFakeTTY(&buf)
	r.Status("msg\x1b[1Ainjected")
	time.Sleep(50 * time.Millisecond)
	r.Stop()
	got := buf.String()
	// After stop the clear sequence is the last write; strip it for the check.
	if strings.Contains(got, "\x1b[1A") {
		t.Errorf("expected CSI cursor-up stripped from Status output, got %q", got)
	}
}

func TestDeferWarn_SanitizesInput(t *testing.T) {
	var buf bytes.Buffer
	r := newNonTTY(&buf)
	r.DeferWarn("bad\x1b[1Avalue")
	r.FlushDeferred()
	r.Stop()
	got := buf.String()
	if strings.Contains(got, "\x1b") {
		t.Errorf("expected ANSI sequences stripped from DeferWarn output, got %q", got)
	}
}

// --- Concurrent safety (smoke test) ---

func TestTTY_ConcurrentStatusAndLog(t *testing.T) {
	var buf syncBuffer
	r := newTTYReporterWithFlag(&buf, true)

	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r.Status("step")
			time.Sleep(10 * time.Millisecond)
			r.Log("done %d", n)
		}(i)
	}
	wg.Wait()
	r.Stop()
}

// syncBuffer is a thread-safe bytes.Buffer for concurrent tests.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
