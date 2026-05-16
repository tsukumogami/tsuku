package main

import (
	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/installevents"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/telemetry"
)

// newEventBus constructs the install lifecycle bus and registers the
// in-process subscribers. Called from each command that constructs an
// install.Manager so the foreground binary and the auto-apply
// subprocess (which is the same binary re-invoked with a hidden
// subcommand) both wire the full subscriber set.
//
// The telemetry client may be nil; in that case the telemetry
// subscriber is skipped. The notices subscriber is always registered
// because the notice store is the user-visible source of truth and
// has no opt-out.
//
// Each process gets a fresh bus (constructed per command invocation).
// Subscribers are independent across processes: the foreground
// command's bus and the auto-apply subprocess's bus do not share
// state — they synchronize through the filesystem (notice files,
// state.json), identical to today's behavior.
func newEventBus(cfg *config.Config, tc *telemetry.Client) *installevents.Bus {
	bus := installevents.NewBus(nil)
	bus.Subscribe("notices", notices.NewSubscriber(notices.NoticesDir(cfg.HomeDir)))
	if tc != nil && !tc.IsDisabled() {
		bus.Subscribe("telemetry", telemetry.NewSubscriber(tc))
	}
	return bus
}
