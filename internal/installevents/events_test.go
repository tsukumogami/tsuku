package installevents

import (
	"context"
	"testing"
)

// TestWithSource_SourceFromContext_RoundTrip verifies that a Source
// written via WithSource can be read back via SourceFromContext, for
// every concrete Source constant in the vocabulary.
func TestWithSource_SourceFromContext_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		src  Source
	}{
		{"manual", SourceManual},
		{"auto", SourceAuto},
		{"project-auto", SourceProjectAuto},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := WithSource(context.Background(), tc.src)
			got := SourceFromContext(ctx)
			if got != tc.src {
				t.Fatalf("SourceFromContext(WithSource(ctx, %q)) = %q, want %q", tc.src, got, tc.src)
			}
		})
	}
}

// TestSourceFromContext_EmptyCtx verifies that a context that never
// had WithSource called returns the empty Source value. This is the
// "forgot to call WithSource" signal that the bus uses to drop events
// with a diagnostic log.
func TestSourceFromContext_EmptyCtx(t *testing.T) {
	if got := SourceFromContext(context.Background()); got != "" {
		t.Fatalf("SourceFromContext(Background()) = %q, want empty string", got)
	}
	if got := SourceFromContext(context.TODO()); got != "" {
		t.Fatalf("SourceFromContext(TODO()) = %q, want empty string", got)
	}
}

// TestSourceFromContext_NilCtx verifies the documented contract that a
// nil context returns the empty Source rather than panicking. Production
// code never passes nil here; the guard exists for paranoid callers and
// is asserted via a defensive reflection-free path so staticcheck's
// "do not pass a nil Context" rule doesn't trigger on the test itself.
func TestSourceFromContext_NilCtx(t *testing.T) {
	var nilCtx context.Context // typed nil
	if got := SourceFromContext(nilCtx); got != "" {
		t.Fatalf("SourceFromContext(typed-nil ctx) = %q, want empty string", got)
	}
}

// TestWithSource_DoesNotLeakIntoOtherContexts verifies that overwriting
// the Source on a derived context doesn't mutate the parent. This is
// the standard context.WithValue contract; we exercise it here so a
// future refactor that "optimizes" the implementation can't silently
// break it.
func TestWithSource_DoesNotLeakIntoOtherContexts(t *testing.T) {
	parent := WithSource(context.Background(), SourceManual)
	child := WithSource(parent, SourceAuto)

	if got := SourceFromContext(parent); got != SourceManual {
		t.Errorf("parent.Source = %q, want %q (parent was mutated)", got, SourceManual)
	}
	if got := SourceFromContext(child); got != SourceAuto {
		t.Errorf("child.Source = %q, want %q", got, SourceAuto)
	}
}

// foreignKey is a non-installevents key type used in
// TestSrcKey_DoesNotCollideWithForeignKey. It demonstrates the typed
// srcKey struct{} prevents collisions with any other context-key value
// outside the installevents package -- even if a foreign package
// declared a key with the same identifier name.
type foreignKey struct{}

// TestSrcKey_DoesNotCollideWithForeignKey verifies that the unexported
// typed key srcKey isolates Source storage from any other key type a
// caller could define, even one that happens to also be named srcKey
// in a different package. This is a compile-time invariant -- the
// test exists so reviewers can read it as documentation, not because
// the runtime check could otherwise fail.
func TestSrcKey_DoesNotCollideWithForeignKey(t *testing.T) {
	ctx := WithSource(context.Background(), SourceManual)
	//nolint:staticcheck // ctx.Value with foreign key intentionally exercises the no-collision invariant
	if v := ctx.Value(foreignKey{}); v != nil {
		t.Fatalf("foreignKey leak: ctx.Value(foreignKey{}) = %v, want nil", v)
	}
	if got := SourceFromContext(ctx); got != SourceManual {
		t.Fatalf("SourceFromContext(ctx) = %q, want %q after foreign key probe", got, SourceManual)
	}
}
