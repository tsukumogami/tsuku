// Package eventtest provides small helpers for tests that need to drive
// install.Manager methods. The Source enum is normally injected via
// installevents.WithSource(ctx, src) at CLI entry points; in tests, the
// common case is "I just need a ctx that carries SourceManual so the bus
// doesn't drop the event for an empty Source".
package eventtest

import (
	"context"
	"testing"

	"github.com/tsukumogami/tsuku/internal/installevents"
)

// WithSourceManual returns a context derived from context.Background()
// that carries installevents.SourceManual. Most install.Manager tests
// only care that *some* Source is set; this helper saves them from
// importing installevents and constructing the context inline.
//
// The *testing.T argument exists so future revisions can attach test
// scope (e.g. cancel-on-cleanup) without touching every call site.
func WithSourceManual(t *testing.T) context.Context {
	t.Helper()
	return installevents.WithSource(context.Background(), installevents.SourceManual)
}
