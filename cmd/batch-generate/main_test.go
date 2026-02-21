package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTransitionOpenBreakers_pastTimeout(t *testing.T) {
	ctrl := &batchControlFile{
		CircuitBreaker: map[string]*circuitBreakerEntry{
			"npm": {
				State:   "open",
				OpensAt: "2026-02-19T10:00:00Z",
			},
		},
	}

	now, _ := time.Parse("2006-01-02T15:04:05Z", "2026-02-19T11:00:00Z")
	state, modified := transitionOpenBreakers(ctrl, now)

	if !modified {
		t.Fatal("expected modified=true when opens_at has passed")
	}
	if state["npm"] != "half-open" {
		t.Errorf("state[npm] = %q, want %q", state["npm"], "half-open")
	}
	if ctrl.CircuitBreaker["npm"].State != "half-open" {
		t.Errorf("ctrl entry not updated: got %q, want %q", ctrl.CircuitBreaker["npm"].State, "half-open")
	}
}

func TestTransitionOpenBreakers_beforeTimeout(t *testing.T) {
	ctrl := &batchControlFile{
		CircuitBreaker: map[string]*circuitBreakerEntry{
			"npm": {
				State:   "open",
				OpensAt: "2026-02-19T12:00:00Z",
			},
		},
	}

	now, _ := time.Parse("2006-01-02T15:04:05Z", "2026-02-19T11:00:00Z")
	state, modified := transitionOpenBreakers(ctrl, now)

	if modified {
		t.Fatal("expected modified=false when opens_at is in the future")
	}
	if state["npm"] != "open" {
		t.Errorf("state[npm] = %q, want %q", state["npm"], "open")
	}
}

func TestTransitionOpenBreakers_closedUnchanged(t *testing.T) {
	ctrl := &batchControlFile{
		CircuitBreaker: map[string]*circuitBreakerEntry{
			"npm": {State: "closed"},
		},
	}

	now := time.Now().UTC()
	state, modified := transitionOpenBreakers(ctrl, now)

	if modified {
		t.Fatal("expected modified=false for closed breaker")
	}
	if state["npm"] != "closed" {
		t.Errorf("state[npm] = %q, want %q", state["npm"], "closed")
	}
}

func TestTransitionOpenBreakers_halfOpenUnchanged(t *testing.T) {
	ctrl := &batchControlFile{
		CircuitBreaker: map[string]*circuitBreakerEntry{
			"npm": {State: "half-open"},
		},
	}

	now := time.Now().UTC()
	state, modified := transitionOpenBreakers(ctrl, now)

	if modified {
		t.Fatal("expected modified=false for half-open breaker")
	}
	if state["npm"] != "half-open" {
		t.Errorf("state[npm] = %q, want %q", state["npm"], "half-open")
	}
}

func TestTransitionOpenBreakers_emptyOpensAt(t *testing.T) {
	ctrl := &batchControlFile{
		CircuitBreaker: map[string]*circuitBreakerEntry{
			"npm": {
				State:   "open",
				OpensAt: "",
			},
		},
	}

	now := time.Now().UTC()
	state, modified := transitionOpenBreakers(ctrl, now)

	if modified {
		t.Fatal("expected modified=false when opens_at is empty")
	}
	if state["npm"] != "open" {
		t.Errorf("state[npm] = %q, want %q", state["npm"], "open")
	}
}

func TestTransitionOpenBreakers_multipleEcosystems(t *testing.T) {
	ctrl := &batchControlFile{
		CircuitBreaker: map[string]*circuitBreakerEntry{
			"homebrew": {
				State:   "open",
				OpensAt: "2026-02-19T08:00:00Z",
			},
			"npm": {
				State: "closed",
			},
			"pypi": {
				State:   "open",
				OpensAt: "2026-02-20T12:00:00Z",
			},
		},
	}

	now, _ := time.Parse("2006-01-02T15:04:05Z", "2026-02-19T10:00:00Z")
	state, modified := transitionOpenBreakers(ctrl, now)

	if !modified {
		t.Fatal("expected modified=true (homebrew should transition)")
	}
	if state["homebrew"] != "half-open" {
		t.Errorf("homebrew = %q, want %q", state["homebrew"], "half-open")
	}
	if state["npm"] != "closed" {
		t.Errorf("npm = %q, want %q", state["npm"], "closed")
	}
	if state["pypi"] != "open" {
		t.Errorf("pypi = %q, want %q (opens_at in the future)", state["pypi"], "open")
	}
}

func TestTransitionOpenBreakers_jsonRoundTrip(t *testing.T) {
	input := `{
  "enabled": true,
  "disabled_ecosystems": [],
  "reason": "",
  "incident_url": "",
  "disabled_by": "",
  "disabled_at": "",
  "expected_resume": "",
  "circuit_breaker": {
    "homebrew": {
      "state": "open",
      "failures": 5,
      "last_failure": "2026-02-19T07:07:22Z",
      "opens_at": "2026-02-19T08:07:22Z"
    },
    "npm": {
      "state": "closed",
      "failures": 0,
      "last_failure": "",
      "opens_at": ""
    }
  },
  "budget": {
    "macos_minutes_used": 0,
    "linux_minutes_used": 0,
    "week_start": "",
    "sampling_active": false
  }
}
`
	var ctrl batchControlFile
	if err := json.Unmarshal([]byte(input), &ctrl); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	now, _ := time.Parse("2006-01-02T15:04:05Z", "2026-02-19T10:00:00Z")
	state, modified := transitionOpenBreakers(&ctrl, now)

	if !modified {
		t.Fatal("expected modified=true")
	}
	if state["homebrew"] != "half-open" {
		t.Errorf("homebrew = %q, want half-open", state["homebrew"])
	}

	// Verify the JSON round-trip preserves structure.
	output, err := json.MarshalIndent(ctrl, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	outputStr := string(output)

	if !strings.Contains(outputStr, `"state": "half-open"`) {
		t.Error("marshaled output missing half-open state for homebrew")
	}
	if !strings.Contains(outputStr, `"enabled": true`) {
		t.Error("marshaled output missing top-level enabled field")
	}
	if !strings.Contains(outputStr, `"macos_minutes_used": 0`) {
		t.Error("marshaled output missing budget fields")
	}
}

func TestTransitionOpenBreakers_writesFile(t *testing.T) {
	dir := t.TempDir()
	controlPath := filepath.Join(dir, "batch-control.json")
	input := `{
  "enabled": true,
  "disabled_ecosystems": [],
  "reason": "",
  "incident_url": "",
  "disabled_by": "",
  "disabled_at": "",
  "expected_resume": "",
  "circuit_breaker": {
    "npm": {
      "state": "open",
      "failures": 5,
      "last_failure": "2026-02-19T07:00:00Z",
      "opens_at": "2026-02-19T08:00:00Z"
    }
  },
  "budget": {
    "macos_minutes_used": 0,
    "linux_minutes_used": 0,
    "week_start": "",
    "sampling_active": false
  }
}
`
	if err := os.WriteFile(controlPath, []byte(input), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Simulate what main() does: read, transition, write back.
	data, err := os.ReadFile(controlPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var ctrl batchControlFile
	if err := json.Unmarshal(data, &ctrl); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	now, _ := time.Parse("2006-01-02T15:04:05Z", "2026-02-19T10:00:00Z")
	state, modified := transitionOpenBreakers(&ctrl, now)

	if !modified {
		t.Fatal("expected modified=true")
	}
	if state["npm"] != "half-open" {
		t.Fatalf("npm = %q, want half-open", state["npm"])
	}

	updated, err := json.MarshalIndent(ctrl, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	updated = append(updated, '\n')
	if err := os.WriteFile(controlPath, updated, 0644); err != nil {
		t.Fatalf("write back: %v", err)
	}

	// Re-read and verify the file has the updated state.
	data2, err := os.ReadFile(controlPath)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	var ctrl2 batchControlFile
	if err := json.Unmarshal(data2, &ctrl2); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if ctrl2.CircuitBreaker["npm"].State != "half-open" {
		t.Errorf("file state = %q, want half-open", ctrl2.CircuitBreaker["npm"].State)
	}
	if ctrl2.CircuitBreaker["npm"].Failures != 5 {
		t.Errorf("failures = %d, want 5 (should be preserved)", ctrl2.CircuitBreaker["npm"].Failures)
	}
	if !ctrl2.Enabled {
		t.Error("enabled should be preserved as true")
	}
}
