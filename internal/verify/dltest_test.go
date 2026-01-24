package verify

import (
	"encoding/json"
	"testing"
)

func TestDlopenResult_JSONParsing_Success(t *testing.T) {
	input := `[{"path":"/lib/libc.so.6","ok":true}]`

	var results []DlopenResult
	if err := json.Unmarshal([]byte(input), &results); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	r := results[0]
	if r.Path != "/lib/libc.so.6" {
		t.Errorf("Path = %q, want %q", r.Path, "/lib/libc.so.6")
	}
	if !r.OK {
		t.Error("OK = false, want true")
	}
	if r.Error != "" {
		t.Errorf("Error = %q, want empty", r.Error)
	}
}

func TestDlopenResult_JSONParsing_Failure(t *testing.T) {
	input := `[{"path":"/nonexistent.so","ok":false,"error":"cannot open shared object file"}]`

	var results []DlopenResult
	if err := json.Unmarshal([]byte(input), &results); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	r := results[0]
	if r.Path != "/nonexistent.so" {
		t.Errorf("Path = %q, want %q", r.Path, "/nonexistent.so")
	}
	if r.OK {
		t.Error("OK = true, want false")
	}
	if r.Error != "cannot open shared object file" {
		t.Errorf("Error = %q, want %q", r.Error, "cannot open shared object file")
	}
}

func TestDlopenResult_JSONParsing_Mixed(t *testing.T) {
	input := `[
		{"path":"/lib/libc.so.6","ok":true},
		{"path":"/lib/libpthread.so.0","ok":true},
		{"path":"/nonexistent.so","ok":false,"error":"not found"}
	]`

	var results []DlopenResult
	if err := json.Unmarshal([]byte(input), &results); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// First two should be OK
	if !results[0].OK || !results[1].OK {
		t.Error("first two results should be OK")
	}

	// Third should be failure
	if results[2].OK {
		t.Error("third result should be failure")
	}
	if results[2].Error != "not found" {
		t.Errorf("Error = %q, want %q", results[2].Error, "not found")
	}
}

func TestDlopenResult_JSONParsing_Empty(t *testing.T) {
	input := `[]`

	var results []DlopenResult
	if err := json.Unmarshal([]byte(input), &results); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}
