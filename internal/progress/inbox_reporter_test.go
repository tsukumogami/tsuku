package progress

import (
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/notices"
)

func TestInboxReporter_ImplementsReporter(t *testing.T) {
	var _ Reporter = NewInboxReporter("tool", t.TempDir())
}

func TestInboxReporter_StopNoMessagesNoWrite(t *testing.T) {
	dir := t.TempDir()
	r := NewInboxReporter("tool", dir)
	r.Stop()

	all, err := notices.ReadAllNotices(dir)
	if err != nil {
		t.Fatalf("ReadAllNotices: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected no notices, got %d", len(all))
	}
}

func TestInboxReporter_WarnAccumulatesInOrder(t *testing.T) {
	dir := t.TempDir()
	r := NewInboxReporter("mytool", dir)
	r.Warn("first")
	r.Warn("second")
	r.Warn("third")
	r.Stop()

	all, err := notices.ReadAllNotices(dir)
	if err != nil || len(all) != 1 {
		t.Fatalf("expected 1 notice, got %d; err=%v", len(all), err)
	}
	if len(all[0].Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(all[0].Messages))
	}
	if all[0].Messages[0] != "first" || all[0].Messages[1] != "second" || all[0].Messages[2] != "third" {
		t.Errorf("unexpected message order: %v", all[0].Messages)
	}
}

func TestInboxReporter_FlushDeferredOrderImmediateThenDeferred(t *testing.T) {
	dir := t.TempDir()
	r := NewInboxReporter("mytool", dir)
	r.Warn("immediate-one")
	r.DeferWarn("deferred-one")
	r.Warn("immediate-two")
	r.FlushDeferred()
	r.Stop()

	all, err := notices.ReadAllNotices(dir)
	if err != nil || len(all) != 1 {
		t.Fatalf("expected 1 notice, got %d; err=%v", len(all), err)
	}
	msgs := all[0].Messages
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d: %v", len(msgs), msgs)
	}
	// FlushDeferred appends deferred to immediate in order
	if msgs[0] != "immediate-one" || msgs[1] != "immediate-two" || msgs[2] != "deferred-one" {
		t.Errorf("unexpected order: %v", msgs)
	}
}

func TestInboxReporter_FlushDeferredNoDiskWrite(t *testing.T) {
	dir := t.TempDir()
	r := NewInboxReporter("mytool", dir)
	r.DeferWarn("deferred")
	r.FlushDeferred()

	// FlushDeferred must not write to disk
	all, err := notices.ReadAllNotices(dir)
	if err != nil {
		t.Fatalf("ReadAllNotices: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("FlushDeferred should not write to disk, got %d notices", len(all))
	}
}

func TestInboxReporter_KindEscalatesOnVersionFallbackPrefix(t *testing.T) {
	dir := t.TempDir()
	r := NewInboxReporter("mytool", dir)
	r.Warn("version_fallback: installed v1.0 instead of v2.0")
	r.Stop()

	all, err := notices.ReadAllNotices(dir)
	if err != nil || len(all) != 1 {
		t.Fatalf("expected 1 notice, got %d; err=%v", len(all), err)
	}
	if all[0].Kind != notices.KindVersionFallback {
		t.Errorf("Kind = %q, want %q", all[0].Kind, notices.KindVersionFallback)
	}
}

func TestInboxReporter_KindDefaultsToAutoApplyResult(t *testing.T) {
	dir := t.TempDir()
	r := NewInboxReporter("mytool", dir)
	r.Warn("some warning without fallback prefix")
	r.Stop()

	all, err := notices.ReadAllNotices(dir)
	if err != nil || len(all) != 1 {
		t.Fatalf("expected 1 notice, got %d; err=%v", len(all), err)
	}
	if all[0].Kind != notices.KindAutoApplyResult {
		t.Errorf("Kind = %q, want %q", all[0].Kind, notices.KindAutoApplyResult)
	}
}

func TestInboxReporter_MessageCapAt50(t *testing.T) {
	dir := t.TempDir()
	r := NewInboxReporter("mytool", dir)
	for i := 0; i < 60; i++ {
		r.Warn("msg %d", i)
	}
	r.Stop()

	all, err := notices.ReadAllNotices(dir)
	if err != nil || len(all) != 1 {
		t.Fatalf("expected 1 notice; err=%v", err)
	}
	if len(all[0].Messages) != inboxMaxMessages {
		t.Errorf("expected %d messages (cap), got %d", inboxMaxMessages, len(all[0].Messages))
	}
}

func TestInboxReporter_PerMessageTruncationAt512(t *testing.T) {
	dir := t.TempDir()
	r := NewInboxReporter("mytool", dir)
	long := strings.Repeat("x", 600)
	r.Warn("%s", long)
	r.Stop()

	all, err := notices.ReadAllNotices(dir)
	if err != nil || len(all) != 1 {
		t.Fatalf("expected 1 notice; err=%v", err)
	}
	if len(all[0].Messages[0]) != inboxMaxMsgLen {
		t.Errorf("message length = %d, want %d", len(all[0].Messages[0]), inboxMaxMsgLen)
	}
}

func TestInboxReporter_ANSIStrippedBeforeStorage(t *testing.T) {
	dir := t.TempDir()
	r := NewInboxReporter("mytool", dir)
	r.Warn("\x1b[31mred text\x1b[0m")
	r.Stop()

	all, err := notices.ReadAllNotices(dir)
	if err != nil || len(all) != 1 {
		t.Fatalf("expected 1 notice; err=%v", err)
	}
	msg := all[0].Messages[0]
	if strings.Contains(msg, "\x1b") {
		t.Errorf("ANSI escape not stripped: %q", msg)
	}
	if msg != "red text" {
		t.Errorf("msg = %q, want %q", msg, "red text")
	}
}

func TestInboxReporter_StopAfterStopIsNoop(t *testing.T) {
	dir := t.TempDir()
	r := NewInboxReporter("mytool", dir)
	r.Warn("msg")
	r.Stop()
	// Second Stop must not panic or write duplicate notice
	r.Stop()

	all, err := notices.ReadAllNotices(dir)
	if err != nil {
		t.Fatalf("ReadAllNotices: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 notice after double Stop, got %d", len(all))
	}
}

func TestInboxReporter_ToolNamePreserved(t *testing.T) {
	dir := t.TempDir()
	r := NewInboxReporter("gh", dir)
	r.Warn("something happened")
	r.Stop()

	all, err := notices.ReadAllNotices(dir)
	if err != nil || len(all) != 1 {
		t.Fatalf("expected 1 notice; err=%v", err)
	}
	if all[0].Tool != "gh" {
		t.Errorf("Tool = %q, want %q", all[0].Tool, "gh")
	}
}

func TestInboxReporter_TimestampSet(t *testing.T) {
	dir := t.TempDir()
	before := time.Now()
	r := NewInboxReporter("mytool", dir)
	r.Warn("msg")
	r.Stop()
	after := time.Now()

	all, _ := notices.ReadAllNotices(dir)
	if len(all) != 1 {
		t.Fatal("expected 1 notice")
	}
	ts := all[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v outside [%v, %v]", ts, before, after)
	}
}
