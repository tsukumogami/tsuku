package installevents

import (
	"fmt"
	"sync"

	"github.com/tsukumogami/tsuku/internal/log"
)

// Subscriber receives events from the bus. The Handle method runs
// synchronously inside Publish; it must not block on external I/O.
type Subscriber interface {
	Handle(event Event)
}

// Caps protect the bus from runaway publishers and runaway subscribers.
// A nested Publish from inside a subscriber's Handle is queued (not
// recursed) so depth tracks the cause-and-effect chain length rather
// than the call stack.
const (
	depthCap = 16
	queueCap = 1024
)

// Bus delivers events synchronously to registered subscribers in
// deterministic registration order. Each Handle call is wrapped in
// defer/recover; panics and errors never propagate to the publisher.
// Nested Publish calls from inside a Handle are queued and flushed
// after the current event's subscribers all complete.
//
// The zero-value Bus is not ready for use; construct with NewBus.
//
// Bus methods are safe to call from multiple goroutines, but no
// guarantee is made about ordering when concurrent Publish calls race.
// Production callers Publish from a single goroutine per process.
type Bus struct {
	mu   sync.Mutex
	subs []namedSub
	diag log.Logger

	// publishing is true while a top-level Publish is dispatching.
	// Re-entrant Publish calls during this window go on the queue.
	publishing bool

	// currentDepth tracks the depth of the event currently being
	// dispatched (0 for the top-level event, 1+ for events enqueued
	// from within a Handle).
	currentDepth int

	// queue holds events enqueued during the current top-level Publish.
	queue []queueEntry

	// frozen prevents Subscribe after Publish has started; protects the
	// snapshot semantics relied on by tests.
	frozen bool

	// Diagnostic counters incremented when events are dropped. Reading
	// is protected by mu. Used by package tests; not exposed externally.
	depthDrops      int
	queueDrops      int
	emptySourceDrops int
}

type namedSub struct {
	name string
	sub  Subscriber
}

type queueEntry struct {
	event Event
	depth int
}

// NewBus constructs a Bus that writes diagnostic messages through
// the supplied Logger at Debug level. If diag is nil, NewBus uses
// log.Default(), which respects the process-wide verbosity setting.
//
// Production wiring should pass a configured logger (typically
// log.Default() after main has called log.SetDefault). The deliberate
// choice not to accept a generic io.Writer means a caller cannot
// accidentally route subscriber diagnostics to stderr; surfacing them
// would interrupt user output during a normal Publish.
func NewBus(diag log.Logger) *Bus {
	if diag == nil {
		diag = log.Default()
	}
	return &Bus{diag: diag}
}

// NewBusForTest constructs a Bus that writes diagnostics into a Logger
// supplied by the test. Use this exclusively from _test.go files.
func NewBusForTest(diag log.Logger) *Bus {
	if diag == nil {
		diag = log.NewNoop()
	}
	return &Bus{diag: diag}
}

// Subscribe registers sub under name. Subscriber names appear in
// diagnostic output so a panicking subscriber can be identified.
// Calling Subscribe after the first Publish has begun is a logic
// bug and is silently ignored (with a diagnostic log line); the
// snapshot of subscribers taken at the first Publish is what
// subsequent Publish calls dispatch to.
func (b *Bus) Subscribe(name string, sub Subscriber) {
	if b == nil || sub == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.frozen {
		b.diag.Warn("installevents: Subscribe after first Publish ignored",
			"subscriber", name)
		return
	}
	b.subs = append(b.subs, namedSub{name: name, sub: sub})
}

// Publish dispatches event to every registered subscriber in
// registration order. Subscribers run synchronously inside this call;
// when Publish returns, every subscriber has either handled the event
// or its handler panicked and was recovered. Re-entrant Publish calls
// from inside a Handle are queued and flushed after the current
// event's subscribers all complete, preserving causal order.
//
// Empty-Source events are dropped with a log line — every publisher
// must specify a Source.
//
// Publish on a nil receiver is a no-op, so an install.Manager
// constructed without WithEventBus is safe.
func (b *Bus) Publish(event Event) {
	if b == nil || event == nil {
		return
	}
	if event.GetSource() == "" {
		b.mu.Lock()
		b.emptySourceDrops++
		b.mu.Unlock()
		b.diag.Warn("installevents: dropping event with empty Source",
			"type", fmt.Sprintf("%T", event))
		return
	}

	b.mu.Lock()
	if b.publishing {
		// Re-entrant: enqueue at currentDepth+1 unless caps would be exceeded.
		nextDepth := b.currentDepth + 1
		if nextDepth >= depthCap {
			b.depthDrops++
			b.diag.Warn("installevents: dropping event at depth cap",
				"type", fmt.Sprintf("%T", event), "depth", nextDepth, "cap", depthCap)
			b.mu.Unlock()
			return
		}
		if len(b.queue) >= queueCap {
			b.queueDrops++
			b.diag.Warn("installevents: dropping event at queue cap",
				"type", fmt.Sprintf("%T", event), "queue_size", len(b.queue), "cap", queueCap)
			b.mu.Unlock()
			return
		}
		b.queue = append(b.queue, queueEntry{event: event, depth: nextDepth})
		b.mu.Unlock()
		return
	}

	// Top-level Publish: become the active publisher. Snapshot
	// subscribers so adds during dispatch don't change the set.
	b.publishing = true
	b.frozen = true
	b.currentDepth = 0
	subs := append([]namedSub(nil), b.subs...)
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		b.publishing = false
		b.currentDepth = 0
		// Discard any residual queue (shouldn't happen except via a panic
		// in the drain loop, but be defensive).
		b.queue = b.queue[:0]
		b.mu.Unlock()
	}()

	// Dispatch the top-level event.
	b.dispatch(event, subs)

	// Drain the queue. Each iteration may enqueue more events via a
	// subscriber's nested Publish, up to the caps.
	for {
		b.mu.Lock()
		if len(b.queue) == 0 {
			b.mu.Unlock()
			return
		}
		next := b.queue[0]
		b.queue = b.queue[1:]
		b.currentDepth = next.depth
		b.mu.Unlock()

		b.dispatch(next.event, subs)
	}
}

// dispatch runs every subscriber's Handle for the given event,
// wrapping each in defer/recover so a panic in one subscriber doesn't
// prevent the others from observing the event.
func (b *Bus) dispatch(event Event, subs []namedSub) {
	for _, sub := range subs {
		b.runOne(sub, event)
	}
}

// runOne calls sub.Handle(event) with a recover guard.
func (b *Bus) runOne(sub namedSub, event Event) {
	defer func() {
		if r := recover(); r != nil {
			b.diag.Error("installevents: subscriber panicked",
				"subscriber", sub.name,
				"type", fmt.Sprintf("%T", event),
				"recover", fmt.Sprintf("%v", r))
		}
	}()
	sub.sub.Handle(event)
}
