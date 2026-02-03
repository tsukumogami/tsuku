package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/tsukumogami/tsuku/internal/userconfig"
)

const (
	// EnvNoTelemetry disables telemetry when set to any non-empty value.
	EnvNoTelemetry = "TSUKU_NO_TELEMETRY"

	// EnvTelemetry disables telemetry when set to "0" or "false".
	// This is an alias for TSUKU_NO_TELEMETRY for users who expect
	// TSUKU_TELEMETRY=0 to work.
	EnvTelemetry = "TSUKU_TELEMETRY"

	// EnvDebug enables debug mode: prints events to stderr without sending.
	EnvDebug = "TSUKU_TELEMETRY_DEBUG"

	// DefaultEndpoint is the URL where telemetry events are sent.
	DefaultEndpoint = "https://telemetry.tsuku.dev/event"

	// DefaultTimeout is the HTTP request timeout.
	DefaultTimeout = 2 * time.Second
)

// Client sends telemetry events. It is safe for concurrent use.
type Client struct {
	endpoint string
	timeout  time.Duration
	disabled bool
	debug    bool
}

// DisabledByEnv reports whether telemetry is disabled via environment variables.
// It checks TSUKU_NO_TELEMETRY (any non-empty value) and TSUKU_TELEMETRY ("0" or "false").
func DisabledByEnv() bool {
	if os.Getenv(EnvNoTelemetry) != "" {
		return true
	}
	if v := os.Getenv(EnvTelemetry); v == "0" || v == "false" {
		return true
	}
	return false
}

// NewClient creates a telemetry client.
// It checks environment variables first (takes precedence), then config file.
// TSUKU_TELEMETRY_DEBUG enables debug mode.
func NewClient() *Client {
	disabled := false

	// Environment variable takes precedence
	if DisabledByEnv() {
		disabled = true
	} else {
		// Check config file
		cfg, err := userconfig.Load()
		if err == nil && !cfg.Telemetry {
			disabled = true
		}
	}

	return &Client{
		endpoint: DefaultEndpoint,
		timeout:  DefaultTimeout,
		disabled: disabled,
		debug:    os.Getenv(EnvDebug) != "",
	}
}

// NewClientWithOptions creates a telemetry client with custom options.
// This is primarily useful for testing.
func NewClientWithOptions(endpoint string, timeout time.Duration, disabled, debug bool) *Client {
	return &Client{
		endpoint: endpoint,
		timeout:  timeout,
		disabled: disabled,
		debug:    debug,
	}
}

// IsDisabled returns true if telemetry is disabled.
func (c *Client) IsDisabled() bool {
	return c.disabled
}

// Send sends an event asynchronously. It never blocks and never returns errors.
// If telemetry is disabled, this is a no-op.
// If debug mode is enabled, the event is printed to stderr instead of being sent.
func (c *Client) Send(event Event) {
	if c.disabled {
		return
	}

	if c.debug {
		data, _ := json.Marshal(event)
		fmt.Fprintf(os.Stderr, "[telemetry] %s\n", data)
		return
	}

	// Fire-and-forget: spawn goroutine, no waiting
	go c.send(event)
}

// send performs the actual HTTP request. Called in a goroutine by Send.
func (c *Client) send(event Event) {
	c.sendJSON(event)
}

// SendLLM sends an LLM event asynchronously. It never blocks and never returns errors.
// If telemetry is disabled, this is a no-op.
// If debug mode is enabled, the event is printed to stderr instead of being sent.
func (c *Client) SendLLM(event LLMEvent) {
	if c.disabled {
		return
	}

	if c.debug {
		data, _ := json.Marshal(event)
		fmt.Fprintf(os.Stderr, "[telemetry] %s\n", data)
		return
	}

	// Fire-and-forget: spawn goroutine, no waiting
	go c.sendJSON(event)
}

// SendDiscovery sends a discovery event asynchronously. It never blocks and never returns errors.
// If telemetry is disabled, this is a no-op.
// If debug mode is enabled, the event is printed to stderr instead of being sent.
func (c *Client) SendDiscovery(event DiscoveryEvent) {
	if c.disabled {
		return
	}

	if c.debug {
		data, _ := json.Marshal(event)
		fmt.Fprintf(os.Stderr, "[telemetry] %s\n", data)
		return
	}

	// Fire-and-forget: spawn goroutine, no waiting
	go c.sendJSON(event)
}

// sendJSON performs the actual HTTP request for any event type.
func (c *Client) sendJSON(event interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	data, err := json.Marshal(event)
	if err != nil {
		return // Silent failure
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		return // Silent failure
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return // Silent failure (timeout, network error, etc.)
	}
	resp.Body.Close()
	// Ignore response status - we don't retry
}
