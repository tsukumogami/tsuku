package notices

import (
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/installevents"
)

// MaxErrorBytes caps the size of an Err.Error() string persisted to a
// notice file. The cap prevents HTTP response bodies, multi-line
// stack traces, and other noisy upstream error text from
// contaminating the store.
const MaxErrorBytes = 512

// Subscriber translates installevents into on-disk notice mutations.
// Each Handle call touches only the file for the event's Tool —
// the subscriber-locality contract documented in the design.
type Subscriber struct {
	dir string // notices directory ($TSUKU_HOME/notices)
}

// NewSubscriber returns a Subscriber that writes to dir.
func NewSubscriber(dir string) *Subscriber {
	return &Subscriber{dir: dir}
}

// Handle reacts to one event from the bus. Errors from WriteNotice
// / RemoveNotice are deliberately swallowed: notices are best-effort
// and a subscriber error must not block install success. The bus
// itself logs through its own diagnostic logger; this Handle stays
// quiet.
func (s *Subscriber) Handle(event installevents.Event) {
	switch e := event.(type) {
	case installevents.Installed:
		_ = WriteNotice(s.dir, &Notice{
			Tool:             e.Tool,
			AttemptedVersion: e.Version,
			Verb:             VerbInstall,
			Kind:             kindFor(e.Source),
			Timestamp:        e.Timestamp,
			Shown:            false,
		})
	case installevents.Updated:
		_ = WriteNotice(s.dir, &Notice{
			Tool:             e.Tool,
			AttemptedVersion: e.ToVersion,
			Verb:             VerbUpdate,
			Kind:             kindFor(e.Source),
			Timestamp:        e.Timestamp,
			Shown:            false,
		})
	case installevents.RolledBack:
		_ = WriteNotice(s.dir, &Notice{
			Tool:             e.Tool,
			AttemptedVersion: e.ToVersion,
			Verb:             VerbRollback,
			Kind:             kindFor(e.Source),
			Timestamp:        e.Timestamp,
			Shown:            false,
		})
	case installevents.Removed:
		_ = RemoveNotice(s.dir, e.Tool)
	case installevents.InstallFailed:
		_ = s.writeFailure(e.Tool, e.AttemptedVersion, VerbInstall, e.Err, e.Source, e.Timestamp)
	case installevents.UpdateFailed:
		_ = s.writeFailure(e.Tool, e.AttemptedVersion, VerbUpdate, e.Err, e.Source, e.Timestamp)
	case installevents.RollbackFailed:
		_ = s.writeFailure(e.Tool, e.AttemptedVersion, VerbRollback, e.Err, e.Source, e.Timestamp)
	case installevents.RemoveFailed:
		_ = s.writeFailure(e.Tool, e.AttemptedVersion, VerbRemove, e.Err, e.Source, e.Timestamp)

	case installevents.LibraryInstalled:
		_ = WriteNotice(s.dir, &Notice{
			Tool:             LibraryNoticePrefix + e.Library,
			AttemptedVersion: e.Version,
			Verb:             VerbInstall,
			Kind:             kindFor(e.Source),
			Timestamp:        e.Timestamp,
			Shown:            false,
		})
	case installevents.LibraryRemoved:
		_ = RemoveNotice(s.dir, LibraryNoticePrefix+e.Library)
	case installevents.LibraryInstallFailed:
		_ = s.writeFailure(LibraryNoticePrefix+e.Library, e.AttemptedVersion, VerbInstall, e.Err, e.Source, e.Timestamp)
	case installevents.LibraryRemoveFailed:
		_ = s.writeFailure(LibraryNoticePrefix+e.Library, e.AttemptedVersion, VerbRemove, e.Err, e.Source, e.Timestamp)
	}
}

// writeFailure builds a Notice for a failure event, computing the
// new ConsecutiveFailures count from any prior notice on disk.
func (s *Subscriber) writeFailure(tool, version, verb string, err error, source installevents.Source, ts time.Time) error {
	consec := 1
	if prior, _ := ReadNotice(s.dir, tool); prior != nil {
		consec = prior.ConsecutiveFailures + 1
	}
	return WriteNotice(s.dir, &Notice{
		Tool:                tool,
		AttemptedVersion:    version,
		Verb:                verb,
		Error:               sanitizeError(err),
		Kind:                kindFor(source),
		ConsecutiveFailures: consec,
		Timestamp:           ts,
		Shown:               false,
	})
}

// kindFor maps a Source to the notice Kind for backward compatibility
// with renderers that check Kind. Auto-apply notices still use
// KindAutoApplyResult so existing logic in the renderer continues to
// distinguish them; everything else uses the empty-string default
// (KindUpdateResult).
//
// This mapping must not return single-view kinds (KindVersionFallback,
// KindShellInitChange) — doing so would let publishers control notice
// persistence. The kindForSubset invariant test in subscriber_test.go
// enforces this.
func kindFor(source installevents.Source) string {
	if source == installevents.SourceAuto {
		return KindAutoApplyResult
	}
	return KindUpdateResult
}

// sanitizeError makes an error string safe to persist to a notice
// file and render to the user. Two transformations:
//
//  1. Replace newlines with " / " so the renderer prints a single
//     line. Multi-line errors hide subsequent lines under terminal
//     scrolling and look strange in `tsuku notices` output.
//  2. Truncate to MaxErrorBytes with a "…" suffix. Prevents HTTP
//     response bodies, stack traces, and other noisy upstream text
//     from contaminating the store.
//
// nil error returns "".
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	// Replace both \r\n and standalone \n / \r with " / ".
	s = strings.ReplaceAll(s, "\r\n", " / ")
	s = strings.ReplaceAll(s, "\n", " / ")
	s = strings.ReplaceAll(s, "\r", " / ")
	if len(s) > MaxErrorBytes {
		// Truncate, leaving room for the ellipsis. Use byte slicing — we
		// accept that the truncation point may split a multi-byte rune;
		// the goal is a hard byte cap, not text fidelity.
		const ellipsis = "…"
		s = s[:MaxErrorBytes-len(ellipsis)] + ellipsis
	}
	return s
}
