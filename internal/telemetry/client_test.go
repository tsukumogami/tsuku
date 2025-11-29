package telemetry

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewClient_Default(t *testing.T) {
	// Clear env vars using t.Setenv with empty string won't work,
	// so we unset them and ignore errors (they may not be set)
	_ = os.Unsetenv(EnvNoTelemetry)
	_ = os.Unsetenv(EnvDebug)

	c := NewClient()

	if c.endpoint != DefaultEndpoint {
		t.Errorf("endpoint = %q, want %q", c.endpoint, DefaultEndpoint)
	}
	if c.timeout != DefaultTimeout {
		t.Errorf("timeout = %v, want %v", c.timeout, DefaultTimeout)
	}
	if c.disabled {
		t.Error("disabled = true, want false")
	}
	if c.debug {
		t.Error("debug = true, want false")
	}
}

func TestNewClient_Disabled(t *testing.T) {
	t.Setenv(EnvNoTelemetry, "1")

	c := NewClient()

	if !c.disabled {
		t.Error("disabled = false, want true")
	}
	if !c.IsDisabled() {
		t.Error("IsDisabled() = false, want true")
	}
}

func TestNewClient_DisabledAnyValue(t *testing.T) {
	// Any non-empty value should disable telemetry
	t.Setenv(EnvNoTelemetry, "true")

	c := NewClient()

	if !c.disabled {
		t.Error("disabled = false, want true")
	}
}

func TestNewClient_Debug(t *testing.T) {
	t.Setenv(EnvDebug, "1")

	c := NewClient()

	if !c.debug {
		t.Error("debug = false, want true")
	}
}

func TestSend_Disabled(t *testing.T) {
	// Create a server that should never receive requests
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClientWithOptions(server.URL, time.Second, true, false)
	c.Send(NewInstallEvent("test", "", "1.0.0", false))

	// Give time for goroutine to potentially run
	time.Sleep(50 * time.Millisecond)

	if called {
		t.Error("server was called when telemetry was disabled")
	}
}

func TestSend_Debug(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	c := NewClientWithOptions("http://unused", time.Second, false, true)
	c.Send(NewInstallEvent("test-recipe", "@LTS", "1.0.0", false))

	// Restore stderr and read captured output
	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "[telemetry]") {
		t.Errorf("output does not contain [telemetry] prefix: %q", output)
	}
	if !strings.Contains(output, "test-recipe") {
		t.Errorf("output does not contain recipe name: %q", output)
	}
	if !strings.Contains(output, "install") {
		t.Errorf("output does not contain action: %q", output)
	}
}

func TestSend_Success(t *testing.T) {
	received := make(chan Event, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		var event Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Errorf("failed to decode event: %v", err)
		}
		received <- event
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClientWithOptions(server.URL, time.Second, false, false)
	c.Send(NewInstallEvent("nodejs", "@LTS", "22.0.0", false))

	select {
	case event := <-received:
		if event.Action != "install" {
			t.Errorf("Action = %q, want %q", event.Action, "install")
		}
		if event.Recipe != "nodejs" {
			t.Errorf("Recipe = %q, want %q", event.Recipe, "nodejs")
		}
	case <-time.After(time.Second):
		t.Error("event not received within timeout")
	}
}

func TestSend_Timeout(t *testing.T) {
	// Server that delays longer than timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClientWithOptions(server.URL, 50*time.Millisecond, false, false)

	// This should not block despite server delay
	start := time.Now()
	c.Send(NewInstallEvent("test", "", "1.0.0", false))
	elapsed := time.Since(start)

	// Send should return immediately (fire-and-forget)
	if elapsed > 10*time.Millisecond {
		t.Errorf("Send blocked for %v, expected immediate return", elapsed)
	}
}

func TestSend_ServerError(t *testing.T) {
	// Server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClientWithOptions(server.URL, time.Second, false, false)

	// Should not panic or return error
	c.Send(NewInstallEvent("test", "", "1.0.0", false))

	// Give time for goroutine to complete
	time.Sleep(50 * time.Millisecond)
}

func TestSend_NetworkError(t *testing.T) {
	// Use an endpoint that will fail
	c := NewClientWithOptions("http://localhost:1", 100*time.Millisecond, false, false)

	// Should not panic or return error
	c.Send(NewInstallEvent("test", "", "1.0.0", false))

	// Give time for goroutine to complete
	time.Sleep(150 * time.Millisecond)
}
