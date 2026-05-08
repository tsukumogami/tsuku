package progress

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tsukumogami/tsuku/internal/notices"
)

const (
	inboxMaxMessages = 50
	inboxMaxMsgLen   = 512
)

// InboxReporter is a Reporter that accumulates Warn and DeferWarn messages in
// memory and writes a single Notice to $TSUKU_HOME/notices/ on Stop(). It is
// used in the background auto-apply path where no terminal is available.
//
// Security notice: callers must not pass values from internal/secrets/ to Warn
// or DeferWarn. The notices directory is world-readable; secrets must never
// reach persisted notice files.
type InboxReporter struct {
	toolName   string
	noticesDir string

	mu        sync.Mutex
	immediate []string
	deferred  []string
}

// NewInboxReporter creates an InboxReporter that writes to noticesDir on Stop.
func NewInboxReporter(toolName string, noticesDir string) *InboxReporter {
	return &InboxReporter{
		toolName:   toolName,
		noticesDir: noticesDir,
	}
}

// Status is a no-op: there is no terminal in the background path.
func (r *InboxReporter) Status(msg string) {}

// Log is a no-op: there is no log sink in the background path.
func (r *InboxReporter) Log(format string, args ...any) {}

// Warn formats the message, sanitizes ANSI sequences, and appends it to the
// immediate accumulation slice (up to the 50-message cap; each message is
// truncated to 512 characters).
func (r *InboxReporter) Warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	msg = SanitizeDisplayString(msg)
	if len(msg) > inboxMaxMsgLen {
		msg = msg[:inboxMaxMsgLen]
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.immediate)+len(r.deferred) < inboxMaxMessages {
		r.immediate = append(r.immediate, msg)
	}
}

// DeferWarn formats the message, sanitizes ANSI sequences, and appends it to
// the deferred slice (same cap and truncation as Warn).
func (r *InboxReporter) DeferWarn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	msg = SanitizeDisplayString(msg)
	if len(msg) > inboxMaxMsgLen {
		msg = msg[:inboxMaxMsgLen]
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.immediate)+len(r.deferred) < inboxMaxMessages {
		r.deferred = append(r.deferred, msg)
	}
}

// FlushDeferred moves all deferred messages to the immediate slice in order.
// No disk write occurs.
func (r *InboxReporter) FlushDeferred() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.immediate = append(r.immediate, r.deferred...)
	r.deferred = nil
}

// Stop writes accumulated messages to a single Notice on disk. Returns early
// without writing when no messages have accumulated. Kind is set to
// KindVersionFallback if any message contains the "version_fallback:" prefix;
// otherwise KindAutoApplyResult.
func (r *InboxReporter) Stop() {
	r.mu.Lock()
	all := make([]string, 0, len(r.immediate)+len(r.deferred))
	all = append(all, r.immediate...)
	all = append(all, r.deferred...)
	r.immediate = nil
	r.deferred = nil
	r.mu.Unlock()

	if len(all) == 0 {
		return
	}

	kind := notices.KindAutoApplyResult
	for _, msg := range all {
		if strings.HasPrefix(msg, "version_fallback:") {
			kind = notices.KindVersionFallback
			break
		}
	}

	_ = notices.WriteNotice(r.noticesDir, &notices.Notice{
		Tool:      r.toolName,
		Kind:      kind,
		Messages:  all,
		Timestamp: time.Now(),
	})
}
