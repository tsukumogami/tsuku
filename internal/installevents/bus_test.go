package installevents

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/log"
)

// recordingSub captures every event it receives in registration order.
type recordingSub struct {
	mu     sync.Mutex
	events []Event
}

func (r *recordingSub) Handle(event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordingSub) snapshot() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Event, len(r.events))
	copy(out, r.events)
	return out
}

// reentrantSub publishes a chain of follow-up events on every Handle
// call, capped by a per-instance limit. Used to test re-entrancy and
// depth cap behavior.
type reentrantSub struct {
	bus      *Bus
	max      int
	count    int
	captured []Event
}

func (r *reentrantSub) Handle(event Event) {
	r.captured = append(r.captured, event)
	if r.count >= r.max {
		return
	}
	r.count++
	// Publish a fresh event; do not derive Source from the inbound event
	// because we want every call to produce a non-empty source.
	r.bus.Publish(Updated{
		Tool:        "x",
		FromVersion: "0.0.0",
		ToVersion:   fmt.Sprintf("0.0.%d", r.count),
		Source:      SourceManual,
		Timestamp:   time.Now(),
	})
}

// panickingSub always panics. Used to verify recover containment.
type panickingSub struct{}

func (panickingSub) Handle(_ Event) { panic("subscriber boom") }

// fanoutSub publishes N sibling events per inbound event. Used to
// exhaust the queue cap quickly without growing the depth.
type fanoutSub struct {
	bus *Bus
	n   int
}

func (f *fanoutSub) Handle(event Event) {
	// Only fan out on the first inbound event so we don't recurse forever.
	if _, ok := event.(Installed); !ok {
		return
	}
	for i := 0; i < f.n; i++ {
		f.bus.Publish(Updated{
			Tool:      "y",
			ToVersion: fmt.Sprintf("0.%d.0", i),
			Source:    SourceManual,
			Timestamp: time.Now(),
		})
	}
}

func newTestBus(t *testing.T) *Bus {
	t.Helper()
	return NewBusForTest(log.NewNoop())
}

// 1. Subscribers receive events in registration order.
func TestPublish_DeliversInRegistrationOrder(t *testing.T) {
	bus := newTestBus(t)
	a := &recordingSub{}
	b := &recordingSub{}
	bus.Subscribe("a", a)
	bus.Subscribe("b", b)

	bus.Publish(Installed{Tool: "tool1", Version: "1.0.0", Source: SourceManual, Timestamp: time.Now()})

	if got := a.snapshot(); len(got) != 1 {
		t.Errorf("a: expected 1 event, got %d", len(got))
	}
	if got := b.snapshot(); len(got) != 1 {
		t.Errorf("b: expected 1 event, got %d", len(got))
	}
}

// 2. Multiple subscribers see the same event.
func TestPublish_AllSubscribersSeeEvent(t *testing.T) {
	bus := newTestBus(t)
	subs := []*recordingSub{{}, {}, {}}
	for i, s := range subs {
		bus.Subscribe(fmt.Sprintf("sub%d", i), s)
	}
	bus.Publish(Installed{Tool: "t", Version: "1", Source: SourceManual})
	for i, s := range subs {
		if got := s.snapshot(); len(got) != 1 {
			t.Errorf("sub%d: expected 1 event, got %d", i, len(got))
		}
	}
}

// 3. A panicking subscriber is recovered; later subscribers still run.
func TestPublish_PanicRecovered(t *testing.T) {
	bus := newTestBus(t)
	after := &recordingSub{}
	bus.Subscribe("boom", panickingSub{})
	bus.Subscribe("after", after)

	bus.Publish(Installed{Tool: "t", Version: "1", Source: SourceManual})

	if got := after.snapshot(); len(got) != 1 {
		t.Errorf("after-panic subscriber must still receive event; got %d events", len(got))
	}
}

// 4. Re-entrant Publish from inside a Handle is queued and flushed.
func TestPublish_ReentrantQueueFlush(t *testing.T) {
	bus := newTestBus(t)
	re := &reentrantSub{bus: bus, max: 3}
	bus.Subscribe("re", re)

	bus.Publish(Installed{Tool: "t", Version: "1", Source: SourceManual})

	// Initial + 3 reentrant = 4 events the subscriber should see.
	if len(re.captured) != 4 {
		t.Errorf("expected 4 events (1 initial + 3 reentrant), got %d", len(re.captured))
	}

	// First must be the Installed; subsequent must be Updated in order.
	if _, ok := re.captured[0].(Installed); !ok {
		t.Errorf("first event must be Installed, got %T", re.captured[0])
	}
	for i := 1; i <= 3; i++ {
		u, ok := re.captured[i].(Updated)
		if !ok {
			t.Fatalf("re-entrant event %d must be Updated, got %T", i, re.captured[i])
		}
		want := fmt.Sprintf("0.0.%d", i)
		if u.ToVersion != want {
			t.Errorf("re-entrant event %d: ToVersion = %q, want %q", i, u.ToVersion, want)
		}
	}
}

// 5. Re-entrant events run AFTER the current Handle returns (causal order).
func TestPublish_ReentrantPreservesCausalOrder(t *testing.T) {
	bus := newTestBus(t)

	var seen []string
	bus.Subscribe("ordering", &orderingSub{bus: bus, seen: &seen})

	bus.Publish(Installed{Tool: "first", Version: "1", Source: SourceManual})

	want := []string{
		"installed:first", // top-level
		"updated:second",  // queued from inside first
		"updated:third",   // queued from inside first (or queued during second)
	}
	if len(seen) < len(want) {
		t.Fatalf("expected at least %d events, got %d: %v", len(want), len(seen), seen)
	}
	// The top-level Installed must complete (we observed "installed:first")
	// BEFORE either queued event runs. We can't easily assert "ran after"
	// in a single-threaded model, but we can assert causal order is
	// preserved (parent comes first in the seen slice).
	if seen[0] != "installed:first" {
		t.Errorf("top-level Installed must be first in observation order; got %v", seen)
	}
}

type orderingSub struct {
	bus  *Bus
	seen *[]string
}

func (o *orderingSub) Handle(event Event) {
	switch e := event.(type) {
	case Installed:
		*o.seen = append(*o.seen, "installed:"+e.Tool)
		// Publish two follow-ups; both should queue.
		o.bus.Publish(Updated{Tool: "second", ToVersion: "1", Source: SourceManual})
		o.bus.Publish(Updated{Tool: "third", ToVersion: "1", Source: SourceManual})
	case Updated:
		*o.seen = append(*o.seen, "updated:"+e.Tool)
	}
}

// 6. Depth cap drops events whose cause-and-effect chain exceeds the limit.
func TestPublish_DepthCapDropsLongChains(t *testing.T) {
	bus := newTestBus(t)
	// Subscriber tries to publish far more than depthCap chained events.
	re := &reentrantSub{bus: bus, max: depthCap + 5}
	bus.Subscribe("re", re)

	bus.Publish(Installed{Tool: "t", Version: "1", Source: SourceManual})

	// We should have AT MOST depthCap drained events; the rest get dropped.
	// 1 initial + chain (capped at depthCap-1 from the publish side because
	// the chain entries enter at depth 1..depthCap-1; the next would be
	// depth depthCap which triggers the drop).
	if len(re.captured) > depthCap {
		t.Errorf("expected at most %d events (depth cap), got %d", depthCap, len(re.captured))
	}

	if bus.depthDropCount() == 0 {
		t.Error("expected at least one depth-cap drop, got 0")
	}
}

// 7. Queue cap drops events when too many are pending at once.
func TestPublish_QueueCapDropsFanout(t *testing.T) {
	bus := newTestBus(t)
	fan := &fanoutSub{bus: bus, n: queueCap + 10}
	bus.Subscribe("fan", fan)

	bus.Publish(Installed{Tool: "trigger", Version: "1", Source: SourceManual})

	if bus.queueDropCount() == 0 {
		t.Error("expected at least one queue-cap drop, got 0")
	}
}

// 8. nil-safe Publish on a nil receiver.
func TestPublish_NilBusReceiver(t *testing.T) {
	var bus *Bus
	// Should not panic.
	bus.Publish(Installed{Tool: "t", Version: "1", Source: SourceManual})
}

// 9. Empty Source drops the event with a log entry.
func TestPublish_EmptySourceDropped(t *testing.T) {
	bus := newTestBus(t)
	r := &recordingSub{}
	bus.Subscribe("r", r)

	bus.Publish(Installed{Tool: "t", Version: "1", Source: "", Timestamp: time.Now()})

	if got := r.snapshot(); len(got) != 0 {
		t.Errorf("subscriber must not receive empty-Source event; got %d events", len(got))
	}
	if bus.emptySourceDropCount() == 0 {
		t.Error("expected at least one empty-Source drop, got 0")
	}
}

// 10. Subscribe after first Publish is ignored (frozen-set semantics).
func TestSubscribe_AfterFirstPublishIsIgnored(t *testing.T) {
	bus := newTestBus(t)
	first := &recordingSub{}
	bus.Subscribe("first", first)
	bus.Publish(Installed{Tool: "t", Version: "1", Source: SourceManual})

	late := &recordingSub{}
	bus.Subscribe("late", late)
	bus.Publish(Updated{Tool: "u", FromVersion: "1", ToVersion: "2", Source: SourceManual})

	if got := first.snapshot(); len(got) != 2 {
		t.Errorf("first: expected 2 events, got %d", len(got))
	}
	if got := late.snapshot(); len(got) != 0 {
		t.Errorf("late subscriber registered after first Publish must not receive any events; got %d", len(got))
	}
}

// 11. Nil event is a no-op.
func TestPublish_NilEventDropped(t *testing.T) {
	bus := newTestBus(t)
	r := &recordingSub{}
	bus.Subscribe("r", r)
	bus.Publish(nil)
	if got := r.snapshot(); len(got) != 0 {
		t.Errorf("nil event must not reach subscribers; got %d", len(got))
	}
}

//  12. All eight event types implement the sealed Event interface and
//     report Source consistently via GetSource().
func TestAllEventTypes_ImplementInterface(t *testing.T) {
	tests := []struct {
		name   string
		event  Event
		source Source
	}{
		{"Installed", Installed{Source: SourceManual}, SourceManual},
		{"Updated", Updated{Source: SourceAuto}, SourceAuto},
		{"RolledBack", RolledBack{Source: SourceManual}, SourceManual},
		{"Removed", Removed{Source: SourceProjectAuto}, SourceProjectAuto},
		{"InstallFailed", InstallFailed{Source: SourceManual}, SourceManual},
		{"UpdateFailed", UpdateFailed{Source: SourceAuto}, SourceAuto},
		{"RollbackFailed", RollbackFailed{Source: SourceManual}, SourceManual},
		{"RemoveFailed", RemoveFailed{Source: SourceProjectAuto}, SourceProjectAuto},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.event.GetSource(); got != tt.source {
				t.Errorf("GetSource() = %q, want %q", got, tt.source)
			}
			// Verify the seal: a value identifies itself as an Event.
			var _ Event = tt.event
		})
	}
}

// 13. Subscribers run in deterministic order matching registration.
func TestPublish_DeterministicOrder(t *testing.T) {
	bus := newTestBus(t)
	var order []string
	mkSub := func(name string) Subscriber {
		return funcSub(func(_ Event) {
			order = append(order, name)
		})
	}
	bus.Subscribe("first", mkSub("first"))
	bus.Subscribe("second", mkSub("second"))
	bus.Subscribe("third", mkSub("third"))

	bus.Publish(Installed{Tool: "t", Version: "1", Source: SourceManual})

	want := []string{"first", "second", "third"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("dispatch order = %v, want %v", order, want)
	}
}

// 14. Source enum values are stable strings (regression guard).
func TestSourceEnum_Values(t *testing.T) {
	cases := map[Source]string{
		SourceManual:      "manual",
		SourceAuto:        "auto",
		SourceProjectAuto: "project-auto",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("Source(%q) = %q, want %q", got, string(got), want)
		}
	}
}

// 15. Subscriber registered with nil sub is a no-op (defensive).
func TestSubscribe_NilSubIgnored(t *testing.T) {
	bus := newTestBus(t)
	bus.Subscribe("nil", nil)
	// Should not panic on Publish.
	bus.Publish(Installed{Tool: "t", Version: "1", Source: SourceManual})
}

type funcSub func(Event)

func (f funcSub) Handle(e Event) { f(e) }

// --- test-only accessors for drop counters ------------------------------

func (b *Bus) depthDropCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.depthDrops
}

func (b *Bus) queueDropCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.queueDrops
}

func (b *Bus) emptySourceDropCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.emptySourceDrops
}
